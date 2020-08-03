// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// Package main implements an injection function for resource reservations and
// is run with `kustomize config run -- DIR/`.

// TODO(https://github.com/kubeflow/gcp-blueprints/issues/27): We should make this configurable and follow
// the model of image-prefix. This means instead of using a separate main.go we should use kustomize-fns/main.go
// and just register this function in the dispatcher.
package remove_namespace

import (
	"fmt"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"strings"
)

type groupKind struct {
	Group string
	Kind  string
}

const (
	Kind       = "RemoveNamespace"
	APIVersion = "kubeflow.org/v1alpha1"
)

var (

	// List of clusterResources to match and remove namespace from.
	// TODO(jlewi): How could we make this configurable? I think you want to put the relevant data
	// into the YAML files that get processed by the transform. We could define an annotation and then
	// the transform could look for any resource with this annotation and remove the namespace if it exists.
	clusterKinds = []groupKind{
		{
			Kind:  "Profile",
			Group: "kubeflow.org",
		},
		{
			Kind:  "ClusterRbacConfig",
			Group: "rbac.istio.io",
		},
		{
			Kind:  "ClusterIssuer",
			Group: "cert-manager.io",
		},
		{
			Kind:  "CompositeController",
			Group: "metacontroller.k8s.io",
		},
	}
)

// Filter returns a new RemoveNamespaceFunction
func Filter() kio.Filter {
	return &RemoveNamespaceFunction{}
}

// RemoveNamespaceFunction implements the RemoveNamespace Function
type RemoveNamespaceFunction struct {
	// Kind is the API name.  Must be RemoveNamespace.
	Kind string `yaml:"kind"`

	// APIVersion is the API version.  Must be examples.kpt.dev/v1alpha1
	APIVersion string `yaml:"apiVersion"`

	// Metadata defines instance metadata.
	Metadata Metadata `yaml:"metadata"`

	// Spec defines the desired declarative configuration.
	Spec Spec `yaml:"spec"`
}

type Metadata struct {
	// Name is the name of the RemoveNamespace Resources
	Name string `yaml:"name"`

	// Namespace is the namespace of the RemoveNamespace Resources
	Namespace string `yaml:"namespace"`

	// Labels are labels applied to the RemoveNamespace Resources
	Labels map[string]string `yaml:"labels"`

	// Annotations are annotations applied to the RemoveNamespace Resources
	Annotations map[string]string `yaml:"annotations"`
}

type Spec struct {
	ClusterKinds []*ClusterKind   `yaml:"clusterKinds"`
}

type ClusterKind struct {
	Kind string `yaml:"kind"`
	Group string `yaml:"group"`
}

func (f *RemoveNamespaceFunction) init() error {
	if f.Metadata.Name == "" {
	return fmt.Errorf("must specify ImagePrefix name")
	}

	if f.Metadata.Labels == nil {
	f.Metadata.Labels = map[string]string{}
	}

	if f.Spec.ClusterKinds == nil {
	f.Spec.ClusterKinds = []*ClusterKind{}
	}
	return nil
}


// Filter looks for resources of the specified kind and removes them
func (f *RemoveNamespaceFunction) Filter(inputs []*yaml.RNode) ([]*yaml.RNode, error) {
	if err := f.init(); err != nil {
		return nil, err
	}

	errors := []error{}

	for _, r := range inputs {
		if err := f.removeNamespace(r); err !=nil {
			errors = append(errors, err)
		}
	}
	return inputs, utilerrors.NewAggregate(errors)
}

func getGroup(apiVersion string) string {
	pieces := strings.Split(apiVersion, "/")

	if len(pieces) == 1 {
		return apiVersion
	}

	// Strip the last piece as the version
	pieces = pieces[0 : len(pieces)-1]
	return strings.Join(pieces, "/")
}

// remove namespace from cluster resources.
func (f *RemoveNamespaceFunction) removeNamespace(r *yaml.RNode) error {

	meta, err := r.GetMeta()
	if err != nil {
		return err
	}

	// TODO(jlewi): Does kustomize provide built in functions for filtering to a list of kinds?
	isMatch := false
	for _, c := range f.Spec.ClusterKinds {
		group := getGroup(meta.APIVersion)

		if group == c.Group && meta.Kind == c.Kind {
			isMatch = true
			break
		}
	}

	// Skip this object because it is not an allowed kind.
	if !isMatch {
		return nil
	}

	return r.PipeE(
		yaml.LookupCreate(yaml.ScalarNode, "metadata"),
		yaml.FieldClearer{
			Name: "namespace",
		},
	)
}
