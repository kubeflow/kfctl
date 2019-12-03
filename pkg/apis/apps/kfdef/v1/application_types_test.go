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

package v1

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

type FakePluginSpec struct {
	Param     string `json:"param,omitempty"`
	BoolParam bool   `json:"boolParam,omitempty"`
}

func TestKfDef_GetPluginSpec(t *testing.T) {
	// Test that we can properly parse the gcp structs.
	type testCase struct {
		Filename   string
		PluginKind string
		Expected   *FakePluginSpec
	}

	cases := []testCase{
		{
			Filename:   "kfctl_plugin_test.yaml",
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
		d := &KfDef{}
		err := yaml.Unmarshal(buf, d)
		if err != nil {
			t.Fatalf("Could not parse as KfDef error %v", err)
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

func TestKfDef_SetPluginSpec(t *testing.T) {
	// Test that we can properly parse the gcp structs.
	type testCase struct {
		PluginKind string
		Expected   *FakePluginSpec
	}

	cases := []testCase{
		{
			PluginKind: "fakeplugin",
			Expected: &FakePluginSpec{
				Param:     "oldparam",
				BoolParam: true,
			},
		},
		// Override the existing plugin
		{
			PluginKind: "fakeplugin",
			Expected: &FakePluginSpec{
				Param:     "newparam",
				BoolParam: true,
			},
		},
		// Add a new plugin
		{
			PluginKind: "fakeplugin",
			Expected: &FakePluginSpec{
				Param:     "newparam",
				BoolParam: true,
			},
		},
	}

	d := &KfDef{}

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

func TestKfDef_GetSecret(t *testing.T) {
	d := &KfDef{
		Spec: KfDefSpec{
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

func TestKfDef_SetSecret(t *testing.T) {
	type testCase struct {
		Input    KfDef
		Secret   Secret
		Expected KfDef
	}

	cases := []testCase{
		// No Secrets exist
		{
			Input: KfDef{},
			Secret: Secret{
				Name: "s1",
				SecretSource: &SecretSource{
					LiteralSource: &LiteralSource{
						Value: "v1",
					},
				},
			},
			Expected: KfDef{
				Spec: KfDefSpec{
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
			Input: KfDef{
				Spec: KfDefSpec{
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
			Expected: KfDef{
				Spec: KfDefSpec{
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
		i := &KfDef{}
		*i = c.Input
		i.SetSecret(c.Secret)

		if !reflect.DeepEqual(*i, c.Expected) {
			pGot, _ := Pformat(i)
			pWant, _ := Pformat(c.Expected)
			t.Errorf("Error setting secret %v; got;\n%v\nwant;\n%v", c.Secret.Name, pGot, pWant)
		}
	}
}

func Test_DeleteApplication(t *testing.T) {
	type testCase struct {
		Filename      string
		DeleteAppName string
		Expected      []Application
	}

	cases := []testCase{
		{
			Filename:      "kfctl_plugin_test.yaml",
			DeleteAppName: "delete",
			Expected: []Application{
				{
					Name: "keep",
				},
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
		d := &KfDef{}
		err := yaml.Unmarshal(buf, d)
		if err != nil {
			t.Fatalf("Could not parse as KfDef error %v", err)
		}

		d.DeleteApplication(c.DeleteAppName)
		if !reflect.DeepEqual(d.Spec.Applications, c.Expected) {
			pGot, _ := Pformat(d.Spec.Applications)
			pWant, _ := Pformat(c.Expected)
			t.Errorf("Error deleting applicaitons got;\n%v\nwant;\n%v", pGot, pWant)
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
