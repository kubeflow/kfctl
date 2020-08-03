// Package virtual_service implements a kustomize function for configuring virtual services
package virtual_service

import (
	"fmt"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/kustomize/kyaml/kio"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	Kind       = "VirtualServiceTransform"
	APIVersion = "kubeflow.org/v1alpha1"
	VSKind     = "VirtualService"
)

var _ kio.Filter = &VirtualServiceFunction{}

// Filter returns a new VirtualServiceFunction
func Filter() kio.Filter {
	return &VirtualServiceFunction{}
}

// VirtualServiceFunction implements the ImagePrefix Function
type VirtualServiceFunction struct {
	// Kind is the API name.  Must be ImagePrefix.
	Kind string `yaml:"kind"`

	// APIVersion is the API version.  Must be examples.kpt.dev/v1alpha1
	APIVersion string `yaml:"apiVersion"`

	// Metadata defines instance metadata.
	Metadata Metadata `yaml:"metadata"`

	// Spec defines the desired declarative configuration.
	Spec Spec `yaml:"spec"`
}

type Metadata struct {
	// Name is the name of the ImagePrefix Resources
	Name string `yaml:"name"`

	// Namespace is the namespace of the ImagePrefix Resources
	Namespace string `yaml:"namespace"`

	// Labels are labels applied to the ImagePrefix Resources
	Labels map[string]string `yaml:"labels"`

	// Annotations are annotations applied to the ImagePrefix Resources
	Annotations map[string]string `yaml:"annotations"`
}

type Spec struct {
	// Gateway is the gateway to use for all virtual services
	Gateway string `yaml:"gateway"`
}

func (f *VirtualServiceFunction) init() error {
	if f.Metadata.Name == "" {
		return fmt.Errorf("must specify %v name", Kind)
	}

	if f.Metadata.Labels == nil {
		f.Metadata.Labels = map[string]string{}
	}

	return nil
}

// Filter looks for virtual services and sets the gateway on them.
func (f *VirtualServiceFunction) Filter(inputs []*yaml.RNode) ([]*yaml.RNode, error) {
	if err := f.init(); err != nil {
		return nil, err
	}

	errors := []error{}

	for _, r := range inputs {
		if err := f.setGateway(r); err != nil {
			errors = append(errors, err)
		}
	}
	return inputs, utilerrors.NewAggregate(errors)
}

// setGateway sets the gateway on virtual services
func (f *VirtualServiceFunction) setGateway(r *yaml.RNode) error {
	meta, err := r.GetMeta()
	if err != nil {
		return err
	}

	if meta.Kind != VSKind {
		// Not a virtual service; skip this resource
		return nil
	}

	value, err := yaml.Parse(fmt.Sprintf("[%s]", f.Spec.Gateway))

	if err != nil {
		return err

	}
	return r.PipeE(
		yaml.LookupCreate(yaml.ScalarNode, "spec"),
		yaml.FieldSetter{
			Name:  "gateways",
			Value: value,
		},
	)
}
