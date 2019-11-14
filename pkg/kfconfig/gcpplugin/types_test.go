package gcpplugin

import (
	kfapis "github.com/kubeflow/kfctl/v3/pkg/apis"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
	kfutils "github.com/kubeflow/kfctl/v3/pkg/utils"
	"testing"
)

func TestGcpPluginSpec_IsValid(t *testing.T) {

	type testCase struct {
		input    *GcpPluginSpec
		expected error
	}

	cases := []testCase{
		{
			// Neither IAP or BasicAuth is set
			input: &GcpPluginSpec{
				Auth: &Auth{},
			},
			expected: &kfapis.KfError{
				Code: int(kfapis.INVALID_ARGUMENT),
			},
		},
		{
			// Both IAP and BasicAuth set
			input: &GcpPluginSpec{
				Auth: &Auth{
					BasicAuth: &BasicAuth{
						Username: "jlewi",
						Password: &kfconfig.SecretRef{
							Name: "somesecret",
						},
					},
					IAP: &IAP{
						OAuthClientId: "jlewi",
						OAuthClientSecret: &kfconfig.SecretRef{
							Name: "somesecret",
						},
					},
				},
			},
			expected: &kfapis.KfError{
				Code: int(kfapis.INVALID_ARGUMENT),
			},
		},

		// Validate basic auth.
		{
			input: &GcpPluginSpec{
				Auth: &Auth{
					BasicAuth: &BasicAuth{
						Username: "jlewi",
						Password: &kfconfig.SecretRef{
							Name: "somesecret",
						},
					},
				},
			},
			expected: nil,
		},
		{
			input: &GcpPluginSpec{
				Auth: &Auth{
					BasicAuth: &BasicAuth{
						Username: "jlewi",
					},
				},
			},
			expected: &kfapis.KfError{
				Code: int(kfapis.INVALID_ARGUMENT),
			},
		},

		{
			input: &GcpPluginSpec{
				Auth: &Auth{
					BasicAuth: &BasicAuth{
						Password: &kfconfig.SecretRef{
							Name: "somesecret",
						},
					},
				},
			},
			expected: &kfapis.KfError{
				Code: int(kfapis.INVALID_ARGUMENT),
			},
		},
		// End Validate basic auth.
		// End Validate IAP.
		{
			input: &GcpPluginSpec{
				Auth: &Auth{
					IAP: &IAP{
						OAuthClientId: "jlewi",
						OAuthClientSecret: &kfconfig.SecretRef{
							Name: "somesecret",
						},
					},
				},
			},
			expected: nil,
		},
		{
			input: &GcpPluginSpec{
				Auth: &Auth{
					IAP: &IAP{
						OAuthClientId: "jlewi",
					},
				},
			},
			expected: &kfapis.KfError{
				Code: int(kfapis.INVALID_ARGUMENT),
			},
		},
		{
			input: &GcpPluginSpec{
				Auth: &Auth{
					IAP: &IAP{
						OAuthClientSecret: &kfconfig.SecretRef{
							Name: "somesecret",
						},
					},
				},
			},
			expected: &kfapis.KfError{
				Code: int(kfapis.INVALID_ARGUMENT),
			},
		},
		{
			input: &GcpPluginSpec{
				Hostname: "this-kfApp-name-is-very-long.endpoints.my-gcp-project-for-kubeflow.cloud.goog",
			},
			expected: &kfapis.KfError{
				Code: int(kfapis.INVALID_ARGUMENT),
			},
		},
	}

	for _, c := range cases {
		err := c.input.IsValid()
		pSpec := kfutils.PrettyPrint(c.input)
		if err != nil {
			if c.expected != nil {
				if err.(*kfapis.KfError).Code != c.expected.(*kfapis.KfError).Code {
					t.Errorf("Spec %v;\n IsValid Got:%v %v", pSpec, err, c.expected)
				}
			} else {
				t.Errorf("Spec %v;\n IsValid Got:%v %v", pSpec, err, c.expected)
			}
		} else if c.expected != nil {
			t.Errorf("Spec %v;\n IsValid Got:%v %v", pSpec, err, c.expected)
		}
	}
}
