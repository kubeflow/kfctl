package kustomize

import (
	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sigs.k8s.io/kustomize/v3/pkg/types"
	"strings"
	"testing"

	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
	"github.com/otiai10/copy"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// This test tests that GenerateKustomizationFile will produce correct kustomization.yaml
func TestGenerateKustomizationFile(t *testing.T) {
	type testCase struct {
		kfDef  *kfconfig.KfConfig
		params []kfconfig.NameValue
		// The directory of a (testing) kustomize package
		packageDir string
		overlays   []string
		// Expected kustomization.yaml
		expectedFile string
	}
	testCases := []testCase{
		{
			kfDef: &kfconfig.KfConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kubeflow",
				},
				Spec: kfconfig.KfConfigSpec{},
			},
			overlays: []string{
				"application",
			},
			packageDir:   "testdata/kustomizeExample/pytorch-operator",
			expectedFile: "testdata/kustomizeExample/pytorch-operator/expected/kustomization.yaml",
		},
		{
			kfDef: &kfconfig.KfConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kubeflow",
				},
				Spec: kfconfig.KfConfigSpec{},
			},
			overlays: []string{
				"istio",
				"application",
				"db",
			},
			packageDir:   "testdata/kustomizeExample/metadata",
			expectedFile: "testdata/kustomizeExample/metadata/expected/kustomization.yaml",
		},
	}
	packageName := "dummy"
	for _, c := range testCases {
		testDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Logf("testdir: %v", testDir)
		err = copy.Copy(c.packageDir, path.Join(testDir, packageName))
		if err != nil {
			t.Fatalf("Failed to copy package to temp dir: %v", err)
		}
		err = GenerateKustomizationFile(c.kfDef, testDir, packageName, c.overlays, c.params)
		if err != nil {
			t.Fatalf("Failed to GenerateKustomizationFile: %v", err)
		}
		data, err := ioutil.ReadFile(path.Join(testDir, packageName, "kustomization.yaml"))
		if err != nil {
			t.Fatalf("Failed to read kustomization.yaml: %v", err)
		}
		expected, err := ioutil.ReadFile(c.expectedFile)
		if err != nil {
			t.Fatalf("Failed to read expected kustomization.yaml: %v", err)
		}

		if diff := cmp.Diff(expected, data); diff != "" {
			t.Fatalf("kustomization.yaml is different from expected. (-want, +got):\n%s", diff)
		}
	}
}

// TestGenerateYamlWithOperatorAnnotation
func TestGenerateYamlWithOperatorAnnotation(t *testing.T) {
	type testCase struct {
		appDir   string
		expected string
	}
	testCases := []testCase{
		{
			appDir:   "testdata/operator",
			expected: "testdata/operator/expected/service.yaml",
		},
	}
	instance := &unstructured.Unstructured{}
	instance.SetAPIVersion("kfdef.apps.kubeflow.org/v1")
	instance.SetKind("KfDef")
	instance.SetName("operator")
	instance.SetNamespace("kubeflow")

	for _, c := range testCases {
		resMap, err := EvaluateKustomizeManifest(c.appDir)
		if err != nil {
			t.Fatalf("Failed to evaluate manifest. Error: %v.", err)
		}
		actual, err := GenerateYamlWithOperatorAnnotation(resMap, instance)
		if err != nil {
			t.Fatalf("Failed to add owner reference. Error: %v.", err)
		}
		expected, err := ioutil.ReadFile(c.expected)
		if err != nil {
			t.Fatalf("Failed to read expected file. Error: %v", err)
		}
		if diff := cmp.Diff(expected, actual); diff != "" {
			t.Fatalf("Set operator annotation is different from expected. (-want, +got):\n%s", diff)
		}
	}
}

func TestCreateStackAppKustomization(t *testing.T) {
	type testCase struct {
		Name     string
		Input    *types.Kustomization
		BasePath string
		Expected *types.Kustomization
	}

	testCases := []testCase{
		{
			Name:     "no-kustomization",
			Input:    nil,
			BasePath: "../../.cache/stacks/gcp",
			Expected: &types.Kustomization{
				TypeMeta: types.TypeMeta{
					APIVersion: "kustomize.config.k8s.io/v1beta1",
					Kind:       "Kustomization",
				},
				Resources: []string{
					"../../.cache/stacks/gcp",
				},
			},
		},
		{
			Name: "merge-kustomization",
			Input: &types.Kustomization{
				PatchesStrategicMerge: []types.PatchStrategicMerge{
					types.PatchStrategicMerge("some-patch.yaml"),
				},
			},
			BasePath: "../../.cache/stacks/gcp",
			Expected: &types.Kustomization{
				TypeMeta: types.TypeMeta{
					APIVersion: "kustomize.config.k8s.io/v1beta1",
					Kind:       "Kustomization",
				},
				Resources: []string{
					"../../.cache/stacks/gcp",
				},
				PatchesStrategicMerge: []types.PatchStrategicMerge{
					types.PatchStrategicMerge("some-patch.yaml"),
				},
			},
		},
	}

	for _, c := range testCases {
		testDir, err := ioutil.TempDir("", "testCreateStackAppKustomization-"+c.Name+"-")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Logf("testdir: %v", testDir)

		if c.Input != nil {
			contents, err := yaml.Marshal(c.Input)
			if err != nil {
				t.Fatalf("Error marshalling input kustomization: error; %v", err)
			}

			if err := ioutil.WriteFile(filepath.Join(testDir, "kustomization.yaml"), contents, os.ModePerm); err != nil {
				t.Fatalf("Error writing kustomization: error; %v", err)
			}
		}

		kustomizationFile, err := createStackAppKustomization(testDir, c.BasePath)

		if err != nil {
			t.Fatalf("Failed to create kustomization.yaml for Kubeflow apps stack: %v", err)
		}

		data, err := ioutil.ReadFile(kustomizationFile)
		if err != nil {
			t.Fatalf("Case %v: Failed to read %v: %v", c.Name, kustomizationFile, err)
		}

		expected, err := yaml.Marshal(c.Expected)
		if err != nil {
			t.Fatalf("Failed to marshal expected value: %v", err)
		}

		expectedStr := strings.TrimSpace(string(expected))
		dataStr := strings.TrimSpace(string(data))

		if diff := cmp.Diff(expectedStr, dataStr); diff != "" {
			t.Fatalf("kustomization.yaml is different from expected. (-want, +got):\n%s", diff)
		}
	}
}
