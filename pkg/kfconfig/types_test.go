// Copyright 2018 The Kubeflow Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kfconfig

import (
	"encoding/json"
	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/prometheus/common/log"
	"io"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sigs.k8s.io/kustomize/v3/pkg/types"
	"testing"
)

func TestSyncCache(t *testing.T) {
	type testCase struct {
		input       *KfConfig
		expected    []Cache
		expectedErr error
	}

	// Verify that we can sync some files.
	testDir, _ := ioutil.TempDir("", "")

	srcDir := path.Join(testDir, "src")
	err := os.Mkdir(srcDir, os.ModePerm)

	if err != nil {
		t.Fatalf("Failed to create directoy; %v", err)
	}

	ioutil.WriteFile(path.Join(srcDir, "file1"), []byte("hello world"), os.ModePerm)

	// Verify that we can unpack a local tarball and use it.
	tarballName := "c0e81bedec9a4df8acf568cc5ccacc4bc05a3b38.tar.gz"
	from, err := os.Open(path.Join("./testdata", tarballName))
	if err != nil {
		t.Fatalf("failed to open tarball file: %v", err)
	}
	tarballPath := path.Join(srcDir, tarballName)
	to, err := os.OpenFile(tarballPath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		t.Fatalf("failed to open new file location fortarball file: %v", err)
	}
	if _, err = io.Copy(to, from); err != nil {
		t.Fatalf("tarball copy is failed: %v", err)
	}

	repoName := "testRepo"

	testCases := []testCase{
		{
			input: &KfConfig{
				Spec: KfConfigSpec{
					AppDir: path.Join(testDir, "app1"),
					Repos: []Repo{{
						Name: repoName,
						URI:  srcDir,
					},
					},
				},
			},
			expected: []Cache{
				{
					Name:      repoName,
					LocalPath: path.Join(testDir, "app1", ".cache", repoName),
				},
			},
			expectedErr: nil,
		},
		{
			input: &KfConfig{
				Spec: KfConfigSpec{
					AppDir: path.Join(srcDir, "app1"),
					Repos: []Repo{{
						Name: repoName,
						URI:  srcDir,
					},
					},
				},
			},
			expected:    nil,
			expectedErr: errors.New("SyncCache: could not sync cache when the cache path " + path.Join(srcDir, "app1", ".cache", repoName) + " is sub directory of manifests " + srcDir),
		},
		{
			input: &KfConfig{
				Spec: KfConfigSpec{
					AppDir: ".",
					Repos: []Repo{{
						Name: repoName,
						URI:  srcDir,
					},
					},
				},
			},
			expected: []Cache{
				{
					Name:      repoName,
					LocalPath: path.Join(".cache", repoName),
				},
			},
			expectedErr: nil,
		},
		{
			input: &KfConfig{
				Spec: KfConfigSpec{
					AppDir: path.Join(testDir, "app2"),
					Repos: []Repo{{
						Name: repoName,
						URI:  "file:" + tarballPath,
					},
					},
				},
			},
			expected: []Cache{
				{
					Name:      repoName,
					LocalPath: path.Join(testDir, "app2", ".cache", repoName, "kubeflow-manifests-c0e81be"),
				},
			},
			expectedErr: nil,
		},
		// The following test cases pull from GitHub. The may be worth commenting
		// out in the unittests and only running manually
		//{
		//	input: &KfConfig{
		//		Spec: KfConfigSpec{
		//			AppDir: path.Join(testDir, "app2"),
		//			Repos: []Repo{{
		//				Name: repoName,
		//				URI:  "https://github.com/kubeflow/manifests/archive/master.tar.gz",
		//			},
		//			},
		//		},
		//	},
		//	expected: []Cache {
		//		{
		//			LocalPath: path.Join(testDir, "app2", ".cache", repoName, "manifests-master"),
		//		},
		//	},
		//},
		//{
		//	input: &KfConfig{
		//		Spec: KfConfigSpec{
		//			AppDir: path.Join(testDir, "app3"),
		//			Repos: []Repo{{
		//				Name: repoName,
		//				URI:  "https://github.com/kubeflow/manifests/tarball/pull/187/head?archive=tar.gz",
		//			},
		//			},
		//		},
		//	},
		//	expected: []Cache {
		//		{
		//			LocalPath: path.Join(testDir, "app3", ".cache", repoName, "kubeflow-manifests-c04764b"),
		//		},
		//	},
		//},
	}

	for _, c := range testCases {
		err = c.input.SyncCache()

		// remove the local path for the test case whose AppDir is "."
		if c.input.Spec.AppDir == "." {
			os.RemoveAll(path.Join(".cache", repoName))
		}

		if err != nil {
			if c.expectedErr == nil || err.Error() != c.expectedErr.Error() {
				t.Fatalf("Could not sync cache; %v", err)
			}
		}

		if c.expected != nil {
			actual := c.input.Status.Caches[0].LocalPath
			expected := c.expected[0].LocalPath
			if actual != expected {
				t.Fatalf("LocalPath; got %v; want %v", actual, expected)
			}
		}
	}

}

