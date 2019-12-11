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

def deploy_pytorchjob(record_xml_attribute):
  """Deploy PytorchJob."""
  util.load_kube_config(persist_config=False)
  api_client = k8s_client.ApiClient()

if __name__ == "__main__":
  logging.basicConfig(level=logging.INFO,
                      format=('%(levelname)s|%(asctime)s'
                              '|%(pathname)s|%(lineno)d| %(message)s'),
                      datefmt='%Y-%m-%dT%H:%M:%S',
                      )
  logging.getLogger().setLevel(logging.INFO)
  pytest.main()
