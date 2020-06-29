// Package config_map+merge implements a kustomize function for merging {env: value} to a ConfigMap
package config_map_merge

import (
	"fmt"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"os"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"sigs.k8s.io/kustomize/api/types"
)

const (
	prefixAnnotation = "config-map-merge.kubeflow.org"
)

const (
	Kind       = "ConfigMapMerge"
	APIVersion = "kubeflow.org/v1alpha1"
)

var _ kio.Filter = &ConfigMapMergeFunction{}

// Filter returns a new ConfigMapMergeFunction
func Filter() kio.Filter {
	return &ConfigMapMergeFunction{}
}

// ConfigMapMergeFunction implements the ConfigMapMerge Function
type ConfigMapMergeFunction struct {
	// Kind is the API name.  Must be ConfigMapMerge.
	Kind string `yaml:"kind"`

	// APIVersion is the API version.  Must be examples.kpt.dev/v1alpha1
	APIVersion string `yaml:"apiVersion"`

	// Metadata defines instance metadata.
	Metadata Metadata `yaml:"metadata"`

	// Spec defines the desired declarative configuration.
	Spec Spec `yaml:"spec"`
}

type Metadata struct {
	// Name is the name of the ConfigMapMerge Resources
	Name string `yaml:"name"`

	// Namespace is the namespace of the ConfigMapMerge Resources
	Namespace string `yaml:"namespace"`

	// Labels are labels applied to the ConfigMapMerge Resources
	Labels map[string]string `yaml:"labels"`

	// Annotations are annotations applied to the ConfigMapMerge Resources
	Annotations map[string]string `yaml:"annotations"`
}

type Spec struct {
	ConfigMaps []*types.GeneratorArgs   `yaml:"configMaps"`
}

func (f *ConfigMapMergeFunction) init() error {
	if f.Metadata.Name == "" {
		return fmt.Errorf("must specify ConfigMapMerge name")
	}

	if f.Metadata.Labels == nil {
		f.Metadata.Labels = map[string]string{}
	}

	if f.Spec.ConfigMaps == nil {
		f.Spec.ConfigMaps = []*types.GeneratorArgs{}
	}
	return nil
}

// Filter looks for applications and merge the configMap generator to its kustomization.yaml.
func (f *ConfigMapMergeFunction) Filter(inputs []*yaml.RNode) ([]*yaml.RNode, error) {
	if err := f.init(); err != nil {
		return nil, err
	}

	errors := []error{}

	for _, r := range inputs {
		if err := f.mergeConfigMap(r); err !=nil {
			errors = append(errors, err)
		}
	}
	return inputs, utilerrors.NewAggregate(errors)
}

// mergeConfigMap merges the generator args for the ConfigMap in that application.
func (f *ConfigMapMergeFunction) mergeConfigMap(r *yaml.RNode) error {
	m , _  := r.GetMeta()
	fmt.Fprintf(os.Stderr, "Meta %+v",m)

	// skip if it is not a kustomization.yaml
	if (m.Kind != "Kustomization") {
		return nil
	}

	y, err := yaml.Marshal(f.Spec.ConfigMaps)
	if err != nil {
		return fmt.Errorf("%v",err)
	}

	yr, err := yaml.Parse(string(y))
	if err != nil {
		return fmt.Errorf("%v",err)
	}

	err = r.PipeE(
		yaml.LookupCreate(yaml.SequenceNode, "configMapGenerator"),
		yaml.Append(yr.YNode().Content...))
	if err != nil {
		return fmt.Errorf("%v",err)
	}
	
	return nil
}
