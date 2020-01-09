import logging
import time

import pytest

from kubeflow.testing import util

from retrying import retry
from kubeflow.kfctl.testing.ci.kfam_client.api import DefaultApi
from kubeflow.kfctl.testing.ci.kfam_client.api_client import ApiClient
from kubeflow.kfctl.testing.ci.kfam_client.configuration import Configuration
import kubeflow.kfctl.testing.ci.kfam_client.models as models


logging.basicConfig(level=logging.INFO,
                    format=('%(levelname)s|%(asctime)s'
                            '|%(pathname)s|%(lineno)d| %(message)s'),
                    datefmt='%Y-%m-%dT%H:%M:%S',
                    )
logging.getLogger().setLevel(logging.INFO)

def test_kfam(record_xml_attribute):
  util.set_pytest_junit(record_xml_attribute, "test_kfam_e2e")
  kfam_config = Configuration()
  kfam_config.host = "profiles-kfam.kubeflow:8081/kfam"
  defaultApi = DefaultApi(api_client=ApiClient(configuration=kfam_config))

  # Profile Creation
  profile = models.Profile(
    metadata=models.Metadata(name="testprofile"),
    spec=models.ProfileSpec(owner=models.Subject(kind="User", name="user1@kubeflow.org"))
  )
  defaultApi.create_profile(profile)

  # Verify Profile Creation
  time.sleep(10)
  bindings = defaultApi.read_bindings().bindings()
  assert "testprofile" in [binding._referred_namespace for binding in bindings]


if __name__ == "__main__":
  logging.basicConfig(level=logging.INFO,
                      format=('%(levelname)s|%(asctime)s'
                              '|%(pathname)s|%(lineno)d| %(message)s'),
                      datefmt='%Y-%m-%dT%H:%M:%S',
                      )
  logging.getLogger().setLevel(logging.INFO)
  pytest.main()
