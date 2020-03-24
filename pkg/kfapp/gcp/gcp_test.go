package gcp

import (
	"encoding/json"

	"github.com/gogo/protobuf/proto"
	kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig/gcpplugin"

	"os"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGcp_buildBasicAuthSecret(t *testing.T) {
	type testCase struct {
		Gcp           *Gcp
		GcpPluginSpec *gcpplugin.GcpPluginSpec
		Expected      *v1.Secret
	}

	encodedPassword, err := base64EncryptPassword("somepassword")

	if err != nil {
		t.Fatalf("Could not encode password; %v", err)
	}

	cases := []testCase{
		{
			Gcp: &Gcp{
				kfDef: &kfconfig.KfConfig{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gcpnamespace",
					},
					Spec: kfconfig.KfConfigSpec{
						Plugins: []kfconfig.Plugin{
							{
								Name: "gcp",
							},
						},
						Secrets: []kfconfig.Secret{
							{
								Name: "passwordSecret",
								SecretSource: &kfconfig.SecretSource{
									LiteralSource: &kfconfig.LiteralSource{
										Value: "somepassword",
									},
								},
							},
						},
					},
				},
			},
			GcpPluginSpec: &gcpplugin.GcpPluginSpec{
				Auth: &gcpplugin.Auth{
					BasicAuth: &gcpplugin.BasicAuth{
						Username: "kfuser",
						Password: &kfconfig.SecretRef{
							Name: "passwordSecret",
						},
					},
				},
			},
			Expected: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubeflow-login",
					Namespace: "gcpnamespace",
				},
				Data: map[string][]byte{
					"passwordhash": []byte(encodedPassword),
					"username":     []byte("kfuser"),
				},
			},
		},
	}

	for _, c := range cases {

		err := c.Gcp.kfDef.SetPluginSpec("KfGcpPlugin", c.GcpPluginSpec)

		if err != nil {
			t.Fatalf("Could not set pluginspec")
		}
		actual, err := c.Gcp.buildBasicAuthSecret()

		if err != nil {
			t.Fatalf("Could not get build secret; error %v", err)
		}

		if !reflect.DeepEqual(actual.TypeMeta, c.Expected.TypeMeta) {
			pGot, _ := Pformat(actual.TypeMeta)
			pWant, _ := Pformat(c.Expected.TypeMeta)
			t.Errorf("Error building secret TypeMeta got;\n%v\nwant;\n%v", pGot, pWant)
		}

		for _, k := range []string{"username", "passwordHash"} {
			if string(actual.Data[k]) != string(c.Expected.Data[k]) {
				pGot, _ := actual.Data[k]
				pWant, _ := c.Expected.Data[k]
				t.Errorf("Error building secret Key %v got;\n%v\nwant;\n%v", k, pGot, pWant)
			}
		}
	}
}

