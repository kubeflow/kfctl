/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kustomize

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	errutil "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cenkalti/backoff"
	"github.com/ghodss/yaml"
	"github.com/imdario/mergo"
	kfapisv3 "github.com/kubeflow/kfctl/v3/pkg/apis"
	kftypesv3 "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	kfdefsv3 "github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfdef/v1alpha1"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
	"github.com/kubeflow/kfctl/v3/pkg/utils"
	"github.com/otiai10/copy"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	crdclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	rbacv1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/kustomize/v3/k8sdeps/kunstruct"
	"sigs.k8s.io/kustomize/v3/k8sdeps/transformer"
	"sigs.k8s.io/kustomize/v3/pkg/fs"
	"sigs.k8s.io/kustomize/v3/pkg/image"
	"sigs.k8s.io/kustomize/v3/pkg/loader"
	"sigs.k8s.io/kustomize/v3/pkg/plugins"
	"sigs.k8s.io/kustomize/v3/pkg/resmap"
	"sigs.k8s.io/kustomize/v3/pkg/resource"
	"sigs.k8s.io/kustomize/v3/pkg/target"
	"sigs.k8s.io/kustomize/v3/pkg/types"
	"sigs.k8s.io/kustomize/v3/pkg/validators"
	"sigs.k8s.io/kustomize/v3/plugin/builtin"
)

// kustomize implements KfApp Interface
// It should include functionality needed for the kustomize platform
// In addition to `kustomize build`, there is `kustomize edit ...`
// As noted below there are lots of different ways to use edit
//  kustomize edit add configmap my-configmap --from-file=my-key=file/path --from-literal=my-literal=12345
//  kustomize edit add configmap my-configmap --from-file=file/path
//  kustomize edit add configmap my-configmap --from-env-file=env/path.env
//  kustomize edit add configmap NAME --from-literal=k=v
//  kustomize edit add resource <filepath>
//  kustomize edit add patch <filepath>
//  kustomize edit add base <filepath1>,<filepath2>,<filepath3>
//  kustomize edit set nameprefix <prefix-value>

type MapType int

const (
	basesMap                 MapType = 0
	commonAnnotationsMap     MapType = 1
	commonLabelsMap          MapType = 2
	imagesMap                MapType = 3
	resourcesMap             MapType = 4
	crdsMap                  MapType = 5
	varsMap                  MapType = 6
	configurationsMap        MapType = 7
	configMapGeneratorMap    MapType = 8
	secretsMapGeneratorMap   MapType = 9
	patchesStrategicMergeMap MapType = 10
	patchesJson6902Map       MapType = 11
	OverlayParamName                 = "overlay"
)

type kustomize struct {
	kfDef            *kfconfig.KfConfig
	out              *os.File
	err              *os.File
	componentPathMap map[string]string
	componentMap     map[string]bool
	packageMap       map[string]*[]string
	restConfig       *rest.Config
	// when set to true, apply() will skip local kube config, directly build config from restConfig
	configOverwrite bool
}

const (
	defaultUserId = "anonymous"
	outputDir     = "kustomize"
)

// Setter defines an interface for modifying the plugin.
type Setter interface {
	SetK8sRestConfig(r *rest.Config)
}

// GetKfApp is the common entry point for all implementations of the KfApp interface
func GetKfApp(kfdef *kfconfig.KfConfig) kftypesv3.KfApp {
	_kustomize := &kustomize{
		kfDef: kfdef,
		out:   os.Stdout,
		err:   os.Stderr,
	}

	// We explicitly do not initiate restConfig  here.
	// We want to delay creating the clients until we actually need them.
	// This is for two reasons
	// 1. We want to allow injecting the config and not relying on
	//    $HOME/.kube/config always
	// 2. We want to be able to generate the manifests without the K8s cluster existing.
	// build restConfig using $HOME/.kube/config if the file exists
	return _kustomize
}

// initK8sClients initializes the K8s clients if they haven't already been initialized.
// it is a null op otherwise.
func (kustomize *kustomize) initK8sClients() error {
	if kustomize.restConfig == nil {
		log.Infof("Initializing a default restConfig for Kubernetes")
		kustomize.restConfig = kftypesv3.GetConfig()
	}

	return nil
}

func (kustomize *kustomize) render(app kfconfig.Application) ([]byte, error) {
	kustomizeDir := path.Join(kustomize.kfDef.Spec.AppDir, outputDir)
	resMap, err := EvaluateKustomizeManifest(path.Join(kustomizeDir, app.Name))
	if err != nil {
		log.Errorf("Error evaluating kustomization manifest for %v: %v", app.Name, err)
		return nil, &kfapisv3.KfError{
			Code:    int(kfapisv3.INTERNAL_ERROR),
			Message: fmt.Sprintf("error evaluating kustomization manifest for %v: %v", app.Name, err),
		}
	}

	sortResourceByKind(resMap, utils.InstallOrder)

	// check to set owner references for resources if installed through kubeflow operator
	annotations := kustomize.kfDef.GetAnnotations()
	setOperatorAnnotation := false
	if setOperator, ok := annotations[strings.Join([]string{utils.KfDefAnnotation, utils.SetAnnotation}, "/")]; ok {
		if setOperatorBool, err := strconv.ParseBool(setOperator); err == nil {
			setOperatorAnnotation = setOperatorBool
		}
	}

	//TODO this should be streamed
	var data []byte
	if setOperatorAnnotation {
		// retrieve the UID of the KfDef resource using dynamic client
		config, _ := rest.InClusterConfig()
		dyn, err := dynamic.NewForConfig(config)
		if err != nil {
			return nil, &kfapisv3.KfError{
				Code:    int(kfapisv3.INTERNAL_ERROR),
				Message: fmt.Sprintf("failed to create dynamic client: %v", err),
			}
		}
		kfDefRes := schema.GroupVersionResource{Group: "kfdef.apps.kubeflow.org", Version: "v1", Resource: "kfdefs"}
		instance, err := dyn.Resource(kfDefRes).Namespace(kustomize.kfDef.GetNamespace()).Get(kustomize.kfDef.GetName(), metav1.GetOptions{})
		if err != nil {
			return nil, &kfapisv3.KfError{
				Code:    int(kfapisv3.INTERNAL_ERROR),
				Message: fmt.Sprintf("failed to get the KfDef object: %v", err),
			}
		}
		data, err = GenerateYamlWithOperatorAnnotation(resMap, instance)
		if err != nil {
			return nil, &kfapisv3.KfError{
				Code:    int(kfapisv3.INTERNAL_ERROR),
				Message: fmt.Sprintf("can not encode component %v as yaml: %v", app.Name, err),
			}
		}
	} else {
		data, err = resMap.AsYaml()
		if err != nil {
			return nil, &kfapisv3.KfError{
				Code:    int(kfapisv3.INTERNAL_ERROR),
				Message: fmt.Sprintf("can not encode component %v as yaml: %v", app.Name, err),
			}
		}
	}
	return data, nil
}

// Dump prints the kustomize generated resources to stdout
func (kustomize *kustomize) Dump(resources kftypesv3.ResourceEnum) error {

	applications := make(map[string]bool)
	for _, app := range kustomize.kfDef.Spec.Applications {
		if applications[app.Name] == true {
			// if the application name already
			continue
		}
		applications[app.Name] = true

		data, err := kustomize.render(app)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		fmt.Println("---")
	}
	return nil
}

