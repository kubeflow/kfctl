package remove_namespace

import (
"bytes"
"github.com/kubeflow/kfctl/kustomize-fns/utils"
"github.com/pkg/errors"
"io/ioutil"
"os"
"path"
"sigs.k8s.io/kustomize/kyaml/kio"
"sigs.k8s.io/kustomize/kyaml/yaml"
"testing"
)

func readYaml(path string) ([]*yaml.RNode, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "Error reading path %v", path)
	}

	input := bytes.NewReader(data)
	reader := kio.ByteReader{
		Reader: input,
		// We need to disable adding reader annotations because
		// we want to run some checks about whether annotations are set and
		// adding those annotations interferes with that.
		OmitReaderAnnotations: true,
	}

	nodes, err := reader.Read()

	if err != nil {
		return nil, errors.Wrapf(err, "Error unmarshaling %v", path)
	}

	return nodes, nil
}

func writeYaml(nodes []*yaml.RNode) ([]byte, error) {
	var b bytes.Buffer
	writer := kio.ByteWriter{
		Writer: &b,
	}

	if err := writer.Write(nodes); err != nil {
		return []byte{}, err
	}

	return b.Bytes(), nil
}

func Test_remove_namespace(t *testing.T) {

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
			InputFile:    path.Join(testDir, "in_cluster_resource.yaml"),
			ExpectedFile: path.Join(testDir, "out_cluster_resource.yaml"),
		},
		{
			InputFile:    path.Join(testDir, "ns_resource.yaml"),
			ExpectedFile: path.Join(testDir, "ns_resource.yaml"),
		},
	}

	f := &RemoveNamespaceFunction {
		Spec: Spec{
			ClusterKinds: []*ClusterKind {
				{
					Kind:  "Profile",
					Group: "kubeflow.org",
				},
				{
					Kind:  "ClusterRbacConfig",
					Group: "rbac.istio.io",
				},
			},
		},
	}

	for _, c := range cases {
		nodes, err := readYaml(c.InputFile)

		if err != nil {
			t.Errorf("Error reading YAML: %v", err)
		}

		if len(nodes) != 1 {
			t.Errorf("Expected 1 node in file %v", c.InputFile)
		}
		node := nodes[0]

		err = f.removeNamespace(node)
		if err != nil {
			t.Errorf("prefixImage failed; error %v", err)
			continue
		}

		b, err := writeYaml([]*yaml.RNode{node})

		if err != nil {
			t.Errorf("Error writing yaml; error %v", err)
			continue
		}

		actual := string(b)


		// read the expected yaml and then rewrites using kio.ByteWriter.
		// We do this because ByteWriter makes some formatting decisions and we
		// we want to apply the same formatting to the expected values

		eNode, err := readYaml(c.ExpectedFile)

		if err != nil {
			t.Errorf("Could not read expected file %v; error %v", c.ExpectedFile, err)
		}

		eBytes, err := writeYaml(eNode)

		if err != nil {
			t.Errorf("Could not format expected file %v; error %v", c.ExpectedFile, err)
		}

		expected := string(eBytes)

		if actual != expected {
			utils.PrintDiff(actual, expected)
		}
	}
}
