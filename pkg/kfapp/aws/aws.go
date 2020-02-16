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

package aws

import (
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/gogo/protobuf/proto"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/ghodss/yaml"
	kfapis "github.com/kubeflow/kfctl/v3/pkg/apis"
	kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig/awsplugin"
	"github.com/kubeflow/kfctl/v3/pkg/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	KUBEFLOW_AWS_INFRA_DIR      = "aws_config"
	KUBEFLOW_MANIFEST_DIR       = "kustomize"
	CLUSTER_CONFIG_FILE         = "cluster_config.yaml"
	CLUSTER_FEATURE_CONFIG_FILE = "cluster_features.yaml"
	PATH                        = "path"
	BASIC_AUTH_SECRET           = "kubeflow-login"

	// The namespace for Istio
	IstioNamespace = "istio-system"

	// Plugin parameter constants
	AwsPluginName = kfconfig.AWS_PLUGIN_KIND

	// Path in manifests repo to where the additional configs are located
	CONFIG_LOCAL_PATH = "aws/infra_configs"

	DEFAULT_AUDIENCE = "sts.amazonaws.com"
)

// Aws implements KfApp Interface
// It includes the KsApp along with additional Aws types
type Aws struct {
	kfDef     *kfconfig.KfConfig
	iamClient *iam.IAM
	eksClient *eks.EKS
	sess      *session.Session

	cluster *Cluster

	region string
	roles  []string

	istioManifests   []manifest
	ingressManifests []manifest
}

type manifest struct {
	name string
	path string
}

// GetKfApp returns the aws kfapp. It's called by coordinator.GetKfApp
func GetPlatform(kfdef *kfconfig.KfConfig) (kftypes.Platform, error) {
	istioManifests := []manifest{
		{
			name: "Istio CRDs",
			path: path.Join(KUBEFLOW_MANIFEST_DIR, "istio-crds", "base", "crds.yaml"),
		},
		{
			name: "Istio Control Plane",
			path: path.Join(KUBEFLOW_MANIFEST_DIR, "istio-install", "base", "istio-noauth.yaml"),
		},
	}

	ingressManifests := []manifest{
		{
			name: "ALB Ingress",
			path: path.Join(KUBEFLOW_MANIFEST_DIR, "istio-ingress", "base", "ingress.yaml"),
		},
	}

	session := session.Must(session.NewSession())

	_aws := &Aws{
		kfDef:            kfdef,
		sess:             session,
		iamClient:        iam.New(session),
		eksClient:        eks.New(session),
		istioManifests:   istioManifests,
		ingressManifests: ingressManifests,
	}

	return _aws, nil
}

// GetPluginSpec gets the plugin spec.
func (aws *Aws) GetPluginSpec() (*awsplugin.AwsPluginSpec, error) {
	awsPluginSpec := &awsplugin.AwsPluginSpec{}

	err := aws.kfDef.GetPluginSpec(AwsPluginName, awsPluginSpec)

	return awsPluginSpec, err
}

// GetK8sConfig is only used with ksonnet packageManager. NotImplemented in this version, return nil to use default config for API compatibility.
func (aws *Aws) GetK8sConfig() (*rest.Config, *clientcmdapi.Config) {
	return nil, nil
}

func createNamespace(k8sClientset *clientset.Clientset, namespace string) error {
	_, err := k8sClientset.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err == nil {
		log.Infof("Namespace %v already exists...", namespace)
		return nil
	}
	log.Infof("Creating namespace: %v", namespace)
	_, err = k8sClientset.CoreV1().Namespaces().Create(
		&v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		},
	)

	return err
}

func deleteNamespace(k8sClientset *clientset.Clientset, namespace string) error {
	_, err := k8sClientset.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil {
		log.Infof("Namespace %v does not exist, skip deleting", namespace)
		return nil
	}
	log.Infof("Deleting namespace: %v", namespace)
	background := metav1.DeletePropagationBackground
	err = k8sClientset.CoreV1().Namespaces().Delete(
		namespace, &metav1.DeleteOptions{
			PropagationPolicy: &background,
		},
	)

	return err
}

// Create a new EKS cluster if needed
func (aws *Aws) createEKSCluster() error {
	config, err := aws.getFeatureConfig()
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Reading config file error: %v", err),
		}
	}

	if _, ok := config["managed_cluster"]; !ok {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Unable to read YAML"),
		}
	}

	if config["managed_cluster"] == true {
		log.Infoln("Start to create eks cluster. Please wait for 10-15 mins...")
		clusterConfigFile := filepath.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR, CLUSTER_CONFIG_FILE)
		output, err := exec.Command("eksctl", "create", "cluster", "--config-file="+clusterConfigFile).Output()
		if err != nil {
			return &kfapis.KfError{
				Code:    int(kfapis.INVALID_ARGUMENT),
				Message: fmt.Sprintf("Call 'eksctl create cluster --config-file=%s' with errors: %v", clusterConfigFile, string(output)),
			}
		}
		log.Infoln(string(output))

		nodeGroupIamRoles, getRoleError := aws.getWorkerNodeGroupRoles(aws.kfDef.Name)
		if getRoleError != nil {
			return errors.WithStack(getRoleError)
		}

		aws.roles = nodeGroupIamRoles
	} else {
		log.Infof("You already have cluster setup. Skip creating new eks cluster. ")
	}

	return nil
}

