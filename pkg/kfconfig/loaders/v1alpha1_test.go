package loaders

import (
	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	kfdeftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfdef/v1alpha1"
	kfgcpplugin "github.com/kubeflow/kfctl/v3/pkg/apis/apps/plugins/gcp/v1alpha1"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
	kfutils "github.com/kubeflow/kfctl/v3/pkg/utils"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"testing"
)

func TestV1alpha1_ConvertToKfConfigs(t *testing.T) {
	type testCase struct {
		Input    string
		Expected string
	}

	cases := []testCase{
		testCase{
			Input:    "v1alpha1.yaml",
			Expected: "kfconfig_v1alpha1.yaml",
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

		v1alpha1 := V1alpha1{}
		config, err := v1alpha1.LoadKfConfig(obj)
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
			t.Errorf("Loaded KfConfig doesn't match: %v", cmp.Diff(pGot, pWant))
		}
	}
}

func TestV1alpha1_ConvertToKfDef(t *testing.T) {
	type testCase struct {
		Input    string
		Expected string
	}

	cases := []testCase{
		testCase{
			Input:    "kfconfig_v1alpha1.yaml",
			Expected: "v1alpha1.yaml",
		},
	}

	for _, c := range cases {
		wd, _ := os.Getwd()
		fPath := path.Join(wd, "testdata", c.Input)

		buf, bufErr := ioutil.ReadFile(fPath)
		if bufErr != nil {
			t.Fatalf("Error reading file %v; error %v", fPath, bufErr)
		}
		config := &kfconfig.KfConfig{}
		err := yaml.Unmarshal(buf, config)
		if err != nil {
			t.Fatalf("Error when unmarshaling KfConfig: %v", err)
		}

		v1alpha1 := V1alpha1{}
		got := &kfdeftypes.KfDef{}
		if err = v1alpha1.LoadKfDef(*config, got); err != nil {
			t.Fatalf("Error converting to KfDef: %v", err)
		}
		gcpSpec := &kfgcpplugin.GcpPluginSpec{}
		err = got.GetPluginSpec(kftypes.GCP, gcpSpec)
		if err != nil {
			t.Fatalf("Error when getting spec: %v", err)
		}
		newSpec := &kfgcpplugin.GcpPluginSpec{}
		newSpec.CreatePipelinePersistentStorage = gcpSpec.CreatePipelinePersistentStorage
		newSpec.EnableWorkloadIdentity = gcpSpec.EnableWorkloadIdentity
		newSpec.DeploymentManagerConfig = gcpSpec.DeploymentManagerConfig
		err = got.SetPluginSpec(kftypes.GCP, newSpec)
		if err != nil {
			t.Fatalf("Error when writing back GcpPluginSpec: %v", err)
		}

		ePath := path.Join(wd, "testdata", c.Expected)
		eBuf, err := ioutil.ReadFile(ePath)
		if err != nil {
			t.Fatalf("Error when reading KfDef: %v", err)
		}
		want := &kfdeftypes.KfDef{}
		err = yaml.Unmarshal(eBuf, want)
		if err != nil {
			t.Fatalf("Error when unmarshaling to KfDef: %v", err)
		}

		if !reflect.DeepEqual(got, want) {
			pGot := kfutils.PrettyPrint(got)
			pWant := kfutils.PrettyPrint(want)
			t.Errorf("Loaded KfConfig doesn't match: %v", cmp.Diff(pGot, pWant))
		}
	}
}
