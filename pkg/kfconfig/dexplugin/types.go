package dexplugin

import (
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +k8s:openapi-gen=true
// Placeholder for the plugin API.
type KfDexPlugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DexPluginSpec `json:"spec,omitempty"`
}

// AwsPlugin defines the extra data provided by the GCP Plugin in KfDef
type DexPluginSpec struct {
	Auth *Auth `json:"auth,omitempty"`

	Domain string `json:"domain,omitempty"`
}

type Auth struct {
	StaticPasswordAuth *StaticPasswordAuth `json:"staticPasswordAuth,omitempty"`

	UseStaticPassword bool `json:"useStaticPassword,omitempty"`
}

type StaticPasswordAuth struct {
	Username string              `json:"username,omitempty"`
	Password *kfconfig.SecretRef `json:"password,omitempty"`
}

// IsValid returns true if the spec is a valid and complete spec.
// If false it will also return a string providing a message about why its invalid.
func (plugin *DexPluginSpec) IsValid() (bool, string) {
	staticPasswordSet := plugin.Auth.UseStaticPassword

	if staticPasswordSet {
		msg := ""

		isValid := true

		if plugin.Auth.StaticPasswordAuth.Username == "" {
			isValid = false
			msg += "StaticPasswordAuth requires username."
		}

		if plugin.Auth.StaticPasswordAuth.Password == nil {
			isValid = false
			msg += "StaticPasswordAuth requires password."
		}

		return isValid, msg
	}

	return true, ""

}