func (aws *Aws) attachPoliciesToRoles(roles []string) error {
	config, err := aws.getFeatureConfig()
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Reading config file error: %v", err),
		}
	}

	for _, iamRole := range roles {
		aws.attachIamInlinePolicy(iamRole, "iam_alb_ingress_policy",
			filepath.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR, "iam_alb_ingress_policy.json"))
		aws.attachIamInlinePolicy(iamRole, "iam_csi_fsx_policy",
			filepath.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR, "iam_csi_fsx_policy.json"))

		aws.attachIamInlinePolicy(iamRole, "iam_profile_controller_policy",
			filepath.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR, "iam_profile_controller_policy.json"))

		if config["worker_node_group_logging"] == "true" {
			aws.attachIamInlinePolicy(iamRole, "iam_cloudwatch_policy",
				filepath.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR, "iam_cloudwatch_policy.json"))
		}
	}

	return nil
}

// TODO: Deprecate cluster_config.yaml and put all settings in clusterSpec
// TODO: Once CloudFormation add support for master log/ private access, we can configure in cluster_config.yaml.
// https://github.com/weaveworks/eksctl/issues/778
// https://github.com/weaveworks/eksctl/pull/847/files
func (aws *Aws) updateEKSClusterConfig() error {
	return nil
}

func (aws *Aws) getWorkerNodeGroupRoles(clusterName string) ([]string, error) {
	// List all the roles and figure out nodeGroupWorkerRole
	input := &iam.ListRolesInput{}
	listRolesOutput, err := aws.iamClient.ListRoles(input)

	if err != nil {
		return nil, &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Call not list roles with errors: %v", err),
		}
	}

	var nodeGroupIamRoles []string
	for _, output := range listRolesOutput.Roles {
		if strings.HasPrefix(*output.RoleName, "eksctl-"+clusterName+"-") && strings.Contains(*output.RoleName, "NodeInstanceRole") {
			nodeGroupIamRoles = append(nodeGroupIamRoles, *output.RoleName)
		}
	}

	return nodeGroupIamRoles, nil
}

func copyFile(source string, dest string) error {
	from, err := os.Open(source)
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("cannot open input file for copying: %v", err),
		}
	}
	defer from.Close()
	to, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("cannot create dest file %v  Error %v", dest, err),
		}
	}
	defer to.Close()
	_, err = io.Copy(to, from)
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("copy failed source %v dest %v Error %v", source, dest, err),
		}
	}

	return nil
}

// updateClusterConfig replaces placeholders in cluster_config.yaml
func (aws *Aws) updateClusterConfig(clusterConfigFile string) error {
	buf, err := ioutil.ReadFile(clusterConfigFile)
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Error when reading template %v: %v", clusterConfigFile, err),
		}
	}

	var data map[string]interface{}
	if err = yaml.Unmarshal(buf, &data); err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Error when unmarshaling template %v: %v", clusterConfigFile, err),
		}
	}

	res, ok := data["metadata"]
	if !ok {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: "Invalid cluster config - not able to find metadata entry.",
		}
	}

	// Replace placeholder with clusterName and Region
	metadata := res.(map[string]interface{})
	metadata["name"] = aws.kfDef.Name
	metadata["region"] = aws.region
	data["metadata"] = metadata

	if buf, err = yaml.Marshal(data); err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("Error when marshaling for %v: %v", clusterConfigFile, err),
		}
	}
	if err = ioutil.WriteFile(clusterConfigFile, buf, 0644); err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("Error when writing to %v: %v", clusterConfigFile, err),
		}
	}

	return nil
}

