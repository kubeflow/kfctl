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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

// kfdefInstances keep all KfDef CRs watched by the operator
var kfdefInstances = map[string]struct{}{}

// whether the 2nd controller is added
var b2ndController = false

// the manager
var kfdefManager manager.Manager

// the stop channel for the 2nd controller
var stop chan struct{}

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager) error {
	kfdefManager = m
	return Add(kfdefManager)
}

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
	err = c.Watch(&source.Kind{Type: &kfdefv1.KfDef{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
			namespacedName := types.NamespacedName{Name: a.Meta.GetName(), Namespace: a.Meta.GetNamespace()}
			finalizers := sets.NewString(a.Meta.GetFinalizers()...)
			if !finalizers.Has(finalizer) {
				// assume this is a CREATE event
				log.Infof("Adding finalizer %v: %v.", finalizer, namespacedName)
				finalizers.Insert(finalizer)
				instance := &kfdefv1.KfDef{}
				err = mgr.GetClient().Get(context.TODO(), namespacedName, instance)
				if err != nil {
					log.Errorf("Failed to get kfdef CR. Error: %v.", err)
					return nil
				}
				instance.SetFinalizers(finalizers.List())
				err = mgr.GetClient().Update(context.TODO(), instance)
				if err != nil {
					log.Errorf("Failed to update kfdef with finalizer. Error: %v.", err)
				}
				// let the UPDATE event request queue
				return nil
			}
			log.Infof("Watch a change for KfDef CR: %v.%v.", a.Meta.GetName(), a.Meta.GetNamespace())
			return []reconcile.Request{{NamespacedName: namespacedName}}
		}),
	}, kfdefPredicates)
	if err != nil {
		return err
	}

	// Watch for changes to kfdef resource and requeue the owner KfDef
	err = watchKubeflowResources(c, mgr.GetClient(), watchedResources)
	if err != nil {
		return err
	}
	log.Infof("Controller added to watch on Kubeflow resources with known GVK.")
	return nil
}

