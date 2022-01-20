# hypershift-deployment-controller
## Controller for managing Hypershift HostedClusters with ACM

## Purpose of this Custom Resource Definition and Controller
This is to be an interface for ACM (Advanced Cluster Management for Kubernetes) to work with Hypershift in the following use cases
  | Use case   | ManagementCluster install | HostedCluster control plane | ACM creates Infrastructure | User brings infrastructure |
  | :---------:| :-----------------------: | :-------------------------: | :------------------------: | :------------------------: |
  | (1) ManagementCluster on ACM cluster    | ACM                         | ACM                        | AWS & IBM |
  | (2) ManagementCluster on ManagedCluster | ManagedCluster              | ACM                        | AWS & IBM |

  Note: Only 1 is supported at this time

## Hypershift on the ACM cluster (CURRENTLY SUPPORTED)
This allows you to create multiple Hypershift HostedClusters on the ACM cluster.

### Preparing the ACM cluster to be a ManagementCluster (Run once)
1. Connect to the ACM cluster with `oc` cli
2. Git Clone the Hypershift repository
  ```shell
  git clone git@github.com:openshift/hypershift.git
  ```
3. Follow the README.md and build the project to get the `hypershift` cli
  ```shell
  make build
  ```
4. Use the quickstart to install the Hypershift operator with the `hypershift` cli
  https://hypershift-docs.netlify.app/getting-started/
  Complete `Prerequisites` and `Before you begin`
  You need to create the S3 bucket, and run the `hypershift install ...` command. Ignore the rest.

### Using the Hypeershift Deployment Controller
1. Clone this repository
#### Run the controller from your development environment
2. Make sure you are connected to the ACM/OCP cluster where you ran `hypershift install` and launch the controller
  ```shell
  go run pkg/main.go
  ```
#### Run the controller in OCP
2. Make sure you are connected to the ACM/OCP cluster where you ran `hypershift install` and launch the controller
  ```shell
  oc apply -k config/deployment
  ```
  This will create the service account, roles, role-bindings and deployment that runs the controller. It currently uses the image `quay.io/jpacker/hypershift-deployment-controller`
#### Creating the Hypershift HostedCluster
3. In another shell, create the namespace `clusters` and set that as the default for you context
4. Make sure you have a copy of the AWS Provider Connection in this namespace. You can use the ACM console to create it in the `clusters` namespace.
4. Create a `HypershiftDeployment` resource (hd), and watch the logs.  The `hd` resource will create the infrastructure in AWS if: `Spec.Infrastructure.Configure: True`. Once all the AWS resources are created, it will create a `HostedCluster` resource and `NodePool`.  You will also notice that the `hd.Spec.HostedClusterSpec` and `hd.Spec.NodePools[]` keys are filled in with the values that were used to create the kube resources.
5. You can add additional NodePools, by editing the NodePools array, and adding addional specs. The easiest way to do this is to copy the existing spec, and just change the name.

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
