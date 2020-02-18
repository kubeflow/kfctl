package aws

import (
	"fmt"
	"github.com/kubeflow/kfctl/v3/pkg/utils"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"testing"
)

func TestAws(t *testing.T) {
	fmt.Println(utils.GetEksctlVersion())
}

func TestListFiles(t *testing.T) {
	files, err := ioutil.ReadDir("/Users/shjiaxin/go-workspace/src/github.com/kubeflow/manifests/aws/infra_configs")
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range files {
		fmt.Println(f.Name())
	}
}
