# Creating a hosted cluster on a remote hosting cluster without HypershiftDeployment CR

## How HypershiftDeployment creates a hosted cluster on a remote hosting cluster

When you create a HypershiftDeployment CR, the HypershiftDeployment operator creates a manifestwork CR in the target managed (hypershift hosting) cluster's namespace on the ACM hub cluster. The payload of the manifestwork CR includes:

- the namespace where the hosted cluster is created
- HostedCluster CR
- NodePool CRs
- Pull secret
- Control plane operator credential secret
- Cloud control credential secret
- Node management credential secret
- SSH key secret
- etcd encryption secret

The work agent running on the target managed (hypershift hosting) cluster reconciles with this manifestwork to create the payload resources on the cluster. You can find more detaila on the manifestwork CR here https://open-cluster-management.io/concepts/manifestwork.

This is a sample manifestwork YAML.

```YAML
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: my-hosted-cluster
  namespace: my-hosting-cluster
spec:
  deleteOption:
    propagationPolicy: Orphan
  manifestConfigs:
  - feedbackRules:
    - jsonPaths:
      - name: reason
        path: .status.conditions[?(@.type=="Available")].reason
      - name: status
        path: .status.conditions[?(@.type=="Available")].status
      - name: message
        path: .status.conditions[?(@.type=="Available")].message
      - name: progress
        path: .status.version.history[?(@.state!="")].state
      type: JSONPaths
    resourceIdentifier:
      group: hypershift.openshift.io
      name: my-hosted-cluster
      namespace: clusters
      resource: hostedclusters
  - feedbackRules:
    - jsonPaths:
      - name: reason
        path: .status.conditions[?(@.type=="Ready")].reason
      - name: status
        path: .status.conditions[?(@.type=="Ready")].status
      - name: message
        path: .status.conditions[?(@.type=="Ready")].message
      type: JSONPaths
    resourceIdentifier:
      group: hypershift.openshift.io
      name: my-hosted-cluster
      namespace: clusters
      resource: nodepools
  workload:
    manifests:
    - apiVersion: v1
      kind: Namespace
      metadata:
        name: clusters
      spec: {}
      status: {}
    - apiVersion: hypershift.openshift.io/v1alpha1
      kind: HostedCluster
      metadata:
        annotations:
          cluster.open-cluster-management.io/hypershiftdeployment: default/my-hosted-cluster
        name: my-hosted-cluster
        namespace: clusters
      spec:
        autoscaling: {}
        controllerAvailabilityPolicy: SingleReplica
        dns:
          baseDomain: base.domain.com
          privateZoneID: ABCDE
          publicZoneID: FGIJK
        etcd:
          managed:
            storage:
              persistentVolume:
                size: 4Gi
              type: PersistentVolume
          managementType: Managed
        fips: false
        infraID: my-hosted-cluster-12345
        infrastructureAvailabilityPolicy: SingleReplica
        issuerURL: https://rj-aws-hyper.s3.us-east-1.amazonaws.com/my-hosted-cluster-12345
        networking:
          machineCIDR: 10.0.0.0/16
          networkType: OVNKubernetes
          podCIDR: 10.132.0.0/14
          serviceCIDR: 172.31.0.0/16
        olmCatalogPlacement: management
        platform:
          aws:
            cloudProviderConfig:
              subnet:
                id: subnet-123456789
              vpc: vpc-123456789
              zone: us-east-1a
            controlPlaneOperatorCreds:
              name: my-hosted-cluster-cpo-creds
            endpointAccess: Public
            kubeCloudControllerCreds:
              name: my-hosted-cluster-cloud-ctrl-creds
            nodePoolManagementCreds:
              name: my-hosted-cluster-node-mgmt-creds
            region: us-east-1
            resourceTags:
            - key: kubernetes.io/cluster/my-hosted-cluster-12345
              value: owned
            rolesRef:
              controlPlaneOperatorARN: ""
              imageRegistryARN: arn:aws:iam::987654321:role/my-hosted-cluster-12345-openshift-image-registry
              ingressARN: arn:aws:iam::987654321:role/my-hosted-cluster-12345-openshift-ingress
              kubeCloudControllerARN: ""
              networkARN: arn:aws:iam::987654321:role/my-hosted-cluster-12345-cloud-network-config-controller
              nodePoolManagementARN: ""
              storageARN: arn:aws:iam::987654321:role/my-hosted-cluster-12345-aws-ebs-csi-driver-controller
          type: AWS
        pullSecret:
          name: my-hosted-cluster-pull-secret
        release:
          image: quay.io/openshift-release-dev/ocp-release:4.11.0-rc.5-x86_64
        secretEncryption:
          aescbc:
            activeKey:
              name: my-hosted-cluster-etcd-encryption-key
          type: aescbc
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
        sshKey:
          name: my-hosted-cluster-ssh-key
    - apiVersion: hypershift.openshift.io/v1alpha1
      kind: NodePool
      metadata:
        labels:
          hypershift.openshift.io/auto-created-for-infra: my-hosted-cluster-12345
        name: my-hosted-cluster
        namespace: clusters
      spec:
        clusterName: my-hosted-cluster
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
            instanceProfile: my-hosted-cluster-12345-worker
            instanceType: t3.large
            rootVolume:
              size: 35
              type: gp3
            securityGroups:
            - id: sg-13247589643
            subnet:
              id: subnet-13247589643
          type: AWS
        release:
          image: quay.io/openshift-release-dev/ocp-release:4.11.0-rc.5-x86_64
        replicas: 2
    - apiVersion: v1
      data:
        .dockerconfigjson: docker-config-json-content
      kind: Secret
      metadata:
        labels:
          hypershift.openshift.io/auto-created-for-infra: my-hosted-cluster-12345
        name: my-hosted-cluster-pull-secret
        namespace: clusters
    - apiVersion: v1
      data:
        credentials: base64-encoded-cpo-creds
      kind: Secret
      metadata:
        labels:
          hypershift.openshift.io/auto-created-for-infra: my-hosted-cluster-12345
        name: my-hosted-cluster-cpo-creds
        namespace: clusters
    - apiVersion: v1
      data:
        credentials: base64-encoded-cloud-ctrl-creds
      kind: Secret
      metadata:
        labels:
          hypershift.openshift.io/auto-created-for-infra: my-hosted-cluster-12345
        name: my-hosted-cluster-cloud-ctrl-creds
        namespace: clusters
    - apiVersion: v1
      data:
        credentials: base64-encoded-node-mgmt-creds
      kind: Secret
      metadata:
        labels:
          hypershift.openshift.io/auto-created-for-infra: my-hosted-cluster-12345
        name: my-hosted-cluster-node-mgmt-creds
        namespace: clusters
    - apiVersion: v1
      data:
        id_rsa: private-ssh-key
        id_rsa.pub: public-ssh-key
      kind: Secret
      metadata:
        name: my-hosted-cluster-ssh-key
        namespace: clusters
    - apiVersion: v1
      data:
        key: base64-encoded-etcd-encryption-key
      kind: Secret
      metadata:
        name: my-hosted-cluster-etcd-encryption-key
        namespace: clusters
      type: Opaque
```

