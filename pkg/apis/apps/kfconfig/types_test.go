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
	"github.com/prometheus/common/log"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"testing"
)

func TestSyncCache(t *testing.T) {
	type testCase struct {
		input    *KfConfig
		expected []Cache
	}

	// Verify that we can sync some files.
	testDir, _ := ioutil.TempDir("", "")

	srcDir := path.Join(testDir, "src")
	err := os.Mkdir(srcDir, os.ModePerm)

	if err != nil {
		t.Fatalf("Failed to create directoy; %v", err)
	}

	ioutil.WriteFile(path.Join(srcDir, "file1"), []byte("hello world"), os.ModePerm)

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

		if err != nil {
			t.Fatalf("Could not sync cache; %v", err)
		}

		actual := c.input.Status.Caches[0].LocalPath
		expected := c.expected[0].LocalPath
		if actual != expected {
			t.Fatalf("LocalPath; got %v; want %v", actual, expected)
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
			t.Errorf("Error parsing plugin spec got;\n%v\nwant;\n%v", pGot, pWant)
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
			t.Errorf("Secret %v value is wrong; got %v; want %v", c.SecretName, actual, c.ExpectedValue)
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