// Apply deploys kustomize generated resources to the kubenetes api server
func (kustomize *kustomize) Apply(resources kftypesv3.ResourceEnum) error {
	var restConfig *rest.Config = nil
	if kustomize.configOverwrite && kustomize.restConfig != nil {
		restConfig = kustomize.restConfig
	}
	apply, err := utils.NewApply(kustomize.kfDef.ObjectMeta.Namespace, restConfig)
	if err != nil {
		return err
	}

	// Read clusterName and write to KfDef.
	kubeconfig := kftypesv3.GetKubeConfig()
	if kubeconfig == nil {
		log.Warnf("Unable to load .kubeconfig.")
	} else {
		currentCtx := kubeconfig.CurrentContext
		if ctx, ok := kubeconfig.Contexts[currentCtx]; !ok || ctx == nil {
			log.Errorf("Cannot find current-context in kubeconfig.")
		} else {
			log.Infof("Log cluster name into KfDef: %v", ctx.Cluster)
			kustomize.kfDef.ClusterName = ctx.Cluster
		}
	}

	applications := make(map[string]bool)
	for _, app := range kustomize.kfDef.Spec.Applications {
		if applications[app.Name] == true {
			// if the application name already
			continue
		}
		applications[app.Name] = true

		log.Infof("Deploying application %v", app.Name)
		data, err := kustomize.render(app)
		if err != nil {
			return err
		}

		// TODO(https://github.com/kubeflow/manifests/issues/806): Bump the timeout because cert-manager takes
		// a long time to start. Any application that needs to create a certificate will fail because it won't
		// be able to create certificates if cert-manager is unavailable. We should try to identify Permanent Errors
		// and return a PermanentError to avoid retrying and taking 10 minutes to fail.
		b := utils.NewDefaultBackoff()
		b.MaxElapsedTime = 10 * time.Minute
		err = backoff.RetryNotify(
			func() error {
				return apply.Apply(data)
			},
			b,
			func(e error, duration time.Duration) {
				log.Warnf("Encountered error applying application %v: %v", app.Name, e)
				log.Warnf("Will retry in %.0f seconds.", duration.Seconds())
			})
		if err != nil {
			log.Errorf("Permanently failed applying application %v: %v", app.Name, err)
			return err
		}
		log.Infof("Successfully applied application %v", app.Name)
	}

	// Default user namespace when multi-tenancy enabled
	defaultProfileNamespace := kftypesv3.EmailToDefaultName(kustomize.kfDef.Spec.Email)
	// Default user namespace when multi-tenancy disabled
	anonymousNamespace := "anonymous"
	b := utils.NewDefaultBackoff()
	err = backoff.Retry(func() error {
		if !(apply.IfNamespaceExist(defaultProfileNamespace) || apply.IfNamespaceExist(anonymousNamespace)) {
			msg := "Default user namespace pending creation..."
			log.Warnf(msg)
			return &kfapisv3.KfError{
				Code:    int(kfapisv3.INVALID_ARGUMENT),
				Message: msg,
			}
		}
		return nil
	}, b)
	if err != nil {
		log.Warnf("Default namespace creation skipped")
	}
	return nil
}

// deleteGlobalResources is called from Delete and deletes CRDs, ClusterRoles, ClusterRoleBindings
func (kustomize *kustomize) deleteGlobalResources() error {
	if err := kustomize.initK8sClients(); err != nil {
		return &kfapisv3.KfError{
			Code:    int(kfapisv3.INVALID_ARGUMENT),
			Message: fmt.Sprintf("kustomize plugin couldn't initialize a K8s client: %v", err),
		}
	}
	apiextclientset, err := crdclientset.NewForConfig(kustomize.restConfig)
	if err != nil {
		return &kfapisv3.KfError{
			Code:    int(kfapisv3.INTERNAL_ERROR),
			Message: fmt.Sprintf("couldn't get apiextensions client: %v", err),
		}
	}
	do := &metav1.DeleteOptions{}
	lo := metav1.ListOptions{
		LabelSelector: kftypesv3.DefaultAppLabel + "=" + kustomize.kfDef.Name,
	}
	crdsErr := apiextclientset.CustomResourceDefinitions().DeleteCollection(do, lo)
	if crdsErr != nil {
		return &kfapisv3.KfError{
			Code:    int(kfapisv3.INVALID_ARGUMENT),
			Message: fmt.Sprintf("couldn't delete customresourcedefinitions: %v", crdsErr),
		}
	}
	rbacclient, err := rbacv1.NewForConfig(kustomize.restConfig)
	if err != nil {
		return &kfapisv3.KfError{
			Code:    int(kfapisv3.INTERNAL_ERROR),
			Message: fmt.Sprintf("couldn't get rbac/v1 client: %v", err),
		}
	}
	crbsErr := rbacclient.ClusterRoleBindings().DeleteCollection(do, lo)
	if crbsErr != nil {
		return &kfapisv3.KfError{
			Code:    int(kfapisv3.INVALID_ARGUMENT),
			Message: fmt.Sprintf("couldn't delete clusterrolebindings: %v", crbsErr),
		}
	}
	crsErr := rbacclient.ClusterRoles().DeleteCollection(do, lo)
	if crsErr != nil {
		return &kfapisv3.KfError{
			Code:    int(kfapisv3.INVALID_ARGUMENT),
			Message: fmt.Sprintf("couldn't delete clusterroles: %v", crsErr),
		}
	}
	return nil
}

