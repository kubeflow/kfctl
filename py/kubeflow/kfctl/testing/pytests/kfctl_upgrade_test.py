import logging
import os

import pytest

from kubernetes import client as k8s_client
from kubeflow.kfctl.testing.util import kfctl_go_test_utils as kfctl_util
from kubeflow.testing import util

# The upgrade test is currently failing. Until https://github.com/kubeflow/kfctl/pull/207
# is merged, we need to mark the test as an expected failure to unblock cherrypicks.
@pytest.mark.xfail
def test_upgrade_kubeflow(record_xml_attribute, app_path, kfctl_path, upgrade_spec_path):
  """Test that we can run upgrade on a Kubeflow cluster.

  Args:
    app_path: The app dir of kubeflow deployment.
    kfctl_path: The path to kfctl binary.
    upgrade_spec_path: The path to the upgrade spec file.
  """
  kfctl_util.kfctl_upgrade_kubeflow(app_path, kfctl_path, upgrade_spec_path)
