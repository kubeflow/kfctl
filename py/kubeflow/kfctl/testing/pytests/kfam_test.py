import logging

import pytest

from kubeflow.testing import util
import json
from retrying import retry


logging.basicConfig(level=logging.INFO,
                    format=('%(levelname)s|%(asctime)s'
                            '|%(pathname)s|%(lineno)d| %(message)s'),
                    datefmt='%Y-%m-%dT%H:%M:%S',
                    )
logging.getLogger().setLevel(logging.INFO)

def test_kfam(record_xml_attribute):
  util.set_pytest_junit(record_xml_attribute, "test_kfam_e2e")
  util.load_kube_config()
  util.load_kube_credentials()

  getcmd = "kubectl get pods -n kubeflow -l=app=jupyter-web-app --template '{{range.items}}{{.metadata.name}}{{end}}'"
  jupyterpod = util.run(getcmd.split(' '))[1:-1]

  logging.info("accessing kfam svc from jupyter pod %s" % jupyterpod)

  # Profile Creation
  util.run(['kubectl', 'exec', jupyterpod, '-n', 'kubeflow', '--', 'curl',
            '--silent', '-X', 'POST', '-d',
            '{"metadata":{"name":"testprofile"},"spec":{"owner":{"kind":"User","name":"user1@kubeflow.org"}}}',
            'profiles-kfam.kubeflow:8081/kfam/v1/profiles'])
  # Verify Profile Creation
  bindingsstr = util.run(['kubectl', 'exec', jupyterpod, '-n', 'kubeflow', '--', 'curl', '--silent',
                'profiles-kfam.kubeflow:8081/kfam/v1/bindings'])
  bindings = json.loads(bindingsstr)

  assert "testprofile" in [binding['referredNamespace'] for binding in bindings['bindings']]

if __name__ == "__main__":
  logging.basicConfig(level=logging.INFO,
                      format=('%(levelname)s|%(asctime)s'
                              '|%(pathname)s|%(lineno)d| %(message)s'),
                      datefmt='%Y-%m-%dT%H:%M:%S',
                      )
  logging.getLogger().setLevel(logging.INFO)
  pytest.main()
