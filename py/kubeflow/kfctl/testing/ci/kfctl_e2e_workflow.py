""""Define the E2E workflow for kfctl.

Rapid iteration.

Here are some pointers for rapidly iterating on the workflow during development.

1. You can use the e2e_tool.py to directly launch the workflow on a K8s cluster.
   If you don't have CLI access to the kubeflow-ci cluster (most folks) then
   you would need to setup your own test cluster.

2. To avoid redeploying on successive runs set the following parameters
   --app_name=name for kfapp
   --delete_kubeflow=False

   Setting these parameters will cause the same KF deployment to be reused
   across invocations. As a result successive runs won't have to redeploy KF.

Example running with E2E tool

export PYTHONPATH=${PYTHONPATH}:${KFCTL_REPO}/py:${KUBEFLOW_TESTING_REPO}/py

python -m kubeflow.testing.e2e_tool apply \
  kubeflow.kfctl.testing.ci.kfctl_e2e_workflow.create_workflow
  --name=${USER}-kfctl-test-$(date +%Y%m%d-%H%M%S) \
  --namespace=kubeflow-test-infra \
  --test-endpoint=true \
  --kf-app-name=${KFAPPNAME} \
  --delete-kf=false
  --open-in-chrome=true

We set kf-app-name and delete-kf to false to allow reusing the deployment
across successive runs.

To use code from a pull request set the prow envariables; e.g.

export JOB_NAME="jlewi-test"
export JOB_TYPE="presubmit"
export BUILD_ID=1234
export PROW_JOB_ID=1234
export REPO_OWNER=kubeflow
export REPO_NAME=kubeflow
export PULL_NUMBER=4148
"""

import datetime
from kubeflow.testing import argo_build_util
from kubeflow.testing import util
import logging
import os
import uuid

# The name of the NFS volume claim to use for test files.
NFS_VOLUME_CLAIM = "nfs-external"
# The name to use for the volume to use to contain test data
DATA_VOLUME = "kubeflow-test-volume"

# This is the main dag with the entrypoint
E2E_DAG_NAME = "e2e"
EXIT_DAG_NAME = "exit-handler"

# This is a sub dag containing the suite of tests to run against
# Kubeflow deployment
TESTS_DAG_NAME = "gke-tests"

TEMPLATE_LABEL = "kfctl_e2e"

DEFAULT_REPOS = [
    "kubeflow/kfctl@HEAD",
    "kubeflow/kubeflow@HEAD",
    "kubeflow/testing@HEAD",
    "kubeflow/tf-operator@HEAD"
]