// Delete is called from 'kfctl delete ...'. Will delete all resources deployed from the Apply method
func (kustomize *kustomize) Delete(resources kftypesv3.ResourceEnum) error {
	annotations := kustomize.kfDef.GetAnnotations()
	forceDelete := false
	if forceDel, ok := annotations[strings.Join([]string{utils.KfDefAnnotation, utils.ForceDelete}, "/")]; ok {
		if forceDelBool, err := strconv.ParseBool(forceDel); err == nil {
			forceDelete = forceDelBool
		}
	}
	if forceDelete {
		log.Warnf("Running force deletion.")
	}

	// Get bool value indicating whether this func is called from kubeflow operator
	byOperator := false
	if byOperatorAnn, ok := annotations[strings.Join([]string{utils.KfDefAnnotation, utils.InstallByOperator}, "/")]; ok {
		if byOperatorAnnBol, err := strconv.ParseBool(byOperatorAnn); err == nil {
			byOperator = byOperatorAnnBol
		}
	}

	// Get kubeconfig for cluster and initialize clients
	msg := ""
	kubeconfig := kftypesv3.GetKubeConfig()
	if kubeconfig == nil {
		msg = "unable to load .kubeconfig."
	} else {
		currentCtx := kubeconfig.CurrentContext
		if ctx, ok := kubeconfig.Contexts[currentCtx]; !ok || ctx == nil {
			msg = "cannot find current-context in kubeconfig."
		} else {
			if kustomize.kfDef.ClusterName != ctx.Cluster {
				msg = fmt.Sprintf("cluster name doesn't match: KfDef(%v) v.s. current-context(%v)",
					kustomize.kfDef.ClusterName, ctx.Cluster)
			}
		}
	}
	if msg != "" {
		if forceDelete {
			log.Warnf(msg)
		} else {
			return &kfapisv3.KfError{
				Code:    int(kfapisv3.INVALID_ARGUMENT),
				Message: msg,
			}
		}
	}
	kustomize.initK8sClients()
	kubeclient, err := client.New(kustomize.restConfig, client.Options{})
	if err != nil {
		return &kfapisv3.KfError{
			Code:    int(kfapisv3.INTERNAL_ERROR),
			Message: fmt.Sprintf("error initializing k8s client: %v", err),
		}
	}

	// Delete in reverse application order
	kustomizeDir := path.Join(kustomize.kfDef.Spec.AppDir, outputDir)
	errList := []error{}
	for idx := range kustomize.kfDef.Spec.Applications {
		app := &kustomize.kfDef.Spec.Applications[len(kustomize.kfDef.Spec.Applications)-1-idx]
		log.Infof("Deleting application %v", app.Name)
		resMap, err := EvaluateKustomizeManifest(path.Join(kustomizeDir, app.Name))
		if err != nil {
			log.Errorf("Error evaluating kustomization manifest for %v: %v", app.Name, err)
			return &kfapisv3.KfError{
				Code:    int(kfapisv3.INTERNAL_ERROR),
				Message: fmt.Sprintf("error evaluating kustomization manifest for %v: %v", app.Name, err),
			}
		}

		// Sort resources by kind to make sure we don't experience namespace terminating hanging.
		sortResourceByKind(resMap, utils.UninstallOrder)

		yamlBytes, err := resMap.AsYaml()
		if err != nil {
			return &kfapisv3.KfError{
				Code:    int(kfapisv3.INTERNAL_ERROR),
				Message: fmt.Sprintf("error evaluating kustomization manifest for %v: %v", app.Name, err),
			}
		}
		resources, err := utils.SplitYAML(yamlBytes)
		if err != nil {
			return &kfapisv3.KfError{
				Code:    int(kfapisv3.INTERNAL_ERROR),
				Message: fmt.Sprintf("error splitting yaml: %v", err),
			}
		}
		for _, r := range resources {
			err := utils.DeleteResource(r, kubeclient, 5*time.Minute, byOperator)
			if err != nil {
				msg := fmt.Sprintf("error evaluating kustomization manifest for %v: %v", app.Name, err)
				errList = append(errList, errors.New(msg))
				log.Warn(msg)
			}
		}
	}

	aggrError := errutil.NewAggregate(errList)
	if aggrError != nil {
		return &kfapisv3.KfError{
			Code:    int(kfapisv3.INTERNAL_ERROR),
			Message: fmt.Sprintf("error deleting kustomize manifests: %v", aggrError),
		}
	}

	// Finally, delete the kubeflow namespace
	// TODO(yanniszark): Remove this once the Kubeflow namespace is created by kustomize manifests

	corev1client, err := corev1.NewForConfig(kustomize.restConfig)
	if err != nil {
		return &kfapisv3.KfError{
			Code:    int(kfapisv3.INTERNAL_ERROR),
			Message: fmt.Sprintf("couldn't get core/v1 client: %v", err),
		}
	}
	namespace := kustomize.kfDef.Namespace
	ns, nsMissingErr := corev1client.Namespaces().Get(namespace, metav1.GetOptions{})
	if nsMissingErr == nil {
		// if the func is called by the Kubeflow operator, validate it is installed through the operator
		if byOperator {
			anns := ns.GetAnnotations()
			kfdefAnn := strings.Join([]string{utils.KfDefAnnotation, utils.KfDefInstance}, "/")
			_, found := anns[kfdefAnn]
			if !found {
				return nil
			}
		}

		log.Infof("Deleting namespace: %v", namespace)
		nsErr := corev1client.Namespaces().Delete(ns.Name, metav1.NewDeleteOptions(int64(100)))
		if nsErr != nil {
			return &kfapisv3.KfError{
				Code:    int(kfapisv3.INVALID_ARGUMENT),
				Message: fmt.Sprintf("couldn't delete namespace %v: %v", namespace, nsErr),
			}
		}
	}

	return nil
}

// sortResourceByKind does in-place sort of resources by Kind.
func sortResourceByKind(resMap resmap.ResMap, order utils.SortOrder) {
	resourcesInUninstallOrder := utils.SortByKind(resMap.Resources(), order)

	// Need to remove existing resource and append them in order.
	allIdsToRemove := resMap.AllIds()
	for _, idToRemove := range allIdsToRemove {
		resMap.Remove(idToRemove)
	}

	for _, resourceToAdd := range resourcesInUninstallOrder {
		resMap.Append(resourceToAdd)
	}
}

