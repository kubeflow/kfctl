package apps

import (
	v1 "github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfdef/v1"
	operatorsv1alpha1 "github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes,
		v1.SchemeBuilder.AddToScheme,
		operatorsv1alpha1.AddToScheme)
}
