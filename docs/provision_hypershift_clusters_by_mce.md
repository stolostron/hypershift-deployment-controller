# Provision Hypershift Clusters by MCE

The multicluster-engine(MCE) has been installed and at least one OCP managed cluster. We will make this OCP managed cluster a hypershift management cluster. It is possible to use the hub cluster to act as a hypershift management cluster, however, this requires importing the hub cluster as an OCP managed cluster called `local-cluster`:

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

## Enable the hypershift related components on the hub cluster

Because hypershift is a TP feature, the related components are disabled by default. We should enable it by editing the multiclusterengine resource to set the `spec.overrides.components[?(@.name=='hypershift-preview')].enabled` to `true`
```bash
$ oc get mce multiclusterengine-sample -ojsonpath="{.spec.overrides.components[?(@.name=='hypershift-preview')].enabled}"
true
```

## Turn one of the managed clusters into the hypershift management cluster

We call the cluster with the hypershift operator installed as the hypershift management cluster. In this section, we will use hypershift-addon to install a hypershift operator to one of the managed cluster.


1. If you plan to provision hosted clusters on the AWS platform, create an oidc S3 credentials secret for the hypershift operator, name is `hypershift-operator-oidc-provider-s3-credentials` in the `hypershift-management-cluster` namespace, which one you want to install hypershift operator.

The secret must contain 3 fields:
- `bucket`: An S3 bucket with public access to host OIDC discovery documents for your hypershift clusters
- `credentials`: Credentials to access the bucket
- `region`: Region of the S3 bucket

For details, please check: https://hypershift-docs.netlify.app/getting-started/ , you can create this secret by:
```bash
$ oc create secret generic hypershift-operator-oidc-provider-s3-credentials --from-file=credentials=$HOME/.aws/credentials --from-literal=bucket=<s3-bucket-for-hypershift> --from-literal=region=<region> -n <hypershift-management-cluster>
```

Add the special label to the `hypershift-operator-oidc-provider-s3-credentials` secret so that the secret is backed up for disaster recovery.
```
oc label secret hypershift-operator-oidc-provider-s3-credentials -n <hypershift-management-cluster> cluster.open-cluster-management.io/backup=true
```

2. Create ManagedClusterAddon hypershift-addon
```bash
$ oc apply -f - <<EOF
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: hypershift-addon
  namespace: hypershift-management-cluster # the managed OCP cluster you want to install hypershift operator
spec:
  installNamespace: open-cluster-management-agent-addon
EOF
```

3. Check the hypershift-addon is installed
```bash
$ oc get managedclusteraddons -n local-cluster hypershift-addon
NAME               AVAILABLE   DEGRADED   PROGRESSING
hypershift-addon   True
```

## Provision a hypershift hosted cluster on AWS

After the hypershift operator is installed, we can provision a hypershift hosted cluster by `HypershiftDeployment`

1. Create a cloud provider secret, it has the following format for AWS:
```yaml
apiVersion: v1
metadata:
  name: my-aws-cred
  namespace: default      # Where you will create HypershiftDeployment resources
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

You can create this secret by:
- ACM console: `https://<Advanced-Cluster-Management-Console>/multicloud/credentials/create`

or

- oc commands
```bash
$ oc create secret generic <my-secret> -n <hypershift-deployment-namespace> --from-literal=baseDomain='your.domain.com' --from-literal=aws_access_key_id='your-aws-access-key' --from-literal=aws_secret_access_key='your-aws-secret-key' --from-literal=pullSecret='{"auths":{"cloud.openshift.com":{"auth":"auth-info", "email":"xx@redhat.com"}, "quay.io":{"auth":"auth-info", "email":"xx@redhat.com"} } }' --from-literal=ssh-publickey='your-ssh-publickey' --from-literal=ssh-privatekey='your-ssh-privatekey'

$ oc label secret <my-secret> -n <hypershift-deployment-namespace> cluster.open-cluster-management.io/backup=true
```

Note: `cluster.open-cluster-management.io/backup=true` is added to the secret so that the secret is backed up for disaster recovery.

2. Create a HypershiftDeployment in the cloud provider secret namespace
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
    cloudProvider:
      name: <my-secret>
    configure: True
    platform:
      aws:
        region: <region>
