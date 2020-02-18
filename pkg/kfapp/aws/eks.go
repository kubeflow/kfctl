package aws

import (
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/eks"
	versionChecker "github.com/hashicorp/go-version"
	kfapis "github.com/kubeflow/kfctl/v3/pkg/apis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"os/exec"
	"path/filepath"
)

type Cluster struct {
	name              string
	apiServerEndpoint string
	oidcIssuerUrl     string
	clusterArn        string
	roleArn           string
	kubernetesVersion string
}

// getEksCluster obtain detail info of an eks cluster
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

	return cluster, nil
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

// createEKSCluster creates a new EKS cluster if want kfctl to manage cluster
// @Deprecated. In order to simplify workflow, user should always brings their own cluster and install kubeflow on top of it.
// We still leave this option here and probably remove codes in future version
func (aws *Aws) createEKSCluster() error {
	awsPluginSpec, err := aws.GetPluginSpec()
	if err != nil {
		return err
	}

	if awsPluginSpec.GetManagedCluster() {
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

// deleteEKSCluster deletes eks cluster if current cluster is a managed cluster
func (aws *Aws) deleteEKSCluster() error {
	awsPluginSpec, err := aws.GetPluginSpec()
	if err != nil {
		return err
	}

	// Delete cluster if it's a managed cluster created by kfctl
	if awsPluginSpec.GetManagedCluster() {
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

// isEksctlVersionLessThan compare two version and return true if v1 is less than v2
func isEksctlVersionLessThan(v1, v2 string) (bool, error) {
	version1, err := versionChecker.NewVersion(v1)
	if err != nil {
		return false, err
	}

	version2, err := versionChecker.NewVersion(v2)
	if err != nil {
		return false, err
	}

	if version1.LessThan(version2) {
		return true, nil
	}

	return false, nil
}
