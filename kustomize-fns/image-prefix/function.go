// Package image_prefix implements a kustomize function for changing image prefixes
package image_prefix

import (
	"fmt"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	prefixAnnotation = "image-prefix.kubeflow.org"
)

const (
	Kind       = "ImagePrefix"
	APIVersion = "kubeflow.org/v1alpha1"
)

var _ kio.Filter = &ImagePrefixFunction{}

// Filter returns a new ImagePrefixFunction
func Filter() kio.Filter {
	return &ImagePrefixFunction{}
}

// ImagePrefixFunction implements the ImagePrefix Function
type ImagePrefixFunction struct {
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
	 ImageMappings []*ImageMapping   `yaml:"imageMappings"`
}

type ImageMapping struct {
	Src string `yaml: src`
	Dest string `yaml: dest`
}

func (f *ImagePrefixFunction) init() error {
	if f.Metadata.Name == "" {
		return fmt.Errorf("must specify ImagePrefix name")
	}

	if f.Metadata.Labels == nil {
		f.Metadata.Labels = map[string]string{}
	}

	if f.Spec.ImageMappings == nil {
		f.Spec.ImageMappings = []*ImageMapping{}
	}
	return nil
}

// Filter looks for images with the specified prefixes and replaces them.
func (f *ImagePrefixFunction) Filter(inputs []*yaml.RNode) ([]*yaml.RNode, error) {
	if err := f.init(); err != nil {
		return nil, err
	}

	errors := []error{}

	for _, r := range inputs {
		if err := f.replaceImage(r); err !=nil {
			errors = append(errors, err)
		}
	}
	return inputs, utilerrors.NewAggregate(errors)
}

// replaceImage looks for an annotation changing docker image prefixes and if it is present
// applies it to all the images.
func (f *ImagePrefixFunction) replaceImage(r *yaml.RNode) error {
	// lookup the containers field
	containers, err := r.Pipe(yaml.Lookup("spec", "template", "spec", "containers"))
	if err != nil {
		s, _ := r.String()
		return fmt.Errorf("%v: %s", err, s)
	}

	if containers == nil {
		// doesn't have containers, skip the Resource
		return nil
	}

	// visit each container and apply the cpu and memory reservations
	return containers.VisitElements(func(node *yaml.RNode) error {
		image := node.Field("image").Value.YNode().Value

		newImage := ""

		for _, m := range f.Spec.ImageMappings {
			if strings.HasPrefix(image, m.Src) {
				newImage = m.Dest + image[len(m.Src):len(image)]
				break
			}
		}

		if newImage == "" {
			return nil
		}

		// set image
		err := node.PipeE(
			// lookup resources.requests.cpu, creating the field as a
			// ScalarNode if it doesn't exist
			yaml.LookupCreate(yaml.ScalarNode, "image"),
			// set the field value to the cpuSize
			yaml.Set(yaml.NewScalarRNode(newImage)))
		if err != nil {
			s, _ := r.String()
			return fmt.Errorf("%v: %s", err, s)
		}
		return nil
	})
}