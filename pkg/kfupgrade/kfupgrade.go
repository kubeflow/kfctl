// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package kfupgrade
package kfupgrade

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"k8s.io/client-go/kubernetes/scheme"

	kfapis "github.com/kubeflow/kfctl/v3/pkg/apis"
	kftypesv3 "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	kfupgrade "github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfupgrade/v1alpha1"
	kfdefgcpplugin "github.com/kubeflow/kfctl/v3/pkg/apis/apps/plugins/gcp/v1alpha1"
	"github.com/kubeflow/kfctl/v3/pkg/kfapp/coordinator"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
	kfconfigloaders "github.com/kubeflow/kfctl/v3/pkg/kfconfig/loaders"
	applicationsv1beta1 "github.com/kubernetes-sigs/application/pkg/apis/app/v1beta1"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KfUpgrader struct {
	OldKfCfg   *kfconfig.KfConfig
	NewKfCfg   *kfconfig.KfConfig
	TargetPath string
}

// Given a path to a base config and the existing KfCfg, create and return a new KfCfg
// while keeping the existing KfApp's customizations. Also create a new KfApp in the
// current working directory.
func createNewKfApp(baseConfig string, version string, oldKfCfg *kfconfig.KfConfig) (*kfconfig.KfConfig, string, error) {
	appDir, err := os.Getwd()
	if err != nil {
		return nil, "", &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("could not get current directory %v", err),
		}
	}

	// Load the new KfCfg from the base config
	newKfCfg, err := kfconfigloaders.LoadConfigFromURI(baseConfig)
	if err != nil {
		return nil, "", &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("Could not load %v. Error: %v", baseConfig, err),
		}
	}

	// Merge the previous KfCfg's customized values into the new KfCfg
	MergeKfCfg(oldKfCfg, newKfCfg)

	// Compute hash from the new KfCfg and use it to create the new app directory
	h, err := computeHash(newKfCfg)
	if err != nil {
		return nil, "", &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("Could not compute sha256 hash. Error: %v", err),
		}
	}

	newAppDir := filepath.Join(appDir, h)
	newKfCfg.Spec.AppDir = newAppDir
	newKfCfg.Spec.Version = version
	outputFilePath := filepath.Join(newAppDir, newKfCfg.Spec.ConfigFileName)

	// Make sure the directory is created.
	if _, err := os.Stat(newAppDir); os.IsNotExist(err) {
		log.Infof("Creating directory %v", newAppDir)
		err = os.MkdirAll(newAppDir, os.ModePerm)
		if err != nil {
			log.Errorf("Couldn't create directory %v: %v", newAppDir, err)
			return nil, "", err
		}
	} else {
		log.Infof("App directory %v already exists", newAppDir)
	}

	err = kfconfigloaders.WriteConfigToFile(*newKfCfg)
	if err != nil {
		return nil, "", err
	}

	return newKfCfg, outputFilePath, nil
}

// Given a KfUpgrade config, either find the KfApp that matches the NewKfCfg reference or
// create a new one.
func NewKfUpgrade(upgradeConfig string) (*KfUpgrader, error) {
	// Parse the KfUpgrade spec.
	upgrade, err := kfupgrade.LoadKfUpgradeFromUri(upgradeConfig)
	if err != nil {
		log.Errorf("Could not load %v; error %v", upgradeConfig, err)
		return nil, &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: err.Error(),
		}
	}

	// Try to find the old KfCfg.
	oldKfCfg, _, err := findKfCfg(upgrade.Spec.CurrentKfDef)
	if err != nil || oldKfCfg == nil {
		return nil, &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("Could not find existing KfCfg %v. Error: %v", upgrade.Spec.CurrentKfDef.Name, err),
		}
	}

	// Try to find the new KfCfg.
	newKfCfg, targetPath, err := findKfCfg(upgrade.Spec.NewKfDef)
	if err != nil {
		return nil, &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("Encountered error while trying to find %v: %v", upgrade.Spec.NewKfDef.Name, err),
		}
	}

	// If the new KfCfg is not found, create it
	if newKfCfg == nil {
		newKfCfg, targetPath, err = createNewKfApp(upgrade.Spec.BaseConfigPath, upgrade.Spec.NewKfDef.Version, oldKfCfg)
		if err != nil {
			return nil, &kfapis.KfError{
				Code:    int(kfapis.INTERNAL_ERROR),
				Message: fmt.Sprintf("Encountered error while creating new KfApp %v: %v", upgrade.Spec.NewKfDef.Name, err),
			}
		}
	}

	return &KfUpgrader{
		OldKfCfg:   oldKfCfg,
		NewKfCfg:   newKfCfg,
		TargetPath: targetPath,
	}, nil
}

func computeHash(d *kfconfig.KfConfig) (string, error) {
	h := sha256.New()
	buf := new(bytes.Buffer)
	json.NewEncoder(buf).Encode(d)
	h.Write([]byte(buf.Bytes()))
	id := base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(h.Sum(nil))
	id = strings.ToLower(id)[0:7]

	return id, nil
}

