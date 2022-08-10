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

if [ "${BUCKET_NAME}" == "" ]; then
  printf "\n**WARNING** No BUCKET_NAME found, export it to avoid manual entry\n"
  printf "Enter the S3 bucket name\n"
  read BUCKET_NAME
  if [ "${BUCKET_NAME}" == "" ]; then
    echo "No BUCKET_NAME provided"
    exit 1
  fi
fi

if [ "${BUCKET_REGION}" == "" ]; then
  printf "\n**WARNING** No BUCKET_REGION found, export it to avoid manual entry\n"
  printf "Enter the region that contains the S3 bucket\n"
  read BUCKET_REGION
  if [ "${BUCKET_REGION}" == "" ]; then
    echo "No BUCKET_REGION provided"
    exit 1
  fi
fi

printf "Bucket name   : ${BUCKET_NAME}\n"
printf "Bucket region : ${BUCKET_REGION}\Creating bucket\n"

which aws
if [ $? -ne 0 ]; then
  printf "**WARNING** AWS CLI is not present, please ensure it is installed and in the path\n"
  exit 2
fi

aws s3 ls > /dev/null
if [ $? -ne 0 ]; then
  printf "**WARNING** AWS CLI seems not configured correctly, please check it\n"
  exit 2
fi

aws s3 mb "s3://${BUCKET_NAME}" --region "${BUCKET_REGION}"
aws s3api put-public-access-block --bucket "${BUCKET_NAME}" --public-access-block-configuration BlockPublicAcls=false
aws s3api put-bucket-ownership-controls --bucket "${BUCKET_NAME}" --ownership-controls "Rules=[{ObjectOwnership=ObjectWriter}]"
aws s3api put-bucket-versioning --bucket "${BUCKET_NAME}" --versioning-configuration Status=Suspended
aws s3api delete-bucket-encryption --bucket "${BUCKET_NAME}"
