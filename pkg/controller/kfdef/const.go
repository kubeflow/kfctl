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
	WatchedResources = []schema.GroupVersionKind{
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding"},
		{Group: "", Version: "v1", Kind: "ConfigMap"},
		{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"},
		{Group: "apps", Version: "v1", Kind: "DaemonSet"},
		{Group: "apps", Version: "v1", Kind: "Deployment"},
		{Group: "apps.openshift.io", Version: "v1", Kind: "DeploymentConfig"},
		{Group: "image.openshift.io", Version: "v1", Kind: "ImageStream"},
		{Group: "build.openshift.io", Version: "v1", Kind: "BuildConfig"},
		{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
		{Group: "admissionregistration.k8s.io", Version: "v1", Kind: "MutatingWebhookConfiguration"},
		{Group: "", Version: "v1", Kind: "Namespace"},
		{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
		{Group: "", Version: "v1", Kind: "Secret"},
		{Group: "", Version: "v1", Kind: "Service"},
		{Group: "", Version: "v1", Kind: "ServiceAccount"},
		{Group: "apps", Version: "v1", Kind: "StatefulSet"},
		{Group: "admissionregistration.k8s.io", Version: "v1", Kind: "ValidatingWebhookConfiguration"},
	}

	WatchedKubeflowResources = []schema.GroupVersionKind{
		{Group: "app.k8s.io", Version: "v1beta1", Kind: "Application"},
		{Group: "networking.istio.io", Version: "v1beta1", Kind: "VirtualService"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "Workflow"},
		{Group: "tekton.dev", Version: "v1beta1", Kind: "Condition"},
	}
)
