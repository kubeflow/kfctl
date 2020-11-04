local params = std.extVar("__ksonnet/params").components.unit_tests;

local k = import "k.libsonnet";
local util = import "util.libsonnet";

// TODO(jlewi): Can we get namespace from the environment rather than
// params?
local namespace = params.namespace;

local name = params.name;

local prowEnv = util.parseEnv(params.prow_env);
local bucket = params.bucket;

// mountPath is the directory where the volume to store the test data
// should be mounted.
local mountPath = "/mnt/" + "test-data-volume";
// testDir is the root directory for all data for a particular test run.
local testDir = mountPath + "/" + name;
// outputDir is the directory to sync to GCS to contain the output for this job.
local outputDir = testDir + "/output";
local artifactsDir = outputDir + "/artifacts";
// Source directory where all repos should be checked out
local srcRootDir = testDir + "/src";
// The directory containing the kubeflow/kfctl repo
local srcDir = srcRootDir + "/kubeflow/kfctl";
local kfctlPy = srcDir + "/py/kubeflow/kfctl";

local image = "527798164940.dkr.ecr.us-west-2.amazonaws.com/aws-kubeflow-ci/test-worker:latest";
local testing_image = "527798164940.dkr.ecr.us-west-2.amazonaws.com/aws-kubeflow-ci/test-worker:latest";

// The name of the NFS volume claim to use for test files.
local nfsVolumeClaim = "nfs-external";
// The name to use for the volume to use to contain test data.
local dataVolume = "kubeflow-test-volume";
local kubeflowPy = srcRootDir + "/kubeflow/kubeflow";
// The directory within the kubeflow_testing submodule containing
// py scripts to use.
local kubeflowTestingPy = srcRootDir + "/kubeflow/testing/py";

local cluster = params.cluster_name;

// Build an Argo template to execute a particular command.
// step_name: Name for the template
// command: List to pass as the container command.
// We use separate kubeConfig files for separate clusters
local buildTemplate(step_name, command, working_dir=null, env_vars=[], sidecars=[]) = {
  name: step_name,
  activeDeadlineSeconds: 1800,  // Set 30 minute timeout for each template
  workingDir: working_dir,
  container+: {
    command: command,
    image: image,
    workingDir: working_dir,
    // TODO(jlewi): Change to IfNotPresent.
    imagePullPolicy: "Always",
    env: [
      {
        // Add the source directories to the python path.
        name: "PYTHONPATH",
        value: kubeflowPy + ":" + kubeflowTestingPy + ":" + kfctlPy,
      },
      {
        // EKS cluster name
        name: "CLUSTER_NAME",
        value: cluster,
      },
      {
        name: "GITHUB_TOKEN",
        valueFrom: {
          secretKeyRef: {
            name: "github-token",
            key: "github_token",
          },
        },
      },
      {
        name: "AWS_ACCESS_KEY_ID",
        valueFrom: {
          secretKeyRef: {
            name: "aws-credentials",
            key: "AWS_ACCESS_KEY_ID",
          },
        },
      },
      {
        name: "AWS_SECRET_ACCESS_KEY",
        valueFrom: {
          secretKeyRef: {
            name: "aws-credentials",
            key: "AWS_SECRET_ACCESS_KEY",
          },
        },
      },
      {
        name: "AWS_DEFAULT_REGION",
        value: "us-west-2",
      },
      {
          // EKS Namespace
          name: "EKS_NAMESPACE",
          value: namespace,
      },
    ] + prowEnv + env_vars,
    volumeMounts: [
      {
        name: dataVolume,
        mountPath: mountPath,
      },
    ],
  },
  sidecars: sidecars,
};  // buildTemplate


// Create a list of dictionary.c
// Each item is a dictionary describing one step in the graph.
local dagTemplates = [
  {
    template: buildTemplate("checkout",
                            ["/usr/local/bin/checkout.sh", srcRootDir]),
    dependencies: null,
  },
  {
    template: buildTemplate("create-pr-symlink", [
      "python",
      "-m",
      "kubeflow.testing.cloudprovider.aws.prow_artifacts",
      "--artifacts_dir=" + outputDir,
      "create_pr_symlink_s3",
      "--bucket=" + bucket,
    ]),  // create-pr-symlink
    dependencies: ["checkout"],
  },  // create-pr-symlink
  {
    // Run the kfctl go unittests
    template: buildTemplate("go-kfctl-unit-tests", [
      "make",
      "test-junit",
    ], working_dir=srcDir,
       env_vars=[{
          name: "JUNIT_FILE",
          value: artifactsDir + "/junit_go-kfctl-unit-tests.xml",
       }],
       ) + {
    },  // go-kfctl-unit-tests
    dependencies: ["checkout"],
  },
];

// Each item is a dictionary describing one step in the graph
// to execute on exit
local exitTemplates = [
  {
    template: buildTemplate("copy-artifacts", [
      "python",
      "-m",
      "kubeflow.testing.cloudprovider.aws.prow_artifacts",
      "--artifacts_dir=" + outputDir,
      "copy_artifacts_to_s3",
      "--bucket=" + bucket,
    ]),  // copy-artifacts,
    dependencies: null,
  },
  {
    template:
      buildTemplate("test-dir-delete", [
        "python",
        "-m",
        "testing.util.run_with_retry",
        "--retries=5",
        "--",
        "rm",
        "-rf",
        testDir,
      ]),  // test-dir-delete
    dependencies: ["copy-artifacts"],
  },
];

// Dag defines the tasks in the graph
local dag = {
  name: "e2e",
  // Construct tasks from the templates
  // we will give the steps the same name as the template
  dag: {
    tasks: std.map(function(i) {
      name: i.template.name,
      template: i.template.name,
      dependencies: i.dependencies,
    }, dagTemplates),
  },
};  // dag

// The set of tasks in the exit handler dag.
local exitDag = {
  name: "exit-handler",
  // Construct tasks from the templates
  // we will give the steps the same name as the template
  dag: {
    tasks: std.map(function(i) {
      name: i.template.name,
      template: i.template.name,
      dependencies: i.dependencies,
    }, exitTemplates),
  },
};

// A list of templates for the actual steps
local stepTemplates = std.map(function(i) i.template
                              , dagTemplates) +
                      std.map(function(i) i.template
                              , exitTemplates);


// Add a task to a dag.
local workflow = {
  apiVersion: "argoproj.io/v1alpha1",
  kind: "Workflow",
  metadata: {
    name: name,
    namespace: namespace,
    labels: {
      org: "kubeflow",
      repo: "kfctl",
      workflow: "e2e",
      // TODO(jlewi): Add labels for PR number and commit. Need to write a function
      // to convert list of environment variables to labels.
    },
  },
  spec: {
    entrypoint: "e2e",
    volumes: [
      {
        name: "github-token",
        secret: {
          secretName: "github-token",
        },
      },
      {
        name: dataVolume,
        persistentVolumeClaim: {
          claimName: nfsVolumeClaim,
        },
      },
    ],  // volumes
    // onExit specifies the template that should always run when the workflow completes.
    onExit: "exit-handler",
    templates: [dag, exitDag] + stepTemplates,  // templates
  },  // spec
};  // workflow

std.prune(k.core.v1.list.new([workflow]))
