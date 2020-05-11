package kfdef

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/ghodss/yaml"
	kftypesv3 "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	kfdefv1 "github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfdef/v1"
	"github.com/kubeflow/kfctl/v3/pkg/kfapp/coordinator"
	kfloaders "github.com/kubeflow/kfctl/v3/pkg/kfconfig/loaders"
	kfutils "github.com/kubeflow/kfctl/v3/pkg/utils"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

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
	log.Infof("Adding controller for kfdef.")
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
	log.Infof("Controller added.")
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
				log.Infof("Watch a change for kfdef resource: %v.%v.", a.Meta.GetName(), a.Meta.GetNamespace())

				// retrieve owner KfDef
				for _, owner := range a.Meta.GetOwnerReferences() {
					if owner.Kind == "KfDef" {
						if _, ok := kfdefUIDMap[owner.UID]; ok {
							return []reconcile.Request{{NamespacedName: kfdefUIDMap[owner.UID]}}
						}
					}
				}
				return nil
			}),
		}, ownedResourcePredicates)
		if err != nil {
			log.Errorf("Cannot create watch for resources %v %v/%v: %v.", t.Kind, t.Group, t.Version, err)
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
		log.Infof("Got delete event for %v.%v.", object.GetName(), object.GetNamespace())
		if err == nil && OwnerIsKfDef(object) {
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

// OwnerIsKfDef filters out only resources are children of a kfdef instance
func OwnerIsKfDef(o v1.Object) bool {
	for _, owner := range o.GetOwnerReferences() {
		if owner.Kind == "KfDef" {
			if _, ok := kfdefUIDMap[owner.UID]; ok {
				return true
			}
		}
	}
	return false
}

// Reconcile reads that state of the cluster for a KfDef object and makes changes based on the state read
// and what is in the KfDef.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileKfDef) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Infof("Reconciling KfDef resources. Request.Namespace: %v, Request.Name: %v.", request.Namespace, request.Name)

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

	// add to kfdefUIDMap if not exists
	if _, ok := kfdefUIDMap[instance.UID]; !ok {
		kfdefUIDMap[instance.GetUID()] = request.NamespacedName
	}

	deleted := instance.GetDeletionTimestamp() != nil
	finalizers := sets.NewString(instance.GetFinalizers()...)
	if deleted {
		if !finalizers.Has(finalizer) {
			log.Info("Kfdef deleted.")
			return reconcile.Result{}, nil
		}
		log.Infof("Deleting kfdef.")

		// Delete the kfapp directory
		kfAppDir := path.Join("/tmp", instance.GetNamespace(), instance.GetName())
		if err := os.RemoveAll(kfAppDir); err != nil {
			log.Errorf("Failed to delete the app directory. Error: %v.", err)
			return reconcile.Result{}, err
		}
		log.Infof("kfAppDir deleted.")

		// Remove this KfDef instance
		delete(kfdefUIDMap, instance.GetUID())

		// Remove finalizer once kfDelete is completed.
		finalizers.Delete(finalizer)
		instance.SetFinalizers(finalizers.List())
		finalizerError := r.client.Update(context.TODO(), instance)
		for retryCount := 0; errors.IsConflict(finalizerError) && retryCount < finalizerMaxRetries; retryCount++ {
			// Based on Istio operator at https://github.com/istio/istio/blob/master/operator/pkg/controller/istiocontrolplane/istiocontrolplane_controller.go
			// for finalizer removal errors workaround.
			log.Info("Conflict during finalizer removal, retrying.")
			_ = r.client.Get(context.TODO(), request.NamespacedName, instance)
			finalizers = sets.NewString(instance.GetFinalizers()...)
			finalizers.Delete(finalizer)
			instance.SetFinalizers(finalizers.List())
			finalizerError = r.client.Update(context.TODO(), instance)
		}
		if finalizerError != nil {
			log.Errorf("Error removing finalizer: %v.", finalizerError)
			return reconcile.Result{}, finalizerError
		}
		return reconcile.Result{}, err
	} else if !finalizers.Has(finalizer) {
		log.Infof("Adding finalizer %v: %v.", finalizer, request)
		finalizers.Insert(finalizer)
		instance.SetFinalizers(finalizers.List())
		err = r.client.Update(context.TODO(), instance)
		if err != nil {
			log.Errorf("Failed to update kfdef with finalizer. Error: %v.", err)
			return reconcile.Result{}, err
		}
	}

	// If this is a kfdef change, for now, remove the kfapp config path
	if request.Name == instance.GetName() && request.Namespace == instance.GetNamespace() {
		kfAppDir := path.Join("/tmp", instance.GetNamespace(), instance.GetName())
		if err = os.RemoveAll(kfAppDir); err != nil {
			log.Errorf("Failed to delete the app directory. Error: %v.", err)
			return reconcile.Result{}, err
		}
	}

	err = kfApply(instance)

	// Make the current kfdef as default if kfApply is successed.
	if err == nil {
		log.Infof("KubeFlow Deployment Completed.")
	}
	// If deployment created successfully - don't requeue
	return reconcile.Result{}, err
}

// kfApply is equivalent of kfctl apply
func kfApply(instance *kfdefv1.KfDef) error {
	log.Infof("Creating a new KubeFlow Deployment. KubeFlow.Namespace: %v.", instance.Namespace)
	kfApp, err := kfLoadConfig(instance, "apply")
	if err != nil {
		log.Errorf("Failed to load KfApp. Error: %v.", err)
		return err
	}
	// Apply kfApp.
	err = kfApp.Apply(kftypesv3.K8S)
	return err
}

func kfLoadConfig(instance *kfdefv1.KfDef, action string) (kftypesv3.KfApp, error) {
	// Define kfApp
	kfdefBytes, _ := yaml.Marshal(instance)

	// Make the kfApp directory
	kfAppDir := path.Join("/tmp", instance.GetNamespace(), instance.GetName())
	if err := os.MkdirAll(kfAppDir, 0755); err != nil {
		log.Errorf("Failed to create the app directory. Error: %v.", err)
		return nil, err
	}

	configFilePath := path.Join(kfAppDir, "config.yaml")
	err := ioutil.WriteFile(configFilePath, kfdefBytes, 0644)
	if err != nil {
		log.Errorf("Failed to write config.yaml. Error: %v.", err)
		return nil, err
	}

	if action == "apply" {
		// Indicate to set ownerReferences to the top level resources
		setOwnerReferenceAnn := strings.Join([]string{kfutils.KfDefAnnotation, kfutils.SetOwnerReference}, "/")
		setAnnotations(configFilePath, map[string]string{
			setOwnerReferenceAnn: "true",
		})
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
		log.Errorf("failed to build kfApp from URI %v: Error: %v.", configFilePath, err)

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
