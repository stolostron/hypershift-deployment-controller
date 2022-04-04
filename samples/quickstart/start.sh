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


if [ "${S3_CREDS}" == "" ]; then
  printf "**WARNING** No S3_CREDS found, export it and try again\\n"
  printf "Enter the path and file name for the S3 AWS credential\n"
  read S3_CREDS

  if [ ! -f "${S3_CREDS}" ]; then
    printf "Could not find S3_CREDS file: ${S3_CREDS}"
    exit 1
  fi
fi

if [ "${BUCKET_NAME}" == "" ]; then
  printf "\n**WARNING** No BUCKET_NAME found, export it and try again\n"
  printf "Enter the S3 bucket name\n"
  read BUCKET_NAME
  if [ ${BUCKET_NAME} == "" ]; then
    echo "No BUCKET_NAME provided"
    exit 1
  fi
fi

if [ "${BUCKET_REGION}" == "" ]; then
  printf "\n**WARNING** No BUCKET_REGION found, export it and try again\n"
  printf "Enter the region that constains the S3 bucket\n"
  read BUCKET_REGION
  if [ ${BUCKET_REGION} == "" ]; then
    echo "No BUCKET_REGION provided"
    exit 1
  fi
fi
printf "S3 credentials: ${S3_CREDS}\n"
printf "Bucket name   : ${BUCKET_NAME}\n"
printf "Bucket region : ${BUCKET_REGION}\nTesting bucket\n"

CMD="AWS_SHARED_CREDENTIALS_FILE=${S3_CREDS} aws --region ${BUCKET_REGION} s3 ls ${BUCKET_NAME}"
which aws
if [ $? -eq 0 ]; then
  eval ${CMD}
  if [ $? -ne 0 ]; then
    printf "Bucket with details listed above does not exist\n"
    exit 1
  fi
else
  printf "**WARNING** AWS CLI is not present, so S3_CREDS can not be validated, continuing to test URL\n"
fi

printf "Testing the s3 URL: https://${BUCKET_NAME}.s3.${BUCKET_REGION}.amazonaws.com"
CMD="curl https://${BUCKET_NAME}.s3.${BUCKET_REGION}.amazonaws.com 2>&1 | grep \"Access Denied\""
eval ${CMD}
if [ $? -ne 0 ]; then
  printf "Bucket with details listed above does not exist\n"
  exit 1
fi

oc get mce multiclusterengine-sample
if [ $? -ne 0 ]; then
  echo "multiclusterengine-sample is not available, please install the multi-cluster engine"
  exit 1
fi

oc patch multiclusterengine multiclusterengine-sample --type=merge -p '{"spec":{"overrides":{"components":[{"name":"hypershift-preview","enabled": true}]}}}'

oc apply -f - <<EOF
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: local-cluster
spec:
  hubAcceptsClient: true
  leaseDurationSeconds: 60
EOF

oc get secret -n local-cluster local-cluster-import > /dev/null 2>&1
while [ $? -ne 0 ]; do
  sleep 2
  oc get secret -n local-cluster local-cluster-import > /dev/null 2>&1
done

sleep 10

oc get secret -n local-cluster local-cluster-import -o yaml > sOut1
oc get secret -n local-cluster local-cluster-import -ojsonpath={.data.crds\\.yaml} | base64 -d | oc apply -f -

oc get secret -n local-cluster local-cluster-import -ojsonpath={.data.import\\.yaml} | base64 -d | oc apply -f -

oc apply -f https://raw.githubusercontent.com/stolostron/hypershift-addon-operator/main/example/managedclusteraddon-hypershift-addon.yaml

oc -n local-cluster create secret generic hypershift-operator-oidc-provider-s3-credentials --from-file=credentials=${S3_CREDS} --from-literal=bucket=${BUCKET_NAME} --from-literal=region=${BUCKET_REGION} -n local-cluster