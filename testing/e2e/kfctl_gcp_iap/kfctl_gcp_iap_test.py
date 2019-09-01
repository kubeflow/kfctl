# -*- coding: utf-8 -*-

# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License. You may obtain
# a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.

import unittest
import uuid

from kubernetes import client, config
from kubernetes.client import V1Namespace, V1NamespaceStatus
from kubernetes.client.apis import core_v1_api

def short_uuid():
    id = str(uuid.uuid4())
    return id[-12:]


class TestNamespaces(unittest.TestCase):

    def required_namespaces_exist_test(self):
        namespaces = {
          "kubeflow": False, 
          "istio-system": False,
        }
        config.load_kube_config()
        v1 = client.CoreV1Api()
        for i in v1.list_namespace().items:
            name = i.metadata.name
            if name in namespaces:
              namespaces[name] = True
            fi
        for key, value in namespaces.items():
            if value == False:
                self.assertTrue(False, "%s namespace is missing" % key)


    def required_applications_exist_test(self):
        config.load_kube_config()
        api = client.CustomObjectsApi()
        ret = api.list_namespaced_custom_object(
            group="app.k8s.io",
            version="v1beta1",
            namespace="tekton-pipelines",
            plural="applications", watch=False)

        # get the resource and print out data
        list = ret['items']
        for i in list:
            name = i['metadata']['name']
            print("%s" % name)
            if 'components' in i['status']:
                for j in i['status']['components']:
                    self.assertEqual(j['status'], "Ready", "%s is not ready" % name)
            else:
                self.assertTrue(False, "no components in application %s" % name)
