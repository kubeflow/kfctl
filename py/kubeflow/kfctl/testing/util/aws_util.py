import os
import logging
from kubeflow.testing import util
from kubeflow.testing.cloudprovider.aws import util as aws_util


def aws_auth_load_kubeconfig(cluster_name):
    if os.getenv("AWS_ACCESS_KEY_ID") and os.getenv("AWS_SECRET_ACCESS_KEY"):
        logging.info("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are set;")
    else:
        logging.info("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are not set.")
    util.run(["aws", "eks", "update-kubeconfig", "--name=" + cluster_name])

    aws_util.load_kube_config()