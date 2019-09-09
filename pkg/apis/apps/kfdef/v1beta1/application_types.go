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
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	KfConfigFile = "app.yaml"
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

type KfDefSpec struct {
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

// ParamType indicates the type of an input parameter;
// Used to distinguish between a single string and an array of strings.
type ParamType string

// Valid ParamTypes:
const (
	ParamTypeString ParamType = "string"
	ParamTypeArray  ParamType = "array"
)

// ArrayOrString is a type that can hold a single string or string array.
// Used in JSON unmarshalling so that a single JSON field can accept
// either an individual string or an array of strings.
type ArrayOrString struct {
	Type      ParamType // Represents the stored type of ArrayOrString.
	StringVal string
	ArrayVal  []string
}

// UnmarshalJSON implements the json.Unmarshaller interface.
func (arrayOrString *ArrayOrString) UnmarshalJSON(value []byte) error {
	if value[0] == '"' {
		arrayOrString.Type = ParamTypeString
		return json.Unmarshal(value, &arrayOrString.StringVal)
	}
	arrayOrString.Type = ParamTypeArray
	return json.Unmarshal(value, &arrayOrString.ArrayVal)
}

// MarshalJSON implements the json.Marshaller interface.
func (arrayOrString ArrayOrString) MarshalJSON() ([]byte, error) {
	switch arrayOrString.Type {
	case ParamTypeString:
		return json.Marshal(arrayOrString.StringVal)
	case ParamTypeArray:
		return json.Marshal(arrayOrString.ArrayVal)
	default:
		return []byte{}, fmt.Errorf("impossible ArrayOrString.Type: %q", arrayOrString.Type)
	}
}

type NameValue struct {
	Name  string        `json:"name,omitempty"`
	Value ArrayOrString `json:"value,omitempty"`
}

// Plugin can be used to customize the generation and deployment of Kubeflow
type Plugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec *runtime.Object `json:"spec,omitempty"`
}

// Secret provides information about secrets needed to configure Kubeflow.
// Secrets can be provided via references e.g. a URI so that they won't
// be serialized as part of the KfDefSpec which is intended to be written into source control.
type Secret struct {
	Name         string        `json:"name,omitempty"`
	SecretSource *SecretSource `json:"secretSource,omitempty"`
}

type SecretSource struct {
	LiteralSource *LiteralSource `json:"literalSource,omitempty"`
	HashedSource  *HashedSource  `json:"hashedSource,omitempty"`
	EnvSource     *EnvSource     `json:"envSource,omitempty"`
}

type LiteralSource struct {
	Value string `json:"value,omitempty"`
}

type HashedSource struct {
	HashedValue string `json:"value,omitempty"`
}

type EnvSource struct {
	Name string `json:"Name,omitempty"`
}

// Repo provides information about a repository providing config (e.g. kustomize packages,
// Deployment manager configs, etc...)
type Repo struct {
	// Name is a name to identify the repository.
	Name string `json:"name,omitempty"`
	// URI where repository can be obtained.
	// Can use any URI understood by go-getter:
	// https://github.com/hashicorp/go-getter/blob/master/README.md#installation-and-usage
	Uri string `json:"uri,omitempty"`
}

// KfDefStatus defines the observed state of KfDef
type KfDefStatus struct {
	Conditions []KfDefCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,6,rep,name=conditions"`
	// ReposCache is used to cache information about local caching of the URIs.
	ReposCache map[string]RepoCache `json:"reposCache,omitempty"`
}

type RepoCache struct {
	LocalPath string `json:"localPath,string"`
}

type KfDefConditionType string

const (
	// KfAvailable means Kubeflow is serving.
	KfAvailable KfDefConditionType = "Available"

	// KfDegraded means functionality of Kubeflow is limited.
	KfDegraded KfDefConditionType = "Degraded"

	// KfPluginsProgressing means kfctl is applying plugins.
	KfPluginsProgressing KfDefConditionType = "PluginsProgressing"

	// KfKustomizeProgressing means kfctl is running package manager.
	KfKustomizeProgressing KfDefConditionType = "KustomizeProgressing"
)

type KfDefConditionReason string

const (
	// InvalidKfDefSpecReason indicates the KfDef was not valid.
	InvalidKfDefSpecReason = "InvalidKfDefSpec"

	// InvalidPluginsReason indicates plugin(s) were not valid.
	InvalidPluginsReason = "InvalidPlugins"

	// InvalidSecretsReason indicates secret(s) were not valid.
	InvalidSecretsReason = "InvalidSecrets"

	// ApplyPluginsFailedReason indicates plugin(s) were not applied successfully.
	ApplyPluginsFailedReason = "ApplyPluginsFailed"
)

type KfDefCondition struct {
	// Type of deployment condition.
	Type KfDefConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=KfDefConditionType"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=k8s.io/api/core/v1.ConditionStatus"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty" protobuf:"bytes,6,opt,name=lastUpdateTime"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,7,opt,name=lastTransitionTime"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty" protobuf:"bytes,4,opt,name=reason,casttype=KfDefConditionReason"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty" protobuf:"bytes,5,opt,name=message"`
}
