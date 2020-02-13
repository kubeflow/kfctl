# Kubeflow Operator

Kubeflow Operator helps deploy, monitor and manage the lifecycle of Kubeflow. Built using the [Operator Framework](https://coreos.com/blog/introducing-operator-framework) which offers an open source toolkit to build, test, package operators and manage the lifecycle of operators.

The Operator is currently in incubation phase and is based on this [design doc](https://docs.google.com/document/d/1vNBZOM-gDMpwTbhx0EDU6lDpyUjc7vhT3bdOWWCRjdk/edit#). It is built on top of _kfdef_ CR, and uses _kfctl_ as the nucleus for Controller. Current roadmap for this Operator is listed [here](https://github.com/kubeflow/kfctl/issues/193). 

## Deployment Instructions
1. Clone this repository and deploy the CRD and controller
```shell
# git clone https://github.com/kubeflow/kfctl.git && cd kfctl
OPERATOR_NAMESPACE=operators
kubectl create ns ${OPERATOR_NAMESPACE}
kubectl create -f deploy/crds/kfdef.apps.kubeflow.org_kfdefs_crd.yaml
kubectl create -f deploy/service_account.yaml -n ${OPERATOR_NAMESPACE}
kubectl create clusterrolebinding kubeflow-operator --clusterrole cluster-admin --serviceaccount=${OPERATOR_NAMESPACE}:kubeflow-operator
kubectl create -f deploy/operator.yaml -n ${OPERATOR_NAMESPACE}
```

2. Deploy KfDef. You can optionally apply ResourceQuota if your Kubernetes version is 1.15+, which will allow only one _kfdef_ instance or one deployment of Kubeflow on this cluster, which follows the singleton model.
we use ResourceQuota to provide constraints that only one instance of kfdef is allowed within the Kubeflow namespace.
```shell
KUBEFLOW_NAMESPACE=kubeflow
kubectl create ns ${KUBEFLOW_NAMESPACE}
# kubectl create -f deploy/crds/kfdef_quota.yaml -n ${KUBEFLOW_NAMESPACE} # only deploy this if the k8s cluster is 1.15+ and has resource quota support
kubectl create -f <kfdef> -n ${KUBEFLOW_NAMESPACE}
```

_<kfdef>_ above can point to a remote URL or to a local kfdef file. For e.g. for IBM Cloud, command will be
```shell
kubectl create -f https://raw.githubusercontent.com/kubeflow/manifests/master/kfdef/kfctl_ibm.yaml -n ${KUBEFLOW_NAMESPACE}
```

## Testing Watcher and Reconciler
One of the major benefits of using kfctl as an Operator is to leverage the functionalities around being able to watch and reconcile your Kubeflow deployments. The Operator is watching all the resources with the `kfctl` label. If one of the resources is deleted, 
the reconciler will be triggered and re-apply the kfdef to the Kubernetes Cluster.

1. Check the tf-job-operator deployment is running.
```shell
kubectl get deploy -n ${KUBEFLOW_NAMESPACE} tf-job-operator
# NAME                                          READY   UP-TO-DATE   AVAILABLE   AGE
# tf-job-operator                               1/1     1            1           7m15s
```

2. Delete the tf-job-operator deployment
```shell
kubectl delete deploy -n ${KUBEFLOW_NAMESPACE} tf-job-operator
# deployment.extensions "tf-job-operator" deleted
```

3. Wait for 10 to 15 seconds, then check the tf-job-operator deployment again. 
You will be able to see that the deployment is being recreated by the Operator's reconciliation logic
 
```Shell
kubectl get deploy -n ${KUBEFLOW_NAMESPACE} tf-job-operator
# NAME                                          READY   UP-TO-DATE   AVAILABLE   AGE
# tf-job-operator                               0/1     0            0           10s
```

## Delete KubeFlow
Delete KubeFlow deployment
```shell
kubectl delete kfdef -n ${KUBEFLOW_NAMESPACE} --all
```

Delete KubeFlow Operator
```shell
kubectl delete -f deploy/operator.yaml -n ${OPERATOR_NAMESPACE}
kubectl delete clusterrolebinding kubeflow-operator
kubectl delete -f deploy/service_account.yaml -n ${OPERATOR_NAMESPACE}
kubectl delete -f deploy/crds/kfdef.apps.kubeflow.org_kfdefs_crd.yaml
kubectl delete ns ${OPERATOR_NAMESPACE}
```

## Optional: Registering the Operator to OLM Catalog

Please follow the instructions [here](https://github.com/operator-framework/community-operators/blob/master/docs/testing-operators.md#testing-operator-deployment-on-openshift) to register your Operator to OLM if you are using that to install and manage the Operator.

## TroubleShooting
- When deleting the KubeFlow deployment, it's using kfctl delete in the background where it only deletes the deployment namespace. 
This will make some of KubeFlow pod deployments hanging because _mutatingwebhookconfigurations_ are cluster-wide 
resources and some of the webhooks are watching every pod deployment. Therefore, we need to remove all the _mutatingwebhookconfigurations_ 
so that pod deployments will not be hanging after deleting KubeFlow.
```shell
kubectl delete mutatingwebhookconfigurations admission-webhook-mutating-webhook-configuration
kubectl delete mutatingwebhookconfigurations inferenceservice.serving.kubeflow.org
kubectl delete mutatingwebhookconfigurations istio-sidecar-injector
kubectl delete mutatingwebhookconfigurations katib-mutating-webhook-config
kubectl delete mutatingwebhookconfigurations mutating-webhook-configurations
```

# Development Instructions

### Prerequisites
1. Install [operator-sdk](https://github.com/operator-framework/operator-sdk/blob/master/doc/user/install-operator-sdk.md)
2. Install [golang](https://golang.org/dl/)


## Build Instructions
These steps are based on the [operator-sdk](https://github.com/operator-framework/operator-sdk/blob/master/doc/user-guide.md)
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
operator-sdk build <docker_username>/kubeflow-operator:v0.1.0
```

5. Push the operator image and update the operator.yaml spec with the new operator image.
```shell
docker push <docker_username>/kubeflow-operator:v0.1.0
vi deploy/operator.yaml
```
