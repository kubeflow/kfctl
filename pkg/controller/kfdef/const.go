package kfdef

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// KubeflowLabel represents Label for kfctl deployed resource
	KubeflowLabel = "app.kubernetes.io/managed-by"
)

var (
	// watchedResources contains all resources we will watch and reconcile when changed
	watchedResources = []schema.GroupVersionKind{
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding"},
		{Group: "", Version: "v1", Kind: "ConfigMap"},
		{Group: "apiextensions.k8s.io", Version: "v1beta1", Kind: "CustomResourceDefinition"},
		{Group: "apps", Version: "v1", Kind: "DaemonSet"},
		{Group: "apps", Version: "v1", Kind: "Deployment"},
		{Group: "extensions", Version: "v1beta1", Kind: "Deployment"},
		{Group: "extensions", Version: "v1beta1", Kind: "Ingress"},
		{Group: "admissionregistration.k8s.io", Version: "v1beta1", Kind: "MutatingWebhookConfiguration"},
		{Group: "", Version: "v1", Kind: "Namespace"},
		{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
		{Group: "rbac.authorization.k8s.io", Version: "v1beta1", Kind: "Role"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
		{Group: "rbac.authorization.k8s.io", Version: "v1beta1", Kind: "RoleBinding"},
		{Group: "", Version: "v1", Kind: "Secret"},
		{Group: "", Version: "v1", Kind: "Service"},
		{Group: "", Version: "v1", Kind: "ServiceAccount"},
		{Group: "apps", Version: "v1", Kind: "StatefulSet"},
		{Group: "admissionregistration.k8s.io", Version: "v1beta1", Kind: "ValidatingWebhookConfiguration"},
	}

	watchedKubeflowResources = []schema.GroupVersionKind{
		{Group: "app.k8s.io", Version: "v1beta1", Kind: "Application"},
		{Group: "rbac.istio.io", Version: "v1alpha1", Kind: "ServiceRole"},
		{Group: "rbac.istio.io", Version: "v1alpha1", Kind: "ServiceRoleBinding"},
		{Group: "networking.istio.io", Version: "v1alpha3", Kind: "VirtualService"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "Workflow"},
		{Group: "tekton.dev", Version: "v1alpha1", Kind: "Condition"},
	}
)