// ${KUBEFLOW_SRC}/${KFAPP}/aws_config -> destDir (dest)
func (aws *Aws) generateInfraConfigs() error {
	// 1. Copy and Paste all files from `sourceDir` to `destDir`
	repo, ok := aws.kfDef.GetRepoCache(kftypes.ManifestsRepoName)
	if !ok {
		err := fmt.Errorf("Repo %v not found in KfDef.Status.ReposCache", kftypes.ManifestsRepoName)
		log.Errorf("%v", err)
		return errors.WithStack(err)
	}

	sourceDir := path.Join(repo.LocalPath, CONFIG_LOCAL_PATH)
	destDir := path.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR)

	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		log.Infof("Creating AWS infrastructure configs in directory %v", destDir)
		destDirErr := os.MkdirAll(destDir, os.ModePerm)
		if destDirErr != nil {
			return &kfapis.KfError{
				Code:    int(kfapis.INVALID_ARGUMENT),
				Message: fmt.Sprintf("Cannot create directory %v", destDirErr),
			}
		}
	} else {
		log.Infof("AWS infrastructure configs already exist in directory %v", destDir)
	}

	files := []string{"cluster_config.yaml", "cluster_features.yaml", "iam_alb_ingress_policy.json",
		"iam_cloudwatch_policy.json", "iam_csi_fsx_policy.json"}

	for _, file := range files {
		sourceFile := filepath.Join(sourceDir, file)
		destFile := filepath.Join(destDir, file)
		copyErr := copyFile(sourceFile, destFile)
		if copyErr != nil {
			return &kfapis.KfError{
				Code: copyErr.(*kfapis.KfError).Code,
				Message: fmt.Sprintf("Could not copy %v to %v Error %v",
					sourceFile, destFile, copyErr.(*kfapis.KfError).Message),
			}
		}
	}

	// 2. Reading from cluster_config.yaml and replace placeholders with values in aws.kfDef.Spec.
	clusterConfigFile := filepath.Join(destDir, CLUSTER_CONFIG_FILE)
	if err := aws.updateClusterConfig(clusterConfigFile); err != nil {
		return err
	}

	// 3. Update managed_cluster based on roles
	// By default, let's have managed_cluster true. If user pass roles, we make it false.
	featureCfg, err := aws.getFeatureConfig()
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Can not get AWS feature config file %v", err),
		}
	}

	if aws.roles != nil && len(aws.roles) != 0 {
		featureCfg["managed_cluster"] = false
	} else {
		featureCfg["managed_cluster"] = true
	}

	writeFeatureCfgErr := aws.writeFeatureConfig(featureCfg)
	if writeFeatureCfgErr != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Can not update AWS feature config file %v", err),
		}
	}

	return nil
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

