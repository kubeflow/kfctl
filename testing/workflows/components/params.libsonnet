{
  global: {
    // User-defined global parameters; accessible to all component and environments, Ex:
    // replicas: 4,
  },
  components: {
    kfctl_go_test: {
      bucket: "kubeflow-ci_temp",
      name: "somefakename",
      namespace: "kubeflow-test-infra",
      prow_env: "",
      deleteKubeflow: true,
      gkeApiVersion: "v1",
      workflowName: "kfctl-go",
      useBasicAuth: "false",
      useIstio: "true",
      configPath: "bootstrap/config/kfctl_gcp_iap.yaml",
    },
  },
}
