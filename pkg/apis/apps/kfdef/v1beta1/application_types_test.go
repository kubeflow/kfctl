package v1beta1

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
		PluginName string
		Expected   *FakePluginSpec
	}

	cases := []testCase{
		{
			Filename:   "kfctl_plugin_test.yaml",
			PluginName: "fakeplugin",
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
		err = d.GetPluginSpec(c.PluginName, actual)

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
