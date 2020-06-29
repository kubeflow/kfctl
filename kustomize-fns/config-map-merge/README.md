# Merge ConfigMap to an application

For certain applications, users need to provide custom values for configurations defined
in the ConfigMap. Once `KfDef` is conformed to kustomize v3, it will not allow users to
specify the `parameters`. Therefore, when deploying with `kfctl` command line, there is
a need for extra steps to merge the users-provided configurations with the default from
the `KfDef`. An example usage is `dex-auth` where a department/organization specific
password/secret should only merge with the deployment for that department/organization.
`kpt fn` is a convenient and good approach for this purpose.

This function requires a configuration file defined such as follow:

```yaml
apiVersion: v1alpha1
kind: ConfigMapMerge
metadata:
  name: dex-auth-merge
  annotations:
    config.kubernetes.io/function: |
      container:
        image: aipipeline/kpt-fn:latest
spec:
  configMaps:
  - name: dex-parameters
    behavior: merge
    literals:
    - application_secret="<the-client-secret-of-oidc-service>"
    - github_client_id="<github oauth app client id>"
    - github_client_secret="<github oauth app client secret>"
    - github_hostname="github.ibm.com"
    - github_org_name="whitewater-analytics"
    - oidc_provider="http://dex.auth.svc.cluster.local:5556/dex"
    - oidc_redirect_uris='["/login/oidc"]'
```

This file will be copied to the application's directory where its `kustomization.yaml`
file locates.

Then run the following command

```shell
kpt fn run .
```

to merge the configurations.
