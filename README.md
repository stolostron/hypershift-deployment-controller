# hypershift-deployment-controller
## Controller for managing Hypershift HostedClusters with ACM

## Purpose of this Custom Resource Definition and Controller
This is to be an interface for ACM (Advanced Cluster Management for Kubernetes) to work with Hypershift in the following use cases
  | Use case   | ManagementCluster install | HostedCluster control plane | ACM creates Infrastructure | User brings infrastructure |
  | :---------:| :-----------------------: | :-------------------------: | :------------------------: | :------------------------: |
  | (1) Hosting cluster on the ACM Hub cluster    | ACM                         | ACM                        | AWS, Azure, & Agent |
  | (2) Hosting cluster on a ManagedCluster | ManagedCluster              | ACM                        | AWS & Azure |

## Hypershift on the ACM cluster (CURRENTLY SUPPORTED)
This allows you to create multiple Hypershift HostedClusters on the ACM cluster.

### Preparing the ACM cluster to be a ManagementCluster (Run once)
1. Connect to the ACM/MCE Hub cluster with `oc` cli

2. Clone this repository

3. Complete the `Prerequisites` section in the Hypershift quickstart https://hypershift-docs.netlify.app/getting-started/, making sure you have the route53 base domain and the S3 bucket (This is required for AWS).

#### Activating the environment
3.
  ```shell
  samples/quickstart/start.sh
  ```
  a. You will be prompted for an AWS credential with access to an AWS S3 bucket
  b. You will be prompted for the name of the AWS S3 bucket to use
  c. You will be prompted for the region where the AWS S3 bucket resides

#### Alternative - Developing from your environment
4. Make sure you are connected to the ACM/MCE on an OCP cluster and did not run Step 3 (otherwise this controller is already running)
  ```shell

  make vendor     # Incorporates the dependant Go modules
  make manifests  # Creates the CRD (Custom Resource Definition) for the ..._type.go
  make install    # Installs the CRD
  make run        # Launches the controller
  ```
#### Creating the Hypershift HostedCluster
5. In another shell, create the namespace `clusters` and set that as the default for you context
6. Make sure you have a copy of the ACM AWS Provider Connection in this namespace. You can use the ACM console to create this secret in the `clusters` namespace.
7. Create a `HypershiftDeployment` resource (hd), and watch the logs.  The `hd` resource will create the infrastructure in AWS if: `Spec.Infrastructure.Configure: True`. Once all the AWS resources are created, it will create a `HostedCluster` resource and `NodePool`.  You will also notice that the `hd.Spec.HostedClusterSpec` and `hd.Spec.NodePools[]` keys are filled in with the values that were used to create the kube resources.
8. You can add additional NodePools, by editing the NodePools array, and adding addional specs. The easiest way to do this is to copy the existing spec, and just change the name.

### Checking status
```bash
oc get hd  # displays the status of Infrastructure deployment if configure: true

# NAME     INFRA                  READY   IAM                    READY   PROVIDER REF          FOUND   PROGRESS    AVAILABLE
# sample   ConfiguredAsExpected   True    ConfiguredAsExpected   True    ReferenceAsExpected   Truee   Completed   True
```
If there is problem, looking at the `HypershiftDeployment.Status.Conditions[].message` and you will see a specific error message.  When destroying the infrastructure, you will see similar details on the progres of clean up.

## Deleting HypershiftDeployment
1. Make sure the controller is running
2. Delete the HypershiftDeployment resource

## Deleting a NodePool
1. Make sure the controller is running
2. Edit the HypershiftDeployment resource, and remove the NodePool element from the array. The controller will reconcile the result and delete the NodePool

## Customizing your hosted resource
You can customize the values for HostedCluster (bring your own) or in combination with `Spec.Infrastructure.Configure: True`.

## Bring your own infrastructure
If `Spec.Infrastructure.Configure: False` you must provide a complete `Spec.HostedClusterSpec` and `Spec.NodePools` definition.  Otherwise the two resources will not successfully complete and you will not get a Hypershift cluster.

## Customize your deployment
If `Spec.Infrastructure.Configure: True`, but you supply values for `Spec.HostedClusterSpec` and `Spec.NodePools` the following will be overwritten with values from the Infrastructure build
Spec.HostedCluster:
* HostedCluster.dns
* HostedCluster.infraID
* HostedCluster.issuerURL
* HostedCluster.networking.machineCIDR
* HostedCluster.Platform.aws  (The whole thing)

Spec.NodePool:
* []NodePool.Spec.clusterName  (When it doesn't match the HypershiftDeployment name)
* []NodePool.Spec.platform.aws (When `nil` the entire spec will be created)
* []NodePool.Spec.platform.aws.instanceProfile (When empty `""` it will be set)
* []NodePool.Spec.platform.aws.securityGroups (When `nil` the security group from the infrastructure configuration is used)
* []NodePool.Spec.aws.subnet (When `nil` the Private Subnet ID from the infrastructure configuration is used)