// Use username and password provided by user and create secret for basic auth.
func (aws *Aws) createBasicAuthSecret(client *clientset.Clientset) error {
	awsPluginSpec, err := aws.GetPluginSpec()
	if err != nil {
		return err
	}

	if awsPluginSpec.Auth == nil || awsPluginSpec.Auth.BasicAuth == nil || awsPluginSpec.Auth.BasicAuth.Password.Name == "" {
		err := errors.WithStack(fmt.Errorf("BasicAuth.Password.Name must be set"))
		return err
	}

	password, err := aws.kfDef.GetSecret(awsPluginSpec.Auth.BasicAuth.Password.Name)
	if err != nil {
		log.Errorf("There was a problem getting the password for basic auth; error %v", err)
		return err
	}

	encodedPassword, err := base64EncryptPassword(password)
	if err != nil {
		log.Errorf("There was a problem encrypting the password; %v", err)
		return err
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      BASIC_AUTH_SECRET,
			Namespace: aws.kfDef.Namespace,
		},
		Data: map[string][]byte{
			"username":     []byte(awsPluginSpec.Auth.BasicAuth.Username),
			"passwordhash": []byte(encodedPassword),
		},
	}
	_, err = client.CoreV1().Secrets(aws.kfDef.Namespace).Update(secret)
	if err != nil {
		log.Warnf("Updating basic auth login failed, trying to create one: %v", err)
		return insertSecret(client, BASIC_AUTH_SECRET, aws.kfDef.Namespace, map[string][]byte{
			"username":     []byte(awsPluginSpec.Auth.BasicAuth.Username),
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

// Init initializes aws kfapp - platform
func (aws *Aws) Init(resources kftypes.ResourceEnum) error {
	// 1. Use AWS SDK to check if credentials from (~/.aws/credentials or ENV) and session verify
	commandsTocheck := []string{"aws", "aws-iam-authenticator", "eksctl"}
	for _, command := range commandsTocheck {
		if err := utils.CheckCommandExist(command); err != nil {
			return &kfapis.KfError{
				Code:    int(kfapis.INVALID_ARGUMENT),
				Message: fmt.Sprintf("Could not find command %v in PATH", command),
			}
		}
	}

	// 2. Check if current eksctl version meets minimum requirement
	// [ℹ]  version.Info{BuiltAt:"", GitCommit:"", GitTag:"0.1.32"}
	if err := utils.GetEksctlVersion(); err != nil {
		if err != nil {
			return &kfapis.KfError{
				Code:    int(kfapis.INVALID_ARGUMENT),
				Message: fmt.Sprintf("Can not run eksctl version is %v", err),
			}
		}
	}

	// Should not need to write config here?
	// createConfigErr := aws.kfDef.WriteToConfigFile()
	// if createConfigErr != nil {
	// 	return &kfapis.KfError{
	// 		Code:    int(kfapis.INVALID_ARGUMENT),
	// 		Message: fmt.Sprintf("Cannot create config file app.yaml in %v", aws.kfDef.Spec.AppDir),
	// 	}
	// }

	return nil
}

// Generate generate aws infrastructure configs and aws kfapp manifest
// Remind: Need to be thread-safe: this entry is share among kfctl and deploy app
func (aws *Aws) Generate(resources kftypes.ResourceEnum) error {
	awsDir := path.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR)
	if _, err := os.Stat(awsDir); err == nil {
		log.Infof("folder %v exists, skip aws.Generate", awsDir)
		return nil
	} else if !os.IsNotExist(err) {
		log.Errorf("Stat folder %v error: %v; try deleting it...", awsDir, err)
		_ = os.RemoveAll(awsDir)
	}

	// use aws to call sts get-caller-identity to verify aws credential works.
	if err := utils.CheckAwsStsCallerIdentity(aws.sess); err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Could not authenticate aws client: %v, Please make sure you set up AWS credentials and regions", err),
		}
	}

	if setAwsPluginDefaultsErr := aws.setAwsPluginDefaults(); setAwsPluginDefaultsErr != nil {
		return &kfapis.KfError{
			Code: setAwsPluginDefaultsErr.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("aws set aws plugin defaults Error %v",
				setAwsPluginDefaultsErr.(*kfapis.KfError).Message),
		}
	}

	if awsConfigFilesErr := aws.generateInfraConfigs(); awsConfigFilesErr != nil {
		return &kfapis.KfError{
			Code: awsConfigFilesErr.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("Could not generate cluster configs under %v Error: %v",
				KUBEFLOW_AWS_INFRA_DIR, awsConfigFilesErr.(*kfapis.KfError).Message),
		}
	}

	awsFeatureConfig, getAwsFeatureConfigErr := aws.getFeatureConfig()
	if getAwsFeatureConfigErr != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Reading config file error: %v", getAwsFeatureConfigErr),
		}
	}

	pluginSpec, err := aws.GetPluginSpec()
	if err != nil {
		return errors.WithStack(err)
	}

	if err := aws.kfDef.SetApplicationParameter("aws-alb-ingress-controller", "clusterName", aws.kfDef.Name); err != nil {
		return errors.WithStack(err)
	}

	if err := aws.kfDef.SetApplicationParameter("istio-ingress", "namespace", IstioNamespace); err != nil {
		return errors.WithStack(err)
	}

	if aws.kfDef.Spec.UseBasicAuth {
		if err := aws.kfDef.SetApplicationParameter("istio", "clusterRbacConfig", "OFF"); err != nil {
			return errors.WithStack(err)
		}

		if pluginSpec.Auth.BasicAuth == nil {
			return errors.WithStack(fmt.Errorf("AwsPluginSpec has no BasicAuth but UseBasicAuth set to true"))
		}

		// TODO: enable Basic Auth later
	} else {
		// TODO: Need to change profile header
		//if err := aws.kfDef.SetApplicationParameter("istio", "clusterRbacConfig", "ON"); err != nil {
		//	return errors.WithStack(err)
		//}
		//
		//if pluginSpec.Auth.Cognito == nil && pluginSpec.Auth.Oidc == nil {
		//	return errors.WithStack(fmt.Errorf("AwsPluginSpec has no OIDC or Cognito but UseBasicAuth set to false"))
		//}

		if pluginSpec.Auth.Cognito != nil {
			if err := aws.kfDef.SetApplicationParameter("istio-ingress", "CognitoUserPoolArn", pluginSpec.Auth.Cognito.CognitoUserPoolArn); err != nil {
				return errors.WithStack(err)
			}

			if err := aws.kfDef.SetApplicationParameter("istio-ingress", "CognitoUserPoolDomain", pluginSpec.Auth.Cognito.CognitoUserPoolDomain); err != nil {
				return errors.WithStack(err)
			}

			if err := aws.kfDef.SetApplicationParameter("istio-ingress", "CognitoAppClientId", pluginSpec.Auth.Cognito.CognitoAppClientId); err != nil {
				return errors.WithStack(err)
			}

			if err := aws.kfDef.SetApplicationParameter("istio-ingress", "certArn", pluginSpec.Auth.Cognito.CertArn); err != nil {
				return errors.WithStack(err)
			}
		}

		if pluginSpec.Auth.Oidc != nil {
			if err := aws.kfDef.SetApplicationParameter("istio-ingress", "oidcIssuer", pluginSpec.Auth.Oidc.OidcIssuer); err != nil {
				return errors.WithStack(err)
			}

			if err := aws.kfDef.SetApplicationParameter("istio-ingress", "oidcAuthorizationEndpoint", pluginSpec.Auth.Oidc.OidcAuthorizationEndpoint); err != nil {
				return errors.WithStack(err)
			}

			if err := aws.kfDef.SetApplicationParameter("istio-ingress", "oidcTokenEndpoint", pluginSpec.Auth.Oidc.OidcTokenEndpoint); err != nil {
				return errors.WithStack(err)
			}

			if err := aws.kfDef.SetApplicationParameter("istio-ingress", "oidcUserInfoEndpoint", pluginSpec.Auth.Oidc.OidcUserInfoEndpoint); err != nil {
				return errors.WithStack(err)
			}

			if err := aws.kfDef.SetApplicationParameter("istio-ingress", "certArn", pluginSpec.Auth.Oidc.CertArn); err != nil {
				return errors.WithStack(err)
			}

			if err := aws.kfDef.SetApplicationParameter("istio-ingress", "clientId", pluginSpec.Auth.Oidc.OAuthClientId); err != nil {
				return errors.WithStack(err)
			}

			if err := aws.kfDef.SetApplicationParameter("istio-ingress", "clientSecret", pluginSpec.Auth.Oidc.OAuthClientSecret); err != nil {
				return errors.WithStack(err)
			}
		}
	}

	// Special handling for cloud watch logs of worker node groups
	if awsFeatureConfig["worker_node_group_logging"] == true {
		//aws.kfDef.Spec.Components = append(aws.kfDef.Spec.Components, "fluentd-cloud-watch")
		if err := aws.kfDef.SetApplicationParameter("fluentd-cloud-watch", "clusterName", aws.kfDef.Name); err != nil {
			return errors.WithStack(err)
		}
		if err := aws.kfDef.SetApplicationParameter("fluentd-cloud-watch", "region", aws.region); err != nil {
			return errors.WithStack(err)
		}
	}

	// Special handling for sparkakus
	rand.Seed(time.Now().UnixNano())
	if err := aws.kfDef.SetApplicationParameter("spartakus", "usageId", strconv.Itoa(rand.Int())); err != nil {
		if kfconfig.IsAppNotFound(err) {
			log.Infof("Spartakus not included; not setting usageId")
		}
	}

	// Should not need to write config here.
	// if createConfigErr := aws.kfDef.WriteToConfigFile(); createConfigErr != nil {
	// 	return &kfapis.KfError{
	// 		Code: createConfigErr.(*kfapis.KfError).Code,
	// 		Message: fmt.Sprintf("Cannot create config file app.yaml in %v: %v", aws.kfDef.Spec.AppDir,
	// 			createConfigErr.(*kfapis.KfError).Message),
	// 	}
	// }
	return nil
}

