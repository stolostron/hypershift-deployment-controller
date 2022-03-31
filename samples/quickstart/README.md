# Instructions
This document describes how to quickly get started with Hosting Control Planes and ACM/MCE

## Requirements
1. OpenShift Cluster, version 4.10+ is recommended
2. MCE or ACM installed on this cluster. (Developers: https://github.com/stolostron/deploy or via Operator Hub)
3. AWS artifacts:
   * AWS Service account Key & Secret Key with S3 permissions (ONLY needs S3 bucket permissions)
   * S3 Bucket name (user creates a bucket)
   * Bucket region (this is related to where the bucket was created)

## Quickstart
* Make sure you are connected to the OpenShift cluster
* Run the `start.sh` command
  * If the environment variables `BUCKET_NAME`, `BUCKET_REGION` and `S3_CREDS` is not set, you are prompted for these values

## What it does
1. Enables preview_hypershift
2. Creates a `local-cluster` `managedCluster` for the OpenShift cluster you are installing to
3. Imports the `local-cluster`
4. Applies the Hypershift Operator to the Hosting Service Cluster (Hub)

## Provision a Hosted Control Plane Cluster
1. Create an Cloud Provider Credential in a project
2. Create a HypershiftDeployment resource in the same project