EOF
```

Check each field [definition](./../api/v1alpha1/hypershiftdeployment_types.go)

3. Check the HypershiftDeployment status
```bash
$ oc get hypershiftdeployment -n default hypershift-demo -w
```

4. After the hosted cluster is created, it will be imported to the hub automatically, you can check it with:
```bash
$ oc get managedcluster <hypershiftDeployment.Spec.infraID>
```

## Provision a hypershift hosted cluster on bare-metal

Use the 'Agent' platform for HostedClusters with bare-metal worker nodes. The Agent platform uses the [Infrastructure Operator](https://github.com/openshift/assisted-service) (AKA Assisted Installer) to add worker nodes to a hosted cluster. For a primer on the Infrastructure Operator, see [here](https://github.com/openshift/assisted-service/blob/master/docs/hive-integration/kube-api-getting-started.md). In short, each bare-metal host should be booted with a Discovery Image that is provided by the Infrastructure Operator. The hosts can be booted manually or via user-provided automation, or by utilizing the [Cluster-Baremetal-Operator](https://github.com/openshift/cluster-baremetal-operator/blob/master/README.md) (CBO). Once booted, each host will run an agent process to facilitate discovering the host details and its installation. Each is represented by an Agent custom resource.

When you create a HostedCluster with the Agent platform, HyperShift will install the [Agent CAPI provider](https://github.com/openshift/cluster-api-provider-agent) in the HyperShift control plane namespace.

Upon scaling up a NodePool, a Machine will be created, and the CAPI provider will find a suitable Agent to match this Machine. Suitable means that the Agent is approved, is passing validations, is not currently bound (in use), and has the requirements specified on the NodePool Spec (e.g., minimum CPU/RAM, labels matching the label selector). You may monitor the installation of an Agent by checking its Status and Conditions.

Upon scaling down a NodePool, Agents will be unbound from the corresponding cluster. However, you must boot them with the Discovery Image once again before reusing them.

To use the Agent platform, the Assisted Service component must be enabled in the multiclusterengine resource on MCE or ACM hub cluster to install the infrastructure operator. Then infrastructure environment and bare metal host agents need to be configured prior to provisioning a hosted cluster. It is recommended to use the `local-cluster` managed cluster on MCE/ACM hub cluster as the hosting cluster so that all agent platform information is available to MCE/ACM hub cluster.

If you want to use other MCE/ACM managed cluster as the hosting cluster, Infrastructure Operator must first be installed on the managed cluster. Please see [here](https://hypershift-docs.netlify.app/how-to/agent/create-agent-cluster/) for details. Then infrastructure environment and bare metal host agents need to be configured on the cluster prior to provisioning a hosted cluster.

###### Enable assisted service on hosting cluster on MCE/ACM hub cluster

1. Create two persistent volumes for assisted service.
- `Capacity`: 10Gi
- `Access modes`: ReadWriteOnce
- `Volume mode`: Filesystem
- `StorageClass`: None

2. Enable the Infrastructure Operator.
```bash
$ oc patch multiclusterengine <mce_name> --type=merge -p '{"spec":{"overrides":{"components":[{"name":"assisted-service","enabled": true}]}}}'
```

3. Create the agentserviceconfig object. Double check the `ISO_URL` at https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos/${OCP_VERSION}/latest.
```bash
export DB_VOLUME_SIZE="10Gi"
export FS_VOLUME_SIZE="10Gi"
export OCP_VERSION="4.10"
export ARCH="x86_64"
export OCP_RELEASE_VERSION=$(curl -s https://mirror.openshift.com/pub/openshift-v4/${ARCH}/clients/ocp/latest-${OCP_VERSION}/release.txt | awk '/machine-os / { print $2 }')
export ISO_URL="https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos/${OCP_VERSION}/latest/rhcos-${OCP_VERSION}.3-${ARCH}-live.${ARCH}.iso"
export ROOT_FS_URL="https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos/${OCP_VERSION}/latest/rhcos-live-rootfs.${ARCH}.img"

envsubst <<"EOF" | oc apply -f -
apiVersion: agent-install.openshift.io/v1beta1
kind: AgentServiceConfig
metadata:
 name: agent
spec:
  databaseStorage:
    accessModes:
    - ReadWriteOnce
    resources:
      requests:
        storage: ${DB_VOLUME_SIZE}
  filesystemStorage:
    accessModes:
    - ReadWriteOnce
    resources:
      requests:
        storage: ${FS_VOLUME_SIZE}
  osImages:
    - openshiftVersion: "${OCP_VERSION}"
      version: "${OCP_RELEASE_VERSION}"
      url: "${ISO_URL}"
      rootFSUrl: "${ROOT_FS_URL}"
      cpuArchitecture: "${ARCH}"
EOF
```

4. Wait for the assisted-service pod to be ready.
```bash
until oc wait -n multicluster-engine $(oc get pods -n multicluster-engine -l app=assisted-service -o name) --for condition=Ready --timeout 10s >/dev/null 2>&1 ; do sleep 1 ; done
```

###### Create bare metal host and agent to be used as a worker node on hosting cluster

The number of `BareMetalHost` resources should match the `agent` namespace should match the number of replica in `NodePool`. Follow https://github.com/openshift/hypershift/blob/main/docs/content/how-to/agent/create-agent-cluster.md#adding-a-bare-metal-worker for creating `BareMetalHost` and `agent` resources. Stop when `agent` resources are created. Skip updating the nodepool part of the documentation. Note the namespce for the `agent` resources. This namespace will be used as `agentNamespace` in `HostedCluster` resource in the next section.


###### Provision a hosted cluster on local-cluster hosting cluster (MCE/ACM hub cluster)

Create `HostedCluster` and `NodePool` on the MCE cluster. These will be referenced by `HypershiftDeployment` to provision the hosted cluster on the target hosting cluster. We are going to create the `HostedCluster`, `NodePool` and  `HypershiftDeployment` all in `default` namespace on the MCE cluster. On the hosting cluster, hypershift deployment will create `HostedCluster` and `NodePool` in `clusters` namespace.

**Note: If you are provisioning this hosted cluster on `local-cluster` hosting cluster, do not create `HostedCluster` and `NodePool` resources and reference them because the hypershift operator on MCE cluster for `local-cluster` hosting cluster will reconcile them to create a hosted cluster. Instead, use HypershiftDeployment `spec.hostedClusterSpec` and `spec.nodePools`.


1. Create SSH key secret for `HostedCluster`.
```bash
envsubst <<"EOF" | oc apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: agent-demo-ssh-key
  namespace: default
stringData:
  id_rsa.pub: <SSH public key content>
EOF
```

2. Create pull secret for `HostedCluster`.
```bash
export PS64=$(echo -n <PULL_SECRET_CONTENT> | base64 -w0)
envsubst <<"EOF" | oc apply -f -
apiVersion: v1
data:
 .dockerconfigjson: ${PS64}
kind: Secret
metadata:
 name: agent-demo-pull-secret
 namespace: default
type: kubernetes.io/dockerconfigjson
EOF
```

3. Prepare `HostedCluster` spec. 
```bash
  dns:
    baseDomain: <BASE_DOMAIN>
  infraID: agent-demo
  networking:
    machineCIDR: ""
    networkType: OpenShiftSDN
    podCIDR: 10.132.0.0/14
    serviceCIDR: 172.32.0.0/16
  platform:
    agent:
      agentNamespace: <AGENT_NS_FROM_PREVIOUS_SECTION>
    type: Agent
  pullSecret:
    name: agent-demo-pull-secret
  release:
    image: quay.io/openshift-release-dev/ocp-release:4.10.16-x86_64
  services:
  - service: APIServer
    servicePublishingStrategy:
      nodePort:
        address: <NODE_IP>
      type: NodePort
  - service: OAuthServer
    servicePublishingStrategy:
      nodePort:
        address: <NODE_IP>
      type: NodePort
  - service: OIDC
    servicePublishingStrategy:
      nodePort:
        address: <NODE_IP>
      type: None
  - service: Konnectivity
    servicePublishingStrategy:
      nodePort:
        address: <NODE_IP>
      type: NodePort
  - service: Ignition
    servicePublishingStrategy:
      nodePort:
        address: <NODE_IP>
      type: NodePort
  sshKey:
    name: agent-demo-ssh-key
```

4. Prepare one or more `NodePool` specs.
```bash
name: nodepool1
spec:
  clusterName: agent-demo
  management:
    autoRepair: false
    replace:
      rollingUpdate:
        maxSurge: 1
        maxUnavailable: 0
      strategy: RollingUpdate
    upgradeType: Replace
  platform:
    type: Agent
  release:
    image: quay.io/openshift-release-dev/ocp-release:4.10.16-x86_64
  replicas: 1
```

5. Create `HypershiftDeployment`. Use the `HostedCluster` spec from step 3 and the `NodePool` specs from step 4 and insert them into `spec.hostedClusterSpec` and `spec.NodePools`.
```bash
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: HypershiftDeployment
metadata:
  name: agent-demo
  namespace: default
spec:
  hostingCluster: <HOSTING_CLUSTER_NAMESPACE>
  hostingNamespace: clusters
  infrastructure:
    configure: false 
  hostedClusterSpec:
    dns:
      baseDomain: <BASE_DOMAIN>
    infraID: agent-demo
    networking:
      machineCIDR: ""
      networkType: OpenShiftSDN
      podCIDR: 10.132.0.0/14
      serviceCIDR: 172.32.0.0/16
    platform:
      agent:
        agentNamespace: <AGENT_NS_FROM_PREVIOUS_SECTION>
      type: Agent
    pullSecret:
      name: agent-demo-pull-secret
    release:
      image: quay.io/openshift-release-dev/ocp-release:4.10.16-x86_64
    services:
    - service: APIServer
      servicePublishingStrategy:
        nodePort:
          address: <NODE_IP>
        type: NodePort
    - service: OAuthServer
      servicePublishingStrategy:
        nodePort:
          address: <NODE_IP>
        type: NodePort
    - service: OIDC
      servicePublishingStrategy:
        nodePort:
          address: <NODE_IP>
        type: None
    - service: Konnectivity
      servicePublishingStrategy:
        nodePort:
          address: <NODE_IP>
        type: NodePort
    - service: Ignition
      servicePublishingStrategy:
        nodePort:
          address: <NODE_IP>
        type: NodePort
    sshKey:
      name: agent-demo-ssh-key
  nodePools:
  - name: nodepool1
    spec:
      clusterName: agent-demo
      management:
        autoRepair: false
        replace:
          rollingUpdate:
            maxSurge: 1
            maxUnavailable: 0
          strategy: RollingUpdate
        upgradeType: Replace
      platform:
        type: Agent
      release:
        image: quay.io/openshift-release-dev/ocp-release:4.10.16-x86_64
      replicas: 1
```

6. Apply the `HypershiftDeployment` to provision the hosted cluster on the hosting cluster.


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
