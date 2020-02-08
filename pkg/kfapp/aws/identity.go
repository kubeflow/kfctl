package aws

import (
	"crypto/sha1"
	"crypto/tls"
	json "encoding/json"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	kfapis "github.com/kubeflow/kfctl/v3/pkg/apis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"net/http"
	"net/url"
	"strings"
)

const AWS_DEFAULT_AUDIENCE = "sts.amazonaws.com"
const AWS_SUBJECT = "system:serviceaccount:%s:%s"

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

	log.Infof("Creating OpenIDConnectProvider %v", *output.OpenIDConnectProviderArn)
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
	statement := MakeAssumeRoleWithWebIdentityPolicyDocument(oidcProviderArn, MapOfInterfaces{
		"StringEquals": map[string]string{
			issuerUrlWithProtocol + ":sub": fmt.Sprintf(AWS_SUBJECT, namespace, ksa),
			//issuerUrlWithProtocol + ":aud": AWS_DEFAULT_AUDIENCE,
		},
	})

	assumeRolePolicyDocument := MakePolicyDocument(statement)

	docInBytes, err := json.Marshal(assumeRolePolicyDocument)
	if err != nil {
		return errors.Errorf("%v can not be marshal to bytes", docInBytes)
	}

	roleInput := &iam.CreateRoleInput{
		RoleName:                 awssdk.String(roleName),
		AssumeRolePolicyDocument: awssdk.String(string(docInBytes)),
		Tags: []*iam.Tag{
			{
				Key: awssdk.String("kubeflow/cluster-name"),
				// roleName is like kf-admin-clusterName
				Value: awssdk.String(roleName[strings.LastIndex("roleName", "-")+1:]),
			},
		},
	}

	_, err = aws.iamClient.CreateRole(roleInput)
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
func (aws *Aws) updateRoleTrustIdentity(serviceAccountNamespace string, serviceAccountName string, roleName string) error {
	roleInput := &iam.GetRoleInput{
		RoleName: awssdk.String(roleName),
	}

	output, err := aws.iamClient.GetRole(roleInput)
	if err != nil {
		return err
	}

	// Seems AssumeRolePolicyDocument is URL encoded
	decodeValue, err := url.QueryUnescape(awssdk.StringValue(output.Role.AssumeRolePolicyDocument))
	if err != nil {
		return err
	}

	updatedRolePolicy, err := getUpdatedAssumeRolePolicy(decodeValue, serviceAccountNamespace, serviceAccountName)
	if err != nil {
		return err
	}

	input := &iam.UpdateAssumeRolePolicyInput{
		RoleName:       awssdk.String(roleName),
		PolicyDocument: awssdk.String(updatedRolePolicy),
	}
	if _, err = aws.iamClient.UpdateAssumeRolePolicy(input); err != nil {
		return err
	}

	return nil
}

func getUpdatedAssumeRolePolicy(policyDocument, serviceAccountNamespace, serviceAccountName string) (string, error) {
	var oldDoc MapOfInterfaces
	json.Unmarshal([]byte(policyDocument), &oldDoc)
	var statements []MapOfInterfaces
	statementInBytes, err := json.Marshal(oldDoc["Statement"])
	if err != nil {
		return "", err
	}
	json.Unmarshal(statementInBytes, &statements)

	oidcRoleArn := gjson.Get(policyDocument, "Statement.0.Principal.Federated").String()
	issuerUrlWithProtocol := getIssuerUrlFromRoleArn(oidcRoleArn)

	document := MakeAssumeRoleWithWebIdentityPolicyDocument(oidcRoleArn, MapOfInterfaces{
		"StringEquals": map[string]string{
			issuerUrlWithProtocol + ":sub": fmt.Sprintf(AWS_SUBJECT, serviceAccountNamespace, serviceAccountName),
			//issuerUrlWithProtocol + ":aud": AWS_DEFAULT_AUDIENCE,
		},
	})

	statements = append(statements, document)
	newAssumeRolePolicyDocument := MakePolicyDocument(statements...)
	newPolicyDoc, err := json.Marshal(newAssumeRolePolicyDocument)
	if err != nil {
		return "", err
	}
	return string(newPolicyDoc), nil
}

// getIssuerUrlFromRoleArn parse issuerUrl from Arn: arn:aws:iam::${accountId}:oidc-provider/${issuerUrl}
func getIssuerUrlFromRoleArn(arn string) string {
	return arn[strings.Index(arn, "/")+1:]
}

// MakeAssumeRoleWithWebIdentityPolicyDocument constructs a trust policy for given a web identity provider with given conditions
func MakeAssumeRoleWithWebIdentityPolicyDocument(providerARN string, condition MapOfInterfaces) MapOfInterfaces {
	return MapOfInterfaces{
		"Effect": "Allow",
		"Action": []string{"sts:AssumeRoleWithWebIdentity"},
		"Principal": map[string]string{
			"Federated": providerARN,
		},
		"Condition": condition,
	}
}

// MakePolicyDocument constructs a policy with given statements
func MakePolicyDocument(statements ...MapOfInterfaces) MapOfInterfaces {
	return MapOfInterfaces{
		"Version":   "2012-10-17",
		"Statement": statements,
	}
}

type (
	// MapOfInterfaces is an alias for map[string]interface{}
	MapOfInterfaces = map[string]interface{}
)
