# TODO(jlewi): This code should probably move to kubeflow/testing repo.
# Might also want to split it up into multiple test files.
import logging
import os
import yaml

import googleapiclient.discovery
from oauth2client.client import GoogleCredentials

import pytest

from kubeflow.testing import util
from kubeflow.kfctl.testing.util import deploy_utils

def set_logging():
  logging.basicConfig(level=logging.INFO,
                      format=('%(levelname)s|%(asctime)s'
                              '|%(pathname)s|%(lineno)d| %(message)s'),
                      datefmt='%Y-%m-%dT%H:%M:%S',
                      )
  logging.getLogger().setLevel(logging.INFO)

# TODO(jlewi): We should probably deprecate this and find a better way
# to separate platform specific logic. With GCP and blueprints we won't
# have a KFDef. And when running periodically against auto-deployments
# we also won't have access to the kfapp.
def get_platform_app_name(app_path):
  if not app_path:
    logging.info("--app_path not set; won't use KFDef to set platform")
    return "", ""

  with open(os.path.join(app_path, "tmp.yaml")) as f:
    kfdef = yaml.safe_load(f)
  app_name = kfdef["metadata"]["name"]
  platform = ""
  apiVersion = kfdef["apiVersion"].strip().split("/")
  if len(apiVersion) != 2:
    raise RuntimeError("Invalid apiVersion: " + kfdef["apiVersion"].strip())
  if apiVersion[-1] == "v1alpha1":
    platform = kfdef["spec"]["platform"]
  elif apiVersion[-1] in ["v1beta1", "v1"]:
    for plugin in kfdef["spec"].get("plugins", []):
      if plugin.get("kind", "") == "KfGcpPlugin":
        platform = "gcp"
      elif plugin.get("kind", "") == "KfExistingArriktoPlugin":
        platform = "existing_arrikto"
  else:
    raise RuntimeError("Unknown version: " + apiVersion[-1])
  return platform, app_name

def check_deployments_ready(record_xml_attribute, namespace, name, deployments):
  """Test that Kubeflow deployments are successfully deployed.

  Args:
    namespace: The namespace Kubeflow is deployed to.
  """
  set_logging()
  # TODO(jlewi): Should we do this in the calling function)?
  util.set_pytest_junit(record_xml_attribute, name)

  # Need to activate account for scopes.
  if os.getenv("GOOGLE_APPLICATION_CREDENTIALS"):
    util.run(["gcloud", "auth", "activate-service-account",
              "--key-file=" + os.environ["GOOGLE_APPLICATION_CREDENTIALS"]])

  api_client = deploy_utils.create_k8s_client()

  util.load_kube_config()

  for deployment_name in deployments:
    logging.info("Verifying that deployment %s started...", deployment_name)
    util.wait_for_deployment(api_client, namespace, deployment_name, 10)

def check_statefulsets_ready(record_xml_attribute, namespace, name, stateful_sets):
  """Test that Kubeflow deployments are successfully deployed.

  Args:
    namespace: The namespace to check
  """
  set_logging()
  # TODO(jlewi): Should we do this in the calling function)?
  util.set_pytest_junit(record_xml_attribute, name)

  # Need to activate account for scopes.
  if os.getenv("GOOGLE_APPLICATION_CREDENTIALS"):
    util.run(["gcloud", "auth", "activate-service-account",
              "--key-file=" + os.environ["GOOGLE_APPLICATION_CREDENTIALS"]])

  api_client = deploy_utils.create_k8s_client()

  util.load_kube_config()

  for set_name in stateful_sets:
    logging.info("Verifying that stateful set %s.%s started...", namespace,
                 set_name)
    try:
      util.wait_for_statefulset(api_client, namespace, set_name)
    except:
      # Collect debug information by running describe
      util.run(["kubectl", "-n", namespace, "describe", "statefulsets",
                set_name])
      raise

def test_katib_is_ready(record_xml_attribute, namespace):
  deployment_names = [
    "katib-controller",
    "katib-mysql",
    "katib-db-manager",
    "katib-ui",
  ]
  check_deployments_ready(record_xml_attribute, namespace,
                          "test_katib_is_ready", deployment_names)

def test_metadata_is_ready(record_xml_attribute, namespace):
  deployment_names = [
    "metadata-deployment",
    "metadata-grpc-deployment",
    "metadata-db",
    "metadata-ui",
  ]
  check_deployments_ready(record_xml_attribute, namespace,
                          "test_metadata_is_ready", deployment_names)

def test_pipeline_is_ready(record_xml_attribute, namespace):
  deployment_names = [
    "argo-ui",
    "workflow-controller",
    "minio",
    "mysql",
    "ml-pipeline",
    "ml-pipeline-persistenceagent",
    "ml-pipeline-scheduledworkflow",
    "ml-pipeline-ui",
    "ml-pipeline-viewer-crd",
    "ml-pipeline-visualizationserver",
    "cache-deployer-deployment",
    "cache-server",
  ]
  check_deployments_ready(record_xml_attribute, namespace,
                          "test_pipeline_is_ready", deployment_names)

def test_notebook_is_ready(record_xml_attribute, namespace):
  deployment_names = [
    "jupyter-web-app-deployment",
    "notebook-controller-deployment",
  ]
  check_deployments_ready(record_xml_attribute, namespace,
                          "test_notebook_is_ready", deployment_names)

