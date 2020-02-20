import datetime
import json
import logging
import os
import subprocess
import tempfile
import uuid
from retrying import retry

import pytest

from kubeflow.testing import util
from kubeflow.kfctl.testing.util import deploy_utils
from kubeflow.kfctl.testing.util import gcp_util

# There's really no good reason to run test_endpoint during presubmits.
# We shouldn't need it to feel confident that kfctl is working.
@pytest.mark.skipif(os.getenv("JOB_TYPE") == "presubmit",
                    reason="test endpoint doesn't run in presubmits")
def test_endpoint_is_ready(record_xml_attribute, project, app_path, app_name, use_basic_auth):
  """Test that Kubeflow was successfully deployed.

  Args:
    project: The gcp project that we deployed kubeflow
    app_name: The name of the kubeflow deployment
  """
  util.set_pytest_junit(record_xml_attribute, "test_endpoint_is_ready")

  url = "https://{}.endpoints.{}.cloud.goog".format(app_name, project)
  if use_basic_auth:
    with open(os.path.join(app_path, "login.json"), "r") as f:
      login = json.load(f)
      # Let it fail if login info cannot be found.
      username = login["username"]
      password = login["password"]
    if not gcp_util.basic_auth_is_ready(url, username, password):
      raise Exception("Basic auth endpoint is not ready")
  else:
    # Owned by project kubeflow-ci-deployment.
    os.environ["CLIENT_ID"] = "29647740582-7meo6c7a9a76jvg54j0g2lv8lrsb4l8g.apps.googleusercontent.com"
    if not gcp_util.iap_is_ready(url):
      raise Exception("IAP endpoint is not ready")

# Todo(jlewi): Should this only run in postsubmit?
@pytest.mark.skipif(os.getenv("JOB_TYPE") == "presubmit",
                    reason="test endpoint doesn't run in presubmits")
def test_central_dash_is_running(record_xml_attribute, app_path, kf_host, client_id, service_acct_email, service_acct_key_file):
  """Test that Kubeflow Central Dash was successfully deployed."""
  util.set_pytest_junit(record_xml_attribute, "test_central_dash_is_running")
  os.environ["KF_HOST"] = kf_host       # The url to your cluster, usually: https://<cluster>.endpoints.<project>.cloud.goog/
  os.environ["CLIENT_ID"] = client_id   # This is the CLIENT_ID token used to create the cluster (used in http://deploy.kubeflow.cloud/)
  os.environ["SERVICE_ACCOUNT_EMAIL"] = service_acct_email  # The email address for the service account you created <name>@<project>.iam.gserviceaccount.com
  os.environ["SERVICE_ACCOUNT_KEY"] = service_acct_key_file # Should be a JSON file downloaded from cloud console service accounts

  subprocess.check_call('cd "{}" && npm run test-e2e'.format(app_path), shell=True)

if __name__ == "__main__":
  logging.basicConfig(level=logging.INFO,
                      format=('%(levelname)s|%(asctime)s'
                              '|%(pathname)s|%(lineno)d| %(message)s'),
                      datefmt='%Y-%m-%dT%H:%M:%S',
                      )
  logging.getLogger().setLevel(logging.INFO)
  pytest.main()
