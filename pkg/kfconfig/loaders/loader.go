package loaders

import (
	kfconfig "github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfconfig"
)

type Loader interface {
	LoadKfDef(path string) (*kfconfig.KfConfig, error)
	ToKfDef(config kfconfig.KfConfig) (*interface{}, error)
}