// Generate is called from 'kfctl generate ...' and produces yaml output files under <deployment>/kustomize.
// One yaml file per component
func (kustomize *kustomize) Generate(resources kftypesv3.ResourceEnum) error {
	generate := func() error {
		kustomizeDir := path.Join(kustomize.kfDef.Spec.AppDir, outputDir)

		if _, err := os.Stat(kustomizeDir); err == nil {
			// When using the new stacks code the directory might already exist because it could have
			// been created by calls to SetApplicationParameter. For the legacy code path (no stacks) we preserve
			// the existing code path of not rerunning generate if the directory already exists.
			if !kustomize.kfDef.UsingStacks() {
				// Noop if the directory already exists.
				log.Infof("Folder %v exists, skip kustomize.Generate", kustomizeDir)
				return nil
			}
		} else if !os.IsNotExist(err) {
			log.Errorf("Stat folder %v error: %v; try deleting it...", kustomizeDir, err)
			_ = os.RemoveAll(kustomizeDir)
		}

		kustomizeDirErr := os.MkdirAll(kustomizeDir, os.ModePerm)
		if kustomizeDirErr != nil {
			return &kfapisv3.KfError{
				Code:    int(kfapisv3.INVALID_ARGUMENT),
				Message: fmt.Sprintf("couldn't create directory %v: %v", kustomizeDir, kustomizeDirErr),
			}
		}

		_, ok := kustomize.kfDef.GetRepoCache(kftypesv3.ManifestsRepoName)
		if !ok {
			log.Infof("Repo %v not listed in KfDef.Status; Resync'ing cache", kftypesv3.ManifestsRepoName)
			if err := kustomize.kfDef.SyncCache(); err != nil {
				log.Errorf("Syncing the cached failed: %v", err)
				return errors.WithStack(err)
			}
		}

		// Check again after sync
		_, ok = kustomize.kfDef.GetRepoCache(kftypesv3.ManifestsRepoName)
		if !ok {
			return errors.WithStack(fmt.Errorf("Repo %v not listed in KfDef.Status; ", kftypesv3.ManifestsRepoName))
		}

		// determine whether we are using the new pattern of using kustomize to build stacks.
		// hasStack := kustomize.kfDef.UsingStacks()
		for _, app := range kustomize.kfDef.Spec.Applications {
			log.Infof("Processing application: %v", app.Name)

			if app.KustomizeConfig == nil {
				err := fmt.Errorf("application %v is missing KustomizeConfig", app.Name)
				log.Errorf("%v", err)
				return &kfapisv3.KfError{
					Code:    int(kfapisv3.INTERNAL_ERROR),
					Message: err.Error(),
				}
			}

			repoName := app.KustomizeConfig.RepoRef.Name
			repoCache, ok := kustomize.kfDef.GetRepoCache(repoName)
			if !ok {
				err := fmt.Errorf("application %v refers to repo %v which wasn't found in KfDef.Status.ReposCache", app.Name, repoName)
				log.Errorf("%v", err)
				return &kfapisv3.KfError{
					Code:    int(kfapisv3.INTERNAL_ERROR),
					Message: err.Error(),
				}
			}

			appPath := path.Join(repoCache.LocalPath, app.KustomizeConfig.RepoRef.Path)

			if kustomize.kfDef.UsingStacks() {

				if filepath.IsAbs(appPath) {
					// The appPath needs to be a relative path because we use it as a resource location in the kustomize
					// file
					appDir, err := filepath.Abs(kustomize.kfDef.Spec.AppDir)

					if err != nil {
						errors.WithStack(fmt.Errorf("There was a problem computing absolute path of %v; error; %v ", kustomize.kfDef.Spec.AppDir, err))
					}
					relPath, err := filepath.Rel(appDir, appPath)
					if err != nil {
						errors.WithStack(fmt.Errorf("There was a problem computing filePath.Rel(%v, %v); error; %v ", appDir, appPath, err))
					}

					appPath = relPath
				}
				// We handle generating the kustomize dir for application stacks differently.
				stackAppDir := path.Join(kustomizeDir, app.Name)

				// Path to the stack inside the cache.
				stacksCacheDir := filepath.Join("../..", appPath)
				if _, err := createStackAppKustomization(stackAppDir, stacksCacheDir); err != nil {
					return errors.WithStack(fmt.Errorf("There was a problem building the kustomize app for the Kubeflow application stack; %v ", err))
				}
			} else {
				// TODO(jlewi): This code path should eventually go away once we are fully migrated to the use
				// of stacks.
				// Copy the component to kustomizeDir
				if err := copy.Copy(appPath, path.Join(kustomizeDir, app.Name)); err != nil {
					return &kfapisv3.KfError{
						Code:    int(kfapisv3.INTERNAL_ERROR),
						Message: fmt.Sprintf("couldn't copy application %s: %v", app.Name, err),
					}
				}
				if err := GenerateKustomizationFile(kustomize.kfDef, kustomizeDir, app.Name,
					app.KustomizeConfig.Overlays, app.KustomizeConfig.Parameters); err != nil {
					return &kfapisv3.KfError{
						Code:    int(kfapisv3.INTERNAL_ERROR),
						Message: fmt.Sprintf("couldn't generate kustomization file for component %s: %v", app.Name, err),
					}
				}
			}
		}
		return nil
	}

	switch resources {
	case kftypesv3.PLATFORM:
	case kftypesv3.ALL:
		fallthrough
	case kftypesv3.K8S:
		generateErr := generate()
		if generateErr != nil {
			return fmt.Errorf("Kustomize generate failed: %v", generateErr)
		}
	}
	return nil
}

// createStackAppKustomization generates a kustomization.yaml file suitable for the kubeflow application stack.
// stackAppDir is the directory to create for the kustomize package.
// basePath is the path to the kustomize package to use as the base package.
//
// Returns the path to the kusotmizationFile.
//
// If the kustomization.yaml already exists then the changes are merged in.
func createStackAppKustomization(stackAppDir string, basePath string) (string, error) {
	kustomizationFile := filepath.Join(stackAppDir, kftypesv3.KustomizationFile)

	if _, err := os.Stat(stackAppDir); err == nil {
		// Noop if the directory already exists.
		log.Infof("folder %v exists", stackAppDir)
	} else if os.IsNotExist(err) {
		log.Infof("Creating folder %v", stackAppDir)
		if stackAppDirErr := os.MkdirAll(stackAppDir, os.ModePerm); stackAppDirErr != nil {
			return "", &kfapisv3.KfError{
				Code:    int(kfapisv3.INVALID_ARGUMENT),
				Message: fmt.Sprintf("couldn't create directory %v Error %v", stackAppDir, stackAppDirErr),
			}
		}
	} else {
		return "", errors.WithStack(errors.Wrapf(err, "Unexpected error trying to access directory: %v", stackAppDir))
	}

	kustomization := &types.Kustomization{}

	contents, err := ioutil.ReadFile(kustomizationFile)

	// The kustomization file may not exist yet in which case we keep going because we will just create it.
	if err == nil {
		if err := yaml.Unmarshal(contents, kustomization); err != nil {
			return "", errors.WithStack(errors.Wrapf(err, "Failed to unmashal %v", kustomizationFile))
		}
	} else if err != nil && !os.IsNotExist(err) {
		return "", errors.WithStack(errors.Wrapf(err, "Failed to read: %v", kustomizationFile))
	}

	// Create the kustomization file for the stack directory.
	// We explicitly do not set namespace because we want to use the default namespace set in each kustomize
	// application.
	kustomization.TypeMeta = types.TypeMeta{
		Kind:       "Kustomization",
		APIVersion: "kustomize.config.k8s.io/v1beta1",
	}

	hasBasePath := false
	for _, r := range kustomization.Resources {
		if string(r) == basePath {
			hasBasePath = true
			break
		}
	}

	if !hasBasePath {
		kustomization.Resources = append(kustomization.Resources, basePath)
	}
	yaml, err := yaml.Marshal(kustomization)

	if err != nil {
		return "", errors.WithStack(errors.Wrapf(err, "Error trying to marshal kustomization for kubeflow apps stack:"))
	}

	kustomizationFileErr := ioutil.WriteFile(kustomizationFile, yaml, 0644)
	if kustomizationFileErr != nil {
		return "", &kfapisv3.KfError{
			Code:    int(kfapisv3.INTERNAL_ERROR),
			Message: fmt.Sprintf("error writing to %v Error %v", kustomizationFile, kustomizationFileErr),
		}
	}

	return kustomizationFile, nil
}

// Init is called from 'kfctl init ...' and creates a <deployment> directory with an app.yaml file that
// holds deployment information like components, parameters
func (kustomize *kustomize) Init(resources kftypesv3.ResourceEnum) error {
	return nil
}

// mapDirs is a recursive method that will return a map of component -> path-to-kustomization.yaml
// under the manifests downloaded cache
func (kustomize *kustomize) mapDirs(dirPath string, root bool, depth int, leafMap map[string]string) map[string]string {
	dirName := path.Base(dirPath)
	// package is component, stop here
	if depth == 1 && kustomize.packageMap[dirName] != nil && kustomize.componentMap[dirName] {
		subdirCheck := path.Join(dirPath, dirName)
		// border case manifests/jupyter/jupyter
		if _, err := os.Stat(subdirCheck); err != nil {
			leafMap[dirName] = dirName
			arrayOfComponents := *kustomize.packageMap[dirName]
			arrayOfComponents = append(arrayOfComponents, dirName)
			kustomize.packageMap[dirName] = &arrayOfComponents
			return leafMap
		}
	}
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return leafMap
	}
	for _, f := range files {
		if f.IsDir() {
			leafDir := path.Join(dirPath, f.Name())
			if depth < 2 {
				kustomize.mapDirs(leafDir, false, depth+1, leafMap)
			}
		}
	}
	if depth == 2 {
		repoCache, ok := kustomize.kfDef.GetRepoCache(kftypesv3.ManifestsRepoName)
		if !ok {
			log.Fatal("Manifest repo not found in cache")
		}
		componentPath := extractSuffix(repoCache.LocalPath, dirPath)
		packageName := strings.Split(componentPath, "/")[0]
		if components, exists := kustomize.packageMap[packageName]; exists {
			leafMap[path.Base(dirPath)] = componentPath
			arrayOfComponents := *components
			arrayOfComponents = append(arrayOfComponents, dirName)
			kustomize.packageMap[packageName] = &arrayOfComponents
		}
	}
	return leafMap
}