func (aws *Aws) setAwsPluginDefaults() error {
	awsPluginSpec, err := aws.GetPluginSpec()

	if err != nil {
		return err
	}

	// TODO: enable validation once we support basic auth
	//if isValid, msg := awsPluginSpec.IsValid(); !isValid {
	//	log.Errorf("AwsPluginSpec isn't valid; error %v", msg)
	//	return fmt.Errorf(msg)
	//}

	aws.region = awsPluginSpec.Region
	aws.roles = awsPluginSpec.Roles

	if awsPluginSpec.EnablePodIamPolicy == nil {
		awsPluginSpec.EnablePodIamPolicy = proto.Bool(awsPluginSpec.GetEnablePodIamPolicy())
		log.Infof("EnablePodIamPolicy not set defaulting to %v", *awsPluginSpec.EnablePodIamPolicy)
	}

	if awsPluginSpec.Auth == nil {
		awsPluginSpec.Auth = &awsplugin.Auth{}
	}

	return nil
}

// Apply create eks cluster if needed, bind IAM policy to node group roles and enable cluster level configs.
// Remind: Need to be thread-safe: this entry is share among kfctl and deploy app
func (aws *Aws) Apply(resources kftypes.ResourceEnum) error {
	// use aws to call sts get-caller-identity to verify aws credential works.
	if err := utils.CheckAwsStsCallerIdentity(aws.sess); err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Could not authenticate aws client: %v, Please make sure you set up AWS credentials and regions", err),
		}
	}

	if err := aws.setAwsPluginDefaults(); err != nil {
		return &kfapis.KfError{
			Code: err.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("aws set aws plugin defaults Error %v",
				err.(*kfapis.KfError).Message),
		}
	}

	// 1. Create EKS cluster if needed
	if err := aws.createEKSCluster(); err != nil {
		return &kfapis.KfError{
			Code: err.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("Failed to create EKS cluster %v",
				err.(*kfapis.KfError).Message),
		}
	}

	// 2. For non-eks cluster (kops) or user doesn't enable pod level IAM policy,
	// attach IAM policies like ALB, FSX, EFS, cloudWatch Fluentd to worker node group roles
	// For eks cluster enable pod IAM, we create identity provider and role. Override kubeflow components service account with annotation.
	awsPluginSpec, err := aws.GetPluginSpec()
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Could not get awsPluginSpec %v", err),
		}
	}

	isEksCluster, err := aws.IsEksCluster(aws.kfDef.Name)
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Could not determinte it's EKS cluster %v", err),
		}
	}

	k8sclientset, err := aws.getK8sclient()
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("Could not get k8s client %v", err),
		}
	}

	if err := createNamespace(k8sclientset, aws.kfDef.Namespace); err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("Could not create namespace %v", err),
		}
	}

	// Create IAM role binding for k8s service account.
	if awsPluginSpec.GetEnablePodIamPolicy() && isEksCluster {
		err := aws.setupIamRoleForServiceAccount()
		if err != nil {
			return &kfapis.KfError{
				Code:    int(kfapis.INVALID_ARGUMENT),
				Message: fmt.Sprintf("Could not setup pod IAM policy %v", err),
			}
		}
	}

	// 3. Attach policies to worker node groups. This will be used by non-EKS AWS Kubernetes clusters.
	if err := aws.attachPoliciesToRoles(aws.roles); err != nil {
		return &kfapis.KfError{
			Code: err.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("Failed to attach IAM policies %v",
				err.(*kfapis.KfError).Message),
		}
	}

	// 4. Update cluster configs to enable master log or private access config.
	if err := aws.updateEKSClusterConfig(); err != nil {
		return &kfapis.KfError{
			Code: err.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("Failed to update eks cluster configs %v",
				err.(*kfapis.KfError).Message),
		}
	}

	return nil
}

