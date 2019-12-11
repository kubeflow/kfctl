import logging
import os

import pytest

from kubernetes import client as k8s_client
from kubeflow.kfctl.testing.util import kfctl_go_test_utils as kfctl_util
from kubeflow.testing import util

def test_build_kfctl_go(record_xml_attribute, app_name, app_path, project, use_basic_auth,
                        use_istio, config_path, build_and_apply, kfctl_repo_path,
                        cluster_creation_script, self_signed_cert, values):
  """Test building and deploying Kubeflow.

  Args:
    app_name: kubeflow deployment name.
    app_path: The path to the Kubeflow app.
    project: The GCP project to use.
    use_basic_auth: Whether to use basic_auth.
    use_istio: Whether to use Istio or not
    config_path: Path to the KFDef spec file.
    cluster_creation_script: script invoked to create a new cluster
    build_and_apply: whether to build and apply or apply
    kfctl_repo_path: path to the kubeflow/kfctl repo.
    self_signed_cert: whether to use self-signed cert for ingress.
    values: Comma separated list of variables to substitute into config_path
  """
  util.set_pytest_junit(record_xml_attribute, "test_build_kfctl_go")

  # Need to activate account for scopes.
  if os.getenv("GOOGLE_APPLICATION_CREDENTIALS"):
    util.run([
        "gcloud", "auth", "activate-service-account",
        "--key-file=" + os.environ["GOOGLE_APPLICATION_CREDENTIALS"]
    ])

  # TODO(yanniszark): split this into a separate workflow step
  if cluster_creation_script:
      logging.info("Cluster creation script specified: %s", cluster_creation_script)
      util.run(["/bin/bash", "-c", cluster_creation_script])


  logging.info("using kfctl repo: %s" % kfctl_repo_path)

  if values:
    pairs = values.split(",")
    path_vars = {}
    for p in pairs:
      k, v = p.split("=")
      path_vars[k] = v

    config_path = config_path.format(**path_vars)
    logging.info("config_path after substitution: %s", config_path)

  kfctl_path = kfctl_util.build_kfctl_go(kfctl_repo_path)
  app_path = kfctl_util.kfctl_deploy_kubeflow(
                  app_path, project, use_basic_auth,
                  use_istio, config_path, kfctl_path, build_and_apply)
  if not cluster_creation_script:
      kfctl_util.verify_kubeconfig(app_path)

  # Use self-signed cert for testing to prevent quota limiting.
  if self_signed_cert:
    logging.info("Configuring self signed certificate")
    util.load_kube_credentials()
    api_client = k8s_client.ApiClient()
    ingress_namespace = "istio-system"
    ingress_name = "envoy-ingress"
    tls_endpoint = "{0}.endpoints.{1}.cloud.goog".format(app_name, project)
    logging.info("Configuring self signed cert for %s", tls_endpoint)
    util.use_self_signed_for_ingress(ingress_namespace, ingress_name,
                                     tls_endpoint, api_client)

if __name__ == "__main__":
  logging.basicConfig(
      level=logging.INFO,
      format=('%(levelname)s|%(asctime)s'
              '|%(pathname)s|%(lineno)d| %(message)s'),
      datefmt='%Y-%m-%dT%H:%M:%S',
  )
  logging.getLogger().setLevel(logging.INFO)
  pytest.main()