func (kustomize *kustomize) SetK8sRestConfig(r *rest.Config) {
	kustomize.restConfig = r
	kustomize.configOverwrite = true
}

// GetKustomization will read a kustomization.yaml and return Kustomization type
func GetKustomization(kustomizationPath string) *types.Kustomization {
	kustomizationFile := filepath.Join(kustomizationPath, kftypesv3.KustomizationFile)
	data, err := ioutil.ReadFile(kustomizationFile)
	if err != nil {
		log.Warnf("Cannot get kustomization from %v: %v", kustomizationPath, err)
		return nil
	}
	kustomization := &types.Kustomization{}
	if err = yaml.Unmarshal(data, kustomization); err != nil {
		log.Warnf("Cannot unmarshal kustomization from %v: %v", kustomizationPath, err)
		return nil
	}
	return kustomization
}

// ReadUnstructured will read a resource .yaml and return the Unstructured type
func ReadUnstructured(kfDefFile string) (*unstructured.Unstructured, error) {
	data, err := ioutil.ReadFile(kfDefFile)
	if err != nil {
		return nil, err
	}
	def := &unstructured.Unstructured{}
	if err = yaml.Unmarshal(data, def); err != nil {
		return nil, err
	}
	return def, nil
}

// ReadKfDef will read a config .yaml and return the KfDef type
func ReadKfDef(kfDefFile string) *kfdefsv3.KfDef {
	data, err := ioutil.ReadFile(kfDefFile)
	if err != nil {
		return nil
	}
	kfdef := &kfdefsv3.KfDef{}
	if err = yaml.Unmarshal(data, kfdef); err != nil {
		return nil
	}
	return kfdef
}

// WriteKfDef will write a KfDef to a config .yaml
func WriteKfDef(kfdef *kfdefsv3.KfDef, kfdefpath string) error {
	data, err := yaml.Marshal(kfdef)
	if err != nil {
		return err
	}
	writeErr := ioutil.WriteFile(kfdefpath, data, 0644)
	if writeErr != nil {
		return writeErr
	}
	return nil
}

