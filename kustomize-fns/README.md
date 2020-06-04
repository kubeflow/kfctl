# kpt/kustomize FNs

This directory contains [kpt functions](https://googlecontainertools.github.io/kpt/reference/fn/)
for apply various transformations to your YAMLs.

These kustomizations are intended to be applied using kpt. It looks like support for functions
is being upstreamed into kustomize and currently has alpha support.

## Usage 

The [kpt docs](https://googlecontainertools.github.io/kpt/reference/fn/run/#examples) describe
many ways to invoke kpt functions. For production the most common use will be the 
[declarative method](https://googlecontainertools.github.io/kpt/reference/fn/run/#declaratively-run-one-or-more-functions).

1. In the directory you want to apply the transform define a config specifying the transform

   * Depending on the transform this will either be a [ConfigMap](https://googlecontainertools.github.io/kpt/reference/fn/run/#declaratively-run-one-or-more-functions)
     or a custom resource
     
   * In both cases the annotation `config.kubernetes.io/function` specifies the
     docker image to use for the config.
     
   * As an example for the image prefix transform we define a config like the following
   
     ```
     apiVersion: v1alpha1 # Define a transform to change all the image prefixes to use images from a different registry
     kind: ImagePrefix
     metadata:
       name: use-mirror-images-gcr
       annotations:
         config.kubernetes.io/function: |
           container:
             image: gcr.io/kubeflow-images-public/kpt-fns:v1.0-rc.3-58-g616f986-dirty
     spec:
       imageMappings:
       - src: quay.io/jetstack
         dest: gcr.io/gcp-private-dev/jetstack # {"type":"string","x-kustomize":{"setBy":"kpt","partialSetters":[{"name":"gcloud.core.project","value":"gcp-private-dev"}]}}
       - src: gcr.io/kubeflow-images-public
         dest: gcr.io/gcp-private-dev # {"type":"string","x-kustomize":{"setBy":"kpt","partialSetters":[{"name":"gcloud.core.project","value":"gcp-private-dev"}]}}
     ```
     
1. Run kpt fn

   ```
   kpt fn ${DIR}
   ```  

   * ${DIR} will be the directory containing the YAML files to process and should include
     the YAMLs configuring the functions

## Development

During development it can be useful to run your transforms without building a docker image or
run them in a debugger.

To support this our main binary `kustomize-fns` includes a `debug` subcommand which
allows you to specify the path of a YAML file containing a `ResourceList` defining
the resources and functions to apply. You can produce that file using kpt; e.g.

```
  kpt fn source ${DIR} --function-config=${FN_CFG} 
```

 * *DIR* should be the directory containing the YAML files defining the resources to process
 * *FN_CFG* should be a path to a YAML file specifying one or more functions to apply
 

You can use skaffold to build the docker image

```
skaffold build --kube-context=${KUBE_CONTEXT}
```

You can build the binary by doing

```
go build .
```
