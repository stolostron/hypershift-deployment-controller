# Provision Hypershift Clusters by MCE

Configuring hosted control planes requires a hosting service cluster and a hosted cluster. By deploying the HyperShift operator on an existing cluster, you can make that cluster into a hosting service cluster and start the creation of the hosted cluster. 

Hosted control planes is a Technology Preview feature, so the related components are disabled by default. Enable the feature by editing the `multiclusterengine` custom resource to set the `spec.overrides.components[?(@.name=='hypershift-preview')].enabled` to `true`. 

Enter the following command to ensure that the hosted control planes feature is enabled:

```bash
oc patch mce multiclusterengine-sample --type=merge -p '{"spec":{"overrides":{"components":[{"name":"hypershift-preview","enabled": true}]}}}'
```

## Configuring the hosting service cluster

You can deploy hosted control planes by configuring an existing cluster to function as a hosting service cluster. The hosting service cluster is the OCP cluster where the control planes are hosted, and can be the hub cluster or one of the OCP managed clusters. In this section, we will use hypershift-addon to install a HyperShift operator to one of the managed clusters to make it a hosting cluster.

### Prerequisites

You must have the following prerequisites to configure a hosting service cluster: 

- Multicluster engine operator (MCE) installed on OCP cluster.

- MCE has at least one managed OCP cluster. We will make this OCP managed cluster a hypershift management cluster. It is possible to use the MCE hub cluster as a hypershift management cluster. This requires importing the hub cluster as an OCP managed cluster called `local-cluster`:

```bash
$ oc apply -f - <<EOF
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  labels:
    local-cluster: "true"
  name: local-cluster
spec:
  hubAcceptsClient: true
  leaseDurationSeconds: 60
EOF
```

### Configuring the hosting service cluster

Complete the following steps on the cluster where the multicluster engine operator is installed to enable an {ocp-short} managed cluster as a hosting service cluster:

1. If you plan to provision hosted clusters on the AWS platform, create an OIDC s3 credentials secret for the HyperShift operator, and name it `hypershift-operator-oidc-provider-s3-credentials`. It should reside in managed cluster namespace (i.e., the namespace of the managed cluster that will be used as the hosting service cluster). If you used `local-cluster`, then create the secret in the `local-cluster` namespace

The secret must contain 3 fields:

- `bucket`: An S3 bucket with public access to host OIDC discovery documents for your HyperShift clusters
- `credentials`: A reference to a file that contains the credentials of the `default` profile that can access the bucket. By default, HyperShift only uses the `default` profile to operate the `bucket`.
- `region`: Region of the S3 bucket

