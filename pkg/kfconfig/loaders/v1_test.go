package loaders

import (
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
	kfutils "github.com/kubeflow/kfctl/v3/pkg/utils"
)

func TestV1_expectedConfig(t *testing.T) {
	type testCase struct {
		Input    string
		Expected string
	}

	cases := []testCase{
		testCase{
			Input:    "v1.yaml",
			Expected: "kfconfig_v1.yaml",
		},
	}

	for _, c := range cases {
		wd, _ := os.Getwd()
		fPath := path.Join(wd, "testdata", c.Input)

		buf, bufErr := ioutil.ReadFile(fPath)
		if bufErr != nil {
			t.Fatalf("Error reading file %v; error %v", fPath, bufErr)
		}
		var obj interface{}
		if err := yaml.Unmarshal(buf, &obj); err != nil {
			t.Fatalf("Error when unmarshaling file %v; error %v", fPath, err)
		}

		v1beta1 := V1beta1{}
		config, err := v1beta1.LoadKfConfig(obj)
		if err != nil {
			t.Fatalf("Error converting to KfConfig: %v", err)
		}

		ePath := path.Join(wd, "testdata", c.Expected)
		eBuf, err := ioutil.ReadFile(ePath)
		if err != nil {
			t.Fatalf("Error when reading KfConfig: %v", err)
		}
		expectedConfig := &kfconfig.KfConfig{}
		err = yaml.Unmarshal(eBuf, expectedConfig)
		if err != nil {
			t.Fatalf("Error when unmarshaling KfConfig: %v", err)
		}

		if !reflect.DeepEqual(config, expectedConfig) {
			pGot := kfutils.PrettyPrint(config)
			pWant := kfutils.PrettyPrint(expectedConfig)
			t.Errorf("Loaded KfConfig doesn't match %v", cmp.Diff(pGot, pWant))
		}
	}

}
