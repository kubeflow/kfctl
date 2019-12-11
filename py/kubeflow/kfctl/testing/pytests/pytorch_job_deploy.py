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
  """Deploy Pytorchjob using the pytorch-job component"""
  util.load_kube_config(persist_config=False)
  api_client = k8s_client.ApiClient()
  app_dir = setup_kubeflow_ks_app(args, api_client)

  component = "example-job"
  logging.info("Deploying pytorch.")
  generate_command = [ks, "generate", "pytorch-job", component]

  util.run(generate_command, cwd=app_dir)

  params = {}
  for pair in args.params.split(","):
    k, v = pair.split("=", 1)
    params[k] = v

  ks_deploy(app_dir, component, params, env=None, account=None, namespace=None)

if __name__ == "__main__":
  logging.basicConfig(level=logging.INFO,
                      format=('%(levelname)s|%(asctime)s'
                              '|%(pathname)s|%(lineno)d| %(message)s'),
                      datefmt='%Y-%m-%dT%H:%M:%S',
                      )
  logging.getLogger().setLevel(logging.INFO)
  pytest.main()