// IsEksCluster checks if an AWS cluster is EKS cluster.
func (aws *Aws) IsEksCluster(clusterName string) (bool, error) {
	input := &eks.DescribeClusterInput{
		Name: awssdk.String(clusterName),
	}

	exist := true
	if _, err := aws.eksClient.DescribeCluster(input); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() != eks.ErrCodeResourceNotFoundException {
				return false, err
			}
			exist = false
		}
	}

	return exist, nil
}

func (aws *Aws) Delete(resources kftypes.ResourceEnum) error {
	// use aws to call sts get-caller-identity to verify aws credential works.
	if err := utils.CheckAwsStsCallerIdentity(aws.sess); err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Could not authenticate aws client: %v, Please make sure you set up AWS credentials and regions", err),
		}
	}

	setAwsPluginDefaultsErr := aws.setAwsPluginDefaults()
	if setAwsPluginDefaultsErr != nil {
		return &kfapis.KfError{
			Code: setAwsPluginDefaultsErr.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("aws set aws plugin defaults Error %v",
				setAwsPluginDefaultsErr.(*kfapis.KfError).Message),
		}
	}

	// 1. Delete ingress and istio, cert-manager dependencies
	if err := aws.uninstallK8sDependencies(); err != nil {
		return &kfapis.KfError{
			Code:    err.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("Could not uninstall eks cluster Error: %v", err.(*kfapis.KfError).Message),
		}
	}

	// 2. Detach inline policies from worker IAM Roles
	if err := aws.detachPoliciesToWorkerRoles(); err != nil {
		return &kfapis.KfError{
			Code:    err.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("Could not detach iam role Error: %v", err.(*kfapis.KfError).Message),
		}
	}

	// 3. Delete EKS cluster
	if err := aws.uninstallEKSCluster(); err != nil {
		return &kfapis.KfError{
			Code:    err.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("Could not uninstall eks cluster Error: %v", err.(*kfapis.KfError).Message),
		}
	}

	return nil
}

// writeFeatureConfig writes KfDef to app.yaml
func (aws *Aws) writeFeatureConfig(featureConfig map[string]interface{}) error {
	buf, bufErr := yaml.Marshal(featureConfig)
	if bufErr != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("AWS marshaling error: %v", bufErr),
		}
	}
	featureCfgFilePath := filepath.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR, CLUSTER_FEATURE_CONFIG_FILE)
	featureCfgFilePathErr := ioutil.WriteFile(featureCfgFilePath, buf, 0644)
	if featureCfgFilePathErr != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("AWS config file writing error: %v", featureCfgFilePathErr),
		}
	}
	return nil
}

func (aws *Aws) getFeatureConfig() (map[string]interface{}, error) {
	configPath := filepath.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR, CLUSTER_FEATURE_CONFIG_FILE)
	log.Infof("Reading config file: %v", configPath)

	configBuf, bufErr := ioutil.ReadFile(configPath)
	if bufErr != nil {
		return nil, bufErr
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(configBuf, &config); err != nil {
		return nil, err
	}

	return config, nil
}