func TestGcp_setGcpPluginDefaults(t *testing.T) {
	type testCase struct {
		Name            string
		Input           *kfconfig.KfConfig
		InputSpec       *gcpplugin.GcpPluginSpec
		Env             map[string]string
		EmailGetter     func() (string, error)
		ProjectGetter   func() (string, error)
		ZoneGetter      func() (string, error)
		Expected        *gcpplugin.GcpPluginSpec
		ExpectedEmail   string
		ExpectedProject string
		ExpectedZone    string
	}

	cases := []testCase{
		{
			Name: "no-plugin-basic-auth",
			Input: &kfconfig.KfConfig{
				Spec: kfconfig.KfConfigSpec{
					UseBasicAuth: true,
				},
			},
			Env: map[string]string{
				kftypes.KUBEFLOW_USERNAME: "someuser",
				kftypes.KUBEFLOW_PASSWORD: "password",
			},
			Expected: &gcpplugin.GcpPluginSpec{
				CreatePipelinePersistentStorage: proto.Bool(true),
				EnableWorkloadIdentity:          proto.Bool(true),
				Auth: &gcpplugin.Auth{
					BasicAuth: &gcpplugin.BasicAuth{
						Username: "someuser",
						Password: &kfconfig.SecretRef{
							Name: BasicAuthPasswordSecretName,
						},
					},
				},
			},
		},
		{
			Name: "no-plugin-iap",
			Input: &kfconfig.KfConfig{
				Spec: kfconfig.KfConfigSpec{
					UseBasicAuth: false,
				},
			},
			Env: map[string]string{
				CLIENT_ID: "someclient",
			},
			Expected: &gcpplugin.GcpPluginSpec{
				CreatePipelinePersistentStorage: proto.Bool(true),
				EnableWorkloadIdentity:          proto.Bool(true),
				Auth: &gcpplugin.Auth{
					IAP: &gcpplugin.IAP{
						OAuthClientId: "someclient",
						OAuthClientSecret: &kfconfig.SecretRef{
							Name: CLIENT_SECRET,
						},
					},
				},
			},
		},
		{
			Name: "set-email",
			Input: &kfconfig.KfConfig{
				Spec: kfconfig.KfConfigSpec{
					UseBasicAuth: false,
				},
			},
			Env: map[string]string{
				CLIENT_ID: "someclient",
			},
			Expected: &gcpplugin.GcpPluginSpec{
				CreatePipelinePersistentStorage: proto.Bool(true),
				EnableWorkloadIdentity:          proto.Bool(true),
				Auth: &gcpplugin.Auth{
					IAP: &gcpplugin.IAP{
						OAuthClientId: "someclient",
						OAuthClientSecret: &kfconfig.SecretRef{
							Name: CLIENT_SECRET,
						},
					},
				},
				Project: "myproject",
				Email:   "myemail",
				Zone:    "us-east1-b",
			},
			EmailGetter: func() (string, error) {
				return "myemail", nil
			},
			ProjectGetter: func() (string, error) {
				return "\nmyproject ", nil
			},
			ZoneGetter: func() (string, error) {
				return "\nus-east1-b\n", nil
			},
			ExpectedEmail:   "myemail",
			ExpectedProject: "myproject",
			ExpectedZone:    "us-east1-b",
		},
		{
			// Make sure emails get trimmed.
			Name: "trim-email",
			Input: &kfconfig.KfConfig{
				Spec: kfconfig.KfConfigSpec{
					UseBasicAuth: false,
				},
			},
			Env: map[string]string{
				CLIENT_ID: "someclient",
			},
			Expected: &gcpplugin.GcpPluginSpec{
				CreatePipelinePersistentStorage: proto.Bool(true),
				EnableWorkloadIdentity:          proto.Bool(true),
				Auth: &gcpplugin.Auth{
					IAP: &gcpplugin.IAP{
						OAuthClientId: "someclient",
						OAuthClientSecret: &kfconfig.SecretRef{
							Name: CLIENT_SECRET,
						},
					},
				},
				Email: "myemail",
			},
			EmailGetter: func() (string, error) {
				return "\nmyemail\n", nil
			},
			ExpectedEmail: "myemail",
		},
		// Verify that we don't override createPipelinePersistentStorage.
		{
			// Make sure emails get trimmed.
			Name: "no-override",
			Input: &kfconfig.KfConfig{
				Spec: kfconfig.KfConfigSpec{
					UseBasicAuth: false,
				},
			},
			InputSpec: &gcpplugin.GcpPluginSpec{
				CreatePipelinePersistentStorage: proto.Bool(false),
			},
			Env: map[string]string{
				CLIENT_ID: "someclient",
			},
			Expected: &gcpplugin.GcpPluginSpec{
				CreatePipelinePersistentStorage: proto.Bool(false),
				EnableWorkloadIdentity:          proto.Bool(true),
				Auth: &gcpplugin.Auth{
					IAP: &gcpplugin.IAP{
						OAuthClientId: "someclient",
						OAuthClientSecret: &kfconfig.SecretRef{
							Name: CLIENT_SECRET,
						},
					},
				},
				Email: "myemail",
			},
			EmailGetter: func() (string, error) {
				return "\nmyemail\n", nil
			},
			ExpectedEmail: "myemail",
		},
		{
			Name: "iap-not-overwritten",
			Input: &kfconfig.KfConfig{
				Spec: kfconfig.KfConfigSpec{
					UseBasicAuth: false,
				},
			},
			InputSpec: &gcpplugin.GcpPluginSpec{
				Auth: &gcpplugin.Auth{
					IAP: &gcpplugin.IAP{
						OAuthClientId: "original_client",
						OAuthClientSecret: &kfconfig.SecretRef{
							Name: "original_secret",
						},
					},
				},
			},
			Env: map[string]string{
				CLIENT_ID: "someclient",
			},
			Expected: &gcpplugin.GcpPluginSpec{
				CreatePipelinePersistentStorage: proto.Bool(true),
				EnableWorkloadIdentity:          proto.Bool(true),
				Auth: &gcpplugin.Auth{
					IAP: &gcpplugin.IAP{
						OAuthClientId: "original_client",
						OAuthClientSecret: &kfconfig.SecretRef{
							Name: "original_secret",
						},
					},
				},
			},
		},
		{
			Name: "basic-auth-not-overwritten",
			Input: &kfconfig.KfConfig{
				Spec: kfconfig.KfConfigSpec{
					UseBasicAuth: true,
				},
			},
			InputSpec: &gcpplugin.GcpPluginSpec{
				Auth: &gcpplugin.Auth{
					BasicAuth: &gcpplugin.BasicAuth{
						Username: "original_user",
						Password: &kfconfig.SecretRef{
							Name: "original_secret",
						},
					},
				},
			},
			Env: map[string]string{
				CLIENT_ID: "someclient",
			},
			Expected: &gcpplugin.GcpPluginSpec{
				CreatePipelinePersistentStorage: proto.Bool(true),
				EnableWorkloadIdentity:          proto.Bool(true),
				Auth: &gcpplugin.Auth{
					BasicAuth: &gcpplugin.BasicAuth{
						Username: "original_user",
						Password: &kfconfig.SecretRef{
							Name: "original_secret",
						},
					},
				},
			},
		},
		{
			Name: "dm-configs-not-overwritten",
			Input: &kfconfig.KfConfig{
				Spec: kfconfig.KfConfigSpec{
					UseBasicAuth: true,
				},
			},
			InputSpec: &gcpplugin.GcpPluginSpec{
				Auth: &gcpplugin.Auth{
					BasicAuth: &gcpplugin.BasicAuth{
						Username: "original_user",
						Password: &kfconfig.SecretRef{
							Name: "original_secret",
						},
					},
				},
				DeploymentManagerConfig: &gcpplugin.DeploymentManagerConfig{
					RepoRef: &kfconfig.RepoRef{
						Name: "somerepo",
						Path: "somepath",
					},
				},
			},
			Env: map[string]string{
				CLIENT_ID: "someclient",
			},
			Expected: &gcpplugin.GcpPluginSpec{
				CreatePipelinePersistentStorage: proto.Bool(true),
				EnableWorkloadIdentity:          proto.Bool(true),
				Auth: &gcpplugin.Auth{
					BasicAuth: &gcpplugin.BasicAuth{
						Username: "original_user",
						Password: &kfconfig.SecretRef{
							Name: "original_secret",
						},
					},
				},
				DeploymentManagerConfig: &gcpplugin.DeploymentManagerConfig{
					RepoRef: &kfconfig.RepoRef{
						Name: "somerepo",
						Path: "somepath",
					},
				},
			},
		},
	}

	for index, c := range cases {
		if index > 0 {
			// Unset previous environment variables
			for k, _ := range cases[index-1].Env {
				os.Unsetenv(k)
			}
		}

		for k, v := range c.Env {
			os.Setenv(k, v)
		}

		i := c.Input.DeepCopy()

		if c.InputSpec != nil {
			i.SetPluginSpec(GcpPluginName, c.InputSpec)
		}

		gcp := &Gcp{
			kfDef:            i,
			gcpAccountGetter: c.EmailGetter,
			gcpProjectGetter: c.ProjectGetter,
			gcpZoneGetter:    c.ZoneGetter,
		}

		if err := gcp.setGcpPluginDefaults(); err != nil {
			t.Errorf("Case %v; setGcpPluginDefaults() error %v", c.Name, err)
			continue
		}

		plugin := &gcpplugin.GcpPluginSpec{}
		err := i.GetPluginSpec(GcpPluginName, plugin)

		if err != nil {
			t.Errorf("Case %v; GetPluginSpec() error %v", c.Name, err)
			continue
		}

		if !reflect.DeepEqual(plugin, c.Expected) {
			pGot, _ := Pformat(plugin)
			pWant, _ := Pformat(c.Expected)
			t.Errorf("Case %v; got:\n%v\nwant:\n%v", c.Name, pGot, pWant)
		}

		if c.ExpectedEmail != "" && c.ExpectedEmail != i.Spec.Email {
			t.Errorf("Case %v; email: got %v; want %v", c.Name, i.Spec.Email, c.ExpectedEmail)
		}
		if c.ExpectedProject != "" && c.ExpectedProject != i.Spec.Project {
			t.Errorf("Case %v; project: got %v; want %v", c.Name, i.Spec.Project, c.ExpectedProject)
		}
		if c.ExpectedZone != "" && c.ExpectedZone != i.Spec.Zone {
			t.Errorf("Case %v; zone: got %v; want %v", c.Name, i.Spec.Zone, c.ExpectedZone)
		}

	}
}

