# Run kfctl as Custom Resource Definition using operator 

## Deployment Instructions
1. Clone this repository and deploy the CRD definition and controller.
```shell
# git clone https://github.com/kubeflow/kfctl.git && cd kfctl
kubectl create ns operators
kubectl create -f deploy/crds/kfdef.apps.kubeflow.org_kfdefs_crd.yaml
kubectl create -f deploy/service_account.yaml
kubectl create -f deploy/cluster_role_binding.yaml
kubectl create -f deploy/operator.yaml
```

2. Deploy KfDef and ResourceQuota. Since we only want to allow one kfdef instance on this cluster, 
we use ResourceQuota to provide constraints that only one kfdef is allowed within the kubeflow namespace.
```shell
kubectl create ns kubeflow
# kubectl create -f deploy/crds/kfdef_quota.yaml # only deploy this if the k8s cluster is 1.15+ and has resource quota support
kubectl create -f <kfdef> -n kubeflow
```

## Testing Watcher and Reconciler
The Operator is watching all the resources with the `kfctl` label. If one of the resources is deleted, 
the reconciler will be triggered and re-apply the kfdef to the Kubernetes Cluster.

1. Check the tf-job-operator deployment is running.
```shell
kubectl get deploy -n kubeflow tf-job-operator
# NAME                                          READY   UP-TO-DATE   AVAILABLE   AGE
# tf-job-operator                               1/1     1            1           7m15s
```

2. Delete the tf-job-operator deployment
```shell
kubectl delete deploy -n kubeflow tf-job-operator
# deployment.extensions "tf-job-operator" deleted
```

3. Wait for 10 to 15 seconds, then check the tf-job-operator deployment again. 
You will able see that the deployment is being recreated by the operator's reconciliation. 
```Shell
kubectl get deploy -n kubeflow tf-job-operator
# NAME                                          READY   UP-TO-DATE   AVAILABLE   AGE
# tf-job-operator                               0/1     0            0           10s
```

## Delete KubeFlow
Delete KubeFlow deployment
```shell
kubectl delete kfdef -n kubeflow --all
```

Delete KubeFlow Operator
```shell
kubectl delete -f deploy/operator.yaml
kubectl delete -f deploy/cluster_role_binding.yaml
kubectl delete -f deploy/service_account.yaml
kubectl delete -f deploy/crds/kfdef.apps.kubeflow.org_kfdefs_crd.yaml
kubectl delete ns operators
```

## TroubleShootings
- When deleting the KubeFlow deployment, it's using kfctl delete in the background where it only deletes the deployment namespace. 
This will make some of our pod deployments hanging because mutatingwebhookconfigurations are cluster-wise 
resources and some of the webhooks are watching every pod deployment. Therefore, we need to remove all the mutatingwebhookconfigurations 
so that pod deployments will not be hanging after deleting KubeFlow.
```shell
kubectl delete mutatingwebhookconfigurations --all
```

# Development

### Prerequisites
1. Install [operator-sdk](https://github.com/operator-framework/operator-sdk/blob/master/doc/user/install-operator-sdk.md)
2. Install [golang](https://golang.org/dl/)


## Build Instructions
These steps are base on the [operator-sdk](https://github.com/operator-framework/operator-sdk/blob/master/doc/user-guide.md)
with modifications that are specific for this KubeFlow operator.

1. Clone this repository under your `$GOPATH`. (e.g. `~/go/src/github.com/kubeflow/`)
```shell
git clone https://github.com/kubeflow/kfctl
cd kfctl
```

2. Create vendor dependency.
```shell
go mod vendor
```

3. Fix duplicated logrus library bugs due to the logrus version that kfctl is using.
```shell
pushd vendor/github.com/Sirupsen/logrus/
echo -n '
// +build linux aix

package logrus

import "golang.org/x/sys/unix"

func isTerminal(fd int) bool {
	_, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	return err == nil
} ' > terminal_check_unix.go

popd
```

4. Build the operator image
```shell
operator-sdk build <username>/kubeflow-operator:v0.0.2
```

5. Update the operator.yaml spec with the new operator image.
```shell
vi deploy/operator.yaml
```