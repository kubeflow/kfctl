package controller

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/kubeflow/kfctl/v3/pkg/controller/kfdef"
)

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager) error {
	return kfdef.AddToManager(m)
}