See [Getting started](https://hypershift-docs.netlify.app/getting-started). in the HyperShift documentation for more information about the secret. The following example shows a sample AWS secret creation CLI template:

```bash
$ oc create secret generic hypershift-operator-oidc-provider-s3-credentials --from-file=credentials=$HOME/.aws/credentials --from-literal=bucket=<s3-bucket-for-hypershift> --from-literal=region=<region> -n <managed-cluster-used-as-hosting-service-cluster>
```

**Note:** Disaster recovery backup for the secret is not automatically enabled. Run the following command to add the label that enables the `hypershift-operator-oidc-provider-s3-credentials` secret to be backed up for disaster recovery:

```bash
$ oc label secret hypershift-operator-oidc-provider-s3-credentials -n <managed-cluster-used-as-hosting-service-cluster> cluster.open-cluster-management.io/backup=true
```

#### Enabling AWS Private Link

If you plan to provision hosted clusters on the AWS platform with Private Link, create an AWS credential secret for the HyperShift operator, and name it `hypershift-operator-private-link-credentials`. It should reside in the managed cluster namespace (i.e., the namespace of the managed cluster that will be used as the hosting service cluster). If you used `local-cluster`, then create the secret in the `local-cluster` namespace

Follow the instructions here (steps 1-5): [Deploy AWS private clusters](https://hypershift-docs.netlify.app/how-to/aws/deploy-aws-private-clusters/)

Instructions result in the following AWS resources:

* Create an IAM policy in AWS
* Create an IAM user for the Hypershift operator to use
* Attach the IAM policy to the IAM user
* Create an IAM access key for the user, use the access key and secret in the next section

The secret must contain 3 fields:

- `aws-access-key-id`: AWS credential access key id
- `aws-secret-access-key`: AWS credential access key secret
- `region`: Region for use with Private Link

See [HyperShift Project Documentation](https://hypershift-docs.netlify.app/how-to/aws/deploy-aws-private-clusters/) for details. For convenience, you can create this secret using the CLI by:

```bash
$ oc create secret generic hypershift-operator-private-link-credentials --from-literal=aws-access-key-id=<aws-access-key-id> --from-literal=aws-secret-access-key=<aws-secret-access-key> --from-literal=region=<region> -n <managed-cluster-used-as-hosting-service-cluster>
```

**Note:** Disaster recovery backup for the secret is not automatically enabled. Run the following command to add the label that enables the `hypershift-operator-private-link-credentials` secret to be backed up for disaster recovery: 

```bash
$ oc label secret hypershift-operator-private-link-credentials -n <managed-cluster-used-as-hosting-service-cluster> cluster.open-cluster-management.io/backup=true
```
##### Enable on a HostedCluster

Set the following parameter in HypershiftDeployment, when creating a cluster:
```
spec:
  hostedClusterSpec:
    platform:
      type: AWS
      aws:
        endpointAccess: Private
```

#### Enabling External DNS
If you plan to use service-level DNS for Control Plane Service, create an external DNS credential secret for the HyperShift operator, and name it `hypershift-operator-external-dns-credentials`. It should reside in the managed cluster namespace (i.e., the namespace of the managed cluster that will be used as the hosting service cluster). If you used `local-cluster`, then create the secret in the `local-cluster` namespace

The secret must contain 3 fields:

- `provider`: DNS provider that manages the service-level DNS zone (example: aws)
- `domain-filter`: The service-level domain
- `credentials`: *(Optional, only when using aws keys) - For all external DNS types, a credential file is supported
- `aws-access-key-id`: *OPTIONAL* - When using AWS DNS service, credential access key id
- `aws-secret-access-key`: *OPTIONAL* - When using AWS DNS service, credential access key secret
For details, please check: [HyperShift Project Documentation](https://hypershift-docs.netlify.app/how-to/external-dns/). For convenience, you can create this secret using the CLI by:

```bash
$ oc create secret generic hypershift-operator-external-dns-credentials --from-literal=provider=aws --from-literal=domain-filter=service.my.domain.com --from-file=credentials=<credentials-file> -n <managed-cluster-used-as-hosting-service-cluster>
```

Add the special label to the `hypershift-operator-external-dns-credentials` secret so that the secret is backed up for disaster recovery.

```bash
$ oc label secret hypershift-operator-external-dns-credentials -n <managed-cluster-used-as-hosting-service-cluster> cluster.open-cluster-management.io/backup=true
```

##### Enable on a HostedCluster

Set the following parameter in HypershiftDeployment, when creating a cluster:
```
spec:
  hostedClusterSpec:
    platform:
      type: AWS
      aws:
        endpointAccess: PublicAndPrivate
...
    services:
    - service: APIServer
      servicePublishingStrategy:
        loadBalancer:
          hostname: api-example.service.my.domain.com
        type: LoadBalancer
    - service: OAuthServer
      servicePublishingStrategy:
        route:
          hostname: oauth-example.service.my.domain.com
        type: Route
    - service: Konnectivity
      servicePublishingStrategy:
        type: Route
    - service: Ignition
      servicePublishingStrategy:
        type: Route        

```

2. Install the HyperShift add-on. The cluster that hosts the HyperShift operator is the management cluster. This step uses the hypershift-addon to install the HyperShift operator on a managed cluster. `ManagedClusterAddon` hypershift-addon. Replace `managed-cluster-used-as-hosting-service-cluster` with the name of the managed cluster on which you want to install the HyperShift operator. If you are installing on the MCE hub cluster, then use `local-cluster` for this value.
  
```bash
$ oc apply -f - <<EOF
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: hypershift-addon
  namespace: <managed-cluster-used-as-hosting-service-cluster> # the managed OCP cluster you want to install hypershift operator
spec:
  installNamespace: open-cluster-management-agent-addon
EOF
```

3. Confirm that the `hypershift-addon` is installed by running the following command:
  
```bash
$ oc get managedclusteraddons -n <managed-cluster-used-as-hosting-service-cluster> hypershift-addon
NAME               AVAILABLE   DEGRADED   PROGRESSING
hypershift-addon   True
```

Your HyperShift add-on is installed and the management cluster is available to manage HyperShift hosted clusters.

## Provision a HyperShift hosted cluster on AWS

After installing the HyperShift operator and enabling an existing cluster as a hosting service cluster, you can provision a hypershift hosted cluster via the `HypershiftDeployment` customer resource.

1. Create a cloud provider secret as a credential using the console or a file addition. You must have permissions to create infrastructure resources for your cluster, like VPCs, subnets, and NAT gateways. The account also must correspond to the account for your guest cluster, where your workers live. See [Create AWS infrastructure and IAM resources separately](https://hypershift-docs.netlify.app/how-to/aws/create-infra-iam-separately) in the HyperShift documentation for more information about the required permissions. The secret has the following format for AWS:

```yaml
apiVersion: v1
metadata:
  name: my-aws-cred
  namespace: <hypershift-deployment-ns>      # Where you will create HypershiftDeployment resources
type: Opaque
kind: Secret
stringData:
  ssh-publickey:          # Value
  ssh-privatekey:         # Value
  pullSecret:             # Value, required
  baseDomain:             # Value, required
  aws_secret_access_key:  # Value, required
  aws_access_key_id:      # Value, required
```

The `ssh-publickey` and `ssh-privatekey`, if provided, are used to access the worker nodes of the hosted cluster. If the SSH Key is provided in the `hostedCluster.spec.sshKey` or `hypershiftDeployment.spec.hostedClusterSpec.sshKey`, it takes precedence over the SSH Key provided in the cloud provider secret.

- To create this secret with the console, follow the credential creation steps by accessing *Credentials* in the navigation menu in MCE console: `https://<mce-multicluster-console>/multicloud/credentials/create`
  
![mce-console](./images/mce-console.png)  

- Using the CLI:

> The secret should be created where the HyperShift deployment controller is deployed. By default, it is deployed in the  `multicluster-engine` namespace.

```bash
$ oc create secret generic <my-secret> -n <hypershift-deployment-namespace> --from-literal=baseDomain='your.domain.com' --from-literal=aws_access_key_id='your-aws-access-key' --from-literal=aws_secret_access_key='your-aws-secret-key' --from-literal=pullSecret='{"auths":{"cloud.openshift.com":{"auth":"auth-info", "email":"xx@redhat.com"}, "quay.io":{"auth":"auth-info", "email":"xx@redhat.com"} } }' --from-literal=ssh-publickey='your-ssh-publickey' --from-literal=ssh-privatekey='your-ssh-privatekey'

# label the secret for backup
$ oc label secret <my-secret> -n <hypershift-deployment-namespace> cluster.open-cluster-management.io/backup=true
```

Note: `cluster.open-cluster-management.io/backup=true` is added to the secret so that the secret is backed up for disaster recovery.

1. Create a `HypershiftDeployment` custom resource. The `HypershiftDeployment` custom resource creates the infrastructure in the provider account, configures the infrastructure compute capacity in the created infrastructure, provisions the `nodePools` that use the hosted control plane, and creates a hosted control plane on a hosting service cluster. 

```bash
$ oc apply -f - <<EOF
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: HypershiftDeployment
metadata:
  name: <cluster>
  namespace: default
spec:
  hostingCluster: <hypershift-management-cluster>
  hostingNamespace: clusters
  infrastructure:
    cloudProvider:
      name: <my-secret>
    configure: True
    platform:
      aws:
        region: <region>
EOF
```

Replace `cluster` with the name of the cluster. 

Replace `hypershift-management-cluster` with the name of the cluster that hosts the HyperShift operator. 

Replace `my-secret` with the secret to access your cloud provider. 

Replace `region` with the region of your cloud provider.

Check each field [definition](./../api/v1alpha1/hypershiftdeployment_types.go)

1. Check the `HypershiftDeployment` status:

```bash
$ oc get hypershiftdeployment -n <hypershift-deployment-namespace> -w
```

1. After the hosted cluster is created, it will be imported to the hub automatically, you can check it with:
  
```bash
$ oc get managedcluster <hypershiftDeployment.Spec.infraID>
```

## Access the hosted cluster

The access secrets are stored in the {hypershift-management-cluster} namespace.
The formats of the secrets name are:

- kubeconfig secret: `<hypershiftDeployment.Spec.hostingNamespace>-<hypershiftDeployment.Name>-admin-kubeconfig` (e.g clusters-hypershift-demo-admin-kubeconfig)
- kubeadmin password secret: `<hypershiftDeployment.Spec.hostingNamespace>-<hypershiftDeployment.Name>-kubeadmin-password` (e.g clusters-hypershift-demo-kubeadmin-password)

## Destroying your hypershift Hosted cluster

Delete the HypershiftDeployment resource

```bash
$ oc delete hypershiftdeployment hypershift-demo -n default
```

## Destroying hypershift operator

Delete the hypershift-addon

```bash
$ oc delete managedclusteraddon -n <hypershift-management-cluster> hypershift-addon
```

## Customizing hostedcluster and nodepool specs in HypershiftDeployment custom resource

In a `HypershiftDeployment` custom resource, you can change `hostedcluster` and `nodepool` specifications. For example, you can change the OCP release image of the hosted cluster control plane and/or the nodepool, the management spec of the nodepool or the number of nodes in the nodepool.

```yaml
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: HypershiftDeployment
metadata:
  name: <cluster>
  namespace: default
spec:
  hostingCluster: <hosting-service-cluster>
  hostingNamespace: clusters
  hostedClusterSpec:
    networking:
      machineCIDR: 10.0.0.0/16    # Default
      networkType: OpenShiftSDN
      podCIDR: 10.132.0.0/14      # Default
      serviceCIDR: 172.31.0.0/16  # Default
    platform:
      type: AWS
    pullSecret:
      name: <cluster>-pull-secret    # This secret is created by the controller
    release:
      image: quay.io/openshift-release-dev/ocp-release:4.11.2-x86_64  # Default
    services:
    - service: APIServer
      servicePublishingStrategy:
        type: LoadBalancer
    - service: OAuthServer
      servicePublishingStrategy:
        type: Route
    - service: Konnectivity
      servicePublishingStrategy:
        type: Route
    - service: Ignition
      servicePublishingStrategy:
        type: Route
    sshKey: {}
  nodePools:
  - name: <cluster>
    spec:
      clusterName: <cluster>
      management:
        autoRepair: false
        replace:
          rollingUpdate:
            maxSurge: 1
            maxUnavailable: 0
          strategy: RollingUpdate
        upgradeType: Replace
      platform:
        aws:
          instanceType: m5.large
        type: AWS
      release:
        image: quay.io/openshift-release-dev/ocp-release:4.11.2-x86_64 # Default
      replicas: 2
  infrastructure:
    cloudProvider:
      name: <my-secret>
    configure: True
    platform:
      aws:
        region: <region>
```


## Provision a hypershift hosted cluster on bare-metal

Use the 'Agent' platform for HostedClusters with bare-metal worker nodes. The Agent platform uses the [Infrastructure Operator](https://github.com/openshift/assisted-service) (AKA Assisted Installer) to add worker nodes to a hosted cluster. For a primer on the Infrastructure Operator, see [here](https://github.com/openshift/assisted-service/blob/master/docs/hive-integration/kube-api-getting-started.md). In short, each bare-metal host should be booted with a Discovery Image that is provided by the Infrastructure Operator. The hosts can be booted manually or via user-provided automation, or by utilizing the [Cluster-Baremetal-Operator](https://github.com/openshift/cluster-baremetal-operator/blob/master/README.md) (CBO). Once booted, each host will run an agent process to facilitate discovering the host details and its installation. Each is represented by an Agent custom resource.

When you create a HostedCluster with the Agent platform, HyperShift will install the [Agent CAPI provider](https://github.com/openshift/cluster-api-provider-agent) in the HyperShift control plane namespace.

Upon scaling up a NodePool, a Machine will be created, and the CAPI provider will find a suitable Agent to match this Machine. Suitable means that the Agent is approved, is passing validations, is not currently bound (in use), and has the requirements specified on the NodePool Spec (e.g., minimum CPU/RAM, labels matching the label selector). You may monitor the installation of an Agent by checking its Status and Conditions.

Upon scaling down a NodePool, Agents will be unbound from the corresponding cluster. However, you must boot them with the Discovery Image once again before reusing them.

To use the Agent platform, the Infrastructure Operator must first be installed. Please see [here](https://hypershift-docs.netlify.app/how-to/agent/create-agent-cluster/) for details.

When creating the HostedCluster resource, set spec.platform.type to "Agent" and spec.platform.agent.agentNamespace to the namespace containing the Agent CRs you would like to use. For NodePools, set spec.platform.type to "Agent", and optionally specify a label selector for selecting the Agent CRs to in spec.platform.agent.agentLabelSelector.

The HypershiftDeployment would look like:

```bash
$ oc apply -f - <<EOF
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: HypershiftDeployment
metadata:
  name: hypershift-demo
  namespace: default
spec:
  hostingCluster: hypershift-management-cluster     # the hypershift management cluster name.
  hostingNamespace: clusters     # specify the namespace to which hostedcluster and noodpools belong on the hypershift management cluster.
  infrastructure:
    configure: True
    platform:
  platform:
    agent:
      agentNamespace: ${AGENT_NS}
    type: Agent
EOF
```

**NOTE**: If you wish to use `Agents` from a namespace that isn't the ${hypershift-management-cluster} namespace, you must create a role for capi-provider-agent in that namespace (this is the same namespace as specified in the HypershiftDeployment Spec `spec.platform.agent.agentNamespace`).
~~~sh
envsubst <<"EOF" | oc apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  creationTimestamp: null
  name: capi-provider-role
  namespace: ${AGENT_NS}
rules:
- apiGroups:
  - agent-install.openshift.io
  resources:
  - agents
  verbs:
  - '*'
EOF
~~~