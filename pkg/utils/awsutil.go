/*
Copyright The Kubeflow Authors.

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

package utils

import (
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"os/exec"
	"regexp"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	log "github.com/sirupsen/logrus"
)

// CheckAwsStsCallerIdentity runs GetCallIdentity to make sure aws credentials is configured correctly
func CheckAwsStsCallerIdentity(sess *session.Session) error {
	svc := sts.New(sess)
	input := &sts.GetCallerIdentityInput{}

	result, err := svc.GetCallerIdentity(input)
	if err != nil {
		log.Warnf("AWS Credentials seems not correct %v", err.Error())
		return err
	}

	log.Infof("Caller ARN Info: %s", result)
	return nil
}

// CheckAwsAccountId runs GetCallIdentity to retrieve account information
func CheckAwsAccountId(sess *session.Session) (string, error) {
	svc := sts.New(sess)
	input := &sts.GetCallerIdentityInput{}

	output, err := svc.GetCallerIdentity(input)
	if err != nil {
		log.Warnf("AWS Credentials seems not correct %v", err.Error())
		return "", err
	}

	return awssdk.StringValue(output.Account), nil
}

// CheckCommandExist check if a command can be found in PATH.
func CheckCommandExist(commandName string) error {
	_, err := exec.LookPath(commandName)
	if err != nil {
		return err
	}

	return nil
}

// GetEksctlVersion return eksctl version on user's environment
func GetEksctlVersion() (string, error) {
	log.Infof("Running `eksctl version` ...")
	output, err := exec.Command("eksctl", "version").Output()

	if err != nil {
		log.Errorf("Failed to run `eksctl version` command %v", err)
		return "", err
	}

	// [ℹ]  version.Info{BuiltAt:"", GitCommit:"", GitTag:"0.1.32"}
	r := regexp.MustCompile("[0-9]+.[0-9]+.[0-9]+")
	matchGroups := r.FindStringSubmatch(string(output))

	if len(matchGroups) == 0 {
		return "", fmt.Errorf("can not find eksctl version from %v", string(output))
	}

	version := matchGroups[0]
	log.Infof("eksctl version: %s", version)
	return version, nil
}