class Builder(object):
  def __init__(self, name=None, namespace=None,
               config_path=("https://raw.githubusercontent.com/kubeflow"
                            "/manifests/master/kfdef/kfctl_gcp_iap.yaml"),
               bucket=None,
               test_endpoint=False,
               use_basic_auth=False,
               build_and_apply=False,
               test_target_name=None,
               kf_app_name=None, delete_kf=True,
               extra_repos="",
               **kwargs):
    """Initialize a builder.

    Args:
      name: Name for the workflow.
      namespace: Namespace for the workflow.
      config_path: Path to the KFDef spec file.
      bucket: The bucket to upload artifacts to. If not set use default determined by prow_artifacts.py.
      test_endpoint: Whether to test the endpoint is ready.
      use_basic_auth: Whether to use basic_auth.
      test_target_name: (Optional) Name to use as the test target to group
        tests.
      kf_app_name: (Optional) Name to use for the Kubeflow deployment.
        If not set a unique name is assigned. Only set this if you want to
        reuse an existing deployment across runs.
      delete_kf: (Optional) Don't run the step to delete Kubeflow. Set to
        true if you want to leave the deployment up for some reason.
    """
    self.name = name
    self.namespace = namespace
    self.bucket = bucket
    self.config_path = config_path
    self.build_and_apply = build_and_apply
    #****************************************************************************
    # Define directory locations
    #****************************************************************************
    # mount_path is the directory where the volume to store the test data
    # should be mounted.
    self.mount_path = "/mnt/" + "test-data-volume"
    # test_dir is the root directory for all data for a particular test run.
    self.test_dir = self.mount_path + "/" + self.name
    # output_dir is the directory to sync to GCS to contain the output for this
    # job.
    self.output_dir = self.test_dir + "/output"

    # We prefix the artifacts directory with junit because
    # that's what spyglass/prow requires. This ensures multiple
    # instances of a workflow triggered by the same prow job
    # don't end up clobbering each other
    self.artifacts_dir = self.output_dir + "/artifacts/junit_{0}".format(name)

    # source directory where all repos should be checked out
    self.src_root_dir = self.test_dir + "/src"
    # The directory containing the kubeflow/kfctl repo
    self.src_dir = self.src_root_dir + "/kubeflow/kfctl"
    self.kubeflow_dir = self.src_root_dir + "/kubeflow/kubeflow"

    # Directory in kubeflow/kfctl containing the pytest files.
    self.kfctl_pytest_dir = os.path.join(self.src_dir, "py/kubeflow/kfctl/testing/pytests")

    # Top level directories for python testing code in kfctl.
    self.kfctl_py = os.path.join(self.src_dir, "py")

    # Build a string of key value pairs that can be passed to various test
    # steps to allow them to do substitution into different values.
    values = {
      "srcrootdir": self.src_root_dir,
    }

    value_pairs = ["{0}={1}".format(k,v) for k,v in values.items()]
    self.values_str = ",".join(value_pairs)

    # The directory within the kubeflow_testing submodule containing
    # py scripts to use.
    self.kubeflow_testing_py = self.src_root_dir + "/kubeflow/testing/py"

    self.tf_operator_root = os.path.join(self.src_root_dir,
                                         "kubeflow/tf-operator")
    self.tf_operator_py = os.path.join(self.tf_operator_root, "py")

    self.go_path = self.test_dir

    # Name for the Kubeflow app.
    # This needs to be unique for each test run because it is
    # used to name GCP resources
    # TODO(jlewi): Might be good to include pull number or build id in the name
    # Not sure if being non-deterministic is a good idea.
    # A better approach might be to hash the workflow name to generate a unique
    # name dependent on the workflow name. We know there will be one workflow
    # per cluster.
    self.uuid = uuid.uuid4().hex[0:4]

    # Config name is the name of the config file. This is used to give junit
    # files unique names.
    self.config_name = os.path.splitext(os.path.basename(config_path))[0]

    # The class name to label junit files.
    # We want to be able to group related tests in test grid.
    # Test grid allows grouping by target which corresponds to the classname
    # attribute in junit files.
    # So we set an environment variable to the desired class name.
    # The pytest modules can then look at this environment variable to
    # explicitly override the classname.
    # The classname should be unique for each run so it should take into
    # account the different parameters
    if test_target_name:
      self.test_target_name = test_target_name
    else:
      self.test_target_name = self.config_name

    # app_name is the name of the Kubeflow deployment.
    # This needs to be unique per run since we name GCP resources with it.
    self.app_name = kf_app_name
    if not self.app_name:
      self.app_name = "kfctl-" +  self.uuid

    self.delete_kf = delete_kf

    # GCP service accounts can only be max 30 characters. Service account names
    # are generated by taking the app_name and appending suffixes like "user"
    # and "admin"
    if len(self.app_name) > 20:
      raise ValueError(("app_name {0} is longer than 20 characters; this will"
                        "likely exceed GCP naming restrictions.").format(
                          self.app_name))
    # Directory for the KF app.
    self.app_dir = os.path.join(self.test_dir, self.app_name)
    self.use_basic_auth = use_basic_auth

    # The name space we create KF artifacts in; e.g. TFJob and notebooks.
    # TODO(jlewi): These should no longer be running the system namespace but
    # should move into the namespace associated with the default profile.
    self.steps_namespace = "kubeflow"
    self.test_endpoint = test_endpoint

    self.kfctl_path = os.path.join(self.src_dir, "bin/kfctl")

    # Fetch the main repo from Prow environment.
    self.main_repo = argo_build_util.get_repo_from_prow_env()

    # extra_repos is a list of comma separated repo names with commits,
    # in the format <repo_owner>/<repo_name>@<commit>,
    # e.g. "kubeflow/tf-operator@12345,kubeflow/manifests@23456".
    # This will be used to override the default repo branches.
    self.extra_repos = []
    if extra_repos:
      self.extra_repos = extra_repos.split(',')

    # Keep track of step names that subclasses might want to list as dependencies
    self._run_tests_step_name = None
    self._test_endpoint_step_name = None
    self._test_endpoint_template_name = None

  def _build_workflow(self):
    """Create the scaffolding for the Argo workflow"""
    workflow = {
      "apiVersion": "argoproj.io/v1alpha1",
      "kind": "Workflow",
      "metadata": {
        "name": self.name,
        "namespace": self.namespace,
        "labels": argo_build_util.add_dicts([{
            "workflow": self.name,
            "workflow_template": TEMPLATE_LABEL,
          }, argo_build_util.get_prow_labels()]),
      },
      "spec": {
        "entrypoint": E2E_DAG_NAME,
        # Have argo garbage collect old workflows otherwise we overload the API
        # server.
        "ttlSecondsAfterFinished": 7 * 24 * 60 * 60,
        "volumes": [
          {
            "name": "gcp-credentials",
            "secret": {
              "secretName": "kubeflow-testing-credentials",
            },
          },
          {
            "name": DATA_VOLUME,
            "persistentVolumeClaim": {
              "claimName": NFS_VOLUME_CLAIM,
            },
          },
        ],
        "onExit": EXIT_DAG_NAME,
        "templates": [
          {
           "dag": {
                "tasks": [],
                },
           "name": E2E_DAG_NAME,
          },
          {
           "dag":{
                 "tasks": [],
                },
           "name": TESTS_DAG_NAME,

          },
          {
            "dag": {
              "tasks": [],
              },
              "name": EXIT_DAG_NAME,
            }
        ],
      },  # spec
    } # workflow

    return workflow

  def _build_task_template(self):
    """Return a template for all the tasks"""

    task_template = {'activeDeadlineSeconds': 3000,
     'container': {'command': [],
      'env': [
        {"name": "GOOGLE_APPLICATION_CREDENTIALS",
         "value": "/secret/gcp-credentials/key.json"},
        {"name": "TEST_TARGET_NAME",
         "value": self.test_target_name},
       ],
      'image': 'gcr.io/kubeflow-ci/test-worker:latest',
      'imagePullPolicy': 'Always',
      'name': '',
      'resources': {'limits': {'cpu': '4', 'memory': '4Gi'},
       'requests': {'cpu': '1', 'memory': '1536Mi'}},
      'volumeMounts': [{'mountPath': '/mnt/test-data-volume',
        'name': 'kubeflow-test-volume'},
       {'mountPath': '/secret/gcp-credentials', 'name': 'gcp-credentials'}]},
     'metadata': {'labels': {
       'workflow_template': TEMPLATE_LABEL}},
     'outputs': {}}

    # Define common environment variables to be added to all steps
    common_env = [
      {'name': 'PYTHONPATH',
       'value': ":".join([self.kubeflow_testing_py,
                          self.kfctl_py,
                          self.tf_operator_py])},
      {'name': 'GOPATH',
        'value': self.go_path},
      {'name': 'KUBECONFIG',
       'value': os.path.join(self.test_dir, 'kfctl_test/.kube/kubeconfig')},
    ]

    task_template["container"]["env"].extend(common_env)

    task_template = argo_build_util.add_prow_env(task_template)

    return task_template

  def _build_step(self, name, workflow, dag_name, task_template,
                  command, dependences):
    """Syntactic sugar to add a step to the workflow"""

    step = argo_build_util.deep_copy(task_template)

    step["name"] = name
    step["container"]["command"] = command

    return argo_build_util.add_task_to_dag(workflow, dag_name, step, dependences)

  def _build_tests_dag(self):
    """Build the dag for the set of tests to run against a KF deployment."""

    task_template = self._build_task_template()

    #***************************************************************************
    # Test TFJob
    job_name = self.config_name.replace("_", "-").replace(".", "-")
    step_name = "tfjob-test"
    command = [
      "python",
      "-m",
      "kubeflow.tf_operator.simple_tfjob_tests",
      "--app_dir=" + os.path.join(self.tf_operator_root, "test/workflows"),
      "--tfjob_version=v1",
      # Name is used for the test case name so it should be unique across
      # all E2E tests.
      "--params=name=smoke-tfjob-" + job_name + ",namespace=" +
      self.steps_namespace,
      "--artifacts_path=" + self.artifacts_dir,
      # Skip GPU tests
      "--skip_tests=test_simple_tfjob_gpu",
    ]

    dependences = []
    tfjob_test = self._build_step(step_name, self.workflow, TESTS_DAG_NAME, task_template,
                                  command, dependences)

    #*************************************************************************
    # Test pytorch job
    step_name = "pytorch-job-deploy"
    command = ["pytest",
               "pytorch_job_deploy.py",
               "-s",
               "--timeout=600",
               "--junitxml=" + self.artifacts_dir + "/junit_pytorch-test.xml",
               "--kfctl_repo_path=" + self.src_dir,
               "--namespace=" + self.steps_namespace,
              ]

    dependences = []
    pytorch_test = self._build_step(step_name, self.workflow, TESTS_DAG_NAME, task_template,
                                    command, dependences)
    pytorch_test["container"]["workingDir"] = self.kfctl_pytest_dir
    #***************************************************************************
    # Notebook test
    step_name = "notebook-test"
    command =  ["pytest",
                "jupyter_test.py",
                "-s",
                "--timeout=500",
                "--junitxml=" + self.artifacts_dir + "/junit_jupyter-test.xml",
                "--kfctl_repo_path=" + self.src_dir,
                "--namespace=" + self.steps_namespace,
             ]

    dependences = []

    notebook_test = self._build_step(step_name, self.workflow, TESTS_DAG_NAME, task_template,
                                     command, dependences)
    notebook_test["container"]["workingDir"] = self.kfctl_pytest_dir

    #***************************************************************************
    # Profiles test

    step_name = "profiles-test"
    command =  ["pytest",
                "profiles_test.py",
                # I think -s mean stdout/stderr will print out to aid in debugging.
                # Failures still appear to be captured and stored in the junit file.
                "-s",
                # Test timeout in seconds.
                "--timeout=600",
                "--junitxml=" + self.artifacts_dir + "/junit_profiles-test.xml",
             ]

    dependences = []
    profiles_test = self._build_step(step_name, self.workflow, TESTS_DAG_NAME, task_template,
                                     command, dependences)

    profiles_test["container"]["workingDir"] =  os.path.join(
      self.kubeflow_dir, "py/kubeflow/kubeflow/ci")

    # ***************************************************************************
    # kfam test

    step_name = "kfam-test"
    command = ["pytest",
               "kfam_test.py",
               "-s",
               "--timeout=600",
               "--junitxml=" + self.artifacts_dir + "/junit_kfam-test.xml",
               ]

    dependences = []
    kfam_test = self._build_step(step_name, self.workflow, TESTS_DAG_NAME, task_template,
                                     command, dependences)

    kfam_test["container"]["workingDir"] = self.kfctl_pytest_dir

  def _build_exit_dag(self):
    """Build the exit handler dag"""
    task_template = self._build_task_template()

    #***********************************************************************
    # Delete Kubeflow
    step_name = "kfctl-delete-wrong-host"
    command = [
        "pytest",
        "kfctl_delete_wrong_cluster.py",
        "-s",
        "--log-cli-level=info",
        "--timeout=1000",
        "--junitxml=" + self.artifacts_dir + "/junit_kfctl-go-delete-wrong-cluster-test.xml",
        "--app_path=" + self.app_dir,
        "--kfctl_path=" + self.kfctl_path,
      ]
    if self.delete_kf:
      kfctl_delete_wrong_cluster = self._build_step(step_name, self.workflow, EXIT_DAG_NAME,
                                                    task_template,
                                                    command, [])
      kfctl_delete_wrong_cluster["container"]["workingDir"] = self.kfctl_pytest_dir

    step_name = "kfctl-delete"
    command = [
        "pytest",
        "kfctl_delete_test.py",
        "-s",
        "--log-cli-level=info",
        "--timeout=1000",
        "--junitxml=" + self.artifacts_dir + "/junit_kfctl-go-delete-test.xml",
        "--app_path=" + self.app_dir,
        "--kfctl_path=" + self.kfctl_path,
      ]

    if self.delete_kf:
      kfctl_delete = self._build_step(step_name, self.workflow, EXIT_DAG_NAME,
                                      task_template,
                                      command, ["kfctl-delete-wrong-host"])
      kfctl_delete["container"]["workingDir"] = self.kfctl_pytest_dir

    step_name = "copy-artifacts"
    command = ["python",
               "-m",
               "kubeflow.testing.prow_artifacts",
               "--artifacts_dir=" +
               self.output_dir,
               "copy_artifacts"]

    if self.bucket:
      command = append("--bucket=" + self.bucket)

    dependences = []
    if self.delete_kf:
      dependences = [kfctl_delete["name"]]

    copy_artifacts = self._build_step(step_name, self.workflow, EXIT_DAG_NAME, task_template,
                                      command, dependences)


    step_name = "test-dir-delete"
    command = ["python",
               "-m",
               "kubeflow.kfctl.testing.util.run_with_retry",
               "--retries=5",
               "--",
               "rm",
               "-rf",
               self.test_dir,]
    dependences = [copy_artifacts["name"]]
    copy_artifacts = self._build_step(step_name, self.workflow, EXIT_DAG_NAME, task_template,
                                      command, dependences)

    # We don't want to run from the directory we are trying to delete.
    copy_artifacts["container"]["workingDir"] = "/"

  def build(self):
    self.workflow = self._build_workflow()
    task_template = self._build_task_template()
    py3_template = argo_build_util.deep_copy(task_template)
    py3_template["container"]["image"] = "gcr.io/kubeflow-ci/test-worker-py3:e9afed1-dirty"

    #**************************************************************************
    # Checkout

    # create the checkout step

    checkout = argo_build_util.deep_copy(task_template)

    # Construct the list of repos to checkout
    list_of_repos = DEFAULT_REPOS
    list_of_repos.append(self.main_repo)
    list_of_repos.extend(self.extra_repos)
    repos = util.combine_repos(list_of_repos)
    repos_str = ','.join(['%s@%s' % (key, value) for (key, value) in repos.items()])


    # If we are using a specific branch (e.g. periodic tests for release branch)
    # then we need to use depth = all; otherwise checkout out the branch
    # will fail. Otherwise we checkout with depth=30. We want more than
    # depth=1 because the depth will determine our ability to find the common
    # ancestor which affects our ability to determine which files have changed
    depth = 30
    if os.getenv("BRANCH_NAME"):
      logging.info("BRANCH_NAME=%s; setting detph=all",
                   os.getenv("BRANCH_NAME"))
      depth = "all"

    checkout["name"] = "checkout"
    checkout["container"]["command"] = ["/usr/local/bin/checkout_repos.sh",
                                        "--repos=" + repos_str,
                                        "--depth={0}".format(depth),
                                        "--src_dir=" + self.src_root_dir]

    argo_build_util.add_task_to_dag(self.workflow, E2E_DAG_NAME, checkout, [])

    # Change the workfing directory for all subsequent steps
    task_template["container"]["workingDir"] = os.path.join(
      self.kfctl_pytest_dir)
    py3_template["container"]["workingDir"] = os.path.join(self.kfctl_pytest_dir)

    #**************************************************************************
    # Run build_kfctl and deploy kubeflow

    step_name = "kfctl-build-deploy"
    command = [
        "pytest",
        "kfctl_go_test.py",
        # I think -s mean stdout/stderr will print out to aid in debugging.
        # Failures still appear to be captured and stored in the junit file.
        "-s",
        "--app_name=" + self.app_name,
        "--config_path=" + self.config_path,
        "--values=" + self.values_str,
        "--build_and_apply=" + str(self.build_and_apply),
        # Increase the log level so that info level log statements show up.
        # TODO(https://github.com/kubeflow/testing/issues/372): If we
        # set a unique artifacts dir for each workflow with the proper
        # prefix that should work.
        "--log-cli-level=info",
        "--junitxml=" + self.artifacts_dir + "/junit_kfctl-build-test"
        + self.config_name + ".xml",
        # TODO(jlewi) Test suite name needs to be unique based on parameters.
        #
        "-o", "junit_suite_name=test_kfctl_go_deploy_" + self.config_name,
        "--app_path=" + self.app_dir,
        "--kfctl_repo_path=" + self.src_dir,
        "--self_signed_cert=True",
    ]

    dependences = [checkout["name"]]
    build_kfctl = self._build_step(step_name, self.workflow, E2E_DAG_NAME,
                                   py3_template, command, dependences)

    #**************************************************************************
    # Wait for Kubeflow to be ready
    step_name = "kubeflow-is-ready"
    command = [
           "pytest",
           "kf_is_ready_test.py",
           # I think -s mean stdout/stderr will print out to aid in debugging.
           # Failures still appear to be captured and stored in the junit file.
           "-s",
           # TODO(jlewi): We should update kf_is_ready_test to take the config
           # path and then based on the KfDef spec kf_is_ready_test should
           # figure out what to do.
           "--use_basic_auth={0}".format(self.use_basic_auth),
           # TODO(jlewi): We should be using ISTIO always so can we stop
           # setting this
           "--use_istio=true",
           # Increase the log level so that info level log statements show up.
           "--log-cli-level=info",
           "--junitxml=" + os.path.join(self.artifacts_dir,
                                        "junit_kfctl-is-ready-test-" +
                                        self.config_name + ".xml"),
           # Test suite name needs to be unique based on parameters
           "-o", "junit_suite_name=test_kf_is_ready_" + self.config_name,
           "--app_path=" + self.app_dir,
         ]

    dependences = [build_kfctl["name"]]
    kf_is_ready = self._build_step(step_name, self.workflow, E2E_DAG_NAME, task_template,
                                   command, dependences)


    #**************************************************************************
    # Wait for endpoint to be ready
    if self.test_endpoint:
      self._test_endpoint_step_name = "endpoint-is-ready"
      command = ["pytest",
                 "endpoint_ready_test.py",
                 # I think -s mean stdout/stderr will print out to aid in debugging.
                 # Failures still appear to be captured and stored in the junit file.
                 "-s",
                 # Increase the log level so that info level log statements show up.
                 "--log-cli-level=info",
                 "--junitxml=" + self.artifacts_dir + "/junit_endpoint-is-ready-test-" + self.config_name + ".xml",
                 # Test suite name needs to be unique based on parameters
                 "-o", "junit_suite_name=test_endpoint_is_ready_" + self.config_name,
                 "--app_path=" + self.app_dir,
                 "--app_name=" + self.app_name,
                 "--use_basic_auth={0}".format(self.use_basic_auth),
              ]

      dependencies = [build_kfctl["name"]]
      endpoint_ready = self._build_step(self._test_endpoint_step_name,
                                        self.workflow, E2E_DAG_NAME, py3_template,
                                        command, dependencies)
      self._test_endpoint_template_name = endpoint_ready["name"]

    #**************************************************************************
    # Do kfctl apply again. This test will be skip if it's presubmit.
    step_name = "kfctl-second-apply"
    command = [
           "pytest",
           "kfctl_second_apply.py",
           # I think -s mean stdout/stderr will print out to aid in debugging.
           # Failures still appear to be captured and stored in the junit file.
           "-s",
           "--log-cli-level=info",
           "--junitxml=" + os.path.join(self.artifacts_dir,
                                        "junit_kfctl-second-apply-test-" +
                                        self.config_name + ".xml"),
           # Test suite name needs to be unique based on parameters
           "-o", "junit_suite_name=test_kfctl_second_apply_" + self.config_name,
           "--app_path=" + self.app_dir,
           "--kfctl_path=" + self.kfctl_path,
         ]
    if self.test_endpoint:
      dependences = [kf_is_ready["name"], endpoint_ready["name"]]
    else:
      dependences = [kf_is_ready["name"]]

    kf_second_apply = self._build_step(step_name, self.workflow, E2E_DAG_NAME, task_template,
                                       command, dependences)

    self._build_tests_dag()

    # Add a task to run the dag
    dependencies = [kf_is_ready["name"]]
    self._run_tests_step_name = TESTS_DAG_NAME
    run_tests_template_name = TESTS_DAG_NAME
    argo_build_util.add_task_only_to_dag(self.workflow, E2E_DAG_NAME, self._run_tests_step_name,
                                         run_tests_template_name,
                                         dependencies)

    #***************************************************************************
    # create_pr_symlink
    #***************************************************************************
    # TODO(jlewi): run_e2e_workflow.py should probably create the PR symlink
    step_name = "create-pr-symlink"
    command = ["python",
               "-m",
               "kubeflow.testing.prow_artifacts",
               "--artifacts_dir=" + self.output_dir,
               "create_pr_symlink"]

    if self.bucket:
      command.append(self.bucket)

    dependences = [checkout["name"]]
    symlink = self._build_step(step_name, self.workflow, E2E_DAG_NAME, task_template,
                               command, dependences)

    self._build_exit_dag()


    # Set the labels on all templates
    self.workflow = argo_build_util.set_task_template_labels(self.workflow)

    return self.workflow

# TODO(jlewi): This is an unnecessary layer of indirection around the builder
# We should allow py_func in prow_config to point to the builder and
# let e2e_tool take care of this.
def create_workflow(**kwargs): # pylint: disable=too-many-statements
  """Create workflow returns an Argo workflow to test kfctl upgrades.

  Args:
    name: Name to give to the workflow. This can also be used to name things
     associated with the workflow.
  """

  builder = Builder(**kwargs)

  return builder.build()
