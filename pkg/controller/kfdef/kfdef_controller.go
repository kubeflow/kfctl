package kfdef

import (
	"context"
	"io/ioutil"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/go-logr/logr"
	kftypesv3 "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	kfdefv1 "github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfdef/v1"
	"github.com/kubeflow/kfctl/v3/pkg/kfapp/coordinator"
	kfloaders "github.com/kubeflow/kfctl/v3/pkg/kfconfig/loaders"
	kfutils "github.com/kubeflow/kfctl/v3/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_kfdef")

const (
	finalizer = "kfdef-finalizer.kfdef.apps.kubeflow.org"
	// finalizerMaxRetries defines the maximum number of attempts to add finalizers.
	finalizerMaxRetries = 10
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new KfDef Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileKfDef{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	reqLogger := log.WithValues("add controller")
	reqLogger.Info("Adding controller for kfdef")
	// Create a new controller
	c, err := controller.New("kfdef-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource KfDef
	err = c.Watch(&source.Kind{Type: &kfdefv1.KfDef{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to kfdef resource and requeue the owner KfDef
	err = watchKubeflowResources(c)
	if err != nil {
		return err
	}
	reqLogger.Info("Controller added")
	return nil
}

// watch is monitoring changes for kfctl resources managed by the operator
func watchKubeflowResources(c controller.Controller) error {
	for _, t := range watchedResources {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(schema.GroupVersionKind{
			Kind:    t.Kind,
			Group:   t.Group,
			Version: t.Version,
		})
		err := c.Watch(&source.Kind{Type: u}, &handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
				log.Info("watch a change for kfdef resource: %s.%s", a.Meta.GetName(), a.Meta.GetNamespace())
				return []reconcile.Request{
					{NamespacedName: kfdefSingletonNN},
				}
			}),
		}, ownedResourcePredicates)
		if err != nil {
			log.Info("cannot create watch for resources ", t.Kind, t.Group, t.Version, ". Error: ", err)
		}
	}
	return nil
}

var ownedResourcePredicates = predicate.Funcs{
	CreateFunc: func(_ event.CreateEvent) bool {
		// no action
		return false
	},
	GenericFunc: func(_ event.GenericEvent) bool {
		// no action
		return false
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		object, err := meta.Accessor(e.Object)
		log.Info("got delete event for ", object.GetName(), object.GetNamespace())
		if err != nil {
			return false
		}
		if object.GetLabels()[KubeflowLabel] == "kfctl" {
			return true
		}
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		// no action
		return false
	},
}

// blank assignment to verify that ReconcileKfDef implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileKfDef{}

// ReconcileKfDef reconciles a KfDef object
type ReconcileKfDef struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a KfDef object and makes changes based on the state read
// and what is in the KfDef.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileKfDef) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling KfDef")

	// Fetch the KfDef instance
	instance := &kfdefv1.KfDef{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	deleted := instance.GetDeletionTimestamp() != nil
	finalizers := sets.NewString(instance.GetFinalizers()...)
	if deleted {
		if !finalizers.Has(finalizer) {
			log.Info("kfdef deleted")
			return reconcile.Result{}, nil
		}
		log.Info("Deleting kfdef")

		err = kfDelete(instance, reqLogger)
		if err != nil {
			reqLogger.Info("kfdef deletion failed.")
			return reconcile.Result{}, err
		}
		// Remove finalizer once kfDelete is completed.
		finalizers.Delete(finalizer)
		instance.SetFinalizers(finalizers.List())
		finalizerError := r.client.Update(context.TODO(), instance)
		for retryCount := 0; errors.IsConflict(finalizerError) && retryCount < finalizerMaxRetries; retryCount++ {
			// Based on Istio operator at https://github.com/istio/istio/blob/master/operator/pkg/controller/istiocontrolplane/istiocontrolplane_controller.go
			// for finalizer removal errors workaround.
			log.Info("conflict during finalizer removal, retrying")
			_ = r.client.Get(context.TODO(), request.NamespacedName, instance)
			finalizers = sets.NewString(instance.GetFinalizers()...)
			finalizers.Delete(finalizer)
			instance.SetFinalizers(finalizers.List())
			finalizerError = r.client.Update(context.TODO(), instance)
		}
		if finalizerError != nil {
			log.Info("error removing finalizer ", finalizerError)
			return reconcile.Result{}, finalizerError
		}
		return reconcile.Result{}, err
	} else if !finalizers.Has(finalizer) {
		log.Info("Adding finalizer ", finalizer, request)
		finalizers.Insert(finalizer)
		instance.SetFinalizers(finalizers.List())
		err := r.client.Update(context.TODO(), instance)
		if err != nil {
			log.Info("Failed to update kfdef with finalizer ", err)
			return reconcile.Result{}, err
		}
	}
	err = kfApply(instance, reqLogger)

	// Make the current kfdef as default if kfApply is successed.
	if err == nil {
		kfdefSingletonNN = request.NamespacedName
		reqLogger.Info("KubeFlow Deployment Completed.")
	}
	// If deployment created successfully - don't requeue
	return reconcile.Result{}, err
}

// kfApply is equivalent of kfctl apply
func kfApply(instance *kfdefv1.KfDef, reqLogger logr.Logger) error {
	reqLogger.Info("Creating a new KubeFlow Deployment", "KubeFlow.Namespace", instance.Namespace)
	kfApp, err := kfLoadConfig(instance, reqLogger, "apply")
	if err != nil {
		reqLogger.Info("Failed to load KfApp: ", err)
		return err
	}
	// Apply kfApp.
	err = kfApp.Apply(kftypesv3.K8S)
	return err
}

// kfDelete is equivalent of kfctl delete
func kfDelete(instance *kfdefv1.KfDef, reqLogger logr.Logger) error {
	reqLogger.Info("Deleting the KubeFlow Deployment", "KubeFlow.Namespace", instance.Namespace)
	kfApp, err := kfLoadConfig(instance, reqLogger, "delete")
	if err != nil {
		reqLogger.Info("Failed to load KfApp: ", err)
		return err
	}
	err = kfApp.Delete(kftypesv3.ALL)
	return err
}

func kfLoadConfig(instance *kfdefv1.KfDef, reqLogger logr.Logger, action string) (kftypesv3.KfApp, error) {
	// Define kfApp
	kfdefBytes, _ := yaml.Marshal(instance)
	configFilePath := "/tmp/config.yaml"
	err := ioutil.WriteFile(configFilePath, kfdefBytes, 0644)
	if err != nil {
		reqLogger.Info("Failed to write config.yaml ", err)
		return nil, err
	}
	if action == "delete" {
		// Enable force delete since inClusterConfig has no ./kube/config file to pass the delete safety check.
		forceDeleteAnn := strings.Join([]string{kfutils.KfDefAnnotation, kfutils.ForceDelete}, "/")
		setAnnotations(configFilePath, map[string]string{
			forceDeleteAnn: "true",
		})
	}
	kfApp, err := coordinator.NewLoadKfAppFromURI(configFilePath)
	if err != nil {
		reqLogger.Info("failed to build kfApp from URI ", configFilePath, err)
		return nil, err
	}
	return kfApp, nil
}

func setAnnotations(configPath string, annotations map[string]string) error {
	config, err := kfloaders.LoadConfigFromURI(configPath)
	if err != nil {
		return err
	}
	anns := config.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}
	for ann, val := range annotations {
		anns[ann] = val
	}
	config.SetAnnotations(anns)
	return kfloaders.WriteConfigToFile(*config)
}