The payload resources are under `spec.workload.manifests`. The control plane operator credential secret, cloud control credential secret, and node management credential secret are base64 encode of the following content. Replace the role ARN with your ARN but keep `web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token`.


```
[default]
	role_arn = arn:aws:iam::987654321:role/my-hosted-cluster-12345-cloud-controller
	web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token
```

Once these resources are created on the hosting cluster, then the hypershift operator reconciles with these resources to create the specified hosted cluster. This manifestwork CR is the delivery or placement mechanism for a hosted cluster.

## How a hosted cluster is automatically imported into ACM hub cluster as a managed cluster

When you create a HypershiftDeployment CR, the HypershiftDeployment operator also creates a ManagedCluster CR on the ACM hub cluster.

```YAML
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  annotations:
    import.open-cluster-management.io/hosting-cluster-name: my-hosting-cluster
    import.open-cluster-management.io/klusterlet-deploy-mode: Hosted
    open-cluster-management/created-via: other
  labels:
    cloud: auto-detect
    cluster.open-cluster-management.io/clusterset: default
    name: my-hosted-cluster-12345
    vendor: OpenShift
  name: my-hosted-cluster-12345
spec:
  hubAcceptsClient: true
  leaseDurationSeconds: 60
```

The name of the managed cluster is the `infra ID` of the hosted cluster. `hubAcceptsClient: true` means that the ACM hub accepts or approves this managed cluster.

