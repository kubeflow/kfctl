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
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

const (
	KUBEFLOW_AWS_INFRA_DIR = "aws_config"
	KUBEFLOW_MANIFEST_DIR  = "kustomize"
	CLUSTER_CONFIG_FILE    = "cluster_config.yaml"
	PATH                   = "path"
	BASIC_AUTH_SECRET      = "kubeflow-login"
	// Path in manifests repo to where the additional configs are located
	CONFIG_LOCAL_PATH = "aws/infra_configs"

	ALB_OIDC_SECRET = "alb-oidc-secret"

	// Namespace for istio
	IstioNamespace = "istio-system"

	// Plugin parameter constants
	AwsPluginName = kfconfig.AWS_PLUGIN_KIND

	MINIMUM_EKSCTL_VERSION = "0.1.32"

	KUBEFLOW_ADMIN_ROLE_NAME = "kf-admin-%v-%v"
	KUBEFLOW_USER_ROLE_NAME  = "kf-user-%v-%v"
)

// Aws implements KfApp Interface
// It includes the KsApp along with additional Aws types
type Aws struct {
	kfDef     *kfconfig.KfConfig
	iamClient *iam.IAM
	eksClient *eks.EKS
	sess      *session.Session
	k8sClient *clientset.Clientset

	cluster *Cluster

	region string
	roles  []string

	istioManifests       []manifest
	ingressManifests     []manifest
	certManagerManifests []manifest
}

type manifest struct {
	name string
	path string
}

// GetKfApp returns the aws kfapp. It's called by coordinator.GetKfApp
func GetPlatform(kfdef *kfconfig.KfConfig) (kftypes.Platform, error) {
	// Manifest lists are used in `Delete` to make sure we track and clean up all the resources.
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
	certManagerManifests := []manifest{
		{
			name: "Cert Manager",
			path: path.Join(KUBEFLOW_MANIFEST_DIR, "cert-manager-crds", "base", "crd.yaml"),
		},
		{
			name: "Cert Manager API Service",
			path: path.Join(KUBEFLOW_MANIFEST_DIR, "cert-manager", "base", "api-service.yaml"),
		},
		{
			name: "Cert Manager MutationWebhookConfig",
			path: path.Join(KUBEFLOW_MANIFEST_DIR, "cert-manager", "base", "mutating-webhook-configuration.yaml"),
		},
		{
			name: "Cert Manager ValidatingWebhookConfiguration",
			path: path.Join(KUBEFLOW_MANIFEST_DIR, "cert-manager", "base", "validating-webhook-configuration.yaml"),
		},
	}

	// set aws.sess with shared config file information, such as region
	session := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	k8sClient, err := getK8sclient()
	if err != nil {
		return nil, err
	}

	_aws := &Aws{
		kfDef:                kfdef,
		sess:                 session,
		iamClient:            iam.New(session),
		eksClient:            eks.New(session),
		k8sClient:            k8sClient,
		istioManifests:       istioManifests,
		ingressManifests:     ingressManifests,
		certManagerManifests: certManagerManifests,
	}

	return _aws, nil
}

// GetPluginSpec gets the plugin spec.
func (aws *Aws) GetPluginSpec() (*awsplugin.AwsPluginSpec, error) {
	awsPluginSpec := &awsplugin.AwsPluginSpec{}
	err := aws.kfDef.GetPluginSpec(AwsPluginName, awsPluginSpec)
	return awsPluginSpec, err
}

func (aws *Aws) attachPoliciesToRoles(roles []string) error {
	awsPluginSpec, err := aws.GetPluginSpec()
	if err != nil {
		return err
	}

	for _, iamRole := range roles {
		aws.attachIamInlinePolicy(iamRole, "iam_alb_ingress_policy",
			filepath.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR, "iam_alb_ingress_policy.json"))
		aws.attachIamInlinePolicy(iamRole, "iam_profile_controller_policy",
			filepath.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR, "iam_profile_controller_policy.json"))
		//aws.attachIamInlinePolicy(iamRole, "iam_csi_fsx_policy",
		//	filepath.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR, "iam_csi_fsx_policy.json"))
		if awsPluginSpec.GetEnableNodeGroupLog() {
			aws.attachIamInlinePolicy(iamRole, "iam_cloudwatch_policy",
				filepath.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR, "iam_cloudwatch_policy.json"))
		}
	}

	return nil
}

