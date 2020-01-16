// Copyright 2019 The Kubeflow Authors
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

package dex

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/asaskevich/govalidator"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"

	kfapis "github.com/kubeflow/kfctl/v3/pkg/apis"
	kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig/dexplugin"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	staticPasswordAuthSecret = "kubeflow-login"
	kubeflowDomain           = "kubeflow-domain"

	// DexPluginName Plugin parameter constants
	DexPluginName                        = kfconfig.DEX_PLUGIN_KIND
	StaticPasswordAuthPasswordSecretName = "password"
)

// Dex implements KfApp Interface
type Dex struct {
	kfDef *kfconfig.KfConfig
}

// GetPlatform returns the dex kfapp. It's called by coordinator.GetKfApp
func GetPlatform(kfdef *kfconfig.KfConfig) (kftypes.Platform, error) {
	_dex := &Dex{
		kfDef: kfdef,
	}

	return _dex, nil
}

// GetPluginSpec gets the plugin spec.
func (dex *Dex) GetPluginSpec() (*dexplugin.DexPluginSpec, error) {
	dexPluginSpec := &dexplugin.DexPluginSpec{}

	err := dex.kfDef.GetPluginSpec(DexPluginName, dexPluginSpec)

	return dexPluginSpec, err
}

// GetK8sConfig is only used with ksonnet packageManager. NotImplemented in this version, return nil to use default config for API compatibility.
func (dex *Dex) GetK8sConfig() (*rest.Config, *clientcmdapi.Config) {
	return nil, nil
}

func insertConfigMap(client *clientset.Clientset, configMapName string, namespace string, data map[string]string) error {
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
		Data: data,
	}
	_, err := client.CoreV1().ConfigMaps(namespace).Create(configMap)
	if err == nil {
		return nil
	} else {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: err.Error(),
		}
	}
}

func insertSecret(client *clientset.Clientset, secretName string, namespace string, data map[string][]byte) error {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: data,
	}
	_, err := client.CoreV1().Secrets(namespace).Create(secret)
	if err == nil {
		return nil
	} else {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: err.Error(),
		}
	}
}

func (dex *Dex) getAuthNamespace() string {
	if authNamespace, ok := dex.kfDef.GetApplicationParameter("dex", "namespace"); ok {
		return authNamespace
	}
	return dex.kfDef.Namespace
}

func (dex *Dex) getIstioNamespace() string {
	if istioNamespace, ok := dex.kfDef.GetApplicationParameter("oidc-authservice", "namespace"); ok {
		return istioNamespace
	}
	return dex.kfDef.Namespace
}

// createDomainConfigMap creates a configMap containing the domain on which
// Kubeflow's dashboard will be served. TLS cert is issued on this Domain.
func (dex *Dex) createDomainConfigMap(client *clientset.Clientset) error {
	dexPluginSpec, err := dex.GetPluginSpec()
	if err != nil {
		return err
	}

	ctx := context.Background()
	k8sClientset, err := dex.getK8sClientset(ctx)
	if err != nil {
		return err
	}

	istioNamespace := dex.getIstioNamespace()
	if err = createNamespace(k8sClientset, istioNamespace); err != nil {
		return err
	}

	configMapData := map[string]string{
		"dns_name": "",
		"ip":       "",
	}
	if govalidator.IsDNSName(dexPluginSpec.Domain) {
		configMapData["dns_name"] = dexPluginSpec.Domain
	} else if govalidator.IsIP(dexPluginSpec.Domain) {
		configMapData["ip"] = dexPluginSpec.Domain
	}
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeflowDomain,
			Namespace: istioNamespace,
		},
		Data: configMapData,
	}
	_, err = client.CoreV1().ConfigMaps(istioNamespace).Update(configMap)
	if err != nil {
		log.Warnf("Updating configMap for Dex domain failed, trying to create one: %v", err)
		return insertConfigMap(client, kubeflowDomain, istioNamespace, configMapData)
	}
	return nil
}

