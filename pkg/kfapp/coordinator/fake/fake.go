// package fake provides a fake implementation of the coordinator for use in tests
package fake

import (
	"path"

	kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	gcpFake "github.com/kubeflow/kfctl/v3/pkg/kfapp/gcp/fake"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
	kfloaders "github.com/kubeflow/kfctl/v3/pkg/kfconfig/loaders"
)

type FakeCoordinator struct {
	KfDef   *kfconfig.KfConfig
	Plugins map[string]kftypes.KfApp
}

func (f *FakeCoordinator) Apply(resources kftypes.ResourceEnum) error {
	return nil
}

func (f *FakeCoordinator) Delete(resources kftypes.ResourceEnum) error {
	return nil
}

func (f *FakeCoordinator) Dump(resources kftypes.ResourceEnum) error {
	return nil
}

func (f *FakeCoordinator) Generate(resources kftypes.ResourceEnum) error {
	return nil
}

func (f *FakeCoordinator) Init(resources kftypes.ResourceEnum) error {
	return nil
}

func (f *FakeCoordinator) GetKfDef() *kfconfig.KfConfig {
	return f.KfDef
}

func (f *FakeCoordinator) GetPlugin(name string) (kftypes.KfApp, bool) {
	a, ok := f.Plugins[name]
	return a, ok
}

type FakeBuilder struct {
}

func (b *FakeBuilder) CreateKfAppCfgFile(def *kfconfig.KfConfig) (string, error) {
	return path.Join(def.Spec.AppDir, kftypes.KfConfigFile), nil
}

func (b *FakeBuilder) LoadKfAppCfgFile(cfgFile string) (kftypes.KfApp, error) {
	d, err := kfloaders.LoadConfigFromURI(cfgFile)

	if err != nil {
		return nil, err
	}
	f := &FakeCoordinator{
		KfDef:   d,
		Plugins: make(map[string]kftypes.KfApp),
	}

	for _, p := range d.Spec.Plugins {
		if p.Name == kftypes.GCP {
			f.Plugins[kftypes.GCP] = &gcpFake.FakeGcp{}
			break
		}
	}
	return f, nil
}