type FakePluginSpec struct {
	Param     string `json:"param,omitempty"`
	BoolParam bool   `json:"boolParam,omitempty"`
}

func TestKfConfig_GetPluginSpec(t *testing.T) {
	// Test that we can properly parse the gcp structs.
	type testCase struct {
		Filename   string
		PluginName string
		PluginKind PluginKindType
		Expected   *FakePluginSpec
	}

	cases := []testCase{
		{
			Filename:   "kfctl_plugin_test.yaml",
			PluginName: "fake",
			PluginKind: "fakeplugin",
			Expected: &FakePluginSpec{
				Param:     "someparam",
				BoolParam: true,
			},
		},
	}

	for _, c := range cases {
		wd, _ := os.Getwd()
		fPath := path.Join(wd, "testdata", c.Filename)

		buf, bufErr := ioutil.ReadFile(fPath)
		if bufErr != nil {
			t.Fatalf("Error reading file %v; error %v", fPath, bufErr)
		}

		log.Infof("Want ")
		d := &KfConfig{}
		err := yaml.Unmarshal(buf, d)
		if err != nil {
			t.Fatalf("Could not parse as KfConfig error %v", err)
		}

		actual := &FakePluginSpec{}
		err = d.GetPluginSpec(c.PluginKind, actual)

		if err != nil {
			t.Fatalf("Could not get plugin spec; error %v", err)
		}

		if !reflect.DeepEqual(actual, c.Expected) {
			pGot, _ := Pformat(actual)
			pWant, _ := Pformat(c.Expected)
			t.Errorf("Error parsing plugin spec got;\n%v\nwant;\n%v", pGot, pWant)
		}
	}
}

func TestKfConfig_SetPluginSpec(t *testing.T) {
	// Test that we can properly parse the gcp structs.
	type testCase struct {
		PluginName string
		PluginKind PluginKindType
		Expected   *FakePluginSpec
	}

	cases := []testCase{
		{
			PluginName: "fake",
			PluginKind: "fakeplugin",
			Expected: &FakePluginSpec{
				Param:     "oldparam",
				BoolParam: true,
			},
		},
		// Override the existing plugin
		{
			PluginName: "fake",
			PluginKind: "fakeplugin",
			Expected: &FakePluginSpec{
				Param:     "newparam",
				BoolParam: true,
			},
		},
		// Add a new plugin
		{
			PluginName: "fake",
			PluginKind: "fakeplugin",
			Expected: &FakePluginSpec{
				Param:     "newparam",
				BoolParam: true,
			},
		},
	}

	d := &KfConfig{}

	for _, c := range cases {
		err := d.SetPluginSpec(c.PluginKind, c.Expected)

		if err != nil {
			t.Fatalf("Could not set plugin spec; error %v", err)
		}

		actual := &FakePluginSpec{}
		err = d.GetPluginSpec(c.PluginKind, actual)

		if err != nil {
			t.Fatalf("Could not get plugin spec; error %v", err)
		}

		if !reflect.DeepEqual(actual, c.Expected) {
			pGot, _ := Pformat(actual)
			pWant, _ := Pformat(c.Expected)
			t.Errorf("Error parsing plugin, plugin spec doesn't match %v", cmp.Diff(pGot, pWant))
		}
	}
}