// TODO: To be implemented. Consider to have EKS cluster config support.
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
			Message: fmt.Sprintf("cannot create dest file %v : %v", dest, err),
		}
	}
	defer to.Close()
	_, err = io.Copy(to, from)
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("copy failed source %v dest %v: %v", source, dest, err),
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

// ${BASE_DIR}/${KFAPP}/aws_config -> destDir (dest)
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
			return destDirErr
		}
	} else {
		log.Infof("AWS infrastructure configs already exist in directory %v", destDir)
	}

	// List all the files under source directory
	files, err := ioutil.ReadDir(sourceDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		sourceFile := filepath.Join(sourceDir, file.Name())
		destFile := filepath.Join(destDir, file.Name())
		copyErr := copyFile(sourceFile, destFile)
		if copyErr != nil {
			return &kfapis.KfError{
				Code: copyErr.(*kfapis.KfError).Code,
				Message: fmt.Sprintf("Could not copy %v to %v: %v",
					sourceFile, destFile, copyErr.(*kfapis.KfError).Message),
			}
		}
	}

	// 2. Reading from cluster_config.yaml and replace placeholders with values in aws.kfDef.Spec.
	clusterConfigFile := filepath.Join(destDir, CLUSTER_CONFIG_FILE)
	if err := aws.updateClusterConfig(clusterConfigFile); err != nil {
		return err
	}

	// 3. Update managed_cluster
	// @Deprecated. Don't need to update the field, we add configs part of awsPluginSpec. It's false by default
	return nil
}

func (aws *Aws) generateBasicAuthPasswordHash() (string, error) {
	awsPluginSpec, err := aws.GetPluginSpec()
	if err != nil {
		return "", err
	}

	if awsPluginSpec.Auth == nil || awsPluginSpec.Auth.BasicAuth == nil || awsPluginSpec.Auth.BasicAuth.Password == "" {
		err := errors.WithStack(fmt.Errorf("BasicAuth.Password must be set if enabled BasicAuth"))
		return "", err
	}

	encodedPassword, err := base64EncryptPassword(awsPluginSpec.Auth.BasicAuth.Password)
	if err != nil {
		log.Errorf("There was a problem encrypting the password; %v", err)
		return "", err
	}

	return encodedPassword, nil
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
	version, err := utils.GetEksctlVersion()
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Can not run eksctl version %v", err),
		}
	}

	if lessThan, err := isEksctlVersionLessThan(version, MINIMUM_EKSCTL_VERSION); err != nil || lessThan {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("eksctl version has to be great than %s %v", MINIMUM_EKSCTL_VERSION, err),
		}
	}

	return nil
}

