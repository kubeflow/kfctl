package config_map_merge

import (
	"github.com/kubeflow/kfctl/kustomize-fns/utils"
	"os"
	"path"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"testing"
)

func Test_merge_config_map(t *testing.T) {

	type testCase struct {
		InputFile    string
		ExpectedFile string
	}

	cwd, err := os.Getwd()

	if err != nil {
		t.Fatalf("Error getting current directory; error %v", err)
	}

	testDir := path.Join(cwd, "test_data")

	cases := []testCase{
		{
			InputFile:    path.Join(testDir, "kustomization.yaml"),
			ExpectedFile: path.Join(testDir, "expected.yaml"),
		},
	}

	f := &ConfigMapMergeFunction{
		Spec: Spec{
			ConfigMaps: []*types.GeneratorArgs{
				{
					Name:     "dex-parameters",
					Behavior: "merge",
					KvPairSources: types.KvPairSources{
						LiteralSources: []string{"github_hostname=github.ibm.com", "github_org_name=kubeflow"},
					},
				},
			},
		},
	}

	for _, c := range cases {
		nodes, err := utils.ReadYaml(c.InputFile)

		if err != nil {
			t.Errorf("Error reading YAML: %v", err)
		}

		if len(nodes) != 1 {
			t.Errorf("Expected 1 node in file %v", c.InputFile)
		}
		node := nodes[0]

		err = f.mergeConfigMap(node)
		if err != nil {
			t.Errorf("mergeConfigMap failed; error %v", err)
			continue
		}

		b, err := utils.WriteYaml([]*yaml.RNode{node})

		if err != nil {
			t.Errorf("Error writing yaml; error %v", err)
			continue
		}

		actual := string(b)

		// read the expected yaml and then rewrites using kio.ByteWriter.
		// We do this because ByteWriter makes some formatting decisions and we
		// we want to apply the same formatting to the expected values

		eNode, err := utils.ReadYaml(c.ExpectedFile)

		if err != nil {
			t.Errorf("Could not read expected file %v; error %v", c.ExpectedFile, err)
		}

		eBytes, err := utils.WriteYaml(eNode)

		if err != nil {
			t.Errorf("Could not format expected file %v; error %v", c.ExpectedFile, err)
		}

		expected := string(eBytes)

		if actual != expected {
			utils.PrintDiff(actual, expected)
		}
	}
}