// MergeKustomization will merge the child into the parent
// if the child has no bases, then the parent just needs to add the child as base
// otherwise the parent needs to merge with behaviors
// Multiple overlays are constrained in what they can merge
// which exclude NamePrefixes, NameSuffixes, CommonLabels, CommonAnnotations.
// Any of these will generate an error
func MergeKustomization(compDir string, targetDir string, kfDef *kfconfig.KfConfig, params []kfconfig.NameValue,
	parent *types.Kustomization, child *types.Kustomization, kustomizationMaps map[MapType]map[string]bool) error {

	paramMap := make(map[string]string)
	for _, nv := range params {
		paramMap[nv.Name] = nv.Value
	}
	updateParamFiles := func() error {
		paramFile := filepath.Join(targetDir, kftypesv3.KustomizationParamFile)
		if _, err := os.Stat(paramFile); err == nil {
			params, paramFileErr := readLines(paramFile)
			if paramFileErr != nil {
				return &kfapisv3.KfError{
					Code:    int(kfapisv3.INVALID_ARGUMENT),
					Message: fmt.Sprintf("could not open %v: %v", paramFile, paramFileErr),
				}
			}
			// in params.env look for name=value that we can substitute from componentParams[component]
			// or if there is just namespace= or project= - fill in the values from KfDef
			for i, param := range params {
				paramName := strings.Split(param, "=")[0]
				if val, ok := paramMap[paramName]; ok && val != "" {
					switch paramName {
					case "generateName":
						arr := strings.Split(param, "=")
						if len(arr) == 1 || arr[1] == "" {
							b := make([]byte, 4) //equals 8 charachters
							rand.Read(b)
							s := hex.EncodeToString(b)
							val += s
						}
					}
					params[i] = paramName + "=" + val
				} else {
					switch paramName {
					case "appName":
						params[i] = paramName + "=" + kfDef.Name
					case "namespace":
						params[i] = paramName + "=" + kfDef.Namespace
					case "project":
						params[i] = paramName + "=" + kfDef.Spec.Project
					}
				}
			}
			paramFileErr = writeLines(params, paramFile)
			if paramFileErr != nil {
				return &kfapisv3.KfError{
					Code:    int(kfapisv3.INTERNAL_ERROR),
					Message: fmt.Sprintf("could not update %v: %v", paramFile, paramFileErr),
				}
			}
		}
		return nil
	}

	updateGeneratorArgs := func(parentGeneratorArgs *types.GeneratorArgs, childGeneratorArgs types.GeneratorArgs) {
		if childGeneratorArgs.EnvSources != nil && len(childGeneratorArgs.EnvSources) > 0 {
			parentGeneratorArgs.EnvSources = make([]string, 0)
			for _, envSource := range childGeneratorArgs.EnvSources {
				envAbsolutePathSource := path.Join(targetDir, envSource)
				parentGeneratorArgs.EnvSources = append(parentGeneratorArgs.EnvSources, extractSuffix(compDir, envAbsolutePathSource))
			}
		}
		if childGeneratorArgs.EnvSource != "" {
			envAbsolutePathSource := path.Join(targetDir, childGeneratorArgs.EnvSource)
			envSource := extractSuffix(compDir, envAbsolutePathSource)
			parentGeneratorArgs.EnvSource = envSource
		}
		if childGeneratorArgs.FileSources != nil && len(childGeneratorArgs.FileSources) > 0 {
			parentGeneratorArgs.FileSources = make([]string, 0)
			for _, fileSource := range childGeneratorArgs.FileSources {
				fileAbsolutePathSource := path.Join(targetDir, fileSource)
				parentGeneratorArgs.EnvSource = extractSuffix(compDir, fileAbsolutePathSource)
			}
		}
		if childGeneratorArgs.LiteralSources != nil && len(childGeneratorArgs.LiteralSources) > 0 {
			parentGeneratorArgs.LiteralSources = make([]string, 0)
			for _, literalSource := range childGeneratorArgs.LiteralSources {
				parentGeneratorArgs.LiteralSources = append(parentGeneratorArgs.LiteralSources, literalSource)
			}
		}
	}

	updateConfigMapArgs := func(parentConfigMapArgs *types.ConfigMapArgs, childConfigMapArgs types.ConfigMapArgs) {
		parentConfigMapArgs.Name = childConfigMapArgs.Name
		parentConfigMapArgs.Namespace = childConfigMapArgs.Namespace
		updateGeneratorArgs(&parentConfigMapArgs.GeneratorArgs, childConfigMapArgs.GeneratorArgs)
		behavior := types.NewGenerationBehavior(childConfigMapArgs.Behavior)
		switch behavior {
		case types.BehaviorCreate:
			if _, ok := kustomizationMaps[configMapGeneratorMap][childConfigMapArgs.Name]; !ok {
				parent.ConfigMapGenerator = append(parent.ConfigMapGenerator, *parentConfigMapArgs)
				kustomizationMaps[configMapGeneratorMap][childConfigMapArgs.Name] = true
			}
		case types.BehaviorMerge, types.BehaviorReplace, types.BehaviorUnspecified:
			fallthrough
		default:
			parentConfigMapArgs.Behavior = behavior.String()
			parent.ConfigMapGenerator = append(parent.ConfigMapGenerator, *parentConfigMapArgs)
			kustomizationMaps[configMapGeneratorMap][childConfigMapArgs.Name] = true
		}
	}

	if err := updateParamFiles(); err != nil {
		return err
	}
	if child.Bases == nil {
		basePath := extractSuffix(compDir, targetDir)
		if _, ok := kustomizationMaps[basesMap][basePath]; !ok {
			parent.Bases = append(parent.Bases, basePath)
			kustomizationMaps[basesMap][basePath] = true
		}
		return nil
	}
	for _, value := range child.Bases {
		baseAbsolutePath := path.Join(targetDir, value)
		basePath := extractSuffix(compDir, baseAbsolutePath)
		if _, ok := kustomizationMaps[basesMap][basePath]; !ok {
			parent.Bases = append(parent.Bases, basePath)
			kustomizationMaps[basesMap][basePath] = true
		} else {
			childPath := extractSuffix(compDir, targetDir)
			kustomizationMaps[basesMap][childPath] = true
		}
	}
	if child.NamePrefix != "" && parent.NamePrefix == "" {
		parent.NamePrefix = child.NamePrefix
	}
	if child.NameSuffix != "" && parent.NameSuffix == "" {
		parent.NameSuffix = child.NameSuffix
	}
	for k, v := range child.CommonLabels {
		//allow replacement
		parent.CommonLabels[k] = v
		kustomizationMaps[commonLabelsMap][k] = true
	}
	for k, v := range child.CommonAnnotations {
		//allow replacement
		parent.CommonAnnotations[k] = v
		kustomizationMaps[commonAnnotationsMap][k] = true
	}

	if child.GeneratorOptions != nil && parent.GeneratorOptions == nil {
		parent.GeneratorOptions = child.GeneratorOptions
	}
	for _, value := range child.Resources {
		resourceAbsoluteFile := filepath.Join(targetDir, string(value))
		resourceFile := extractSuffix(compDir, resourceAbsoluteFile)
		if _, ok := kustomizationMaps[resourcesMap][resourceFile]; !ok {
			parent.Resources = append(parent.Resources, resourceFile)
			kustomizationMaps[resourcesMap][resourceFile] = true
		}
	}
	for _, value := range child.Images {
		imageName := value.Name
		if _, ok := kustomizationMaps[imagesMap][imageName]; !ok {
			parent.Images = append(parent.Images, value)
			kustomizationMaps[imagesMap][imageName] = true
		} else {
			kFile := filepath.Join(targetDir, kftypesv3.KustomizationFile)
			log.Warnf("Ignoring image %v specified in %v", imageName, kFile)
		}
	}
	for _, value := range child.Crds {
		if _, ok := kustomizationMaps[crdsMap][value]; !ok {
			parent.Crds = append(parent.Crds, value)
			kustomizationMaps[crdsMap][value] = true
		} else {
			kFile := filepath.Join(targetDir, kftypesv3.KustomizationFile)
			log.Warnf("Ignoring crd %v specified in %v", value, kFile)
		}
	}
	for _, value := range child.ConfigMapGenerator {
		parentConfigMapArgs := new(types.ConfigMapArgs)
		updateConfigMapArgs(parentConfigMapArgs, value)
	}
	for _, value := range child.SecretGenerator {
		secretName := value.Name
		secretBehavior := types.NewGenerationBehavior(value.Behavior)
		updateGeneratorArgs(&value.GeneratorArgs, value.GeneratorArgs)
		switch secretBehavior {
		case types.BehaviorCreate:
			if _, ok := kustomizationMaps[secretsMapGeneratorMap][secretName]; !ok {
				parent.SecretGenerator = append(parent.SecretGenerator, value)
				kustomizationMaps[secretsMapGeneratorMap][secretName] = true
			}
		case types.BehaviorMerge, types.BehaviorReplace:
			parent.SecretGenerator = append(parent.SecretGenerator, value)
			kustomizationMaps[secretsMapGeneratorMap][secretName] = true
		default:
			value.Behavior = secretBehavior.String()
			parent.SecretGenerator = append(parent.SecretGenerator, value)
			kustomizationMaps[secretsMapGeneratorMap][secretName] = true
		}
	}
	for _, value := range child.Vars {
		varName := value.Name
		if _, ok := kustomizationMaps[varsMap][varName]; !ok {
			parent.Vars = append(parent.Vars, value)
			kustomizationMaps[varsMap][varName] = true
		} else {
			kFile := filepath.Join(targetDir, kftypesv3.KustomizationFile)
			log.Warnf("Ignoring var %v specified in %v", varName, kFile)
		}
	}
	for _, value := range child.PatchesStrategicMerge {
		patchAbsoluteFile := filepath.Join(targetDir, string(value))
		patchFile := extractSuffix(compDir, patchAbsoluteFile)
		if _, ok := kustomizationMaps[patchesStrategicMergeMap][patchFile]; !ok {
			patchFileCasted := types.PatchStrategicMerge(patchFile)
			parent.PatchesStrategicMerge = append(parent.PatchesStrategicMerge, patchFileCasted)
			kustomizationMaps[patchesStrategicMergeMap][patchFile] = true
		}
	}
	// json patches are aggregated and merged into local patch files
	for _, value := range child.PatchesJson6902 {
		patchJson := new(types.PatchJson6902)
		patchJson.Target = value.Target
		patchAbsolutePath := filepath.Join(targetDir, value.Path)
		patchJson.Path = extractSuffix(compDir, patchAbsolutePath)
		// patchJson.Path can be used for multiple targets, hence kustomizationMaps key is patchJson.Path+"-"+patchJson.Target.Name"
		patchJsonMapKey := patchJson.Path + "-" + patchJson.Target.Name
		if _, ok := kustomizationMaps[patchesJson6902Map][patchJsonMapKey]; !ok {
			parent.PatchesJson6902 = append(parent.PatchesJson6902, *patchJson)
			kustomizationMaps[patchesJson6902Map][patchJsonMapKey] = true
		}
	}
	for _, value := range child.Configurations {
		configurationAbsolutePath := filepath.Join(targetDir, value)
		configurationPath := extractSuffix(compDir, configurationAbsolutePath)
		if _, ok := kustomizationMaps[configurationsMap][configurationPath]; !ok {
			parent.Configurations = append(parent.Configurations, configurationPath)
			kustomizationMaps[configurationsMap][configurationPath] = true
		}
	}
	return nil
}