func TestKfConfig_GetSecret(t *testing.T) {
	d := &KfConfig{
		Spec: KfConfigSpec{
			AppDir: "someapp",
			Secrets: []Secret{
				{
					Name: "s1",
					SecretSource: &SecretSource{
						LiteralSource: &LiteralSource{
							Value: "somedata",
						},
					},
				},
				{
					Name: "s2",
					SecretSource: &SecretSource{
						EnvSource: &EnvSource{
							Name: "s2",
						},
					},
				},
			},
		},
	}

	type testCase struct {
		SecretName    string
		ExpectedValue string
	}

	cases := []testCase{
		{
			SecretName:    "s1",
			ExpectedValue: "somedata",
		},
		{
			SecretName:    "s2",
			ExpectedValue: "somesecret",
		},
	}

	os.Setenv("s2", "somesecret")
	for _, c := range cases {
		actual, err := d.GetSecret(c.SecretName)
		if err != nil {
			t.Errorf("Error getting secret %v; error %v", c.SecretName, err)
		}

		if actual != c.ExpectedValue {
			t.Errorf("Secret %v value doesn't match %v", c.SecretName, cmp.Diff(actual, c.ExpectedValue))
		}
	}
}

func TestKfConfig_SetSecret(t *testing.T) {
	type testCase struct {
		Input    KfConfig
		Secret   Secret
		Expected KfConfig
	}

	cases := []testCase{
		// No Secrets exist
		{
			Input: KfConfig{},
			Secret: Secret{
				Name: "s1",
				SecretSource: &SecretSource{
					LiteralSource: &LiteralSource{
						Value: "v1",
					},
				},
			},
			Expected: KfConfig{
				Spec: KfConfigSpec{
					Secrets: []Secret{
						{
							Name: "s1",
							SecretSource: &SecretSource{
								LiteralSource: &LiteralSource{
									Value: "v1",
								},
							},
						},
					},
				},
			},
		},
		// Override a secret
		{
			Input: KfConfig{
				Spec: KfConfigSpec{
					Secrets: []Secret{
						{
							Name: "s1",
							SecretSource: &SecretSource{
								LiteralSource: &LiteralSource{
									Value: "oldvalue",
								},
							},
						},
					},
				},
			},
			Secret: Secret{
				Name: "s1",
				SecretSource: &SecretSource{
					LiteralSource: &LiteralSource{
						Value: "newvalue",
					},
				},
			},
			Expected: KfConfig{
				Spec: KfConfigSpec{
					Secrets: []Secret{
						{
							Name: "s1",
							SecretSource: &SecretSource{
								LiteralSource: &LiteralSource{
									Value: "newvalue",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		i := &KfConfig{}
		*i = c.Input
		i.SetSecret(c.Secret)

		if !reflect.DeepEqual(*i, c.Expected) {
			pGot, _ := Pformat(i)
			pWant, _ := Pformat(c.Expected)
			t.Errorf("Error setting secret %v; got;\n%v\nwant;\n%v", c.Secret.Name, pGot, pWant)
		}
	}
}

func TestKfConfig_addPatchStratgicMerge(t *testing.T) {
	type testCase struct {
		Input     *types.Kustomization
		PatchName string
		Expected  *types.Kustomization
	}

	cases := []testCase{
		// New parameter
		{
			Input:     &types.Kustomization{},
			PatchName: "new-config.yaml",
			Expected: &types.Kustomization{
				PatchesStrategicMerge: []types.PatchStrategicMerge{
					types.PatchStrategicMerge("new-config.yaml"),
				},
			},
		},
		// Existing parameter
		{
			Input: &types.Kustomization{
				PatchesStrategicMerge: []types.PatchStrategicMerge{
					types.PatchStrategicMerge("config1.yaml"),
					types.PatchStrategicMerge("exists-config.yaml"),
					types.PatchStrategicMerge("config2.yaml"),
				},
			},
			PatchName: "exists-config.yaml",
			Expected: &types.Kustomization{
				PatchesStrategicMerge: []types.PatchStrategicMerge{
					types.PatchStrategicMerge("config1.yaml"),
					types.PatchStrategicMerge("exists-config.yaml"),
					types.PatchStrategicMerge("config2.yaml"),
				},
			},
		},
	}

	for _, c := range cases {
		addPatchStratgicMerge(c.Input, c.PatchName)
		if !reflect.DeepEqual(c.Input, c.Expected) {
			pGot, _ := Pformat(c.Input)
			pWant, _ := Pformat(c.Expected)
			t.Errorf("Error adding patch %v; got;\n%v\nwant;\n%v", c.PatchName, pGot, pWant)
		}
	}
}

func Test_setApplicationParameterInConfigMap(t *testing.T) {
	type testCase struct {
		Name                  string
		InputKustomization    *types.Kustomization
		InputPatch            *v1.ConfigMap
		AppName               string
		ParameterName         string
		Value                 string
		ExpectedKustomization *types.Kustomization
		ExpectedPatch         *v1.ConfigMap
	}

	cases := []testCase{
		// New parameter
		{
			Name:               "No-patch",
			InputKustomization: &types.Kustomization{},
			InputPatch:         nil,
			AppName:            "app",
			ParameterName:      "param",
			Value:              "value",
			ExpectedKustomization: &types.Kustomization{
				PatchesStrategicMerge: []types.PatchStrategicMerge{
					types.PatchStrategicMerge("app-config.yaml"),
				},
			},
			ExpectedPatch: &v1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "app-config",
				},
				Data: map[string]string{
					"param": "value",
				},
			},
		},
		// Modify existing parameter
		{
			Name:               "existing-parameter",
			InputKustomization: &types.Kustomization{},
			InputPatch: &v1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "app-config",
				},
				Data: map[string]string{
					"a":     "b",
					"param": "oldvalue",
					"d":     "e",
				},
			},
			AppName:       "app",
			ParameterName: "param",
			Value:         "newvalue",
			ExpectedKustomization: &types.Kustomization{
				PatchesStrategicMerge: []types.PatchStrategicMerge{
					types.PatchStrategicMerge("app-config.yaml"),
				},
			},
			ExpectedPatch: &v1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "app-config",
				},
				Data: map[string]string{
					"a":     "b",
					"param": "newvalue",
					"d":     "e",
				},
			},
		},
	}

	for _, c := range cases {
		// Create a kustomization app based on the inputs.
		testDir, err := ioutil.TempDir("", "testSetApplicationParameter-"+c.Name)

		log.Infof("Test case: %v; directory: %v", c.Name, testDir)
		if err != nil {
			t.Fatalf("Could not creat temporary directory: %v", err)
		}

		type pair struct {
			o    interface{}
			path string
		}

		pairs := []pair{
			{
				o:    c.InputKustomization,
				path: filepath.Join(testDir, "kustomization.yaml"),
			},
		}

		if c.InputPatch != nil {
			pairs = append(pairs, pair{
				o:    c.InputPatch,
				path: filepath.Join(testDir, c.AppName+"-config.yaml"),
			})
		}

		for _, p := range pairs {
			kBytes, err := yaml.Marshal(p.o)

			if err != nil {
				t.Fatalf("Could not marshal yaml error: %v", err)
			}
			if err := ioutil.WriteFile(p.path, kBytes, os.ModePerm); err != nil {
				t.Fatalf("Could not write file: %v; error %v", p.path, err)
			}
		}
		setApplicationParameterInConfigMap(testDir, c.AppName, c.ParameterName, c.Value)

		parse := func(path string, o interface{}) {
			b, err := ioutil.ReadFile(path)

			if err != nil {
				t.Fatalf("Could not read file: %v; error %v", path, err)
			}

			if err := yaml.Unmarshal(b, o); err != nil {
				t.Fatalf("Could not read unmarshal yaml; error: %v", err)
			}
		}

		actualKustomization := &types.Kustomization{}
		actualPatch := &v1.ConfigMap{}

		parse(filepath.Join(testDir, "kustomization.yaml"), actualKustomization)
		parse(filepath.Join(testDir, c.AppName+"-config.yaml"), actualPatch)

		if !reflect.DeepEqual(actualKustomization, c.ExpectedKustomization) {
			pGot, _ := Pformat(actualKustomization)
			pWant, _ := Pformat(c.ExpectedKustomization)
			t.Errorf("Case %v: kustomization.yaml: got;\n%v\nwant;\n%v", c.Name, pGot, pWant)
		}

		if !reflect.DeepEqual(actualPatch, c.ExpectedPatch) {
			pGot, _ := Pformat(actualPatch)
			pWant, _ := Pformat(c.ExpectedPatch)
			t.Errorf("Case %v: kustomization.yaml: got;\n%v\nwant;\n%v", c.Name, pGot, pWant)
		}
	}
}

func TestKfConfig_SetApplicationParameter(t *testing.T) {
	type testCase struct {
		Input     *KfConfig
		AppName   string
		ParamName string
		Value     string
		Expected  *KfConfig
	}

	cases := []testCase{
		// New parameter
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name:            "app1",
							KustomizeConfig: &KustomizeConfig{},
						},
					},
				},
			},
			AppName:   "app1",
			ParamName: "p1",
			Value:     "v1",
			Expected: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Parameters: []NameValue{
									{
										Name:  "p1",
										Value: "v1",
									},
								},
							},
						},
					},
				},
			},
		},
		// Override parameter
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Parameters: []NameValue{
									{
										Name:  "p1",
										Value: "old1",
									},
								},
							},
						},
					},
				},
			},
			AppName:   "app1",
			ParamName: "p1",
			Value:     "v1",
			Expected: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Parameters: []NameValue{
									{
										Name:  "p1",
										Value: "v1",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		c.Input.SetApplicationParameter(c.AppName, c.ParamName, c.Value)
		if !reflect.DeepEqual(c.Input, c.Expected) {
			pGot, _ := Pformat(c.Input)
			pWant, _ := Pformat(c.Expected)
			t.Errorf("Error setting App %v; Param %v; value %v; got;\n%v\nwant;\n%v", c.AppName, c.ParamName, c.Value, pGot, pWant)
		}
	}
}

