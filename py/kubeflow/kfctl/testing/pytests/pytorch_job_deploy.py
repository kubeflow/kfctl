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

@pytest.mark.xfail(reason=("See: https://github.com/kubeflow/kfctl/issues/199; "
                           "test is flaky."))
def test_deploy_pytorchjob(record_xml_attribute, kfctl_repo_path, namespace):
  """Deploy PytorchJob."""
  util.load_kube_config()
  util.load_kube_credentials()
  logging.info("using kfctl repo: %s" % kfctl_repo_path)
  util.run(["kubectl", "apply", "-f",
            os.path.join(kfctl_repo_path,
                         "py/kubeflow/kfctl/testing/pytests/testdata/pytorch_job.yaml")])
  api_client = k8s_client.ApiClient()
  api = k8s_client.CoreV1Api(api_client)

  # If the call throws exception, let it emit as an error case.
  resp = api.list_namespaced_pod(namespace)
  names = {
      "pytorch-mnist-ddp-cpu-master-0": False,
      "pytorch-mnist-ddp-cpu-worker-0": False,
  }

  for pod in resp.items:
    name = pod.metadata.name
    if name in names:
      names[name] = True

  msg = []
  for n in names:
    if not names[n]:
      msg.append("pod %s is not found" % n)
  if msg:
    raise ValueError("; ".join(msg))

if __name__ == "__main__":
  logging.basicConfig(level=logging.INFO,
                      format=('%(levelname)s|%(asctime)s'
                              '|%(pathname)s|%(lineno)d| %(message)s'),
                      datefmt='%Y-%m-%dT%H:%M:%S',
                      )
  logging.getLogger().setLevel(logging.INFO)
  pytest.main()
