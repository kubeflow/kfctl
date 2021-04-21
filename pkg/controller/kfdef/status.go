package kfdef

import (
	"context"
	"reflect"

	kfdefv1 "github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfdef/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const DeploymentCompleted string = "Kubeflow Deployment completed"

// The setKfDefStatus method accepts a custom resource of type KfDef type
// It retrieves the current stored version of the resource and compares the
// status subresource. If different, the status is updated
func (r *ReconcileKfDef) setKfDefStatus(cr *kfdefv1.KfDef) error {
	ctx := context.Background()
	objKey := types.NamespacedName{
		Namespace: cr.Namespace,
		Name:      cr.Name,
	}
	current := &kfdefv1.KfDef{}
	if err := r.client.Get(ctx, objKey, current); err != nil {
		return err
	}

	if !reflect.DeepEqual(cr.Status, current.Status) {
		current.Status = cr.Status
		return r.client.Status().Update(ctx, current)
	}

	return nil
}

func (r *ReconcileKfDef) reconcileStatus(cr *kfdefv1.KfDef) error {
	return r.setKfDefStatus(cr)
}

func getReconcileStatus(cr *kfdefv1.KfDef, err error) error {
	conditions := []kfdefv1.KfDefCondition{}

	if err != nil {
		conditions = append(conditions, kfdefv1.KfDefCondition{
			LastUpdateTime: cr.CreationTimestamp,
			Status:         corev1.ConditionTrue,
			Reason:         err.Error(),
			Type:           kfdefv1.KfDegraded,
		})
	}

	conditions = append(conditions, kfdefv1.KfDefCondition{
		LastUpdateTime: cr.CreationTimestamp,
		Status:         corev1.ConditionTrue,
		Reason:         DeploymentCompleted,
		Type:           kfdefv1.KfAvailable,
	})

	cr.Status.Conditions = conditions

	return err
}
