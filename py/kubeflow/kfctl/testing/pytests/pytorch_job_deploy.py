import datetime
import logging
import os
import subprocess
import tempfile
import uuid
import yaml

from retrying import retry

import pytest

from kubeflow.testing import util
from kubernetes import client as k8s_client
from googleapiclient import discovery
from oauth2client.client import GoogleCredentials

def test_deploy_pytorchjob(record_xml_attribute, kfctl_repo_path):
  """Deploy PytorchJob."""
  util.load_kube_config()
  util.load_kube_credentials()
  logging.info("using kfctl repo: %s" % kfctl_repo_path)
  util.run(["kubectl", "apply", "-f",
            os.path.join(kfctl_repo_path,
                         "py/kubeflow/kfctl/testing/pytests/testdata/pytorch_job.yaml")])

if __name__ == "__main__":
  logging.basicConfig(level=logging.INFO,
                      format=('%(levelname)s|%(asctime)s'
                              '|%(pathname)s|%(lineno)d| %(message)s'),
                      datefmt='%Y-%m-%dT%H:%M:%S',
                      )
  logging.getLogger().setLevel(logging.INFO)
  pytest.main()
