package loaders

import (
	kftypesv3 "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
)

func maybeGetPlatform(pluginKind string) string {
	platforms := map[string]string{
		string(kfconfig.AWS_PLUGIN_KIND): kftypesv3.AWS,
		string(kfconfig.GCP_PLUGIN_KIND): kftypesv3.GCP,
		string(kfconfig.DEX_PLUGIN_KIND): kftypesv3.DEX,
	}

	p, ok := platforms[pluginKind]
	if ok {
		return p
	} else {
		return ""
	}
}