func TestKfConfig_GetApplicationParameter(t *testing.T) {
	type testCase struct {
		Input     *KfConfig
		AppName   string
		ParamName string
		Expected  string
		HasParam  bool
	}

	cases := []testCase{
		// No parameter
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name:            "app1",
							KustomizeConfig: &KustomizeConfig{},
						},
					},
				},
			},
			AppName:   "app1",
			ParamName: "p1",
			Expected:  "",
			HasParam:  false,
		},
		// Has Parameter
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app2",
							KustomizeConfig: &KustomizeConfig{
								Parameters: []NameValue{
									{
										Name:  "p1",
										Value: "old1",
									},
								},
							},
						},
					},
				},
			},
			AppName:   "app2",
			ParamName: "p1",
			Expected:  "old1",
			HasParam:  true,
		},
	}

	for _, c := range cases {
		v, hasParam := c.Input.GetApplicationParameter(c.AppName, c.ParamName)

		if c.HasParam != hasParam {
			t.Errorf("Error getting App %v; Param %v; hasParam; got; %v; want %v", c.AppName, c.ParamName, hasParam, c.HasParam)
		}

		if c.Expected != v {
			t.Errorf("Error getting App %v; Param %v; got; %v; want %v", c.AppName, c.ParamName, c, c.Expected)
		}
	}
}

