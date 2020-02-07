package aws

import (
	"bytes"
	"crypto/sha1"
	"crypto/tls"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	kfapis "github.com/kubeflow/kfctl/v3/pkg/apis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"net/http"
	"strings"
)

// checkIdentityProviderExists will return true when the iam identity provider exists, it may return errors
// if it was unable to call IAM API
func (aws *Aws) checkIdentityProviderExists(accountId, issuerURLWithoutProtocol string) (bool, string, error) {
	input := &iam.GetOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: awssdk.String(
			fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", accountId, issuerURLWithoutProtocol),
		),
	}
	_, err := aws.iamClient.GetOpenIDConnectProvider(input)
	if err != nil {
		awsError := err.(awserr.Error)
		if awsError.Code() == iam.ErrCodeNoSuchEntityException {
			return false, "", nil
		}
		return false, "", err
	}

	return true, awssdk.StringValue(input.OpenIDConnectProviderArn), nil
}

// getIssueCAThumbprint will generate CAThumbprint from a given issuerURL
func (aws *Aws) getIssueCAThumbprint(issuerURL string) (string, error) {
	var issuerCAThumbprint string

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			Proxy: http.ProxyFromEnvironment,
		},
	}

	response, err := client.Get(issuerURL)
	if err != nil {
		return "", errors.Wrap(err, "connecting to issuer OIDC")
	}

	if response.TLS != nil {
		if numCerts := len(response.TLS.PeerCertificates); numCerts >= 1 {
			root := response.TLS.PeerCertificates[numCerts-1]
			issuerCAThumbprint = fmt.Sprintf("%x", sha1.Sum(root.Raw))
			return issuerCAThumbprint, nil
		}
	}
	return "", errors.Errorf("unable to get OIDC issuer's certificate")
}

// createIdentityProvider create an OpenIDConnectProvider, it's one to one mapping to EKS cluster.
func (aws *Aws) createIdentityProvider(issuerUrl string) (string, error) {
	issuerCAThumbprint, err := aws.getIssueCAThumbprint(issuerUrl)

	oidcProviderInput := &iam.CreateOpenIDConnectProviderInput{
		ClientIDList:   []*string{awssdk.String(DEFAULT_AUDIENCE)},
		ThumbprintList: []*string{awssdk.String(issuerCAThumbprint)},
		Url:            awssdk.String(issuerUrl),
	}

	output, err := aws.iamClient.CreateOpenIDConnectProvider(oidcProviderInput)
	if err != nil {
		return "", errors.Wrap(err, "creating OIDC identity provider")
	}

	return awssdk.StringValue(output.OpenIDConnectProviderArn), nil
}

// DeleteIdentityProvider will delete the identity provider using IAM API, it may return an error the API call fails
func (aws *Aws) DeleteIdentityProvider(providerArn string) error {
	input := &iam.DeleteOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: awssdk.String(providerArn),
	}
	if _, err := aws.iamClient.DeleteOpenIDConnectProvider(input); err != nil {
		return errors.Wrap(err, "deleting OIDC provider")
	}
	return nil
}

func (aws *Aws) checkWebIdentityRoleExist(roleName string) error {
	input := &iam.GetRoleInput{
		RoleName: awssdk.String(roleName),
	}

	if _, err := aws.iamClient.GetRole(input); err != nil {
		return err
	}

	return nil
}

