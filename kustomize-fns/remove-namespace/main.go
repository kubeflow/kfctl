// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// Package main implements an injection function for resource reservations and
// is run with `kustomize config run -- DIR/`.
package main

import (

"fmt"
"os"
"sigs.k8s.io/kustomize/kyaml/fn/framework"
"sigs.k8s.io/kustomize/kyaml/yaml"
"strings"
)

type groupKind struct {
	Group string
	Kind string
}

var (

	// List of clusterResources to match and remove namespace from.
	// TODO(jlewi): How could we make this configurable?
	clusterKinds = []groupKind{
		{
			Kind: "Profile",
			Group: "kubeflow.org",

		},
		{
			Kind: "ClusterRbacConfig",
			Group: "rbac.istio.io",
		},
		{
			Kind: "ClusterIssuer",
			Group: "cert-manager.io",
		},
		{
			Kind: "CompositeController",
			Group: "metacontroller.k8s.io",
		},
	}

)

func main() {
	fmt.Printf("Running remove-namespace")
	resourceList := &framework.ResourceList{}
	cmd := framework.Command(resourceList, func() error {
		for _, r := range resourceList.Items {
			if err := removeNamespace(r); err != nil {
				return err
			}
		}
		return nil
	})
	if err := cmd.Execute(); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}

func getGroup(apiVersion string) string {
	pieces := strings.Split(apiVersion, "/")

	if len(pieces) == 1 {
		return apiVersion
	}

	// Strip the last piece as the version
	pieces = pieces[0:len(pieces) -1 ]
	return strings.Join(pieces, "/")
}

// remove namespace from cluster resources.
func removeNamespace(r *yaml.RNode) error {

	// check for the tshirt-size annotations
	meta, err := r.GetMeta()
	if err != nil {
		return err
	}

	// TODO(jlewi): Does kustomize provide built in functions for filtering to a list of kinds?
	isMatch := false
	for _, c := range clusterKinds {
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
		// set the field value to the cpuSize
		yaml.FieldClearer{
			Name: "namespace",
		},
		)
}