func TestKfConfig_DeleteApplication(t *testing.T) {
	type testCase struct {
		Input           *KfConfig
		AppNameToDelete string
		Expected        *KfConfig
	}

	cases := []testCase{
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name:            "app1",
							KustomizeConfig: &KustomizeConfig{},
						},
						{
							Name:            "app2",
							KustomizeConfig: &KustomizeConfig{},
						},
					},
				},
			},
			AppNameToDelete: "app1",
			Expected: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name:            "app2",
							KustomizeConfig: &KustomizeConfig{},
						},
					},
				},
			},
		},
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Parameters: []NameValue{
									{
										Name:  "p1",
										Value: "old1",
									},
								},
							},
						},
					},
				},
			},
			AppNameToDelete: "app1",
			Expected: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{},
				},
			},
		},
	}

	for _, c := range cases {
		c.Input.DeleteApplication(c.AppNameToDelete)
		if !reflect.DeepEqual(c.Input, c.Expected) {
			pGot, _ := Pformat(c.Input)
			pWant, _ := Pformat(c.Expected)
			t.Errorf("Error setting App %v; got;\n%v\nwant;\n%v", c.AppNameToDelete, pGot, pWant)
		}
	}
}

func TestKfConfig_AddApplicationOverlay(t *testing.T) {
	type testCase struct {
		Input        *KfConfig
		AppName      string
		OverlayToAdd string
		Expected     *KfConfig
	}

	cases := []testCase{
		// overlay already exist
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay1",
									"overlay2",
									"overlay3",
								},
							},
						},
					},
				},
			},
			AppName:      "app1",
			OverlayToAdd: "overlay1",
			Expected: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay1",
									"overlay2",
									"overlay3",
								},
							},
						},
					},
				},
			},
		},
		// app not found
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay1",
								},
							},
						},
					},
				},
			},
			AppName:      "app2",
			OverlayToAdd: "overlay1",
			Expected: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay1",
								},
							},
						},
					},
				},
			},
		},
		// normal
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay1",
								},
							},
						},
					},
				},
			},
			AppName:      "app1",
			OverlayToAdd: "overlay2",
			Expected: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay1",
									"overlay2",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		c.Input.AddApplicationOverlay(c.AppName, c.OverlayToAdd)
		if !reflect.DeepEqual(c.Input, c.Expected) {
			pGot, _ := Pformat(c.Input)
			pWant, _ := Pformat(c.Expected)
			t.Errorf("Error setting App %v; got;\n%v\nwant;\n%v", c.OverlayToAdd, pGot, pWant)
		}
	}
}

