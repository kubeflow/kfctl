local params = import '../../components/params.libsonnet';

params + {
  components+: {
    // Insert component parameter overrides here. Ex:
    // guestbook +: {
    // name: "guestbook-dev",
    // replicas: params.global.replicas,
    // },
    workflows+: {
      bucket: 'kubeflow-releasing-artifacts',
      gcpCredentialsSecretName: 'gcp-credentials',
      name: 'jlewi-kubeflow-kfctl-release-403-2f58',
      namespace: 'kubeflow-releasing',
      project: 'kubeflow-releasing',
      prow_env: 'JOB_NAME=kubeflow-kfctl-release,JOB_TYPE=presubmit,PULL_NUMBER=403,REPO_NAME=kubeflow-kfctl,REPO_OWNER=kubeflow,BUILD_NUMBER=2f58',
      versionTag: 'v20180226-403',
      zone: 'us-central1-a',
    },
    unit_tests+: {
      name: 'kubeflow-kfctl-release-111-2222',
      namespace: 'kubeflow-test-infra',
      prow_env: 'JOB_NAME=kubeflow-kfctl-aws-utest,JOB_TYPE=presubmit,PULL_NUMBER=111,REPO_NAME=kfctl,REPO_OWNER=kubeflow,BUILD_NUMBER=2222',
      versionTag: 'v20200911-111',
    },
  },
}