// createWebIdentityRole creates an IAM role with trusted entity Web Identity
func (aws *Aws) createWebIdentityRole(oidcProviderArn, issuerUrlWithProtocol, roleName, namespace, ksa string) error {
	assumeRolePolicyDocument := []byte(
`{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Effect": "Allow",
			"Principal": {
				"Federated": "$(roleArn)"
			},
			"Action": "sts:AssumeRoleWithWebIdentity",
			"Condition": {
				"StringEquals": {
					"$(oidcProvider):aud": "sts.amazonaws.com",
					"$(oidcProvider):sub": "system:serviceaccount:$(namespace):$(ksa)"
				}
			}
		}
	]
}`)

	assumeRolePolicyDocument = bytes.Replace(assumeRolePolicyDocument, []byte("$(roleArn)"), []byte(oidcProviderArn), -1)
	assumeRolePolicyDocument = bytes.Replace(assumeRolePolicyDocument, []byte("$(oidcProvider)"), []byte(issuerUrlWithProtocol), -1)
	assumeRolePolicyDocument = bytes.Replace(assumeRolePolicyDocument, []byte("$(namespace)"), []byte(namespace), -1)
	assumeRolePolicyDocument = bytes.Replace(assumeRolePolicyDocument, []byte("$(ksa)"), []byte(ksa), -1)

	roleInput := &iam.CreateRoleInput{
		RoleName:                 awssdk.String(roleName),
		AssumeRolePolicyDocument: awssdk.String(string(assumeRolePolicyDocument)),
		Tags: []*iam.Tag{
			{
				Key: awssdk.String("kubeflow/cluster-name"),
				// roleName is like kf-admin-clusterName
				Value: awssdk.String(roleName[strings.LastIndex("roleName", "-")+1:]),
			},
		},
	}

	_, err := aws.iamClient.CreateRole(roleInput)
	if err != nil {
		return err
	}
	return nil
}

// deleteWebIdentityRole delete a role we created for Web Identity
func (aws *Aws) deleteWebIdentityRole(roleArn string) error {
	panic("To be implemented")
}

// createOrUpdateK8sServiceAccount creates or updates k8s service account with annotation
func (aws *Aws) createOrUpdateK8sServiceAccount(k8sClientset *clientset.Clientset, namespace, saName, accountId, iamRoleName string) error {
	iamRoleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, iamRoleName)
	existingSA, err := k8sClientset.CoreV1().ServiceAccounts(namespace).Get(saName, metav1.GetOptions{})
	if err == nil {
		log.Infof("Service account %v already exists", saName)
		if existingSA.Annotations == nil {
			existingSA.Annotations = map[string]string{}
		}

		existingSA.Annotations["eks.amazonaws.com/role-arn"] = iamRoleArn
		_, err = k8sClientset.CoreV1().ServiceAccounts(namespace).Update(existingSA)
		if err != nil {
			return &kfapis.KfError{
				Code:    int(kfapis.INTERNAL_ERROR),
				Message: err.Error(),
			}
		}
		return nil
	}

	log.Infof("Can not find service account, Creating service account %s/%s", namespace, saName)
	_, err = k8sClientset.CoreV1().ServiceAccounts(namespace).Create(
		&v1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: namespace,
				Annotations: map[string]string{
					"eks.amazonaws.com/role-arn": iamRoleArn,
				},
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

// updateRoleTrustIdentity add namespace/service to IAM Role trust entity
func (aws *Aws) updateRoleTrustIdentity(namespace string, ksa string, roleName string) error {
	//roleInput := &iam.GetRoleInput{
	//	RoleName:                 awssdk.String(roleName),
	//}
	//
	//output, err := aws.iamClient.GetRole(roleInput)
	//if err != nil {
	//	return err
	//}
	//
	//assumeRolePolicyDocument := awssdk.StringValue(output.Role.AssumeRolePolicyDocument)
	//
	//
	//conditionStrMap := gjson.Get(assumeRolePolicyDocument, "Statement.0.Condition.StringEquals").Map()
	//sub := fmt.Sprintf("system:serviceaccount:%s:%s", namespace, ksa)
	//
	//
	//if strings.Contains(condition, sub) {
	//	log.Warnf("%s has been added into policy document %s", sub, condition)
	//	return nil
	//}
	//
	//condition += sub + ","
	//doc["Statement"][0]["Condition"]["StringEquals"] = condition
	//
	//input := &iam.UpdateAssumeRolePolicyInput{
	//	RoleName: awssdk.String(roleName),
	//	PolicyDocument: assumeRolePolicyDocument,
	//}
	//if _, err := aws.iamClient.UpdateAssumeRolePolicy(input); err != nil {
	//	return err
	//}

	return nil
}

