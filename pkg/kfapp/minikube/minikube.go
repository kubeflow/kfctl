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

package minikube

import (
	"fmt"

	"github.com/ghodss/yaml"

	//"github.com/kubeflow/kfctl/v3/config"
	kfapis "github.com/kubeflow/kfctl/v3/pkg/apis"
	kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"

	//kfdefs "github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfdef/v1alpha1"
	"io/ioutil"

	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	//"os/user"
	"path/filepath"
	//"strconv"
	//"strings"
)

// Minikube implements KfApp Interface
type Minikube struct {
	kfconfig.KfConfig
}

func Getplatform(kfdef *kfconfig.KfConfig) kftypes.Platform {
	_minikube := &Minikube{
		KfConfig: *kfdef,
	}
	return _minikube
}

// GetK8sConfig return nil; minikube will use default kube config file
func (minikube *Minikube) GetK8sConfig() (*rest.Config, *clientcmdapi.Config) {
	return nil, nil
}

func (minikube *Minikube) Apply(resources kftypes.ResourceEnum) error {
	//mount_local_fs
	//setup_tunnels
	return nil
}

func (minikube *Minikube) Delete(resources kftypes.ResourceEnum) error {
	return nil
}

func (minikube *Minikube) Dump(resources kftypes.ResourceEnum) error {
	return nil
}

func (minikube *Minikube) generate() error {
	// TODO: fix with applications

	// remove Katib package and component
	// minikube.Spec.Packages = kftypes.RemoveItem(minikube.Spec.Packages, "katib")
	// minikube.Spec.Components = kftypes.RemoveItem(minikube.Spec.Components, "katib")
	// minikube.Spec.ComponentParams["application"] = []config.NameValue{
	// 	{
	// 		Name:  "components",
	// 		Value: "[" + strings.Join(kftypes.QuoteItems(minikube.Spec.Components), ",") + "]",
	// 	},
	// }
	// usr, err := user.Current()
	// if err != nil {
	// 	return &kfapis.KfError{
	// 		Code:    int(kfapis.INVALID_ARGUMENT),
	// 		Message: fmt.Sprintf("Could not get current user; error %v", err),
	// 	}
	// }
	// uid := usr.Uid
	// gid := usr.Gid
	// minikube.Spec.ComponentParams["jupyter"] = []config.NameValue{
	// 	{
	// 		Name:  string(kftypes.PLATFORM),
	// 		Value: minikube.Spec.Platform,
	// 	},
	// 	{
	// 		Name:  "accessLocalFs",
	// 		Value: strconv.FormatBool(minikube.Spec.MountLocal),
	// 	},
	// 	{
	// 		Name:  "disks",
	// 		Value: "local-notebooks",
	// 	},
	// 	{
	// 		Name:  "notebookUid",
	// 		Value: uid,
	// 	},
	// 	{
	// 		Name:  "notebookGid",
	// 		Value: gid,
	// 	},
	// }
	// minikube.Spec.ComponentParams["ambassador"] = []config.NameValue{
	// 	{
	// 		Name:  string(kftypes.PLATFORM),
	// 		Value: minikube.Spec.Platform,
	// 	},
	// 	{
	// 		Name:  "replicas",
	// 		Value: "1",
	// 	},
	// }
	return nil
}

func (minikube *Minikube) Generate(resources kftypes.ResourceEnum) error {
	switch resources {
	case kftypes.K8S:
	case kftypes.ALL:
		fallthrough
	case kftypes.PLATFORM:
		generateErr := minikube.generate()
		if generateErr != nil {
			return &kfapis.KfError{
				Code:    int(kfapis.INTERNAL_ERROR),
				Message: fmt.Sprintf("minikube generate failed Error: %v", generateErr),
			}
		}
	}
	createConfigErr := minikube.writeConfigFile()
	if createConfigErr != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("cannot create config file app.yaml in %v", minikube.KfConfig.Spec.AppDir),
		}
	}
	return nil
}

func (minikube *Minikube) Init(kftypes.ResourceEnum) error {
	return nil
}

func (minikube *Minikube) writeConfigFile() error {
	buf, bufErr := yaml.Marshal(minikube.KfConfig)
	if bufErr != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("cannot marshal config file: %v", bufErr),
		}
	}
	cfgFilePath := filepath.Join(minikube.KfConfig.Spec.AppDir, kftypes.KfConfigFile)
	cfgFilePathErr := ioutil.WriteFile(cfgFilePath, buf, 0644)
	if cfgFilePathErr != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("cannot write config file: %v", cfgFilePathErr),
		}
	}
	return nil
}
