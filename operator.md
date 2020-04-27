# Kubeflow Operator

Kubeflow Operator helps deploy, monitor and manage the lifecycle of Kubeflow. Built using the [Operator Framework](https://coreos.com/blog/introducing-operator-framework) which offers an open source toolkit to build, test, package operators and manage the lifecycle of operators.

The Operator is currently in incubation phase and is based on this [design doc](https://docs.google.com/document/d/1vNBZOM-gDMpwTbhx0EDU6lDpyUjc7vhT3bdOWWCRjdk/edit#). It is built on top of _kfdef_ CR, and uses _kfctl_ as the nucleus for Controller. Current roadmap for this Operator is listed [here](https://github.com/kubeflow/kfctl/issues/193). The Operator is also [published on OperatorHub](https://operatorhub.io/operator/kubeflow)

## Deployment Instructions
1. Clone this repository and use Kustomize to build the manifests
```shell
# git clone https://github.com/kubeflow/kfctl.git && cd kfctl
OPERATOR_NAMESPACE=operators
kubectl create ns ${OPERATOR_NAMESPACE}

cd deploy/
kustomize edit set namespace ${OPERATOR_NAMESPACE}
#kustomize edit add resource kustomize/include/quota # only deploy this if the k8s cluster is 1.15+ and has resource quota support
kustomize build | kubectl apply -f -
```

2. Setup Kubeflow namespace. You can optionally apply ResourceQuota if your Kubernetes version is 1.15+, which will allow only one _kfdef_ instance or one deployment of Kubeflow on the cluster. This follows the singleton model, and is the current recommended and supported mode.
we use ResourceQuota to provide constraints that only one instance of kfdef is allowed within the Kubeflow namespace.
```shell
KUBEFLOW_NAMESPACE=kubeflow
kubectl create ns ${KUBEFLOW_NAMESPACE}
```

3. Deploy KfDef. _kfdef_ can point to a remote URL or to a local kfdef file. To use the set of default kfdefs from Kubeflow, follow the [Deploy with default kfdefs](#deploy-with-default-kfdefs) section below.
```shell
kubectl create -f <kfdef> -n ${KUBEFLOW_NAMESPACE}
```

#### Deploy with default kfdefs
To use the set of default kfdefs from Kubeflow, you will have to insert the `metadata.name` field before you can apply it to Kubernetes. Below are the commands for applying the Kubeflow 1.0 _kfdef_ using Operator. For e.g. for IBM Cloud, commands will be
> If you are pointing the kfdef file on the local machine, set the `KFDEF` to the kfdef file path and skip the `curl` command.

First point to your Cloud provider kfdef. For e.g. for OpenShift, point to the kfdef in OpenDataHub repo

```shell
export KFDEF_URL=https://raw.githubusercontent.com/opendatahub-io/manifests/v0.7-branch-openshift/kfdef/kfctl_openshift.yaml
```

Similary for GCP, IBM Cloud etc. you can point to the respective kfdefs in Kubeflow repository, e.g.

```shell
export KFDEF_URL=https://raw.githubusercontent.com/kubeflow/manifests/master/kfdef/kfctl_ibm.yaml
```

Then specify the `KUBEFLOW_DEPLOYMENT_NAME` you want to give to your deployment

```shell
export KUBEFLOW_DEPLOYMENT_NAME=kubeflow
export KFDEF=$(echo "${KFDEF_URL}" | rev | cut -d/ -f1 | rev)
curl -L ${KFDEF_URL} > ${KFDEF}
```

Next, we need to update the KFDEF file with the KUBEFLOW_DEPLOYMENT_NAME. We strongly recommend to install the [yq](https://github.com/mikefarah/yq) tool and run the `yq` command. However, if you can't install `yq`, you can run the `perl` command to do the same thing assuming you are using one of the kfdefs under the [manifests repository](https://github.com/kubeflow/manifests/tree/master/kfdef).

```shell
yq w ${KFDEF} 'metadata.name' ${KUBEFLOW_DEPLOYMENT_NAME} > ${KFDEF}.tmp && mv ${KFDEF}.tmp ${KFDEF}
# perl -pi -e $'s@metadata:@metadata:\\\n  name: '"${KUBEFLOW_DEPLOYMENT_NAME}"'@' ${KFDEF}
```

Lastly, deploy the kfdef resource to the cluster.
```shell
kubectl create -f ${KFDEF} -n ${KUBEFLOW_NAMESPACE}
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

* Delete KubeFlow deployment
```shell
kubectl delete kfdef -n ${KUBEFLOW_NAMESPACE} --all
```

> Note that the users profile namespaces created by `profile-controller` will not be deleted. The `${KUBEFLOW_NAMESPACE}` created outside of the operator will not be deleted either.

* Delete KubeFlow Operator
```shell
kubectl delete -f deploy/operator.yaml -n ${OPERATOR_NAMESPACE}
kubectl delete clusterrolebinding kubeflow-operator
kubectl delete -f deploy/service_account.yaml -n ${OPERATOR_NAMESPACE}
kubectl delete -f deploy/crds/kfdef.apps.kubeflow.org_kfdefs_crd.yaml
kubectl delete ns ${OPERATOR_NAMESPACE}
```

## Optional: Registering the Operator to OLM Catalog

Please follow the instructions [here](https://github.com/operator-framework/community-operators/blob/master/docs/testing-operators.md#testing-operator-deployment-on-openshift) to register your Operator to OLM if you are using that to install and manage the Operator. If you want to leverage the OperatorHub, please use the default [Kubeflow Operator registered there](https://operatorhub.io/operator/kubeflow)

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
3. Install [kustomize](https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md)


## Build Instructions
These steps are based on the [operator-sdk](https://github.com/operator-framework/operator-sdk/blob/master/doc/user-guide.md)
with modifications that are specific for this KubeFlow operator.

1. Clone this repository under your `$GOPATH`. (e.g. `~/go/src/github.com/kubeflow/`)
```shell
git clone https://github.com/kubeflow/kfctl
cd kfctl
```

2. Build and push the operator
```shell
export OPERATOR_IMG=<docker_username>/kubeflow-operator
make build-operator
make push-operator
```
