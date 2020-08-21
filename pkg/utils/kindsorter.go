package utils

import (
	"sigs.k8s.io/kustomize/v3/pkg/resource"
	"sort"
)

// SortOrder is an ordering of Kinds.
type SortOrder []string

// InstallOrder is the order in which resources should be installed (by Kind).

// Those occurring earlier in the list get installed before those occurring later in the list.
var InstallOrder SortOrder = []string{
	"Namespace",
	"ResourceQuota",
	"LimitRange",
	"PodSecurityPolicy",
	"Secret",
	"ConfigMap",
	"StorageClass",
	"PersistentVolume",
	"PersistentVolumeClaim",
	"ServiceAccount",
	"CustomResourceDefinition",
	"ClusterRole",
	"ClusterRoleBinding",
	"Role",
	"RoleBinding",
	"Service",
	"DaemonSet",
	"Pod",
	"ReplicationController",
	"ReplicaSet",
	"Deployment",
	"StatefulSet",
	"Job",
	"CronJob",
	"Ingress",
	"MutatingWebhookConfiguration",
	"ValidatingWebhookConfiguration",
	"APIService",
}

// UninstallOrder is the order in which resources should be uninstalled (by Kind).
// Those occurring earlier in the list get uninstalled before those occurring later in the list.
// Reason to move CustomResourceDefinition earlier is we want to leverage finalizer to delete created resources
// like profile -> namespaces, etc
var UninstallOrder SortOrder = []string{
	"APIService",
	"ValidatingWebhookConfiguration",
	"MutatingWebhookConfiguration",
	"CustomResourceDefinition",
	"Ingress",
	"Service",
	"CronJob",
	"Job",
	"StatefulSet",
	"Deployment",
	"ReplicaSet",
	"ReplicationController",
	"Pod",
	"DaemonSet",
	"RoleBinding",
	"Role",
	"ClusterRoleBinding",
	"ClusterRole",
	"ServiceAccount",
	"PersistentVolumeClaim",
	"PersistentVolume",
	"StorageClass",
	"ConfigMap",
	"Secret",
	"PodSecurityPolicy",
	"LimitRange",
	"ResourceQuota",
	"Namespace",
}

// SortByKind does an in-place sort of resources by Kind. Results are sorted by 'ordering'
func SortByKind(manifests []*resource.Resource, ordering SortOrder) []*resource.Resource {
	ks := newKindSorter(manifests, ordering)
	sort.Sort(ks)
	return ks.resources
}

type kindSorter struct {
	ordering  map[string]int
	resources []*resource.Resource
}

func newKindSorter(r []*resource.Resource, s SortOrder) *kindSorter {
	o := make(map[string]int, len(s))
	for v, k := range s {
		o[k] = v
	}

	return &kindSorter{
		resources: r,
		ordering:  o,
	}
}

func (k *kindSorter) Len() int { return len(k.resources) }

func (k *kindSorter) Swap(i, j int) { k.resources[i], k.resources[j] = k.resources[j], k.resources[i] }

func (k *kindSorter) Less(i, j int) bool {
	a := k.resources[i]
	b := k.resources[j]
	first, aok := k.ordering[a.GetKind()]
	second, bok := k.ordering[b.GetKind()]
	// if same kind (including unknown) sub sort alphanumeric
	if first == second {
		// if both are unknown and of different kind sort by kind alphabetically
		if !aok && !bok && a.GetKind() != b.GetKind() {
			return a.GetKind() < b.GetKind()
		}
		return a.GetKind() < b.GetKind()
	}
	// unknown kind is last
	if !aok {
		return false
	}
	if !bok {
		return true
	}
	// sort different kinds
	return first < second
}