// watch is monitoring changes for kfctl resources managed by the operator
func watchKubeflowResources(c controller.Controller, r client.Client, watchedResources []schema.GroupVersionKind) error {
	for _, t := range watchedResources {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(schema.GroupVersionKind{
			Kind:    t.Kind,
			Group:   t.Group,
			Version: t.Version,
		})
		err := c.Watch(&source.Kind{Type: u}, &handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
				anns := a.Meta.GetAnnotations()
				kfdefAnn := strings.Join([]string{kfutils.KfDefAnnotation, kfutils.KfDefInstance}, "/")
				_, found := anns[kfdefAnn]
				if found {
					kfdefCr := strings.Split(anns[kfdefAnn], ".")
					namespacedName := types.NamespacedName{Name: kfdefCr[0], Namespace: kfdefCr[1]}
					instance := &kfdefv1.KfDef{}
					err := r.Get(context.TODO(), types.NamespacedName{Name: kfdefCr[0], Namespace: kfdefCr[1]}, instance)
					if err != nil {
						if errors.IsNotFound(err) {
							// KfDef CR may have been deleted
							return nil
						}
					} else if instance.GetDeletionTimestamp() != nil {
						// KfDef is being deleted
						return nil
					}
					log.Infof("Watch a change for Kubeflow resource: %v.%v.", a.Meta.GetName(), a.Meta.GetNamespace())
					return []reconcile.Request{{NamespacedName: namespacedName}}
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

var kfdefPredicates = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		object, _ := meta.Accessor(e.Object)
		log.Infof("Got create event for %v.%v.", object.GetName(), object.GetNamespace())
		return true
	},
	GenericFunc: func(e event.GenericEvent) bool {
		object, _ := meta.Accessor(e.Object)
		log.Infof("Got generic event for %v.%v.", object.GetName(), object.GetNamespace())
		return true
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		object, _ := meta.Accessor(e.Object)
		log.Infof("Got delete event for %v.%v.", object.GetName(), object.GetNamespace())
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		object, _ := meta.Accessor(e.ObjectOld)
		log.Infof("Got update event for %v.%v.", object.GetName(), object.GetNamespace())

		upd, _ := meta.Accessor(e.ObjectNew)
		// these cases will result in a reconcile request
		// 1. the finalizer is added 2. the deletiontimestamp is added 3. generation is increased
		if len(object.GetFinalizers()) == 0 && len(upd.GetFinalizers()) > 0 {
			return true
		}
		if object.GetDeletionTimestamp() == nil && upd.GetDeletionTimestamp() != nil {
			return true
		}
		if upd.GetGeneration() > object.GetGeneration() {
			return true
		}
		return false
	},
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
		// handle deletion event
		object, err := meta.Accessor(e.Object)
		if err != nil {
			return false
		}
		log.Infof("Got delete event for %v.%v.", object.GetName(), object.GetNamespace())
		// if this object has an owner, let the owner handle the appropriate recovery
		if len(object.GetOwnerReferences()) > 0 {
			return false
		}
		return true
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

	deleted := instance.GetDeletionTimestamp() != nil
	finalizers := sets.NewString(instance.GetFinalizers()...)
	if deleted {
		if !finalizers.Has(finalizer) {
			log.Info("Kfdef deleted.")
			return reconcile.Result{}, nil
		}
		log.Infof("Deleting kfdef.")

		// stop the 2nd controller
		if len(kfdefInstances) == 1 {
			close(stop)
			b2ndController = false
		}

		// Uninstall Kubeflow
		err = kfDelete(instance)
		if err == nil {
			log.Infof("KubeFlow Deployment Deleted.")
		} else {
			// log an error and continue for cleanup. It does not make sense to retry the delete.
			log.Errorf("Failed to delete Kubeflow.")
		}

		// Delete the kfapp directory
		kfAppDir := path.Join("/tmp", instance.GetNamespace(), instance.GetName())
		if err := os.RemoveAll(kfAppDir); err != nil {
			log.Errorf("Failed to delete the app directory. Error: %v.", err)
			return reconcile.Result{}, err
		}
		log.Infof("kfAppDir deleted.")

		// Remove this KfDef instance
		delete(kfdefInstances, strings.Join([]string{instance.GetName(), instance.GetNamespace()}, "."))

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
		return reconcile.Result{}, nil
	} else if !finalizers.Has(finalizer) {
		log.Infof("Normally this should not happen. Adding finalizer %v: %v.", finalizer, request)
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
	if err == nil {
		log.Infof("KubeFlow Deployment Completed.")

		// add to kfdefInstances if not exists
		if _, ok := kfdefInstances[strings.Join([]string{instance.GetName(), instance.GetNamespace()}, ".")]; !ok {
			kfdefInstances[strings.Join([]string{instance.GetName(), instance.GetNamespace()}, ".")] = struct{}{}
		}

		if b2ndController == false {
			c, err := controller.New("kubeflow-controller", kfdefManager, controller.Options{Reconciler: r})
			if err != nil {
				return reconcile.Result{}, nil
			}
			// Watch for changes to kfdef resource and requeue the owner KfDef
			err = watchKubeflowResources(c, kfdefManager.GetClient(), watchedKubeflowResources)
			if err != nil {
				return reconcile.Result{}, nil
			}
			stop = make(chan struct{})
			go func() {
				// Start the controller
				if err := c.Start(stop); err != nil {
					log.Error(err, "cannot run the 2nd Kubeflow controller")
				}
			}()
			log.Infof("Controller added to watch resources from CRDs created by Kubeflow deployment.")
			b2ndController = true
		}
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

// kfDelete is equivalent of kfctl delete
func kfDelete(instance *kfdefv1.KfDef) error {
	log.Infof("Uninstall Kubeflow. KubeFlow.Namespace: %v.", instance.Namespace)
	kfApp, err := kfLoadConfig(instance, "delete")
	if err != nil {
		log.Errorf("Failed to load KfApp. Error: %v.", err)
		return err
	}
	// Delete kfApp.
	err = kfApp.Delete(kftypesv3.K8S)
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
		// Indicate to add annotation to the top level resources
		setAnnotationAnn := strings.Join([]string{kfutils.KfDefAnnotation, kfutils.SetAnnotation}, "/")
		setAnnotations(configFilePath, map[string]string{
			setAnnotationAnn: "true",
		})
	}

	if action == "delete" {
		// Enable force delete since inClusterConfig has no ./kube/config file to pass the delete safety check.
		forceDeleteAnn := strings.Join([]string{kfutils.KfDefAnnotation, kfutils.ForceDelete}, "/")
		setAnnotations(configFilePath, map[string]string{
			forceDeleteAnn: "true",
		})

		// Indicate the Kubeflow is installed by the operator
		byOperatorAnn := strings.Join([]string{kfutils.KfDefAnnotation, kfutils.InstallByOperator}, "/")
		setAnnotations(configFilePath, map[string]string{
			byOperatorAnn: "true",
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
