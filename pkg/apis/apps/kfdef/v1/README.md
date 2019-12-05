## `KfDef` defines an interface for controlling Kubeflow resources

In the following sections we explain what fields in `KfDefSpec` means.

### KfDefSpec.Applications

This is the list of applications that Kubeflow cluster will be installed.

- Name: Name identifier of the application.
- KustomizeConfig: Configurations for Kustomize to find the manifests and additional information to apply to.

#### KustomizeConfig

- RepoRef:
  - Name: Name of the repo Kustomize will be looking into.  The repo must be on the list of `KfDefSpec.Repos`.
  - Path: Relative path in the repo to find the manifests.
- Overlays: A list of names to be applied as overlays.
- Parameters: Name/value pair of parameters to override the parameters defined in manifests.  Example: [link](https://github.com/kubeflow/manifests/blob/master/profiles/base/params.env#L3-L4)

### KfDefSpec.Plugins

This is the list of plugins that Kubeflow will be run on top of.  For example, platforms like GCP/AWS are part of plugins.

- Definitions of plugins should go to [plugins folder](https://github.com/kubeflow/kfctl/tree/master/pkg/apis/apps/plugins).
- Plugins must have associated [kfapp handler](https://github.com/kubeflow/kfctl/tree/master/pkg/kfapp) for them to be applied.

### KfDefSpec.Secrets

This is a set of secrets Kubeflow needs during installation.

- LiteralSource: User provides the secret information as literal string into `KfDef` yaml file.  This is not recommended and we are planning to deprecate it.
- EnvSource: User provides the name of ENV var and we will use the value for create the secret.

### KfDefSpec.Repos

This is a list of GIT repositories that we will cache and use as reference during installations.

- Name: The name identifier of the repository cached.
- URI: The URI to download the repository.
