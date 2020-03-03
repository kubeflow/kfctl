package loaders

import (
	"os"
	"path"
	"testing"
)

// Make sure literal secrets are keeped during load kfdef -> kfconfig
func Test_loadKfdefLiteralSecrets(t *testing.T) {
	wd, _ := os.Getwd()
	kfconfig, err := LoadConfigFromURI(path.Join(wd, "testdata", "kfctl_gcp_basic_auth.0.7.0.yaml"))
	if err != nil || kfconfig == nil {
		t.Error(err)
	}
	if kfconfig.Spec.Secrets[0].SecretSource.LiteralSource.Value != "passwordVal" {
		t.Error("Secrets dropped during kfdef -> kfconfig!")
	}

}
