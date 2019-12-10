"""Run kfctl delete as a pytest. Deletion should fail because the host
is wrong.

We use this in order to generate a junit_xml file.
"""
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
from googleapiclient import discovery
from oauth2client.client import GoogleCredentials

def test_kfctl_delete_wrong_cluster(record_xml_attribute, kfctl_path, app_path, project,
                                    cluster_deletion_script):
  util.set_pytest_junit(record_xml_attribute, "test_kfctl_delete_wrong_cluster")
  if not kfctl_path:
    raise ValueError("kfctl_path is required")

  if not app_path:
    raise ValueError("app_path is required")

  logging.info("Using kfctl path %s", kfctl_path)
  logging.info("Using app path %s", app_path)

  kfdef_path = os.path.join(app_path, "tmp.yaml")
  kfdef = {}
  with open(kfdef_path, "r") as f:
    kfdef = yaml.safe_load(f)

  # Make sure we copy the correct host instead of string reference.
  cluster = kfdef.get("metadata", {}).get("clusterName", "")[:]
  if not cluster:
    raise ValueError("cluster is not written to kfdef")

  @retry(stop_max_delay=60*3*1000)
  def run_delete():
    try:
      # Put an obvious wrong cluster into KfDef
      kfdef["metadata"]["clusterName"] = "dummy"
      with open(kfdef_path, "w") as f:
        yaml.dump(kfdef, f)
      util.run([kfctl_path, "delete", "--delete_storage", "-V", "-f", kfdef_path],
               cwd=app_path)
    except subprocess.CalledProcessError as e:
      if e.output.find("cluster name doesn't match") != -1:
        return
      else:
        # Re-throw error if it's not expected.
        raise e
    finally:
      # Restore the correct host info.
      kfdef["metadata"]["clusterName"] = cluster[:]
      with open(kfdef_path, "w") as f:
        yaml.dump(kfdef, f)

  run_delete()

if __name__ == "__main__":
  logging.basicConfig(level=logging.INFO,
                      format=('%(levelname)s|%(asctime)s'
                              '|%(pathname)s|%(lineno)d| %(message)s'),
                      datefmt='%Y-%m-%dT%H:%M:%S',
                      )
  logging.getLogger().setLevel(logging.INFO)
  pytest.main()
