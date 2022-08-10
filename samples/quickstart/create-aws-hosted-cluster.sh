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

if [ "${CLOUD_PROVIDER_SECRET_NAMESPACE}" == "" ]; then
  printf "\n**WARNING** No CLOUD_PROVIDER_SECRET_NAMESPACE found, export it to avoid manual entry\n"
  printf "Enter the cloud provider secret namespace\n"
  read -r CLOUD_PROVIDER_SECRET_NAMESPACE
  if [ "${CLOUD_PROVIDER_SECRET_NAMESPACE}" == "" ]; then
    echo "No CLOUD_PROVIDER_SECRET_NAMESPACE provided"
    exit 1
  fi
fi

if [ "${CLOUD_PROVIDER_SECRET_NAME}" == "" ]; then
  printf "\n**WARNING** No CLOUD_PROVIDER_SECRET_NAME found, export it to avoid manual entry\n"
  printf "Enter the cloud provider secret name\n"
  read -r CLOUD_PROVIDER_SECRET_NAME
  if [ "${CLOUD_PROVIDER_SECRET_NAME}" == "" ]; then
    echo "No CLOUD_PROVIDER_SECRET_NAME provided"
    exit 1
  fi
fi

if [ "${INFRA_REGION}" == "" ]; then
  printf "\n**WARNING** No INFRA_REGION found, export it to avoid manual entry\n"
  printf "Enter the infrastructure region\n"
  read -r INFRA_REGION
  if [ "${INFRA_REGION}" == "" ]; then
    echo "No INFRA_REGION provided"
    exit 1
  fi
fi



if [ "${HYPERSHIFT_DEPLOYMENT_NAME}" == "" ]; then
  printf "\n**INFO** No HYPERSHIFT_DEPLOYMENT_NAME found, use default value \"hypershift-test\"\n"
  HYPERSHIFT_DEPLOYMENT_NAME="hypershift-test"
fi

printf 'Cloud provider secret namespace: %s\n' "${CLOUD_PROVIDER_SECRET_NAMESPACE}"
printf 'Cloud provider secret name     : %s\n' "${CLOUD_PROVIDER_SECRET_NAME}"
printf 'Infrastructure region          : %s\n' "${INFRA_REGION}"
printf 'Hypershift deployment name     : %s\n' "${HYPERSHIFT_DEPLOYMENT_NAME}"

oc get secret -n "${CLOUD_PROVIDER_SECRET_NAMESPACE}" "${CLOUD_PROVIDER_SECRET_NAME}"
if [ $? -ne 0 ]; then
  printf "Secret %s/%s not found\n" "${CLOUD_PROVIDER_SECRET_NAMESPACE}" "${CLOUD_PROVIDER_SECRET_NAME}"
  printf "You can create the secret via https://console-openshift-console.apps.<your-openshift-domain>/multicloud/credentials"
  exit 1
fi


oc apply -f - <<EOF
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: HypershiftDeployment
metadata:
  name: ${HYPERSHIFT_DEPLOYMENT_NAME}
  namespace: ${CLOUD_PROVIDER_SECRET_NAMESPACE}
spec:
  hostingCluster: local-cluster
  hostingNamespace: clusters
  infrastructure:
    cloudProvider:
      name: ${CLOUD_PROVIDER_SECRET_NAME}
    configure: True
    platform:
      aws:
        region: ${INFRA_REGION}
EOF

echo "wait for managed cluster addon hypershift addon to be available ..."
oc wait --for=condition=Available managedclusteraddon/hypershift-addon -n local-cluster --timeout=600s
if [ $? -ne 0 ]
then
  echo "hypershift addon installation failed"
  exit 1
else
  echo "hypershift addon installed successfully, now you can provision a hosted control plane cluster by HypershiftDeployment"
fi

managed_cluster_name=$(oc get managedcluster | grep ${HYPERSHIFT_DEPLOYMENT_NAME} | awk '{print $1}')
echo "managed_cluster_name: ${managed_cluster_name}"
oc get managedclusteraddon -n ${managed_cluster_name}

echo "wait for the managed cluster to be available"
oc wait --for=condition=ManagedClusterConditionAvailable managedcluster/${managed_cluster_name} --timeout=600s
if [ $? -ne 0 ]
then
  echo "timeout for waiting managed cluster ${managed_cluster_name} to be available"
  exit 1
else
  echo "managed cluster ${managed_cluster_name} is available now"
fi

oc get managedcluster ${managed_cluster_name}
oc get pod -n klusterlet-${managed_cluster_name}

oc get managedclusteraddon -n ${managed_cluster_name}

echo "wait for the work manager addon to be available"
oc wait --for=condition=Available managedclusteraddon/work-manager -n ${managed_cluster_name} --timeout=600s
if [ $? -ne 0 ]
then
  echo "timeout for waiting managed cluster ${managed_cluster_name} work manager addon to be available"
  exit 1
else
  echo "managed cluster ${managed_cluster_name} work manager addon is available now"
fi

oc get managedclusteraddon -n ${managed_cluster_name}
oc get managedcluster ${managed_cluster_name}

hosted_cluster_console_url=$(oc get managedcluster ${managed_cluster_name} -ojsonpath='{.status.clusterClaims[?(@.name=="consoleurl.cluster.open-cluster-management.io")].value}')
echo "hosted cluster console url: ${hosted_cluster_console_url}"
