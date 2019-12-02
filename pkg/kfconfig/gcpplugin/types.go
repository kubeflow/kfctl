package gcpplugin

import (
	"fmt"
	kfapis "github.com/kubeflow/kfctl/v3/pkg/apis"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +k8s:openapi-gen=true
// Placeholder for the plugin API.
type KfGcpPlugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GcpPluginSpec `json:"spec,omitempty"`
}

// GcpPlugin defines the extra data provided by the GCP Plugin in KfDef
type GcpPluginSpec struct {
	Auth *Auth `json:"auth,omitempty"`

	// SAClientId if supplied grant this service account cluster admin access
	// TODO(jlewi): Might want to make it a list
	SAClientId string `json:"username,omitempty"`

	// CreatePipelinePersistentStorage indicates whether to create storage.
	// Use a pointer so we can distinguish unset values.
	CreatePipelinePersistentStorage *bool `json:"createPipelinePersistentStorage,omitempty"`

	// EnableWorkloadIdentity indicates whether to enable workload identity.
	// Use a pointer so we can distinguish unset values.
	EnableWorkloadIdentity *bool `json:"enableWorkloadIdentity,omitempty"`

	// DeploymentManagerConfig provides location of the deployment manager configs.
	DeploymentManagerConfig *DeploymentManagerConfig `json:"deploymentManagerConfig,omitempty"`

	Project         string `json:"project,omitempty"`
	Email           string `json:"email,omitempty"`
	IpName          string `json:"ipName,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	Zone            string `json:"zone,omitempty"`
	UseBasicAuth    bool   `json:"useBasicAuth"`
	SkipInitProject bool   `json:"skipInitProject,omitempty"`
	DeleteStorage   bool   `json:"deleteStorage,omitempty"`
}

type Auth struct {
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`
	IAP       *IAP       `json:"iap,omitempty"`
}

type BasicAuth struct {
	Username string              `json:"username,omitempty"`
	Password *kfconfig.SecretRef `json:"password,omitempty"`
}

type IAP struct {
	OAuthClientId     string              `json:"oAuthClientId,omitempty"`
	OAuthClientSecret *kfconfig.SecretRef `json:"oAuthClientSecret,omitempty"`
}

type DeploymentManagerConfig struct {
	RepoRef *kfconfig.RepoRef `json:"repoRef,omitempty"`
}

// IsValid returns nil if the spec is a valid and complete spec.
// If not nil it will return a `KfError` with an error message.
func (s *GcpPluginSpec) IsValid() error {
	if len(s.Hostname) > 63 {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Invaid host name: host name %s is longer than 63 characters. Please shorten the metadata.name.", s.Hostname),
		}
	}
	basicAuthSet := s.Auth.BasicAuth != nil
	iapAuthSet := s.Auth.IAP != nil

	if basicAuthSet == iapAuthSet {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: "Exactly one of BasicAuth and IAP must be set; the other should be nil",
		}
	}

	if basicAuthSet {
		msgs := []string{}
		if s.Auth.BasicAuth.Username == "" {
			msgs = append(msgs, "BasicAuth requires username.")
		}

		if s.Auth.BasicAuth.Password == nil {
			msgs = append(msgs, "BasicAuth requires password.")
		}

		if len(msgs) > 0 {
			return &kfapis.KfError{
				Code:    int(kfapis.INVALID_ARGUMENT),
				Message: strings.Join(msgs, ";"),
			}
		} else {
			return nil
		}
	}

	if iapAuthSet {
		msgs := []string{}
		if s.Auth.IAP.OAuthClientId == "" {
			msgs = append(msgs, "IAP requires OAuthClientId.")
		}

		if s.Auth.IAP.OAuthClientSecret == nil {
			msgs = append(msgs, "IAP requires OAuthClientSecret.")
		}

		if len(msgs) > 0 {
			return &kfapis.KfError{
				Code:    int(kfapis.INVALID_ARGUMENT),
				Message: strings.Join(msgs, ";"),
			}
		} else {
			return nil
		}
	}

	return &kfapis.KfError{
		Code:    int(kfapis.INVALID_ARGUMENT),
		Message: "Either BasicAuth or IAP must be set",
	}
}

func (p *GcpPluginSpec) GetCreatePipelinePersistentStorage() bool {
	if p.CreatePipelinePersistentStorage == nil {
		return true
	}

	v := p.CreatePipelinePersistentStorage
	return *v
}

func (p *GcpPluginSpec) GetEnableWorkloadIdentity() bool {
	if p.EnableWorkloadIdentity == nil {
		return true
	}

	v := p.EnableWorkloadIdentity
	return *v
}
