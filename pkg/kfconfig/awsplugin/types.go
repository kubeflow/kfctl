package awsplugin

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +k8s:openapi-gen=true
// Placeholder for the plugin API.
type KfAwsPlugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AwsPluginSpec `json:"spec,omitempty"`
}

// AwsPlugin defines the extra data provided by the GCP Plugin in KfDef
type AwsPluginSpec struct {
	Auth *Auth `json:"auth,omitempty"`

	Region string `json:"region,omitempty"`

	Roles []string `json:"roles,omitempty"`

	EnablePodIamPolicy *bool `json:"enablePodIamPolicy,omitempty"`

	EnableNodeGroupLog *bool `json:"enableNodeGroupLog,omitempty"`

	ManagedCluster *bool `json:"managedCluster,omitempty"`

	ManagedRelationDatabase *RelationDatabaseConfig `json:"managedRelationDatabase,omitempty"`

	ManagedObjectStorage *ObjectStorageConfig `json:"managedObjectStorage,omitempty"`

	// TODO: Addon is used to host some optional aws specific components
	// EFS, FSX CSI Plugin, Device Plugin, etc
	//AddOns []string `json:"addons,omitempty"`
}

type RelationDatabaseConfig struct {
	Host     string `json:"host,omitempty"`
	Port     *int   `json:"port,omitempty"`
	Database string `json:"database,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type ObjectStorageConfig struct {
	Endpoint   string `json:"endpoint,omitempty"`
	Region     string `json:"region,omitempty"`
	Bucket     string `json:"bucket,omitempty"`
	PathPrefix string `json:"pathPrefix,omitempty"`
}

type Auth struct {
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`
	Oidc      *OIDC      `json:"oidc,omitempty"`
	Cognito   *Coginito  `json:"cognito,omitempty"`
}

type BasicAuth struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type OIDC struct {
	OidcAuthorizationEndpoint string `json:"oidcAuthorizationEndpoint,omitempty"`
	OidcIssuer                string `json:"oidcIssuer,omitempty"`
	OidcTokenEndpoint         string `json:"oidcTokenEndpoint,omitempty"`
	OidcUserInfoEndpoint      string `json:"oidcUserInfoEndpoint,omitempty"`
	CertArn                   string `json:"certArn,omitempty"`
	OAuthClientId             string `json:"oAuthClientId,omitempty"`
	OAuthClientSecret         string `json:"oAuthClientSecret,omitempty"`
}

type Coginito struct {
	CognitoAppClientId    string `json:"cognitoAppClientId,omitempty"`
	CognitoUserPoolArn    string `json:"cognitoUserPoolArn,omitempty"`
	CognitoUserPoolDomain string `json:"cognitoUserPoolDomain,omitempty"`
	CertArn               string `json:"certArn,omitempty"`
}

// IsValid returns true if the spec is a valid and complete spec.
// If false it will also return a string providing a message about why its invalid.
func (plugin *AwsPluginSpec) IsValid() (bool, string) {
	if plugin.Auth.BasicAuth != nil {
		msg := ""
		isValid := true

		if plugin.Auth.BasicAuth.Username == "" {
			isValid = false
			msg += "BasicAuth requires username. "
		}

		if plugin.Auth.BasicAuth.Password == "" {
			isValid = false
			msg += "BasicAuth requires password. "
		}

		return isValid, msg
	}

	if plugin.Auth.Oidc != nil {
		msg := ""
		isValid := true

		if plugin.Auth.Oidc.OidcAuthorizationEndpoint == "" {
			isValid = false
			msg += "OidcAuthorizationEndpoint is required"
		}

		if plugin.Auth.Oidc.OidcIssuer == "" {
			isValid = false
			msg += "OidcIssuer is required"
		}

		if plugin.Auth.Oidc.OidcTokenEndpoint == "" {
			isValid = false
			msg += "OidcTokenEndpoint is required"
		}

		if plugin.Auth.Oidc.OidcUserInfoEndpoint == "" {
			isValid = false
			msg += "OidcUserInfoEndpoint is required"
		}

		if plugin.Auth.Oidc.CertArn == "" {
			isValid = false
			msg += "CertArn is required"
		}

		if plugin.Auth.Oidc.OAuthClientId == "" {
			isValid = false
			msg += "OAuthClientId is required"
		}

		if plugin.Auth.Oidc.OAuthClientSecret == "" {
			isValid = false
			msg += "OAuthClientSecret is required"
		}

		return isValid, msg
	}

	if plugin.Auth.Cognito != nil {
		msg := ""
		isValid := true

		if plugin.Auth.Cognito.CognitoAppClientId == "" {
			isValid = false
			msg += "CognitoAppClientId is required"
		}

		if plugin.Auth.Cognito.CognitoUserPoolArn == "" {
			isValid = false
			msg += "CognitoUserPoolArn is required"
		}

		if plugin.Auth.Cognito.CognitoUserPoolDomain == "" {
			isValid = false
			msg += "CognitoUserPoolDomain is required"
		}

		if plugin.Auth.Cognito.CertArn == "" {
			isValid = false
			msg += "CertArn is required"
		}

		return isValid, msg
	}

	if plugin.ManagedRelationDatabase != nil {
		msg := ""
		isValid := true

		if plugin.ManagedRelationDatabase.Host == "" {
			isValid = false
			msg += "ManagedRelationDatabase.Host is required"
		}

		if plugin.ManagedRelationDatabase.Username == "" {
			isValid = false
			msg += "ManagedRelationDatabase.Username is required"
		}

		if plugin.ManagedRelationDatabase.Password == "" {
			isValid = false
			msg += "ManagedRelationDatabase.Password is required"
		}

		return isValid, msg
	}

	if plugin.ManagedObjectStorage != nil {
		msg := ""
		isValid := true

		if plugin.ManagedObjectStorage.Endpoint == "" {
			isValid = false
			msg += "ManagedObjectStorage.Endpoint is required"
		}

		if plugin.ManagedObjectStorage.Region == "" {
			isValid = false
			msg += "ManagedObjectStorage.Region is required"
		}

		if plugin.ManagedObjectStorage.Bucket == "" {
			isValid = false
			msg += "ManagedObjectStorage.Bucket is required"
		}

		return isValid, msg
	}

	return true, ""
}

// GetEnablePodIamPolicy return true if user want to enable pod iam policy
func (p *AwsPluginSpec) GetEnablePodIamPolicy() bool {
	if p.EnablePodIamPolicy == nil {
		return false
	}

	v := p.EnablePodIamPolicy
	return *v
}

// GetEnableNodeGroupLog return true if user want to enable fluentd cloud watch logs
func (p *AwsPluginSpec) GetEnableNodeGroupLog() bool {
	if p.EnableNodeGroupLog == nil {
		return false
	}

	v := p.EnableNodeGroupLog
	return *v
}

// GetManagedCluster return true if user want to create a new cluster and then deploy kubeflow
func (p *AwsPluginSpec) GetManagedCluster() bool {
	if p.ManagedCluster == nil {
		return false
	}

	v := p.ManagedCluster
	return *v
}
