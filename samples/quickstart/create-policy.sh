#!/bin/bash
# Copyright 2022.
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
#     http://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

function wait_policy_result() {
  set +e
  for((i=0;i<30;i++));
  do
    result=$(oc -n $1 get policy $2 -ojsonpath="{.status.compliant}")
    if [[ -n $result ]]; then
      break
    fi
    echo "sleep 1 second to wait policy $1/$2 to be finished: $i"
    sleep 1
  done
  set -e
}

if [ "${HYPERSHIFT_DEPLOYMENT_NAME}" == "" ]; then
  printf "\n**INFO** No HYPERSHIFT_DEPLOYMENT_NAME found, use default value \"hypershift-test\"\n"
  HYPERSHIFT_DEPLOYMENT_NAME="hypershift-test"
fi

printf 'Hypershift deployment name     : %s\n' "${HYPERSHIFT_DEPLOYMENT_NAME}"

managed_cluster_name=$(oc get managedcluster | grep ${HYPERSHIFT_DEPLOYMENT_NAME} | awk '{print $1}')
echo "managed_cluster_name: ${managed_cluster_name}"

oc apply -f - <<EOF
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: policy-namespace
  annotations:
    policy.open-cluster-management.io/standards: NIST SP 800-53
    policy.open-cluster-management.io/categories: CM Configuration Management
    policy.open-cluster-management.io/controls: CM-2 Baseline Configuration
spec:
  remediationAction: inform
  disabled: false
  policy-templates:
    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata:
          name: policy-namespace-example
        spec:
          remediationAction: inform # the policy-template spec.remediationAction is overridden by the preceding parameter value for spec.remediationAction.
          severity: low
          namespaceSelector:
            exclude: ["kube-*"]
            include: ["default"]
          object-templates:
            - complianceType: musthave
              objectDefinition:
                kind: Namespace # must have namespace 'prod'
                apiVersion: v1
                metadata:
                  name: prod
---
apiVersion: policy.open-cluster-management.io/v1
kind: PlacementBinding
metadata:
  name: binding-policy-namespace
placementRef:
  name: placement-policy-namespace
  kind: PlacementRule
  apiGroup: apps.open-cluster-management.io
subjects:
- name: policy-namespace
  kind: Policy
  apiGroup: policy.open-cluster-management.io
---
apiVersion: apps.open-cluster-management.io/v1
kind: PlacementRule
metadata:
  name: placement-policy-namespace
spec:
  clusterConditions:
  - status: "True"
    type: ManagedClusterConditionAvailable
  clusterSelector:
    # matchExpressions:
    #   - {key: environment, operator: In, values: ["dev"]}
    matchExpressions:
      - key: name
        operator: In
        values:
          - ${managed_cluster_name}
EOF



echo -n "Wait for the policy result..."
wait_policy_result default policy-namespace

policy_result=$(oc get policy policy-namespace -ojsonpath="{.status.status[?(@.clustername==\"${managed_cluster_name}\")].compliant}")
if [ ${policy_result} != "NonCompliant" ]
then
  echo "expect result \"policy_result\", but got \"${policy_result}\""
else
  echo "policy namespace applied to the hosted cluster ${managed_cluster_name} successfully, policy result: \"${policy_result}\""
fi
