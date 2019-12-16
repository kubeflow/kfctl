"""Test jupyter custom resource.
This file tests that we can create notebooks using the Jupyter custom resource.
It is an integration test as it depends on having access to
a Kubeflow cluster with the custom resource test installed.
We use the pytest framework because
  1. It can output results in junit format for prow/gubernator
  2. It has good support for configuring tests using command line arguments
    (https://docs.pytest.org/en/latest/example/simple.html)
Python Path Requirements:
  kubeflow/testing/py - https://github.com/kubeflow/testing/tree/master/py
    * Provides utilities for testing
Manually running the test
  1. Configure your KUBECONFIG file to point to the desired cluster
"""

import logging
import os

import pytest

from kubernetes import client as k8s_client
from kubeflow.testing import util

def test_jupyter(record_xml_attribute, kfctl_repo_path, namespace):
  """Test the jupyter notebook.
  Args:
    record_xml_attribute: Test fixture provided by pytest.
    env: ksonnet environment.
    namespace: namespace to run in.
  """
  # util.load_kube_config appears to hang on python3
  util.load_kube_config()
  util.load_kube_credentials()
  logging.info("using kfctl repo: %s" % kfctl_repo_path)
  util.run(["kubectl", "apply", "-f",
            os.path.join(kfctl_repo_path,
                         "py/kubeflow/kfctl/testing/pytests/testdata/jupyter_test.yaml")])
  # api_client = k8s_client.ApiClient()
  # kube_config.load_kube_config()
  # host = api_client.configuration.host
  # logging.info("Kubernetes master: %s", host)
  # master = host.rsplit("/", 1)[-1]

  # this_dir = os.path.dirname(__file__)
  # app_dir = os.path.join(this_dir, "test_app")

  # ks_cmd = ks_util.get_ksonnet_cmd(app_dir)

  # name = "jupyter-test"
  # service = "jupyter-test"
  # component = "jupyter"
  # params = ""
  # ks_util.setup_ks_app(app_dir, env, namespace, component, params)

  # util.run([ks_cmd, "apply", env, "-c", component], cwd=app_dir)
  # conditions = ["Running"]
  # results = util.wait_for_cr_condition(api_client, GROUP, PLURAL, VERSION,
  #                                      namespace, name, conditions)

  # logging.info("Result of CRD:\n%s", results)

  # # We proxy the request through the APIServer so that we can connect
  # # from outside the cluster.
  # url = ("https://{master}/api/v1/namespaces/{namespace}/services/{service}:80"
  #        "/proxy/default/jupyter/lab?").format(
  #            master=master, namespace=namespace, service=service)
  # logging.info("Request: %s", url)
  # r = send_request(url, verify=False)

  # if r.status_code != requests.codes.OK:
  #   msg = "Request to {0} exited with status code: {1} and content: {2}".format(
  #       url, r.status_code, r.content)
  #   logging.error(msg)
  #   raise RuntimeError(msg)


if __name__ == "__main__":
  logging.basicConfig(
      level=logging.INFO,
      format=('%(levelname)s|%(asctime)s'
              '|%(pathname)s|%(lineno)d| %(message)s'),
      datefmt='%Y-%m-%dT%H:%M:%S',
  )
  logging.getLogger().setLevel(logging.INFO)
  pytest.main()
