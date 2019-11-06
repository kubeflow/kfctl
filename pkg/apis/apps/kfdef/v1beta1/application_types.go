// Copyright 2018 The Kubeflow Authors
//
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

package v1beta1

import (
	"fmt"
	"github.com/ghodss/yaml"
	kfapis "github.com/kubeflow/kfctl/v3/pkg/apis"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	valid "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	"strings"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KfDef is the Schema for the applications API
// +k8s:openapi-gen=true
type KfDef struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KfDefSpec   `json:"spec,omitempty"`
	Status KfDefStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KfDefList contains a list of KfDef
type KfDefList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KfDef `json:"items"`
}

type KfDefSpec struct {
	Version      string        `json:"version,omitempty"`
	Applications []Application `json:"applications,omitempty"`
	Plugins      []Plugin      `json:"plugins,omitempty"`
	Secrets      []Secret      `json:"secrets,omitempty"`
	Repos        []Repo        `json:"repos,omitempty"`
}

// Application defines an application to install
type Application struct {
	Name            string           `json:"name,omitempty"`
	KustomizeConfig *KustomizeConfig `json:"kustomizeConfig,omitempty"`
}

type KustomizeConfig struct {
	RepoRef    *RepoRef    `json:"repoRef,omitempty"`
	Overlays   []string    `json:"overlays,omitempty"`
	Parameters []NameValue `json:"parameters,omitempty"`
}

type RepoRef struct {
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

type NameValue struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// Plugin can be used to customize the generation and deployment of Kubeflow
type Plugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec *runtime.RawExtension `json:"spec,omitempty"`
}

// Secret provides information about secrets needed to configure Kubeflow.
// Secrets can be provided via references.
type Secret struct {
	Name         string        `json:"name,omitempty"`
	SecretSource *SecretSource `json:"secretSource,omitempty"`
}

type SecretSource struct {
	LiteralSource *LiteralSource `json:"literalSource,omitempty"`
	EnvSource     *EnvSource     `json:"envSource,omitempty"`
}

type LiteralSource struct {
	Value string `json:"value,omitempty"`
}

type EnvSource struct {
	Name string `json:"name,omitempty"`
}

// SecretRef is a reference to a secret
type SecretRef struct {
	// Name of the secret
	Name string `json:"name,omitempty"`
}

// Repo provides information about a repository providing config (e.g. kustomize packages,
// Deployment manager configs, etc...)
type Repo struct {
	// Name is a name to identify the repository.
	Name string `json:"name,omitempty"`
	// URI where repository can be obtained.
	// Can use any URI understood by go-getter:
	// https://github.com/hashicorp/go-getter/blob/master/README.md#installation-and-usage
	URI string `json:"uri,omitempty"`
}

// KfDefStatus defines the observed state of KfDef
type KfDefStatus struct {
	Conditions []KfDefCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	// ReposCache is used to cache information about local caching of the URIs.
	ReposCache []RepoCache `json:"reposCache,omitempty"`
}

type RepoCache struct {
	Name      string `json:"name,omitempty"`
	LocalPath string `json:"localPath,string"`
}

type KfDefConditionType string

const (
	// KfAvailable means Kubeflow is serving.
	Healthy KfDefConditionType = "Healthy"

	// Unhealthy means one or more Kubeflow services are not healthy.
	Unhealthy KfDefConditionType = "Unhealthy"

	// Pending means Kubeflow services is being updated.
	Pending KfDefConditionType = "Pending"
)

type KfDefCondition struct {
	// Type of deployment condition.
	Type KfDefConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// GetPluginSpec will try to unmarshal the spec for the specified plugin to the supplied
// interface. Returns an error if the plugin isn't defined or if there is a problem
// unmarshaling it.
func (d *KfDef) GetPluginSpec(pluginKind string, s interface{}) error {
	for _, p := range d.Spec.Plugins {
		if p.Kind != pluginKind {
			continue
		}
		// To deserialize it to a specific type we need to first serialize it to bytes
		// and then unserialize it.
		specBytes, err := yaml.Marshal(p.Spec)

		if err != nil {
			log.Errorf("Could not marshal plugin %v args; error %v", pluginKind, err)
			return err
		}

		err = yaml.Unmarshal(specBytes, s)

		if err != nil {
			log.Errorf("Could not unmarshal plugin %v to the provided type; error %v", pluginKind, err)
		}
		return nil
	}
	return &kfapis.KfError{
		Code:    int(kfapis.NOT_FOUND),
		Message: fmt.Sprintf("Plugin not found: %v", pluginKind),
	}
}

// SetPluginSpec sets the requested parameter: add the plugin if it doesn't already exist, or replace existing plugin.
func (d *KfDef) SetPluginSpec(pluginKind string, spec interface{}) error {
	// Convert spec to RawExtension

	r := &runtime.RawExtension{}

	// To deserialize it to a specific type we need to first serialize it to bytes
	// and then unserialize it.
	specBytes, err := yaml.Marshal(spec)

	if err != nil {
		log.Errorf("Could not marshal spec; error %v", err)
		return err
	}

	err = yaml.Unmarshal(specBytes, r)

	if err != nil {
		log.Errorf("Could not unmarshal plugin to RawExtension; error %v", err)
	}

	index := -1

	for i, p := range d.Spec.Plugins {
		if p.Kind == pluginKind {
			index = i
			break
		}
	}

	if index == -1 {
		// Plugin in doesn't exist so add it
		log.Infof("Adding plugin %v", pluginKind)

		p := Plugin{}
		p.Name = string(pluginKind)
		p.Kind = pluginKind
		d.Spec.Plugins = append(d.Spec.Plugins, p)

		index = len(d.Spec.Plugins) - 1
	}

	d.Spec.Plugins[index].Spec = r
	return nil
}

// GetSecret returns the specified secret or an error if the secret isn't specified.
func (d *KfDef) GetSecret(name string) (string, error) {
	for _, s := range d.Spec.Secrets {
		if s.Name != name {
			continue
		}
		if s.SecretSource.LiteralSource != nil {
			return s.SecretSource.LiteralSource.Value, nil
		}
		if s.SecretSource.EnvSource != nil {
			return os.Getenv(s.SecretSource.EnvSource.Name), nil
		}

		return "", fmt.Errorf("No secret source provided for secret %v", name)
	}
	return "", &kfapis.KfError{
		Code:    int(kfapis.NOT_FOUND),
		Message: fmt.Sprintf("Secret not found: %v", name),
	}
}

// SetSecret sets the specified secret; if a secret with the given name already exists it is overwritten.
func (d *KfDef) SetSecret(newSecret Secret) {
	for i, s := range d.Spec.Secrets {
		if s.Name == newSecret.Name {
			d.Spec.Secrets[i] = newSecret
			return
		}
	}
	d.Spec.Secrets = append(d.Spec.Secrets, newSecret)
}

func (d *KfDef) DeleteApplication(appName string) {
	// First we check applications for an application with the specified name.
	if d.Spec.Applications != nil {
		applications := []Application{}
		for _, a := range d.Spec.Applications {
			if a.Name != appName {
				applications = append(applications, a)
			}
		}
		d.Spec.Applications = applications
	}
}

// IsValid returns true if the spec is a valid and complete spec.
// If false it will also return a string providing a message about why its invalid.
func (d *KfDef) IsValid() (bool, string) {
	// Validate KfConfig
	errs := valid.NameIsDNSLabel(d.Name, false)
	if errs != nil && len(errs) > 0 {
		return false, fmt.Sprintf("invalid name due to %v", strings.Join(errs, ","))
	}

	return true, ""
}