// Use email and password provided by user and create secret for staticPassword auth.
func (dex *Dex) createStaticUserAuthSecret(client *clientset.Clientset) error {
	ctx := context.Background()
	k8sClientset, err := dex.getK8sClientset(ctx)
	if err != nil {
		return err
	}

	dexPluginSpec, err := dex.GetPluginSpec()
	if err != nil {
		return err
	}

	if dexPluginSpec.Auth == nil || dexPluginSpec.Auth.StaticPasswordAuth == nil || dexPluginSpec.Auth.StaticPasswordAuth.Password.Name == "" {
		err := errors.WithStack(fmt.Errorf("StaticPasswordAuth.Password.Name must be set"))
		return err
	}

	password, err := dex.kfDef.GetSecret(dexPluginSpec.Auth.StaticPasswordAuth.Password.Name)
	if err != nil {
		log.Errorf("There was a problem getting the password for basic auth; error %v", err)
		return err
	}

	encodedPassword, err := base64EncryptPassword(password)
	if err != nil {
		log.Errorf("There was a problem encrypting the password; %v", err)
		return err
	}

	authNamespace := dex.getAuthNamespace()
	if err = createNamespace(k8sClientset, authNamespace); err != nil {
		return err
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      staticPasswordAuthSecret,
			Namespace: authNamespace,
		},
		Data: map[string][]byte{
			"email":        []byte(dexPluginSpec.Auth.StaticPasswordAuth.Email),
			"passwordhash": []byte(encodedPassword),
		},
	}
	_, err = client.CoreV1().Secrets(authNamespace).Update(secret)
	if err != nil {
		log.Warnf("Updating static user auth login failed, trying to create one: %v", err)
		return insertSecret(client, staticPasswordAuthSecret, authNamespace, map[string][]byte{
			"email":        []byte(dexPluginSpec.Auth.StaticPasswordAuth.Email),
			"passwordhash": []byte(encodedPassword),
		})
	}
	return nil
}

func base64EncryptPassword(password string) (string, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return "", &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Error when hashing password: %v", err),
		}
	}
	encodedPassword := base64.StdEncoding.EncodeToString(passwordHash)

	return encodedPassword, nil
}

// Init initializes dex kfapp - platform
func (dex *Dex) Init(resources kftypes.ResourceEnum) error {

	return nil
}