func TestGcp_setPodDefault(t *testing.T) {
	group := "kubeflow.org"
	version := "v1alpha1"
	kind := "PodDefault"
	namespace := "foo-bar-baz"
	expected := map[string]interface{}{
		"apiVersion": group + "/" + version,
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":      "add-gcp-secret",
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"add-gcp-secret": "true",
				},
			},
			"desc": "add gcp credential",
			"env": []interface{}{
				map[string]interface{}{
					"name":  "GOOGLE_APPLICATION_CREDENTIALS",
					"value": "/secret/gcp/user-gcp-sa.json",
				},
			},
			"volumeMounts": []interface{}{
				map[string]interface{}{
					"name":      "secret-volume",
					"mountPath": "/secret/gcp",
				},
			},
			"volumes": []interface{}{
				map[string]interface{}{
					"name": "secret-volume",
					"secret": map[string]interface{}{
						"secretName": "user-gcp-sa",
					},
				},
			},
		},
	}

	actual := generatePodDefault(group, version, kind, namespace)
	if !reflect.DeepEqual(actual.UnstructuredContent(), expected) {
		pGot, _ := Pformat(actual.UnstructuredContent())
		pWant, _ := Pformat(expected)
		t.Errorf("PodDefault not matching; got\n%v\nwant\n%v", pGot, pWant)
	}
}

// Pformat returns a pretty format output of any value.
// TODO(jlewi): Use utils.PrettyPrint
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
