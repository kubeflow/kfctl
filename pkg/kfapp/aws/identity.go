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

const (
	AWS_DEFAULT_AUDIENCE               = "sts.amazonaws.com"
	AWS_TRUST_IDENTITY_SUBJECT         = "system:serviceaccount:%s:%s"
	AWS_SERVICE_ACCOUNT_ANNOTATION_KEY = "eks.amazonaws.com/role-arn"
	AWS_IAM_ROLE_TAG_KEY               = "kubeflow/cluster-name"
	AWS_IAM_ROLE_ARN                   = "arn:aws:iam::%s:role/%s"
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

// createIdentityProvider create an OpenIDConnectProvider, it's one to one mapping to EKS cluster.
func (aws *Aws) createIdentityProvider(issuerUrl string) (string, error) {
	issuerCAThumbprint, err := getIssueCAThumbprint(issuerUrl)

	oidcProviderInput := &iam.CreateOpenIDConnectProviderInput{
		ClientIDList:   []*string{awssdk.String(AWS_DEFAULT_AUDIENCE)},
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

// DeleteIdentityProvider will delete the identity provider, it may return an error the API call fails
func (aws *Aws) DeleteIdentityProvider(providerArn string) error {
	input := &iam.DeleteOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: awssdk.String(providerArn),
	}
	if _, err := aws.iamClient.DeleteOpenIDConnectProvider(input); err != nil {
		return errors.Wrap(err, "deleting oidc provider")
	}
	return nil
}

// createOrUpdateWebIdentityRole creates an IAM role with trusted entity Web Identity if role doesn't exist
func (aws *Aws) createOrUpdateWebIdentityRole(oidcProviderArn, issuerUrl, roleName, serviceAccountNamespace, serviceAccountName string) error {
	input := &iam.GetRoleInput{
		RoleName: awssdk.String(roleName),
	}

	// Don't need to update role, return immediately.
	if _, err := aws.iamClient.GetRole(input); err != nil {
		// check non exist or other failures.
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case iam.ErrCodeNoSuchEntityException:
				log.Infof("Role %v doesn't exist, creating for KSA %s/%s", roleName, serviceAccountNamespace, serviceAccountName)
			default:
				return err
			}
		} else {
			return err
		}
	} else {
		log.Infof("Role %v exists, skip creating role", roleName)
		return nil
	}

	// Create role
	statement := MakeAssumeRoleWithWebIdentityPolicyDocument(oidcProviderArn, MapOfInterfaces{
		"StringEquals": map[string][]string{
			issuerUrl + ":aud": []string{AWS_DEFAULT_AUDIENCE},
			issuerUrl + ":sub": []string{fmt.Sprintf(AWS_TRUST_IDENTITY_SUBJECT, serviceAccountNamespace, serviceAccountName)},
		},
	})

	assumeRolePolicyDocument := MakePolicyDocument(statement)
	document, err := json.Marshal(assumeRolePolicyDocument)
	if err != nil {
		return errors.Errorf("%v can not be marshal to bytes", document)
	}

	roleInput := &iam.CreateRoleInput{
		RoleName:                 awssdk.String(roleName),
		AssumeRolePolicyDocument: awssdk.String(string(document)),
		Tags: []*iam.Tag{
			{
				Key: awssdk.String(AWS_IAM_ROLE_TAG_KEY),
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

// createOrUpdateK8sServiceAccount creates or updates k8s service account with annotation
func (aws *Aws) createOrUpdateK8sServiceAccount(k8sClientset *clientset.Clientset, serviceAccountNamespace, serviceAccountName, iamRoleArn string) error {
	existingSA, err := k8sClientset.CoreV1().ServiceAccounts(serviceAccountNamespace).Get(serviceAccountName, metav1.GetOptions{})
	if err == nil {
		log.Infof("Service account %v already exists", serviceAccountName)
		if existingSA.Annotations == nil {
			existingSA.Annotations = map[string]string{}
		}

		existingSA.Annotations[AWS_SERVICE_ACCOUNT_ANNOTATION_KEY] = iamRoleArn
		_, err = k8sClientset.CoreV1().ServiceAccounts(serviceAccountNamespace).Update(existingSA)
		if err != nil {
			return &kfapis.KfError{
				Code:    int(kfapis.INTERNAL_ERROR),
				Message: err.Error(),
			}
		}
		return nil
	}

	log.Infof("Can not find existing service account, creating %s/%s", serviceAccountNamespace, serviceAccountName)
	_, err = k8sClientset.CoreV1().ServiceAccounts(serviceAccountNamespace).Create(
		&v1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceAccountName,
				Namespace: serviceAccountNamespace,
				Annotations: map[string]string{
					AWS_SERVICE_ACCOUNT_ANNOTATION_KEY: iamRoleArn,
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

// updateRoleTrustIdentity add namespace/serviceAccount to IAM Role trust entity
func (aws *Aws) updateRoleTrustIdentity(roleName, serviceAccountNamespace, serviceAccountName string) error {
	roleInput := &iam.GetRoleInput{
		RoleName: awssdk.String(roleName),
	}

	output, err := aws.iamClient.GetRole(roleInput)
	if err != nil {
		return err
	}

	// Seems AssumeRolePolicyDocument is URL encoded, decode string to get string policy document
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

// getUpdatedAssumeRolePolicy creates a new policy document with a new ns/sa record
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

	key := fmt.Sprintf("%s:sub", issuerUrlWithProtocol)
	trustIdentity := fmt.Sprintf(AWS_TRUST_IDENTITY_SUBJECT, serviceAccountNamespace, serviceAccountName)

	// We assume we only operator on first statement, don't add/remove new statement
	statement := statements[0]
	statementInBytes, err = json.Marshal(statement)
	identities := gjson.Get(string(statementInBytes), "Condition.StringEquals").Map()

	var originalIdentities []string
	val, ok := identities[key]
	if ok {
		for _, identity := range val.Array() {
			// avoid adding duplicate record
			if identity.Str == trustIdentity {
				return policyDocument, nil
			}
			originalIdentities = append(originalIdentities, identity.Str)
		}
	}
	originalIdentities = append(originalIdentities, trustIdentity)

	document := MakeAssumeRoleWithWebIdentityPolicyDocument(oidcRoleArn, MapOfInterfaces{
		"StringEquals": map[string][]string{
			issuerUrlWithProtocol + ":aud": []string{AWS_DEFAULT_AUDIENCE},
			issuerUrlWithProtocol + ":sub": originalIdentities,
		},
	})
	newAssumeRolePolicyDocument := MakePolicyDocument(document)
	newPolicyDoc, err := json.Marshal(newAssumeRolePolicyDocument)
	if err != nil {
		return "", err
	}
	return string(newPolicyDoc), nil
}

// getIssueCAThumbprint will generate CAThumbprint from a given issuerURL, this is used to create an oidc provider
func getIssueCAThumbprint(issuerURL string) (string, error) {
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

// deleteIAMRole delete an IAM role
func (aws *Aws) deleteIAMRole(roleName string) error {
	input := &iam.DeleteRoleInput{
		RoleName: awssdk.String(roleName),
	}

	if _, err := aws.iamClient.DeleteRole(input); err != nil {
		return err
	}

	return nil
}

// getIAMRoleNameFromIAMRoleArn converts roleArn to roleName
func getIAMRoleNameFromIAMRoleArn(arn string) string {
	return arn[strings.LastIndex(arn, "/")+1:]
}

// getIssuerUrlFromRoleArn parse issuerUrl from Arn: arn:aws:iam::${accountId}:oidc-provider/${issuerUrl}
func getIssuerUrlFromRoleArn(arn string) string {
	return arn[strings.Index(arn, "/")+1:]
}

// MakeAssumeRoleWithWebIdentityPolicyDocument constructs a trust policy statement for given web identity provider with given conditions
func MakeAssumeRoleWithWebIdentityPolicyDocument(providerARN string, condition MapOfInterfaces) MapOfInterfaces {
	return MapOfInterfaces{
		"Effect": "Allow",
		"Action": "sts:AssumeRoleWithWebIdentity",
		"Principal": map[string]string{
			"Federated": providerARN,
		},
		"Condition": condition,
	}
}

// MakePolicyDocument constructs a policy document with given statements
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