// Generate generates dex kfapp manifest
func (dex *Dex) Generate(resources kftypes.ResourceEnum) error {

	if setDexPluginDefaultsErr := dex.setDexPluginDefaults(); setDexPluginDefaultsErr != nil {
		return &kfapis.KfError{
			Code: setDexPluginDefaultsErr.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("set dex plugin defaults Error %v",
				setDexPluginDefaultsErr.(*kfapis.KfError).Message),
		}
	}

	dexPluginSpec, err := dex.GetPluginSpec()
	if err != nil {
		return err
	}

	if dexPluginSpec.Auth.UseStaticPassword {
		if err := dex.kfDef.SetApplicationParameter(
			"dex",
			"static_email",
			dexPluginSpec.Auth.StaticPasswordAuth.Email,
		); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func (dex *Dex) setDexPluginDefaults() error {
	dexPluginSpec, err := dex.GetPluginSpec()

	if err != nil {
		return err
	}

	if dexPluginSpec.Auth.UseStaticPassword {
		log.Infof("Using static password for Dex")
		if dexPluginSpec.Auth.StaticPasswordAuth == nil {
			dexPluginSpec.Auth.StaticPasswordAuth = &dexplugin.StaticPasswordAuth{}
		}

		domain := os.Getenv(kftypes.KubeflowDomain)
		if domain == "" {
			log.Warnf("KUBEFLOW_DOMAIN isn't set. TLS will be set on Istio Ingress Gateway IP.")
		} else if !(govalidator.IsDNSName(domain) || govalidator.IsIP(domain)) {
			return &kfapis.KfError{
				Code: int(kfapis.INVALID_ARGUMENT),
				Message: fmt.Sprintf(
					"KUBEFLOW_DOMAIN: %s is an invalid domain.",
					domain,
				),
			}
		}
		dexPluginSpec.Domain = domain

		email := os.Getenv(kftypes.KubeflowEmail)
		if email == "" {
			log.Errorf("Could not configure static user auth; environment variable %s not set", kftypes.KubeflowEmail)
			return &kfapis.KfError{
				Code: int(kfapis.INVALID_ARGUMENT),
				Message: fmt.Sprintf(
					"Could not configure static user auth; environment variable %s not set",
					kftypes.KubeflowEmail,
				),
			}
		}
		dexPluginSpec.Auth.StaticPasswordAuth.Email = email

		dexPluginSpec.Auth.StaticPasswordAuth.Password = &kfconfig.SecretRef{
			Name: StaticPasswordAuthPasswordSecretName,
		}
		password := os.Getenv(kftypes.KUBEFLOW_PASSWORD)
		if password == "" {
			log.Errorf("Could not configure static user auth; environment variable %s not set", kftypes.KUBEFLOW_PASSWORD)
			return &kfapis.KfError{
				Code:    int(kfapis.INVALID_ARGUMENT),
				Message: fmt.Sprintf("Could not configure basic auth; environment variable %s not set", kftypes.KUBEFLOW_PASSWORD),
			}
		}

		dex.kfDef.SetSecret(kfconfig.Secret{
			Name: StaticPasswordAuthPasswordSecretName,
			SecretSource: &kfconfig.SecretSource{
				EnvSource: &kfconfig.EnvSource{
					Name: kftypes.KUBEFLOW_PASSWORD,
				},
			},
		})
	}

	if err := dex.kfDef.SetPluginSpec(DexPluginName, dexPluginSpec); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// Apply applys kfdef manifests for dex
func (dex *Dex) Apply(resources kftypes.ResourceEnum) error {

	// Inserts configMaps into the cluster
	configMapsErr := dex.createConfigMaps()
	if configMapsErr != nil {
		return &kfapis.KfError{
			Code: configMapsErr.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("dex apply could not create configMaps Error %v",
				configMapsErr.(*kfapis.KfError).Message),
		}
	}

	// Inserts secrets into the cluster
	secretsErr := dex.createSecrets()
	if secretsErr != nil {
		return &kfapis.KfError{
			Code: secretsErr.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("dex apply could not create secrets Error %v",
				secretsErr.(*kfapis.KfError).Message),
		}
	}

	// TODO(krishnadurai): Figure how to set secrets in config.yaml for Dex?

	return nil
}

func (dex *Dex) Delete(resources kftypes.ResourceEnum) error {

	return nil
}

func (dex *Dex) createConfigMaps() error {
	ctx := context.Background()

	k8sClient, err := dex.getK8sClientset(ctx)
	if err != nil {
		return kfapis.NewKfErrorWithMessage(err, "set K8s clientset error")
	}
	log.Infof("Creating Dex configMap for kubeflow domain ...")
	if err := dex.createDomainConfigMap(k8sClient); err != nil {
		return kfapis.NewKfErrorWithMessage(err, "cannot create dex configMap for kubflow domain")
	}
	return nil
}

func (dex *Dex) createSecrets() error {
	ctx := context.Background()

	dexPluginSpec, err := dex.GetPluginSpec()
	if err != nil {
		return err
	}

	k8sClient, err := dex.getK8sClientset(ctx)
	if err != nil {
		return kfapis.NewKfErrorWithMessage(err, "set K8s clientset error")
	}
	log.Infof("Creating Dex secrets...")
	if dexPluginSpec.Auth.UseStaticPassword {
		log.Infof("Creating Dex secrets for staticPassword auth...")
		if err := dex.createStaticUserAuthSecret(k8sClient); err != nil {
			return kfapis.NewKfErrorWithMessage(err, "cannot create dex auth login secret")
		}
	}
	return nil
}

func createNamespace(k8sClientset *clientset.Clientset, namespace string) error {

	log.Infof("Creating namespace: %v", namespace)
	_, err := k8sClientset.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err == nil {
		log.Infof("Namespace already exists...")
		return nil
	}
	log.Infof("Get namespace error: %v", err)
	_, err = k8sClientset.CoreV1().Namespaces().Create(
		&v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		},
	)
	if err == nil {
		return nil
	} else {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: err.Error(),
		}
	}
}

func (dex *Dex) getK8sClientset(ctx context.Context) (*clientset.Clientset, error) {
	config := kftypes.GetConfig()
	if cli, err := clientset.NewForConfig(config); err == nil {
		return cli, nil
	} else {
		return nil, &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("create new ClientConfig error: %v", err),
		}
	}
}
