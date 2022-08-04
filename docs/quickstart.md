# Quick start

You need at least one cluster to initialize as you hub. In this demostration we will start with two clusters, one will be the hub, the other the Management Cluster (the host for our control planes).

## Initializing
* On the first cluster, turn it into a hub by following these instructions:
https://open-cluster-management.io/getting-started/quick-start/

* On the second cluster, enable Hypershift
  1. Create an AWS S3 bucket
  2. Create an AWS service account with access to S3 (if you will deploy clusters with other AWS users, otherwise create an administrative service account for provisioning clusters)
  3. Deploy hypershift:
    ```shell
    git clone git@github.com:openshift/hypershift.git
    cd hypershift
    make build
    ```
  4. Using the `hypershift` binary, setup the second cluster as the Management Cluster
  5. Run the following command:
    ```shell
    REGION=us-east-1                    # Define in step 1
    BUCKET_NAME=your-bucket-name        # Define in step 1
    AWS_CREDS="$HOME/.aws/credentials"  # From step 2

    hypershift install \
    --oidc-storage-provider-s3-bucket-name $BUCKET_NAME \
    --oidc-storage-provider-s3-credentials $AWS_CREDS \
    --oidc-storage-provider-s3-region $REGION
    ```
    There are additional instructions here: https://hypershift-docs.netlify.app/getting-started/

## Provisioning clusters
* Create a cloud provider secret in ACM, it has the following format for AWS:
  ```yaml
    apiVersion: v1
    metadata:
        name: aws4jnp
        namespace: default      # Where you will create HypershiftDeployment resources
    type: Opaque
    kind: Secret
    stringData:
        ssh-publickey:          # Value
        ssh-privatekey:         # Value
        pullSecret:             # Value
        baseDomain:             # Value
        aws_secret_access_key:  # Value
        aws_access_key_id:      # Value
  ```
* The HypershiftDeployment resource is used by Multicluster Engine to deploy Hosted Control Plane clusters.  This resource has the option to configure infrastructre for you or you can provide the infrastructure details.  It will also automatically generate the HostedClusterSpec and/or NodePool.Spec if you do not provide one, allowing you to customize your cluster. The simplest HypershiftDeployment is as follows:

    ```yaml
        apiVersion: cluster.open-cluster-management.io/v1alpha1
        kind: HypershiftDeployment
        metadata:
        name: aws-sample        # Name of the cluster
        namespace: default      # Same namespace as the Cloud Provider secret
        spec:
        infrastructure:
            cloudProvider:
            name: my-cloud-provider-secret  # Cloud Provider secret created in the previous step
            configure: True                 # Whether you want the infrastructure to be created
            platform:
                aws:
                    region: us-west-1
    ```
* Monitoring the deployment:
   1. Watch the HypershiftDeployment resource, make sure all columns are AsExpected
        ```shell
        oc get hd
        ```
    2. Watch the NodePool resource, wait until the `UPDATINGVERSION` and `UPDATING CONFIG` are no longer `True`
        ```shell
        oc get np
        ```
    3. Watch the HostedCluster resource, wait until its `PROGRESS` is `Completed` and `AVAILABLE` is `True`
        ```shell
        oc get hc
        ```

# Destroying your Hosted Control Plane cluster
Delete the HypershiftDeployment resource
```shell
oc delete hd CLUSTER_NAME
```