In the manifestwork YAML sample above, there is this annotation in the HostedCluster payload.

```YAML
        annotations:
          cluster.open-cluster-management.io/hypershiftdeployment: default/my-hosted-cluster
```

The ACM hypershift addon agent on the hosting cluster uses this annotation to know that once a hosted cluster is created with this annotation, it needs to complete the managed cluster registration from the agent side.

## Creating a hosted cluster on a remote hosting cluster without HypershiftDeployment CR

1. Enable hypershift-preview feature in MCE. https://github.com/stolostron/hypershift-deployment-controller/blob/main/docs/provision_hypershift_clusters_by_mce.md#enable-the-hosted-control-planes-related-components-on-the-hub-cluster

2. Enable hypershift addon to turn an ACM managed cluster into a hypershift management cluster. https://github.com/stolostron/hypershift-deployment-controller/blob/main/docs/provision_hypershift_clusters_by_mce.md#turn-one-of-the-managed-clusters-into-the-hypershift-management-cluster

3. Create a manifestwork CR in the hypershift management cluster's (ACM managed cluster's) namespace on the ACM hub cluster. Do not forget to add the following annotation to the HostedCluster resource. The value can be anything.

```YAML
        annotations:
          cluster.open-cluster-management.io/hypershiftdeployment: default/my-hosted-cluster
```

4. Create a ManagedCluster CR on ACM hub cluster.

## How HostedCluster and NodePools status are reported back to the manifestwork from the hosting cluster

Under the manifestwork's `spec.manifestConfigs`, you can specify feedback rules like this.

```YAML
  - feedbackRules:
    - jsonPaths:
      - name: reason
        path: .status.conditions[?(@.type=="Available")].reason
      - name: status
        path: .status.conditions[?(@.type=="Available")].status
      - name: message
        path: .status.conditions[?(@.type=="Available")].message
      - name: progress
        path: .status.version.history[?(@.state!="")].state
      type: JSONPaths
    resourceIdentifier:
      group: hypershift.openshift.io
      name: my-hosted-cluster
      namespace: clusters
      resource: hostedclusters
  - feedbackRules:
    - jsonPaths:
      - name: reason
        path: .status.conditions[?(@.type=="Ready")].reason
      - name: status
        path: .status.conditions[?(@.type=="Ready")].status
      - name: message
        path: .status.conditions[?(@.type=="Ready")].message
      type: JSONPaths
    resourceIdentifier:
      group: hypershift.openshift.io
      name: my-hosted-cluster
      namespace: clusters
      resource: nodepools
```

The `resourceIdentifier` specifies which resource you want feedback from and `jsonPaths` specifies the resource's fields you are interested in. Above feedback rules, you can see the status of hosted cluster and node pool in the status section of the manifestwork on ACM hub cluster. You can also specify more rules to collect other data about the resources from the hosting cluster.

```YAML
      resourceMeta:
        group: hypershift.openshift.io
        kind: HostedCluster
        name: my-hosted-cluster
        namespace: clusters
        ordinal: 1
        resource: hostedclusters
        version: v1alpha1
      statusFeedback:
        values:
        - fieldValue:
            string: HostedClusterAsExpected
            type: String
          name: reason
        - fieldValue:
            string: "True"
            type: String
          name: status
        - fieldValue:
            string: ""
            type: String
          name: message
        - fieldValue:
            string: Completed
            type: String
          name: progress
```

```YAML
      resourceMeta:
        group: hypershift.openshift.io
        kind: NodePool
        name: my-hosted-cluster
        namespace: clusters
        ordinal: 2
        resource: nodepools
        version: v1alpha1
      statusFeedback:
        values:
        - fieldValue:
            string: AsExpected
            type: String
          name: reason
        - fieldValue:
            string: "True"
            type: String
          name: status
```