func TestKfConfig_RemoveApplicationOverlay(t *testing.T) {
	type testCase struct {
		Input           *KfConfig
		AppName         string
		OverlayToRemove string
		Expected        *KfConfig
	}

	cases := []testCase{
		// Normal case - remove overlay on boarder
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay1",
									"overlay2",
									"overlay3",
								},
							},
						},
					},
				},
			},
			AppName:         "app1",
			OverlayToRemove: "overlay1",
			Expected: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay2",
									"overlay3",
								},
							},
						},
					},
				},
			},
		},
		// Normal case - remove overlay in the middle
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay1",
									"overlay2",
									"overlay3",
								},
							},
						},
					},
				},
			},
			AppName:         "app1",
			OverlayToRemove: "overlay2",
			Expected: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay1",
									"overlay3",
								},
							},
						},
					},
				},
			},
		},
		// Can not find app -> remain same
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay1",
									"overlay2",
									"overlay3",
								},
							},
						},
					},
				},
			},
			AppName:         "app2",
			OverlayToRemove: "overlay2",
			Expected: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay1",
									"overlay2",
									"overlay3",
								},
							},
						},
					},
				},
			},
		},
		// Can not find overlay -> remain same
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay1",
									"overlay2",
									"overlay3",
								},
							},
						},
					},
				},
			},
			AppName:         "app1",
			OverlayToRemove: "overlay4",
			Expected: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{
									"overlay1",
									"overlay2",
									"overlay3",
								},
							},
						},
					},
				},
			},
		},
		// no overlay -> remain same
		{
			Input: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{},
							},
						},
					},
				},
			},
			AppName:         "app1",
			OverlayToRemove: "overlay1",
			Expected: &KfConfig{
				Spec: KfConfigSpec{
					Applications: []Application{
						{
							Name: "app1",
							KustomizeConfig: &KustomizeConfig{
								Overlays: []string{},
							},
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		c.Input.RemoveApplicationOverlay(c.AppName, c.OverlayToRemove)
		if !reflect.DeepEqual(c.Input, c.Expected) {
			pGot, _ := Pformat(c.Input)
			pWant, _ := Pformat(c.Expected)
			t.Errorf("Error setting App %v; overlay %v; got;\n%v\nwant;\n%v", c.AppName, c.OverlayToRemove, pGot, pWant)
		}
	}
}

// Pformat returns a pretty format output of any value.
func Pformat(value interface{}) (string, error) {
	if s, ok := value.(string); ok {
		return s, nil
	}
	valueJson, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(valueJson), nil
}