func findKfCfg(kfDefRef *kfupgrade.KfDefRef) (*kfconfig.KfConfig, string, error) {
	var target *kfconfig.KfConfig
	var targetPath string
	err := filepath.Walk(".",
		func(path string, info os.FileInfo, err error) error {
			if target != nil {
				// If we already found the target directory, skip
				return nil
			}

			if strings.Contains(path, ".cache") {
				// Skip cache directories
				return nil
			}

			if !strings.HasSuffix(path, "yaml") {
				// Skip everything that's not a yaml file
				return nil
			}

			kfCfg, err := kfconfigloaders.LoadConfigFromURI(path)
			if err != nil {
				return nil
			}

			if kfCfg.Name == kfDefRef.Name && kfCfg.Spec.Version == kfDefRef.Version {
				log.Infof("Found KfCfg with matching name: %v version: %v at %v", kfCfg.Name, kfCfg.Spec.Version, path)
				target = kfCfg
				targetPath = path
			}

			return err
		})
	if err != nil {
		log.Println(err)
		return nil, "", err
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, "", &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("could not get current directory %v", err),
		}
	}

	return target, filepath.Join(wd, targetPath), err
}

func MergeKfCfg(oldKfCfg *kfconfig.KfConfig, newKfCfg *kfconfig.KfConfig) {
	newKfCfg.Name = oldKfCfg.Name

	// Merge plugins.
	pluginKinds := []kfconfig.PluginKindType{
		kfconfig.AWS_PLUGIN_KIND,
		kfconfig.GCP_PLUGIN_KIND,
		kfconfig.MINIKUBE_PLUGIN_KIND,
		kfconfig.EXISTING_ARRIKTO_PLUGIN_KIND,
	}

	for _, kind := range pluginKinds {
		oldPlugin := kfdefgcpplugin.GcpPluginSpec{}
		err := oldKfCfg.GetPluginSpec(kind, &oldPlugin)

		// If no error, then this plugin is found and we need to copy it to the new KfCfg.
		if err == nil {
			log.Infof("Merging plugin spec: %v\n", kind)
			newKfCfg.SetPluginSpec(kind, &oldPlugin)
		}
	}

	for appIndex, newApp := range newKfCfg.Spec.Applications {
		for _, oldApp := range oldKfCfg.Spec.Applications {
			if newApp.Name == oldApp.Name {
				for paramIndex, newParam := range newApp.KustomizeConfig.Parameters {
					for _, oldParam := range oldApp.KustomizeConfig.Parameters {
						if newParam.Name == oldParam.Name && newParam.Value != oldParam.Value {
							log.Infof("Merging application %v param %v from %v to %v\n",
								newApp.Name, newParam.Name, newParam.Value, oldParam.Value)
							newKfCfg.Spec.Applications[appIndex].KustomizeConfig.Parameters[paramIndex].Value = oldParam.Value
							break
						}
					}
				}
				break
			}
		}
	}
}

func (upgrader *KfUpgrader) Generate() error {
	kfApp, err := coordinator.NewLoadKfAppFromURI(upgrader.TargetPath)
	if err != nil {
		log.Errorf("Failed to build KfApp from URI: %v", err)
		return err
	}

	return kfApp.Generate(kftypesv3.K8S)
}

func (upgrader *KfUpgrader) Apply() error {
	kfApp, err := coordinator.NewLoadKfAppFromURI(upgrader.TargetPath)
	if err != nil {
		log.Errorf("Failed to build KfApp from URI: %v", err)
		return err
	}

	err = kfApp.Generate(kftypesv3.K8S)
	if err != nil {
		log.Errorf("Failed to generate KfApp: %v", err)
		return err
	}

	err = upgrader.DeleteObsoleteResources(upgrader.OldKfCfg.ObjectMeta.Namespace)
	if err != nil {
		log.Errorf("Failed to delete obsolete resources: %v", err)
		return err
	}

	err = upgrader.DeleteObsoleteResources("istio-system")
	if err != nil {
		log.Errorf("Failed to delete obsolete resources: %v", err)
		return err
	}

	return kfApp.Apply(kftypesv3.K8S)
}

func (upgrader *KfUpgrader) Dump() error {
	kfApp, err := coordinator.NewLoadKfAppFromURI(upgrader.TargetPath)
	if err != nil {
		log.Errorf("Failed to build KfApp from URI: %v", err)
		return err
	}

	return kfApp.Dump(kftypesv3.K8S)
}

func (upgrader *KfUpgrader) DeleteObsoleteResources(ns string) error {
	applicationsv1beta1.AddToScheme(scheme.Scheme)

	log.Infof("Deleting resources in in namespace %v", ns)

	objs := []runtime.Object{
		&applicationsv1beta1.Application{},
		&appsv1.Deployment{},
		&appsv1.StatefulSet{},
		&appsv1.ReplicaSet{},
		&appsv1.DaemonSet{},
	}

	for _, obj := range objs {
		err := upgrader.DeleteResources(ns, obj)
		if err != nil {
			return err
		}
	}

	return nil
}

func (upgrader *KfUpgrader) DeleteResources(ns string, obj runtime.Object) error {
	config := kftypesv3.GetConfig()
	kubeClient, err := client.New(config, client.Options{})
	objKind := reflect.TypeOf(obj)

	log.Infof("Deleting resources type %v in in namespace %v", objKind, ns)
	err = kubeClient.DeleteAllOf(context.Background(),
		obj,
		client.InNamespace(ns),
		client.MatchingLabels{
			"app.kubernetes.io/part-of": "kubeflow",
		},
		client.PropagationPolicy(metav1.DeletePropagationBackground))

	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("couldn't delete %v Error: %v", objKind, err),
		}
	}

	return nil
}