def test_centraldashboard_is_ready(record_xml_attribute, namespace):
  check_deployments_ready(record_xml_attribute, namespace,
                          "test_centraldashboard_is_ready",
                          ["centraldashboard"])

def test_profiles_is_ready(record_xml_attribute, namespace):
  check_deployments_ready(record_xml_attribute, namespace,
                          "test_profile_is_ready", ["profiles-deployment"])

def test_pytorch_is_ready(record_xml_attribute, namespace):
  check_deployments_ready(record_xml_attribute, namespace,
                          "test_pytorch_is_ready", ["pytorch-operator"])

def test_tf_job_is_ready(record_xml_attribute, namespace):
  check_deployments_ready(record_xml_attribute, namespace,
                          "test_tf_job_is_ready", ["tf-job-operator"])

def test_istio_is_ready(record_xml_attribute):
  # Starting with 1.1 on GCP at least istio-egressgateway is no longer
  # included by default
  istio_deployments = [
    "istio-ingressgateway",
    "istio-pilot",
    "istio-sidecar-injector",
  ]

  namespace = "istio-system"
  check_deployments_ready(record_xml_attribute, namespace,
                          "test_istio_is_ready", istio_deployments)

def test_knative_is_deployed(record_xml_attribute, platform):

  namespace = "knative-serving"
  deployments = [
          "activator",
          "autoscaler",
          "controller",
  ]

  if platform == "existing_arrikto":
    pytest.skip("knative tests skipped on existing_arrikto")
    return

  check_deployments_ready(record_xml_attribute, namespace,
                          "test_knative_is_deployed", deployments)

  stateful_sets = ["kfserving-controller-manager"]
  check_statefulsets_ready(record_xml_attribute, namespace,
                           "test_knative_is_deployed", stateful_sets)

def test_dex_is_deployed(record_xml_attribute, app_path):
  platform, _ = get_platform_app_name(app_path)

  namespace = "istio-system"
  # knative tests
  if platform != "existing_arrikto":
    pytest.skip("knative tests skipped unless platform=existing_arrikto")
    return

  deployments = ["dex", "authservice"]

  check_deployments_ready(record_xml_attribute, namespace,
                          "test_dex_is_deployed", deployments)

def test_gcp_ingress_services(record_xml_attribute, namespace, platform):
  """Test that Kubeflow was successfully deployed.

  Args:
    namespace: The namespace Kubeflow is deployed to.
  """
  namespace = "istio-system"

  if platform != "gcp":
    pytest.skip("Not running on GCP")
    return

  deployments = ["cloud-endpoints-controller", "iap-enabler"]
  stateful_sets = ["backend-updater"]

  name = "test_gcp_ingress_services"
  check_deployments_ready(record_xml_attribute, namespace,
                          name, deployments)


  check_statefulsets_ready(record_xml_attribute, namespace,
                           name, stateful_sets)

def test_gcp_access(record_xml_attribute, namespace, platform, project):
  """Test that Kubeflow gcp was configured with workload_identity and GCP service account credentails.

  Args:
    namespace: The namespace Kubeflow is deployed to.
  """
  set_logging()
  util.set_pytest_junit(record_xml_attribute, "test_gcp_access")

  # Need to activate account for scopes.
  if os.getenv("GOOGLE_APPLICATION_CREDENTIALS"):
    util.run(["gcloud", "auth", "activate-service-account",
              "--key-file=" + os.environ["GOOGLE_APPLICATION_CREDENTIALS"]])

  api_client = deploy_utils.create_k8s_client()

  if platform != "gcp":

    pytest.skip("Not running on GCP")
    return

  # check secret
  util.check_secret(api_client, namespace, "user-gcp-sa")

  cred = GoogleCredentials.get_application_default()
  # Create the Cloud IAM service object
  service = googleapiclient.discovery.build('iam', 'v1', credentials=cred)

  userSa = 'projects/%s/serviceAccounts/%s-user@%s.iam.gserviceaccount.com' % (project, app_name, project)
  adminSa = 'serviceAccount:%s-admin@%s.iam.gserviceaccount.com' % (app_name, project)

  request = service.projects().serviceAccounts().getIamPolicy(resource=userSa)
  response = request.execute()
  roleToMembers = {}
  for binding in response['bindings']:
    roleToMembers[binding['role']] = set(binding['members'])

  if 'roles/owner' not in roleToMembers:
    raise Exception("roles/owner missing in iam-policy of %s" % userSa)

  if adminSa not in roleToMembers['roles/owner']:
    raise Exception("Admin %s should be owner of user %s" % (adminSa, userSa))

  workloadIdentityRole = 'roles/iam.workloadIdentityUser'
  if workloadIdentityRole not in roleToMembers:
    raise Exception("roles/iam.workloadIdentityUser missing in iam-policy of %s" % userSa)

if __name__ == "__main__":
  logging.basicConfig(level=logging.INFO,
                      format=('%(levelname)s|%(asctime)s'
                              '|%(pathname)s|%(lineno)d| %(message)s'),
                      datefmt='%Y-%m-%dT%H:%M:%S',
                      )
  logging.getLogger().setLevel(logging.INFO)
  # DO NOT SUBMIT
  #test_notebook_is_ready("jupyter", "kubeflow")
  pytest.main()
