// package fake provides a fake implementation of the GCP Plugin
package fake

import (
	kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	"golang.org/x/oauth2"
)

type FakeGcp struct {
	ts oauth2.TokenSource
}

func (g *FakeGcp) Apply(resources kftypes.ResourceEnum) error {
	return nil
}

func (g *FakeGcp) Delete(resources kftypes.ResourceEnum) error {
	return nil
}

func (g *FakeGcp) Dump(resources kftypes.ResourceEnum) error {
	return nil
}

func (g *FakeGcp) Generate(resources kftypes.ResourceEnum) error {
	return nil
}

func (g *FakeGcp) Init(resources kftypes.ResourceEnum) error {
	return nil
}

func (g *FakeGcp) SetTokenSource(s oauth2.TokenSource) error {
	g.ts = s
	return nil
}