func (aws *Aws) uninstallK8sDependencies() error {
	rev := func(manifests []manifest) []manifest {
		var r []manifest
		max := len(manifests)
		for i := 0; i < max; i++ {
			r = append(r, manifests[max-1-i])
		}
		return r
	}

	// 1. Delete Ingress and wait for 15s for alb-ingress-controller to clean up resources
	if err := deleteManifests(rev(aws.ingressManifests)); err != nil {
		return errors.WithStack(err)
	}

	var albCleanUpInSeconds = 15
	log.Infof("Wait for %d seconds for alb ingress controller to clean up ALB", albCleanUpInSeconds)
	time.Sleep(time.Duration(albCleanUpInSeconds) * time.Second)

	// 2. Delete cert-manager manifest.
	// Simplify process by deleting cert-manager namespace, don't have to delete every single manifest
	k8sclientset, err := aws.getK8sclient()
	if err != nil {
		return err
	}

	if err := deleteNamespace(k8sclientset, "cert-manager"); err != nil {
		return errors.WithStack(err)
	}

	// 3. Delete istio dependencies
	if err := deleteManifests(rev(aws.istioManifests)); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func deleteManifests(manifests []manifest) error {
	config := kftypes.GetConfig()
	for _, m := range manifests {
		log.Infof("Deleting %s...", m.name)
		if _, err := os.Stat(m.path); os.IsNotExist(err) {
			log.Warnf("File %s not found", m.path)
			continue
		}
		err := utils.DeleteResourceFromFile(
			config,
			m.path,
		)
		if err != nil {
			log.Errorf("Failed to delete %s: %+v", m.name, err)
			return err
		}
	}
	return nil
}

func (aws *Aws) detachPoliciesToWorkerRoles() error {
	config, err := aws.getFeatureConfig()
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Reading config file error: %v", err),
		}
	}

	if _, ok := config["worker_node_group_logging"]; !ok {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Unable to read feature config YAML: %v", err),
		}
	}

	var roles []string
	awsPluginSpec, err := aws.GetPluginSpec()
	if err != nil {
		return errors.WithStack(err)
	}

	// Find worker roles based on new cluster kfctl created or existing cluster
	if config["managed_cluster"] == true {
		workerRoles, err := aws.getWorkerNodeGroupRoles(aws.kfDef.Name)
		if err != nil {
			return errors.WithStack(err)
		}

		roles = workerRoles
	} else {
		roles = awsPluginSpec.Roles
	}

	// Detach IAM Policies
	for _, iamRole := range roles {
		aws.deleteIamRolePolicy(iamRole, "iam_alb_ingress_policy")
		aws.deleteIamRolePolicy(iamRole, "iam_csi_fsx_policy")
		aws.deleteIamRolePolicy(iamRole, "iam_profile_controller_policy")

		if config["worker_node_group_logging"] == "true" {
			aws.deleteIamRolePolicy(iamRole, "iam_cloudwatch_policy")
		}
	}

	// Delete WebIdentityIAMRole and OIDC Provider
	if awsPluginSpec.GetEnablePodIamPolicy() {
		eksCluster, err := aws.getEksCluster(aws.kfDef.Name)
		if err != nil {
			return err
		}

		aws.deleteIAMRole(fmt.Sprintf("kf-admin-%v", eksCluster.name))
		aws.deleteIAMRole(fmt.Sprintf("kf-user-%v", eksCluster.name))

		accountId, err := utils.CheckAwsAccountId(aws.sess)
		if err != nil {
			return errors.Errorf("Can not find accountId for cluster %v", aws.kfDef.Name)
		}

		// 1. Create Identity Provider.
		issuerURLWithoutProtocol := eksCluster.oidcIssuerUrl[len("https://"):]
		exist, arn, err := aws.checkIdentityProviderExists(accountId, issuerURLWithoutProtocol)
		if err != nil {
			return errors.Errorf("Can not check identity provider existence: %v", err)
		}

		if !exist {
			log.Warnf("Identity provider %v of cluster %v doesnot exist", arn, eksCluster.name)
			return nil
		}

		if err := aws.DeleteIdentityProvider(arn); err != nil {
			return err
		}
	}

	return nil
}

func (aws *Aws) uninstallEKSCluster() error {
	config, err := aws.getFeatureConfig()
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Reading feature config file error: %v", err),
		}
	}

	if _, ok := config["managed_cluster"]; !ok {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Unable to read YAML: %v", err),
		}
	}

	// Delete cluster if it's a managed cluster created by kfctl
	if config["managed_cluster"] == true {
		log.Infoln("Start to delete eks cluster. Please wait for 5 mins...")
		clusterConfigFile := filepath.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR, CLUSTER_CONFIG_FILE)
		output, err := exec.Command("eksctl", "delete", "cluster", "--config-file="+clusterConfigFile).Output()
		log.Infoln("Please go to aws console to check CloudFormation status and double make sure your cluster has been shutdown.")
		if err != nil {
			return &kfapis.KfError{
				Code:    int(kfapis.INVALID_ARGUMENT),
				Message: fmt.Sprintf("could not call 'eksctl delete cluster --config-file=%s': %v", clusterConfigFile, string(output)),
			}
		}
		log.Infoln(string(output))
	}

	return nil
}

func (aws *Aws) deleteIamRolePolicy(roleName, policyName string) error {
	log.Infof("Deleting inline policy %s for iam role %s", policyName, roleName)

	input := &iam.DeleteRolePolicyInput{
		PolicyName: awssdk.String(policyName),
		RoleName:   awssdk.String(roleName),
	}

	result, err := aws.iamClient.DeleteRolePolicy(input)
	// This error can be skipped and should not block delete process.
	// It's possible user make any changes on IAM role.
	if err != nil {
		log.Warnf("Unable to delete iam inline policy %s because %v", policyName, err.Error())
	} else {
		log.Infof("Successfully delete policy from IAM Role %v", result)
	}

	return nil
}

func (aws *Aws) attachIamInlinePolicy(roleName, policyName, policyDocumentPath string) error {
	log.Infof("Attaching inline policy %s for iam role %s", policyName, roleName)
	policyDocumentJSONBytes, _ := ioutil.ReadFile(policyDocumentPath)

	input := &iam.PutRolePolicyInput{
		PolicyDocument: awssdk.String(string(policyDocumentJSONBytes)),
		PolicyName:     awssdk.String(policyName),
		RoleName:       awssdk.String(roleName),
	}

	_, err := aws.iamClient.PutRolePolicy(input)
	if err != nil {
		log.Warnf("Unable to attach iam inline policy %s because %v", policyName, err.Error())
		return nil
	}

	log.Infof("Successfully attach policy to IAM Role %v", roleName)
	return nil
}