// Generate generate aws infrastructure configs and aws kfapp manifest
// Remind: Need to be thread-safe: this entry is share among kfctl and deploy app
func (aws *Aws) Generate(resources kftypes.ResourceEnum) error {
	awsDir := path.Join(aws.kfDef.Spec.AppDir, KUBEFLOW_AWS_INFRA_DIR)
	if _, err := os.Stat(awsDir); err == nil {
		log.Infof("Folder %v exists, skip aws.Generate", awsDir)
		return nil
	} else if !os.IsNotExist(err) {
		log.Errorf("Stat folder %v error: %v; trying to delete it...", awsDir, err)
		_ = os.RemoveAll(awsDir)
	}

	// Use aws sts get-caller-identity to verify aws credential setting
	if err := utils.CheckAwsStsCallerIdentity(aws.sess); err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Could not authenticate aws client: %v, Please make sure you set up AWS credentials and regions", err),
		}
	}

	if setAwsPluginDefaultsErr := aws.setAwsPluginDefaults(); setAwsPluginDefaultsErr != nil {
		return &kfapis.KfError{
			Code: setAwsPluginDefaultsErr.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("set aws plugin defaults: %v",
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

	pluginSpec, err := aws.GetPluginSpec()
	if err != nil {
		return errors.WithStack(err)
	}

	if err := aws.kfDef.SetApplicationParameter("aws-alb-ingress-controller", "cluster-name", aws.kfDef.Name); err != nil {
		return errors.WithStack(err)
	}

	if pluginSpec.Auth != nil && pluginSpec.Auth.BasicAuth != nil && pluginSpec.Auth.BasicAuth.Password != "" {
		if err := aws.kfDef.SetApplicationParameter("dex", "static_email", pluginSpec.Auth.BasicAuth.Username); err != nil {
			return errors.WithStack(err)
		}
		if encodedPasswordHash, err := aws.generateBasicAuthPasswordHash(); err == nil {
			if err := aws.kfDef.SetApplicationParameter("dex", "static_password_hash", encodedPasswordHash); err != nil {
				return errors.WithStack(err)
			}
		} else {
			return errors.WithStack(err)
		}
	} else {
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

		// By default we use cognito overlay in manifest, remove cognito and add oidc overlay if this is enabled.
		if pluginSpec.Auth.Oidc != nil {
			if err := aws.kfDef.SetApplicationParameter("istio", "clusterRbacConfig", "ON"); err != nil {
				return errors.WithStack(err)
			}

			if err := aws.kfDef.RemoveApplicationOverlay("istio-ingress", "cognito"); err != nil {
				return errors.WithStack(err)
			}

			if err := aws.kfDef.AddApplicationOverlay("istio-ingress", "oidc"); err != nil {
				return errors.WithStack(err)
			}

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

			//TODO: consider to use secret from secretGenerator?
			if err := aws.kfDef.SetApplicationParameter("istio-ingress", "oidcSecretName", ALB_OIDC_SECRET); err != nil {
				return errors.WithStack(err)
			}
		}
	}

	// Special handling for cloud watch logs of worker node groups
	if pluginSpec.GetEnableNodeGroupLog() {
		//aws.kfDef.Spec.Components = append(aws.kfDef.Spec.Components, "fluentd-cloud-watch")
		if err := aws.kfDef.SetApplicationParameter("fluentd-cloud-watch", "clusterName", aws.kfDef.Name); err != nil {
			return errors.WithStack(err)
		}
		if err := aws.kfDef.SetApplicationParameter("fluentd-cloud-watch", "region", aws.region); err != nil {
			return errors.WithStack(err)
		}
	}

	// Special handling for managed SQL service
	if pluginSpec.ManagedRelationDatabase != nil {
		// Setup metadata -> remove `db` overlay, add `external-mysql` overlay
		if err := aws.kfDef.RemoveApplicationOverlay("metadata", "db"); err != nil {
			return errors.WithStack(err)
		}

		if err := aws.kfDef.AddApplicationOverlay("metadata", "external-mysql"); err != nil {
			return errors.WithStack(err)
		}

		// add external-mysql to pipeline/api-service and external-mysql to metadata,
		if err := aws.kfDef.SetApplicationParameter("metadata", "MYSQL_HOST", pluginSpec.ManagedRelationDatabase.Host); err != nil {
			return errors.WithStack(err)
		}

		if err := aws.kfDef.SetApplicationParameter("metadata", "MYSQL_USERNAME", string(pluginSpec.ManagedRelationDatabase.Username)); err != nil {
			return errors.WithStack(err)
		}

		if err := aws.kfDef.SetApplicationParameter("metadata", "MYSQL_ROOT_PASSWORD", string(pluginSpec.ManagedRelationDatabase.Password)); err != nil {
			return errors.WithStack(err)
		}

		if pluginSpec.ManagedRelationDatabase.Port != nil {
			if err := aws.kfDef.SetApplicationParameter("metadata", "MYSQL_PORT", fmt.Sprint(*pluginSpec.ManagedRelationDatabase.Port)); err != nil {
				return errors.WithStack(err)
			}
		}

		// Setup pipeline/api-service -> move mysql application, add external-mysql overlay to pipeline/api-service
		if err := aws.kfDef.DeleteApplication("mysql"); err != nil {
			return errors.WithStack(err)
		}

		if err := aws.kfDef.AddApplicationOverlay("api-service", "external-mysql"); err != nil {
			return errors.WithStack(err)
		}

		// add external-mysql to pipeline/api-service and external-mysql to metadata,
		if err := aws.kfDef.SetApplicationParameter("api-service", "mysqlHost", pluginSpec.ManagedRelationDatabase.Host); err != nil {
			return errors.WithStack(err)
		}

		if err := aws.kfDef.SetApplicationParameter("api-service", "mysqlUser", pluginSpec.ManagedRelationDatabase.Username); err != nil {
			return errors.WithStack(err)
		}

		if err := aws.kfDef.SetApplicationParameter("api-service", "mysqlPassword", pluginSpec.ManagedRelationDatabase.Password); err != nil {
			return errors.WithStack(err)
		}
	}

	// Special handling for managed object storage
	if pluginSpec.ManagedObjectStorage != nil {
		// TODO: replace worker-controller, pipeline, etc layer
	}

	// Special handling for sparkakus
	rand.Seed(time.Now().UnixNano())
	if err := aws.kfDef.SetApplicationParameter("spartakus", "usageId", strconv.Itoa(rand.Int())); err != nil {
		if kfconfig.IsAppNotFound(err) {
			log.Infof("Spartakus not included; not setting usageId")
		}
	}

	return nil
}

func (aws *Aws) setAwsPluginDefaults() error {
	awsPluginSpec, err := aws.GetPluginSpec()
	if err != nil {
		return err
	}

	if isValid, msg := awsPluginSpec.IsValid(); !isValid {
		log.Errorf("AwsPluginSpec isn't valid; error %v", msg)
		return fmt.Errorf(msg)
	}

	aws.region = awsPluginSpec.Region
	aws.roles = awsPluginSpec.Roles

	if awsPluginSpec.ManagedCluster == nil {
		awsPluginSpec.ManagedCluster = proto.Bool(awsPluginSpec.GetManagedCluster())
		log.Infof("ManagedCluster set defaulting to %v", utils.PrettyPrint(*awsPluginSpec.ManagedCluster))
	}

	if awsPluginSpec.EnablePodIamPolicy == nil {
		awsPluginSpec.EnablePodIamPolicy = proto.Bool(awsPluginSpec.GetEnablePodIamPolicy())
		log.Infof("EnablePodIamPolicy set defaulting to %v", utils.PrettyPrint(*awsPluginSpec.EnablePodIamPolicy))
	}

	if awsPluginSpec.EnableNodeGroupLog == nil {
		awsPluginSpec.EnableNodeGroupLog = proto.Bool(awsPluginSpec.GetEnableNodeGroupLog())
		log.Infof("EnableNodeGroupLog set defaulting to %v", utils.PrettyPrint(*awsPluginSpec.EnableNodeGroupLog))
	}

	if awsPluginSpec.Auth == nil {
		awsPluginSpec.Auth = &awsplugin.Auth{}
	}

	return nil
}

// Apply create eks cluster if needed, bind IAM policy to node group roles and enable cluster level configs.
// Remind: Need to be thread-safe: this entry is share among kfctl and deploy app
func (aws *Aws) Apply(resources kftypes.ResourceEnum) error {
	// use aws sts get-caller-identity to verify aws credential works.
	if err := utils.CheckAwsStsCallerIdentity(aws.sess); err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("Could not authenticate aws client: %v, Please make sure you set up AWS credentials and regions", err),
		}
	}

	if err := aws.setAwsPluginDefaults(); err != nil {
		return &kfapis.KfError{
			Code: err.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("aws set aws plugin defaults: %v",
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

	if err := createNamespace(aws.k8sClient, aws.kfDef.Namespace); err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("Could not create namespace %v", err),
		}
	}

	if err := createNamespace(aws.k8sClient, IstioNamespace); err != nil {
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
	} else if awsPluginSpec.GetEnablePodIamPolicy() {
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: fmt.Sprintf("IAM for Service Account is not supported on non-EKS cluster %v", err),
		}
	}

	// 3. Attach policies to worker node groups. This will be used by both EKS and non-EKS AWS Kubernetes clusters.
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

	// 5. Setup OIDC create OIDC secret for ALB
	if err := aws.setupOIDC(); err != nil {
		return &kfapis.KfError{
			Code: err.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("Failed to update create OIDC secret for ALB %v",
				err.(*kfapis.KfError).Message),
		}
	}

	return nil
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
			Message: fmt.Sprintf("aws set aws plugin defaults: %v",
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
	if err := aws.detachPoliciesFromWorkerRoles(); err != nil {
		return &kfapis.KfError{
			Code:    err.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("Could not detach iam role Error: %v", err.(*kfapis.KfError).Message),
		}
	}

	// 3. Delete WebIdentityIAMRole and OIDC Provider and pre-configured roles
	if err := aws.deleteWebIdentityRolesAndProvider(); err != nil {
		return &kfapis.KfError{
			Code:    err.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("Could not detach iam role Error: %v", err.(*kfapis.KfError).Message),
		}
	}

	// 4. Delete EKS cluster
	if err := aws.deleteEKSCluster(); err != nil {
		return &kfapis.KfError{
			Code:    err.(*kfapis.KfError).Code,
			Message: fmt.Sprintf("Could not uninstall eks cluster Error: %v", err.(*kfapis.KfError).Message),
		}
	}

	return nil
}

func (aws *Aws) Dump(resources kftypes.ResourceEnum) error {
	return nil
}

// uninstallK8sDependencies delete istio-ingress, istio and cert-manager dependencies.
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
	if err := deleteNamespace(aws.k8sClient, "cert-manager"); err != nil {
		return errors.WithStack(err)
	}

	if err := deleteManifests(rev(aws.certManagerManifests)); err != nil {
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

func (aws *Aws) detachPoliciesFromWorkerRoles() error {
	awsPluginSpec, err := aws.GetPluginSpec()
	if err != nil {
		return errors.WithStack(err)
	}

	var roles []string
	eksCluster, err := aws.getEksCluster(aws.kfDef.Name)
	if err != nil {
		return err
	}

	if awsPluginSpec.GetEnablePodIamPolicy() {
		// no matter it's managed or self-managed cluster, we setup kf-admin roles.
		roles = append(roles, fmt.Sprintf(KUBEFLOW_ADMIN_ROLE_NAME, aws.region, eksCluster.name))
	} else {
		// Find worker roles based on new cluster kfctl created or existing cluster
		if awsPluginSpec.GetManagedCluster() {
			workerRoles, err := aws.getWorkerNodeGroupRoles(aws.kfDef.Name)
			if err != nil {
				return errors.WithStack(err)
			}
			roles = workerRoles
		} else {
			roles = awsPluginSpec.Roles
		}
	}

	// Detach IAM Policies
	for _, iamRole := range roles {
		aws.deleteIamRolePolicy(iamRole, "iam_alb_ingress_policy")
		aws.deleteIamRolePolicy(iamRole, "iam_profile_controller_policy")

		// TODO: use Addon to check permissions
		// aws.deleteIamRolePolicy(iamRole, "iam_csi_fsx_policy")
		if awsPluginSpec.GetEnableNodeGroupLog() {
			aws.deleteIamRolePolicy(iamRole, "iam_cloudwatch_policy")
		}
	}

	return nil
}

// deleteIamRolePolicy detach inline policy from the role
func (aws *Aws) deleteIamRolePolicy(roleName, policyName string) error {
	log.Infof("Deleting inline policy %s for iam role %s", policyName, roleName)

	input := &iam.DeleteRolePolicyInput{
		PolicyName: awssdk.String(policyName),
		RoleName:   awssdk.String(roleName),
	}

	_, err := aws.iamClient.DeleteRolePolicy(input)
	// This error can be skipped and should not block delete process.
	// It's possible user make any changes on IAM role.
	if err != nil {
		log.Warnf("Unable to delete iam inline policy %s because %v", policyName, err.Error())
	}

	return nil
}

// attachIamInlinePolicy attach inline policy to IAM role
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

	// Create Identity Provider.
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

	kubeflowAdminRoleName := fmt.Sprintf(KUBEFLOW_ADMIN_ROLE_NAME, aws.region, eksCluster.name)
	kubeflowUserRoleName := fmt.Sprintf(KUBEFLOW_USER_ROLE_NAME, aws.region, eksCluster.name)

	// Link service account, role and policy
	kubeflowSAIamRoleMapping := map[string]string{
		"kf-admin":                            kubeflowAdminRoleName,
		"alb-ingress-controller":              kubeflowAdminRoleName,
		"profiles-controller-service-account": kubeflowAdminRoleName,
		"fluentd":                             kubeflowAdminRoleName,
		"kf-user":                             kubeflowUserRoleName,
	}

	for ksa, iamRoleName := range kubeflowSAIamRoleMapping {
		// 1. Create AWS IAM Roles with web identity provider as trusted identities
		if err := aws.createOrUpdateWebIdentityRole(oidcProviderArn, issuerURLWithoutProtocol, iamRoleName, aws.kfDef.Namespace, ksa); err != nil {
			return errors.Errorf("Can not create web identity role: %v", err)
		}

		// 2. Create Kubernetes Service Account
		iamRoleArn := fmt.Sprintf(AWS_IAM_ROLE_ARN, accountId, iamRoleName)
		if err := aws.createOrUpdateK8sServiceAccount(aws.k8sClient, aws.kfDef.Namespace, ksa, iamRoleArn); err != nil {
			return errors.Errorf("Can not create Service Account %s/%s, %v", aws.kfDef.Namespace, ksa, err)
		}

		// 3. Link KSA to IAM Role - add service account in Role Trust Relationships
		if err := aws.updateRoleTrustIdentity(iamRoleName, aws.kfDef.Namespace, ksa); err != nil {
			return errors.Errorf("Can not update IAM role trust relationships %v", err)
		}
	}

	// We only want to attach admin role at this moment.
	// Grant kf-user policies later, based on the potential actions use may have, like ECR access, S3 access, etc.
	aws.roles = append(aws.roles, fmt.Sprintf(KUBEFLOW_ADMIN_ROLE_NAME, aws.region, eksCluster.name))
	return nil
}

func (aws *Aws) deleteWebIdentityRolesAndProvider() error {
	awsPluginSpec, err := aws.GetPluginSpec()
	if err != nil {
		return errors.WithStack(err)
	}

	if !awsPluginSpec.GetEnablePodIamPolicy() {
		log.Infof("Pod IAM Policy is not set, skip delete web identity provider")
		return nil
	}

	eksCluster, err := aws.getEksCluster(aws.kfDef.Name)
	if err != nil {
		return err
	}

	// Delete IAM role we created
	kfAdminRoleName := fmt.Sprintf(KUBEFLOW_ADMIN_ROLE_NAME, aws.region, eksCluster.name)
	kfUserRoleName := fmt.Sprintf(KUBEFLOW_USER_ROLE_NAME, aws.region, eksCluster.name)
	aws.deleteIAMRole(kfAdminRoleName)
	aws.deleteIAMRole(kfUserRoleName)
	log.Infof("IAM Role %s, %s has been deleted", kfAdminRoleName, kfUserRoleName)

	accountId, err := utils.CheckAwsAccountId(aws.sess)
	if err != nil {
		return errors.Errorf("Can not find accountId for cluster %v", aws.kfDef.Name)
	}

	// Delete oidc web identity provider
	issuerURLWithoutProtocol := eksCluster.oidcIssuerUrl[len("https://"):]
	exist, arn, err := aws.checkIdentityProviderExists(accountId, issuerURLWithoutProtocol)
	if err != nil {
		return errors.Errorf("Can not check identity provider existence: %v", err)
	}

	if !exist {
		log.Warnf("Identity provider %v of cluster %v does not exist", arn, eksCluster.name)
		return nil
	}

	if err := aws.DeleteIdentityProvider(arn); err != nil {
		return err
	}

	log.Infof("OIDC Identity Provider has been delete %s", issuerURLWithoutProtocol)

	return nil
}

// setupOIDC creates secret for ALB ingress controller
func (aws *Aws) setupOIDC() error {
	awsPluginSpec, err := aws.GetPluginSpec()
	if err != nil {
		return err
	}

	if awsPluginSpec.Auth.Oidc != nil {
		// Create OIDC Secret from clientId and clientSecret.
		_, err = aws.k8sClient.CoreV1().Secrets(IstioNamespace).Get(ALB_OIDC_SECRET, metav1.GetOptions{})
		if err == nil {
			log.Warnf("Secret %v already exists...", ALB_OIDC_SECRET)
			return nil
		}

		// This secret need to be in istio-system, same namespace as istio-ingress
		return createSecret(aws.k8sClient, ALB_OIDC_SECRET, IstioNamespace, map[string][]byte{
			"clientId":     []byte(awsPluginSpec.Auth.Oidc.OAuthClientId),
			"clientSecret": []byte(awsPluginSpec.Auth.Oidc.OAuthClientSecret),
		})
	}

	return nil
}
