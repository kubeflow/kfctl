# Kubeflow Operator

Kubeflow Operator helps deploy, monitor and manage the lifecycle of Kubeflow. Built using the [Operator Framework](https://coreos.com/blog/introducing-operator-framework) which offers an open source toolkit to build, test, package operators and manage the lifecycle of operators.

The Operator is currently in incubation phase and is based on this [design doc](https://docs.google.com/document/d/1vNBZOM-gDMpwTbhx0EDU6lDpyUjc7vhT3bdOWWCRjdk/edit#). It is built on top of _KfDef_ CR, and uses _kfctl_ as the nucleus for Controller. Current roadmap for this Operator is listed [here](https://github.com/kubeflow/kfctl/issues/193). The Operator is also [published on OperatorHub](https://operatorhub.io/operator/kubeflow).

## Prerequisites

* Install [kustomize](https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md)

## Deployment Instructions

1. Clone this repository, build the manifests and install the operator

```shell
git clone https://github.com/kubeflow/kfctl.git && cd kfctl

export OPERATOR_NAMESPACE=operators
kubectl create ns ${OPERATOR_NAMESPACE}

cd deploy/
kustomize edit set namespace ${OPERATOR_NAMESPACE}
# kustomize edit add resource kustomize/include/quota # only deploy this if the k8s cluster is 1.15+ and has resource quota support, which will allow only one _kfdef_ instance or one deployment of Kubeflow on the cluster. This follows the singleton model, and is the current recommended and supported mode.

kustomize build | kubectl apply -f -
```

2. Deploy KfDef
   
_KfDef_ can point to a remote URL or to a local kfdef file. To use the set of default kfdefs from Kubeflow, follow the [Deploy with default kfdefs](#deploy-with-default-kfdefs) section below.

```shell
KUBEFLOW_NAMESPACE=kubeflow
kubectl create ns ${KUBEFLOW_NAMESPACE}
kubectl create -f <kfdef> -n ${KUBEFLOW_NAMESPACE}
```

### Deploy with default kfdefs

To use the set of default kfdefs from Kubeflow, you will have to insert the `metadata.name` field before you can apply it to Kubernetes. Below are the commands for applying the Kubeflow _kfdef_ using Operator. For e.g. for IBM Cloud, commands will be
> If you are pointing the kfdef file on the local machine, set the `KFDEF` to the kfdef file path and skip the `curl` command.

First point to your Cloud provider kfdef. For e.g. for OpenShift, point to the kfdef in OpenDataHub repo

```shell
export KFDEF_URL=https://raw.githubusercontent.com/opendatahub-io/manifests/v0.7-branch-openshift/kfdef/kfctl_openshift.yaml
```

Similary for GCP, IBM Cloud etc. you can point to the respective kfdefs in Kubeflow repository, e.g.

```shell
export KFDEF_URL=https://raw.githubusercontent.com/kubeflow/manifests/master/kfdef/kfctl_ibm.yaml
```

Then specify the `KUBEFLOW_DEPLOYMENT_NAME` you want to give to your deployment. Please note that currently multi-user deployments have a hard dependency on using `kubeflow` as the deployment name.

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

One of the major benefits of using kfctl as an Operator is to leverage the functionalities around being able to watch and reconcile your Kubeflow deployments. The Operator is watching on any cluster events for the _KfDef_ instance, as well as the _Delete_ event for all the resources whose owner is the _KfDef_ instance. Each of such events is queued as a request for the _reconciler_ to apply changes to the _KfDef_ instance. For example, if one of the Kubeflow resources is deleted, the _reconciler_ will be triggered to re-apply the _KfDef_ instance, and re-create the deleted resource on the cluster. Therefore, the Kubeflow deployment with this _KfDef_ instance will recover automatically from the unexpected delete event.

Try following to see the operator watcher and reconciler in action:

1. Check the tf-job-operator deployment is running

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

3. Wait for 10 to 15 seconds, then check the tf-job-operator deployment again

You will be able to see that the deployment is being recreated by the Operator's reconciliation logic.
 
```Shell
kubectl get deploy -n ${KUBEFLOW_NAMESPACE} tf-job-operator
# NAME                                          READY   UP-TO-DATE   AVAILABLE   AGE
# tf-job-operator                               0/1     0            0           10s
```

The Kubeflow operator also support multiple _KfDef_ instances deployment. It watches over all the _KfDef_ instances and handles reconcile requests to all the _KfDef_ instances. To understand more on the operator controller behavior, refer to this [controller-runtime link](https://github.com/kubernetes-sigs/controller-runtime/blob/master/pkg/doc.go).

The operator responds to following events:

* When a _KfDef_ instance is created or updated, the operator's _reconciler_ will be notified of the event and invoke the `Apply` function provided by the [`kfctl` package](https://github.com/kubeflow/kfctl/tree/master/pkg) to deploy Kubeflow. The Kubeflow resources specified with the manifests will be added with the following annotation to indicate that they are owned by this _KfDef_ instance.
  
  ```
  annotations:
    kfctl.kubeflow.io/kfdef-instance: <kfdef-name>.<kfdef-namespace>
  ```
  

* When a _KfDef_ instance is deleted, the operator's _reconciler_ will be notified of the event and invoke the finalizer to run the `Delete` function provided by the [`kfctl` package](https://github.com/kubeflow/kfctl/tree/master/pkg) and go through all applications and components owned by the _KfDef_ instance.

* When any resource deployed as part of a _KfDef_ instance is deleted, the operator's _reconciler_ will be notified of the event and invoke the `Apply` function provided by the [`kfctl` package](https://github.com/kubeflow/kfctl/tree/master/pkg) to re-deploy Kubeflow. The deleted resource will be recreated with the same manifest which was specified when the _KfDef_ instance was created.

## Delete Kubeflow

* Delete Kubeflow deployment, the _KfDef_ instance

```shell
kubectl delete kfdef -n ${KUBEFLOW_NAMESPACE} --all
```

> Note that the users profile namespaces created by `profile-controller` will not be deleted. The `${KUBEFLOW_NAMESPACE}` created outside of the operator will not be deleted either. The delete process usually takes up to 15 minutes because the Operator needs to delete each component sequentially to avoid race conditions such as the [namespace finalizer issue](https://github.com/kubeflow/kfctl/issues/404).

* Delete Kubeflow Operator

```shell
kubectl delete -f deploy/operator.yaml -n ${OPERATOR_NAMESPACE}
kubectl delete clusterrolebinding kubeflow-operator
kubectl delete -f deploy/service_account.yaml -n ${OPERATOR_NAMESPACE}
kubectl delete -f deploy/crds/kfdef.apps.kubeflow.org_kfdefs_crd.yaml
kubectl delete ns ${OPERATOR_NAMESPACE}
```

## Optional: Registering the Operator to OLM Catalog

Please follow the instructions [here](https://github.com/operator-framework/community-operators/blob/master/docs/testing-operators.md#testing-operator-deployment-on-openshift) to register your Operator to OLM if you are using that to install and manage the Operator. If you want to leverage the OperatorHub, please use the default [Kubeflow Operator registered there](https://operatorhub.io/operator/kubeflow)

## Trouble Shooting

* When deleting a Kubeflow deployment, some _mutatingwebhookconfigurations_ may not be removed as they are cluster-wide resources and dynamically created by the individual controller. It's a [known issue](https://github.com/kubeflow/manifests/issues/1379) for some of the Kubeflow components. To remove them, run the following:

```shell
kubectl delete mutatingwebhookconfigurations katib-mutating-webhook-config
kubectl delete mutatingwebhookconfigurations cache-webhook-kubeflow
```

## Development Instructions

### Prerequisites

1. Install [operator-sdk](https://github.com/operator-framework/operator-sdk/blob/master/doc/user/install-operator-sdk.md)

2. Install [golang](https://golang.org/dl/)

3. Install [kustomize](https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md)

### Build Instructions

These steps are based on the [operator-sdk](https://github.com/operator-framework/operator-sdk/blob/master/doc/user-guide.md) with modifications that are specific for this Kubeflow operator.

1. Clone this repository under your `$GOPATH`. (e.g. `~/go/src/github.com/kubeflow/`)

```shell
git clone https://github.com/kubeflow/kfctl
cd kfctl
```

2. Build and push the operator

```shell
export OPERATOR_IMG=<docker_repo>
make build-operator
make push-operator
```

> Note: replace **<docker_repo>** with the image repo name and tag, for example, `docker.io/example/kubeflow-operator:latest`.

3. Follow [Deployment Instructions](#deployment-instructions) section to test the operator with the newly built image

## Current Tested Operators and Pre-built Images

Kubeflow Operator controller logic is based on the [`kfctl` package](https://github.com/kubeflow/kfctl/tree/master/pkg), so for each major release of `kfctl`, an operator image is built and tested with that version of [`manifests`](github.com/kubeflow/manifests) to deploy a _KfDef_ instance. Following table shows what releases have been tested.

|branch tag|operator image|manifests version|kfdef example|note|
|---|---|---|---|---|
|[v1.0](https://github.com/kubeflow/kfctl/tree/v1.0)|[aipipeline/kubeflow-operator:v1.0.0](https://hub.docker.com/layers/aipipeline/kubeflow-operator/v1.0.0/images/sha256-63d00b29a61ff5bc9b0527c8a515cd4cb55de474c45d8e0f65742908ede4d88f?context=repo)|[1.0.0](https://github.com/kubeflow/manifests/tree/f56bb47d7dc5378497ad1e38ea99f7b5ebe7a950)|[kfctl_k8s_istio.v1.0.0.yaml](https://github.com/kubeflow/manifests/blob/f56bb47d7dc5378497ad1e38ea99f7b5ebe7a950/kfdef/kfctl_k8s_istio.v1.0.0.yaml)||
|[v1.0.1](https://github.com/kubeflow/kfctl/tree/v1.0.1)|[aipipeline/kubeflow-operator:v1.0.1](https://hub.docker.com/layers/aipipeline/kubeflow-operator/v1.0.1/images/sha256-828024b773040271e4b547ce9219046f705fb7123e05503d5a2d1428dfbcfb6e?context=repo)|[1.0.1](https://github.com/kubeflow/manifests/tree/v1.0.1)|[kfctl_k8s_istio.v1.0.1.yaml](https://github.com/kubeflow/manifests/blob/v1.0.1/kfdef/kfctl_k8s_istio.v1.0.1.yaml)||
|[v1.0.2](https://github.com/kubeflow/kfctl/tree/v1.0.2)|[aipipeline/kubeflow-operator:v1.0.2](https://hub.docker.com/layers/aipipeline/kubeflow-operator/v1.0.2/images/sha256-18d2ca6f19c1204d5654dfc4cc08032c168e89a95dee68572b9e2aaedada4bda?context=repo)|[1.0.2](https://github.com/kubeflow/manifests/tree/v1.0.2)|[kfctl_k8s_istio.v1.0.2.yaml](https://github.com/kubeflow/manifests/blob/v1.0.2/kfdef/kfctl_k8s_istio.v1.0.2.yaml)||
|[master](https://github.com/kubeflow/kfctl)|[aipipeline/kubeflow-operator:master](https://hub.docker.com/layers/aipipeline/kubeflow-operator/master/images/sha256-e81020c426a12237c7cf84316dbbd0efda76e732233ddd57ef33543381dfb8a1?context=repo)|[master](https://github.com/kubeflow/manifests)|[kfctl_k8s_istio.yaml](https://github.com/kubeflow/manifests/blob/master/kfdef/kfctl_k8s_istio.yaml)|as of 05/15/2020|

> Note: if building a customized operator for a specific version of Kubeflow is desired, you can run `git checkout` to that specific branch tag. Keep in mind to use the matching version of manifests.