// setupIamRoleForServiceAccount will create/reuse IAM identity provider and create/reuse web identity role.
func (aws *Aws) setupIamRoleForServiceAccount() error {
	eksCluster, err := aws.getEksCluster(aws.kfDef.Name)
	if err != nil {
		return err
	}

	accountId, err := utils.CheckAwsAccountId(aws.sess)
	if err != nil {
		return errors.Errorf("Can not find accountId for cluster %v", aws.kfDef.Name)
	}

	// 1. Create Identity Provider.
	issuerURLWithoutProtocol := eksCluster.oidcIssuerUrl[len("https://"):]
	exist, arn, err := aws.checkIdentityProviderExists(accountId, issuerURLWithoutProtocol)
	if err != nil {
		return errors.Errorf("Can not check identity provider existence: %v", err)
	}

	oidcProviderArn := arn
	if !exist {
		arn, err := aws.createIdentityProvider(eksCluster.oidcIssuerUrl)
		if err != nil {
			return errors.Errorf("Can not check identity provider existence: %v", err)
		}
		oidcProviderArn = arn
	}

	kubeflowSAIamRoleMapping := map[string]string{
		"kf-admin":                            fmt.Sprintf("kf-admin-%v", eksCluster.name),
		"alb-ingress-controller":              fmt.Sprintf("kf-admin-%v", eksCluster.name),
		"profiles-controller-service-account": fmt.Sprintf("kf-admin-%v", eksCluster.name),
		"fluentd":                             fmt.Sprintf("kf-admin-%v", eksCluster.name),
		"kf-user":                             fmt.Sprintf("kf-user-%v", eksCluster.name),
	}

	// 2. Create IAM Roles using the web identity provider created in last step.
	for ksa, iamRole := range kubeflowSAIamRoleMapping {
		if err := aws.checkWebIdentityRoleExist(iamRole); err == nil {
			log.Infof("Find existing role %s to reuse for ksa %s", iamRole, ksa)
			continue
		}

		log.Infof("Creating IAM role %s", iamRole)
		if err := aws.createWebIdentityRole(oidcProviderArn, issuerURLWithoutProtocol, iamRole, aws.kfDef.Namespace, ksa); err != nil {
			return errors.Errorf("Can not create web identity role: %v", err)
		}
	}

	// We only want to attach admin role at this moment.
	aws.roles = append(aws.roles, fmt.Sprintf("kf-admin-%v", eksCluster.name))

	// 3. Link KSA to IAM Role.
	k8sclientset, err := aws.getK8sclient()
	if err != nil {
		return err
	}

	for ksa, iamRole := range kubeflowSAIamRoleMapping {
		if err := aws.setupIAMForServiceAccount(k8sclientset, aws.kfDef.Namespace, ksa, accountId, iamRole); err != nil {
			return err
		}
	}

	return nil
}

func (aws *Aws) setupIAMForServiceAccount(k8sclientset *clientset.Clientset, namespace, ksa, accountId, iamRole string) error {
	// Add IAMRole in service account annotation
	if err := aws.createOrUpdateK8sServiceAccount(k8sclientset, namespace, ksa, accountId, iamRole); err != nil {
		return errors.Errorf("Can not link KSA %s/%s to IAM Role %s/%s, %v", namespace, ksa, accountId, iamRole, err)
	}

	// Add service account in Role Trust Relationships
	if err := aws.updateRoleTrustIdentity(aws.kfDef.Namespace, ksa, iamRole); err != nil {
		return errors.Errorf("Can not update IAM role trust relationships %v", err)
	}

	return nil
}

func (aws *Aws) getK8sclient() (*clientset.Clientset, error) {
	home := homeDir()
	kubeconfig := filepath.Join(home, ".kube", "config")

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, errors.Errorf("Failed to create config file from %s", kubeconfig)
	}

	clientset, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, errors.Errorf("Failed to create kubernetes clientset")
	}

	return clientset, nil
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func (aws *Aws) getEksCluster(clusterName string) (*Cluster, error) {
	input := &eks.DescribeClusterInput{
		Name: awssdk.String(clusterName),
	}

	result, err := aws.eksClient.DescribeCluster(input)
	if err != nil {
		return nil, err
	}

	cluster := &Cluster{
		name:              awssdk.StringValue(result.Cluster.Name),
		apiServerEndpoint: awssdk.StringValue(result.Cluster.Endpoint),
		oidcIssuerUrl:     awssdk.StringValue(result.Cluster.Identity.Oidc.Issuer),
		clusterArn:        awssdk.StringValue(result.Cluster.Arn),
		roleArn:           awssdk.StringValue(result.Cluster.RoleArn),
		kubernetesVersion: awssdk.StringValue(result.Cluster.Version),
	}

	log.Infof("EKS cluster info %v", cluster)
	return cluster, nil
}

type Cluster struct {
	name              string
	apiServerEndpoint string
	oidcIssuerUrl     string
	clusterArn        string
	roleArn           string
	kubernetesVersion string
}