// MergeKustomizations will merge base and all overlay kustomization files into
// a single kustomization file
func MergeKustomizations(kfDef *kfconfig.KfConfig, compDir string, overlayParams []string, params []kfconfig.NameValue) (*types.Kustomization, error) {
	kustomizationMaps := CreateKustomizationMaps()
	kustomization := &types.Kustomization{
		TypeMeta: types.TypeMeta{
			APIVersion: types.KustomizationVersion,
			Kind:       types.KustomizationKind,
		},
		Bases:                 make([]string, 0),
		CommonLabels:          make(map[string]string),
		CommonAnnotations:     make(map[string]string),
		PatchesStrategicMerge: make([]types.PatchStrategicMerge, 0),
		PatchesJson6902:       make([]types.PatchJson6902, 0),
		Images:                make([]image.Image, 0),
		Vars:                  make([]types.Var, 0),
		Crds:                  make([]string, 0),
		Resources:             make([]string, 0),
		ConfigMapGenerator:    make([]types.ConfigMapArgs, 0),
		SecretGenerator:       make([]types.SecretArgs, 0),
		Configurations:        make([]string, 0),
	}
	baseDir := path.Join(compDir, "base")
	base := GetKustomization(baseDir)
	if base == nil {
		comp := GetKustomization(compDir)
		if comp != nil {
			return comp, nil
		}
	} else {
		err := MergeKustomization(compDir, baseDir, kfDef, params, kustomization, base, kustomizationMaps)
		if err != nil {
			return nil, &kfapisv3.KfError{
				Code:    int(kfapisv3.INTERNAL_ERROR),
				Message: fmt.Sprintf("error merging kustomization at %v: %v", baseDir, err),
			}
		}
	}
	if params != nil {
		for _, nv := range params {
			name := nv.Name
			switch name {
			case "namespace":
				kustomization.Namespace = nv.Value
			}
		}
	}
	for _, overlayParam := range overlayParams {
		overlayDir := path.Join(compDir, "overlays", overlayParam)
		if _, err := os.Stat(overlayDir); err == nil {
			err := MergeKustomization(compDir, overlayDir, kfDef, params, kustomization,
				GetKustomization(overlayDir), kustomizationMaps)
			if err != nil {
				return nil, &kfapisv3.KfError{
					Code:    int(kfapisv3.INTERNAL_ERROR),
					Message: fmt.Sprintf("error merging kustomization at %v: %v", overlayDir, err),
				}
			}
		} else {
			return nil, &kfapisv3.KfError{
				Code:    int(kfapisv3.INTERNAL_ERROR),
				Message: fmt.Sprintf("no overlay %v for component at %v: %v", overlayParam, compDir, err),
			}
		}
	}
	if len(kustomization.PatchesJson6902) > 0 {
		patches := map[string][]types.PatchJson6902{}
		for _, jsonPatch := range kustomization.PatchesJson6902 {
			key := jsonPatch.Target.Name + "-" + jsonPatch.Target.Kind
			if _, exists := patches[key]; !exists {
				patchArray := make([]types.PatchJson6902, 0)
				patchArray = append(patchArray, jsonPatch)
				patches[key] = patchArray
			} else {
				patches[key] = append(patches[key], jsonPatch)
			}
		}
		kustomization.PatchesJson6902 = make([]types.PatchJson6902, 0)
		patchFile := ""
		for key, values := range patches {
			aggregatedPatch := new(types.PatchJson6902)
			aggregatedPatch.Path = key + ".yaml"
			patchFile = path.Join(compDir, aggregatedPatch.Path)
			aggregatedPatch.Target = new(types.PatchTarget)
			aggregatedPatch.Target.Name = values[0].Target.Name
			aggregatedPatch.Target.Namespace = values[0].Target.Namespace
			aggregatedPatch.Target.Group = values[0].Target.Group
			aggregatedPatch.Target.Version = values[0].Target.Version
			aggregatedPatch.Target.Kind = values[0].Target.Kind
			aggregatedPatch.Target.Gvk = values[0].Target.Gvk
			for _, eachPatch := range values {
				patchPath := path.Join(compDir, eachPatch.Path)
				if _, err := os.Stat(patchPath); err == nil {
					data, err := ioutil.ReadFile(patchPath)
					if err != nil {
						return nil, err
					}
					f, patchErr := os.OpenFile(patchFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					if patchErr != nil {
						return nil, patchErr
					}
					if _, err := f.Write(data); err != nil {
						f.Close()
						return nil, err
					}
					if err := f.Close(); err != nil {
						return nil, err
					}
				}
			}
			kustomization.PatchesJson6902 = append(kustomization.PatchesJson6902, *aggregatedPatch)
		}
	}
	return kustomization, nil
}

// GenerateKustomizationFile will create a kustomization.yaml
// It will parse a args structure that provides mixin or multiple overlays to be merged with the base kustomization file
// for example
//
//   componentParams:
//    tf-job-operator:
//    - name: overlay
//      value: namespaced-gangscheduled
//
// TODO(https://github.com/kubeflow/kubeflow/issues/3491): As part of fixing the discovery
// logic we should change the KfDef spec to provide a list of applications (not a map).
// and preserve order when applying them so we can get rid of the logic hard-coding
// moving some applications to the front.
//
// TODO(jlewi): Why is the path split between root and compPath?
// TODO(jlewi): Why is this taking kfDef and writing kfDef? Is this because it is reordering components?
// TODO(jlewi): This function appears to special case handling of using kustomize
// for KfDef. Presumably this is because of the code in coordinator which is using it to generate
// KfDef from overlays. But this function is also used to generate the manifests for the individual
// kustomize packages.
func GenerateKustomizationFile(kfDef *kfconfig.KfConfig, root string,
	compPath string, overlays []string, params []kfconfig.NameValue) error {

	moveToFront := func(item string, list []string) []string {
		olen := len(list)
		newlist := make([]string, 0)
		for i, component := range list {
			if component == item {
				newlist = append(newlist, list[i])
				newlist = append(newlist, list[0:i]...)
				newlist = append(newlist, list[i+1:olen]...)
				break
			}
		}
		return newlist
	}
	compDir := path.Join(root, compPath)
	kustomization, kustomizationErr := MergeKustomizations(kfDef, compDir, overlays, params)
	if kustomizationErr != nil {
		return kustomizationErr
	}
	if kustomization.Namespace == "" {
		kustomization.Namespace = kfDef.Namespace
	}
	//TODO(#2685) we may want to delegate this to separate tooling so kfctl is not dynamically mixing in overlays.
	if len(kustomization.PatchesStrategicMerge) > 0 {
		basename := filepath.Base(string(kustomization.PatchesStrategicMerge[0]))
		basefile := filepath.Join(compDir, "base", basename)
		def, err := ReadUnstructured(basefile)
		if err != nil {
			return err
		}
		apiVersion := def.GetAPIVersion()
		if apiVersion == kfDef.APIVersion {
			// This code is only invoked when using Kustomize to generate the KFDef spec.
			baseKfDef := ReadKfDef(basefile)
			for _, k := range kustomization.PatchesStrategicMerge {
				overlayfile := filepath.Join(compDir, string(k))
				overlay := ReadKfDef(overlayfile)
				mergeErr := mergo.Merge(&baseKfDef.Spec, overlay.Spec, mergo.WithAppendSlice)
				if mergeErr != nil {
					return mergeErr
				}
			}
			//TODO look at sort options
			//See https://github.com/kubernetes-sigs/kustomize/issues/821
			//TODO upgrade to v2.0.4 when available
			baseKfDef.Spec.Components = moveToFront("application", baseKfDef.Spec.Components)
			baseKfDef.Spec.Components = moveToFront("application-crds", baseKfDef.Spec.Components)
			baseKfDef.Spec.Components = moveToFront("istio", baseKfDef.Spec.Components)
			baseKfDef.Spec.Components = moveToFront("istio-install", baseKfDef.Spec.Components)
			baseKfDef.Spec.Components = moveToFront("istio-crds", baseKfDef.Spec.Components)
			writeErr := WriteKfDef(baseKfDef, basefile)
			if writeErr != nil {
				return writeErr
			}
			kustomization.PatchesStrategicMerge = nil
		}
	}
	buf, bufErr := yaml.Marshal(kustomization)
	if bufErr != nil {
		return bufErr
	}
	kustomizationPath := filepath.Join(compDir, kftypesv3.KustomizationFile)
	kustomizationPathErr := ioutil.WriteFile(kustomizationPath, buf, 0644)
	return kustomizationPathErr
}

// EvaluateKustomizeManifest evaluates the kustomize dir compDir, and returns the resources.
func EvaluateKustomizeManifest(compDir string) (resmap.ResMap, error) {
	fsys := fs.MakeRealFS()
	// We don't enforce the security check because our kustomize packages are such that kustomization.yaml
	// files may refer to patches and resources that are not in the current directory or below them.
	// See http://bit.ly/kf_kustomize_v3
	lrc := loader.RestrictionNone
	ldr, err := loader.NewLoader(lrc, validators.MakeFakeValidator(), compDir, fsys)
	if err != nil {
		return nil, err
	}
	defer ldr.Cleanup()
	rf := resmap.NewFactory(resource.NewFactory(kunstruct.NewKunstructuredFactoryImpl()), transformer.NewFactoryImpl())
	pc := plugins.DefaultPluginConfig()
	kt, err := target.NewKustTarget(ldr, rf, transformer.NewFactoryImpl(), plugins.NewLoader(pc, rf))
	if err != nil {
		return nil, err
	}
	allResources, err := kt.MakeCustomizedResMap()
	if err != nil {
		return nil, err
	}
	err = builtin.NewLegacyOrderTransformerPlugin().Transform(allResources)
	if err != nil {
		return nil, err
	}
	return allResources, nil
}

func WriteKustomizationFile(name string, kustomizeDir string, resMap resmap.ResMap) error {

	// Output the objects.

	yamlResources, yamlResourcesErr := resMap.AsYaml()
	if yamlResourcesErr != nil {
		return &kfapisv3.KfError{
			Code:    int(kfapisv3.INTERNAL_ERROR),
			Message: fmt.Sprintf("error generating yaml: %v", yamlResourcesErr),
		}
	}
	kustomizeFile := filepath.Join(kustomizeDir, name+".yaml")
	kustomizationFileErr := ioutil.WriteFile(kustomizeFile, yamlResources, 0644)
	if kustomizationFileErr != nil {
		return &kfapisv3.KfError{
			Code:    int(kfapisv3.INTERNAL_ERROR),
			Message: fmt.Sprintf("error writing to %v: %v", kustomizeFile, kustomizationFileErr),
		}
	}
	return nil
}

// readLines reads a file into an array of strings
func readLines(path string) ([]string, error) {
	var file, err = os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// writeLines writes a string array to the given file - one line per array entry.
func writeLines(lines []string, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
	return w.Flush()
}

// extractSuffix will return the non-overlapped part of 2 paths eg
// /foo/bar/baz/zed and /foo/bar/ will return baz/zed
func extractSuffix(dirPath string, subDirPath string) string {
	suffix := strings.TrimPrefix(subDirPath, dirPath)[1:]
	return suffix
}

func CreateKustomizationMaps() map[MapType]map[string]bool {
	return map[MapType]map[string]bool{
		basesMap:                 make(map[string]bool),
		commonAnnotationsMap:     make(map[string]bool),
		commonLabelsMap:          make(map[string]bool),
		imagesMap:                make(map[string]bool),
		resourcesMap:             make(map[string]bool),
		crdsMap:                  make(map[string]bool),
		varsMap:                  make(map[string]bool),
		configurationsMap:        make(map[string]bool),
		configMapGeneratorMap:    make(map[string]bool),
		secretsMapGeneratorMap:   make(map[string]bool),
		patchesStrategicMergeMap: make(map[string]bool),
		patchesJson6902Map:       make(map[string]bool),
	}
}

// GenerateYamlWithOperatorAnnotation adds operator info to the annotation to every resource
// some code copied from ResMap.AsYaml() func
func GenerateYamlWithOperatorAnnotation(resMap resmap.ResMap, instance *unstructured.Unstructured) ([]byte, error) {
	firstObj := true
	var b []byte
	buf := bytes.NewBuffer(b)
	for _, res := range resMap.Resources() {
		y, err := res.AsYAML()
		if err != nil {
			return nil, err
		}
		m := &unstructured.Unstructured{}
		if err = yaml.Unmarshal(y, m); err != nil {
			return nil, err
		}

		anns := m.GetAnnotations()
		if anns == nil {
			anns = map[string]string{}
		}
		kfdefAnn := strings.Join([]string{utils.KfDefAnnotation, utils.KfDefInstance}, "/")
		kfdefCr := strings.Join([]string{instance.GetName(), instance.GetNamespace()}, ".")

		addAnnotation := true
		if m.GetKind() == "Namespace" {
			config, _ := rest.InClusterConfig()
			corev1client, err := corev1.NewForConfig(config)
			if err != nil {
				return nil, &kfapisv3.KfError{
					Code:    int(kfapisv3.INTERNAL_ERROR),
					Message: fmt.Sprintf("failed to create corev1 client: %v", err),
				}
			}
			ns, err := corev1client.Namespaces().Get(m.GetName(), metav1.GetOptions{})
			if err == nil {
				log.Infof("Namespace %v already exists.", m.GetName())

				nsAnns := ns.GetAnnotations()
				if nsAnns == nil {
					addAnnotation = false
				} else {
					_, found := nsAnns[kfdefAnn]
					if !found || nsAnns[kfdefAnn] != kfdefCr {
						// if the namespace is not created by this operator, should not append the annotation
						addAnnotation = false
					}
				}
			}
		} else if m.GetKind() == "CustomResourceDefinition" && m.GetName() == "profiles.kubeflow.org" {
			// profiles will contain user info and data, should not remove during uninstall
			addAnnotation = false
		}

		if addAnnotation {
			anns[kfdefAnn] = kfdefCr
			m.SetAnnotations(anns)
		}
		out, err := yaml.Marshal(m)
		if err != nil {
			return nil, err
		}
		if addAnnotation {
			log.Infof("KfDef annotation added for resource %v.%v", m.GetName(), m.GetNamespace())
		}

		if firstObj {
			firstObj = false
		} else {
			if _, err = buf.WriteString("---\n"); err != nil {
				return nil, err
			}
		}
		if _, err = buf.Write(out); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
