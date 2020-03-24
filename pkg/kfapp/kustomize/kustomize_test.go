package kustomize

import (
	"bytes"
	"io/ioutil"
	"path"
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
		if bytes.Compare(data, expected) != 0 {
			t.Fatalf("kustomization.yaml is different from expected.\nactual:\n--------\n%s\nexpected:\n--------\n%s\n", string(data), string(expected))
		}
	}
}

// TestGenerateYamlWithOwnerReferences
func TestGenerateYamlWithOwnerReferences(t *testing.T) {
	type testCase struct {
		appDir		string
		expected 	string
	}
	testCases := []testCase {
		{
			appDir: "testdata/operator",
			expected: "testdata/operator/expected/service.yaml",
		},
	}
	instance := &unstructured.Unstructured{}
	instance.SetAPIVersion("kfdef.apps.kubeflow.org/v1")
	instance.SetKind("KfDef")
	instance.SetName("operator")
	instance.SetUID("7d7fd317-5bf6-45c1-a543-bff27b7b5807")

	for _, c := range testCases {
		resMap, err := EvaluateKustomizeManifest(c.appDir)
		if err != nil {
			t.Fatalf("Failed to evaluate manifest. Error: %v.", err)
		}
		actual, err := GenerateYamlWithOwnerReferences(resMap, instance)
		if err != nil {
			t.Fatalf("Failed to add owner reference. Error: %v.", err)
		}
		expected, err := ioutil.ReadFile(c.expected)
		if err != nil {
			t.Fatalf("Failed to read expected file. Error: %v", err)
		}
		if bytes.Compare(actual, expected) != 0 {
			t.Fatalf("Set owner reference is different from expected.\nactual:\n--------\n%s\nexpected:\n--------\n%s\n", string(actual), string(expected))
		}
	}
}
