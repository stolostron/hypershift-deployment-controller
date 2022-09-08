# Creating a hosted cluster on a remote hosting cluster without HypershiftDeployment CR

## How HypershiftDeployment creates a hosted cluster on a remote hosting cluster

When you create a HypershiftDeployment CR, the HypershiftDeployment operator creates a manifestwork CR in the target managed (hypershift hosting) cluster's namespace on the ACM hub cluster. The payload of the manifestwork CR includes:

- the namespace where the hosted cluster is created
- HostedCluster CR
- NodePool CRs
- Pull secret
- SSH key secret
- etcd encryption secret

The work agent running on the target managed (hypershift hosting) cluster reconciles with this manifestwork to create the payload resources on the cluster. You can find more details on the manifestwork CR here https://open-cluster-management.io/concepts/manifestwork.

This is a sample manifestwork YAML.

```YAML
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: my-hosted-cluster
  namespace: my-hosting-cluster
spec:
  deleteOption:
    propagationPolicy: SelectivelyOrphan
    selectivelyOrphans:
      orphaningRules:
      - group: ""
        name: clusters
        namespace: ""
        resource: namespaces
  manifestConfigs:
  - feedbackRules:
    - jsonPaths:
      - name: ReconciliationSucceeded-reason
        path: .status.conditions[?(@.type=="ReconciliationSucceeded")].reason
      - name: ReconciliationSucceeded-status
        path: .status.conditions[?(@.type=="ReconciliationSucceeded")].status
      - name: ReconciliationSucceeded-message
        path: .status.conditions[?(@.type=="ReconciliationSucceeded")].message
      - name: ReconciliationSucceeded-lastTransitionTime
        path: .status.conditions[?(@.type=="ReconciliationSucceeded")].lastTransitionTime
      - name: ReconciliationSucceeded-observedGeneration
        path: .status.conditions[?(@.type=="ReconciliationSucceeded")].observedGeneration
      - name: ClusterVersionSucceeding-reason
        path: .status.conditions[?(@.type=="ClusterVersionSucceeding")].reason
      - name: ClusterVersionSucceeding-status
        path: .status.conditions[?(@.type=="ClusterVersionSucceeding")].status
      - name: ClusterVersionSucceeding-message
        path: .status.conditions[?(@.type=="ClusterVersionSucceeding")].message
      - name: ClusterVersionSucceeding-lastTransitionTime
        path: .status.conditions[?(@.type=="ClusterVersionSucceeding")].lastTransitionTime
      - name: ClusterVersionSucceeding-observedGeneration
        path: .status.conditions[?(@.type=="ClusterVersionSucceeding")].observedGeneration
      - name: ClusterVersionUpgradeable-reason
        path: .status.conditions[?(@.type=="ClusterVersionUpgradeable")].reason
      - name: ClusterVersionUpgradeable-status
        path: .status.conditions[?(@.type=="ClusterVersionUpgradeable")].status
      - name: ClusterVersionUpgradeable-message
        path: .status.conditions[?(@.type=="ClusterVersionUpgradeable")].message
      - name: ClusterVersionUpgradeable-lastTransitionTime
        path: .status.conditions[?(@.type=="ClusterVersionUpgradeable")].lastTransitionTime
      - name: ClusterVersionUpgradeable-observedGeneration
        path: .status.conditions[?(@.type=="ClusterVersionUpgradeable")].observedGeneration
      - name: Available-reason
        path: .status.conditions[?(@.type=="Available")].reason
      - name: Available-status
        path: .status.conditions[?(@.type=="Available")].status
      - name: Available-message
        path: .status.conditions[?(@.type=="Available")].message
      - name: Available-lastTransitionTime
        path: .status.conditions[?(@.type=="Available")].lastTransitionTime
      - name: Available-observedGeneration
        path: .status.conditions[?(@.type=="Available")].observedGeneration
      - name: ValidConfiguration-reason
        path: .status.conditions[?(@.type=="ValidConfiguration")].reason
      - name: ValidConfiguration-status
        path: .status.conditions[?(@.type=="ValidConfiguration")].status
      - name: ValidConfiguration-message
        path: .status.conditions[?(@.type=="ValidConfiguration")].message
      - name: ValidConfiguration-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidConfiguration")].lastTransitionTime
      - name: ValidConfiguration-observedGeneration
        path: .status.conditions[?(@.type=="ValidConfiguration")].observedGeneration
      - name: SupportedHostedCluster-reason
        path: .status.conditions[?(@.type=="SupportedHostedCluster")].reason
      - name: SupportedHostedCluster-status
        path: .status.conditions[?(@.type=="SupportedHostedCluster")].status
      - name: SupportedHostedCluster-message
        path: .status.conditions[?(@.type=="SupportedHostedCluster")].message
      - name: SupportedHostedCluster-lastTransitionTime
        path: .status.conditions[?(@.type=="SupportedHostedCluster")].lastTransitionTime
      - name: SupportedHostedCluster-observedGeneration
        path: .status.conditions[?(@.type=="SupportedHostedCluster")].observedGeneration
      - name: ValidHostedControlPlaneConfiguration-reason
        path: .status.conditions[?(@.type=="ValidHostedControlPlaneConfiguration")].reason
      - name: ValidHostedControlPlaneConfiguration-status
        path: .status.conditions[?(@.type=="ValidHostedControlPlaneConfiguration")].status
      - name: ValidHostedControlPlaneConfiguration-message
        path: .status.conditions[?(@.type=="ValidHostedControlPlaneConfiguration")].message
      - name: ValidHostedControlPlaneConfiguration-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidHostedControlPlaneConfiguration")].lastTransitionTime
      - name: ValidHostedControlPlaneConfiguration-observedGeneration
        path: .status.conditions[?(@.type=="ValidHostedControlPlaneConfiguration")].observedGeneration
      - name: IgnitionEndpointAvailable-reason
        path: .status.conditions[?(@.type=="IgnitionEndpointAvailable")].reason
      - name: IgnitionEndpointAvailable-status
        path: .status.conditions[?(@.type=="IgnitionEndpointAvailable")].status
      - name: IgnitionEndpointAvailable-message
        path: .status.conditions[?(@.type=="IgnitionEndpointAvailable")].message
      - name: IgnitionEndpointAvailable-lastTransitionTime
        path: .status.conditions[?(@.type=="IgnitionEndpointAvailable")].lastTransitionTime
      - name: IgnitionEndpointAvailable-observedGeneration
        path: .status.conditions[?(@.type=="IgnitionEndpointAvailable")].observedGeneration
      - name: ReconciliationActive-reason
        path: .status.conditions[?(@.type=="ReconciliationActive")].reason
      - name: ReconciliationActive-status
        path: .status.conditions[?(@.type=="ReconciliationActive")].status
      - name: ReconciliationActive-message
        path: .status.conditions[?(@.type=="ReconciliationActive")].message
      - name: ReconciliationActive-lastTransitionTime
        path: .status.conditions[?(@.type=="ReconciliationActive")].lastTransitionTime
      - name: ReconciliationActive-observedGeneration
        path: .status.conditions[?(@.type=="ReconciliationActive")].observedGeneration
      - name: ValidReleaseImage-reason
        path: .status.conditions[?(@.type=="ValidReleaseImage")].reason
      - name: ValidReleaseImage-status
        path: .status.conditions[?(@.type=="ValidReleaseImage")].status
      - name: ValidReleaseImage-message
        path: .status.conditions[?(@.type=="ValidReleaseImage")].message
      - name: ValidReleaseImage-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidReleaseImage")].lastTransitionTime
      - name: ValidReleaseImage-observedGeneration
        path: .status.conditions[?(@.type=="ValidReleaseImage")].observedGeneration
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
      - name: AutoscalingEnabled-reason
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].reason
      - name: AutoscalingEnabled-status
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].status
      - name: AutoscalingEnabled-message
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].message
      - name: AutoscalingEnabled-observedGeneration
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].observedGeneration
      - name: AutoscalingEnabled-lastTransitionTime
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].lastTransitionTime
      - name: UpdateManagementEnabled-reason
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].reason
      - name: UpdateManagementEnabled-status
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].status
      - name: UpdateManagementEnabled-message
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].message
      - name: UpdateManagementEnabled-observedGeneration
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].observedGeneration
      - name: UpdateManagementEnabled-lastTransitionTime
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].lastTransitionTime
      - name: ValidReleaseImage-reason
        path: .status.conditions[?(@.type=="ValidReleaseImage")].reason
      - name: ValidReleaseImage-status
        path: .status.conditions[?(@.type=="ValidReleaseImage")].status
      - name: ValidReleaseImage-message
        path: .status.conditions[?(@.type=="ValidReleaseImage")].message
      - name: ValidReleaseImage-observedGeneration
        path: .status.conditions[?(@.type=="ValidReleaseImage")].observedGeneration
      - name: ValidReleaseImage-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidReleaseImage")].lastTransitionTime
      - name: ValidAMI-reason
        path: .status.conditions[?(@.type=="ValidAMI")].reason
      - name: ValidAMI-status
        path: .status.conditions[?(@.type=="ValidAMI")].status
      - name: ValidAMI-message
        path: .status.conditions[?(@.type=="ValidAMI")].message
      - name: ValidAMI-observedGeneration
        path: .status.conditions[?(@.type=="ValidAMI")].observedGeneration
      - name: ValidAMI-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidAMI")].lastTransitionTime
      - name: ValidMachineConfig-reason
        path: .status.conditions[?(@.type=="ValidMachineConfig")].reason
      - name: ValidMachineConfig-status
        path: .status.conditions[?(@.type=="ValidMachineConfig")].status
      - name: ValidMachineConfig-message
        path: .status.conditions[?(@.type=="ValidMachineConfig")].message
      - name: ValidMachineConfig-observedGeneration
        path: .status.conditions[?(@.type=="ValidMachineConfig")].observedGeneration
      - name: ValidMachineConfig-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidMachineConfig")].lastTransitionTime
      - name: AutorepairEnabled-reason
        path: .status.conditions[?(@.type=="AutorepairEnabled")].reason
      - name: AutorepairEnabled-status
        path: .status.conditions[?(@.type=="AutorepairEnabled")].status
      - name: AutorepairEnabled-message
        path: .status.conditions[?(@.type=="AutorepairEnabled")].message
      - name: AutorepairEnabled-observedGeneration
        path: .status.conditions[?(@.type=="AutorepairEnabled")].observedGeneration
      - name: AutorepairEnabled-lastTransitionTime
        path: .status.conditions[?(@.type=="AutorepairEnabled")].lastTransitionTime
      - name: Ready-reason
        path: .status.conditions[?(@.type=="Ready")].reason
      - name: Ready-status
        path: .status.conditions[?(@.type=="Ready")].status
      - name: Ready-message
        path: .status.conditions[?(@.type=="Ready")].message
      - name: Ready-observedGeneration
        path: .status.conditions[?(@.type=="Ready")].observedGeneration
      - name: Ready-lastTransitionTime
        path: .status.conditions[?(@.type=="Ready")].lastTransitionTime
      type: JSONPaths
    resourceIdentifier:
      group: hypershift.openshift.io
      name: my-hosted-cluster-nodepool-1
      namespace: clusters
      resource: nodepools
  - feedbackRules:
    - jsonPaths:
      - name: AutoscalingEnabled-reason
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].reason
      - name: AutoscalingEnabled-status
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].status
      - name: AutoscalingEnabled-message
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].message
      - name: AutoscalingEnabled-observedGeneration
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].observedGeneration
      - name: AutoscalingEnabled-lastTransitionTime
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].lastTransitionTime
      - name: UpdateManagementEnabled-reason
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].reason
      - name: UpdateManagementEnabled-status
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].status
      - name: UpdateManagementEnabled-message
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].message
      - name: UpdateManagementEnabled-observedGeneration
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].observedGeneration
      - name: UpdateManagementEnabled-lastTransitionTime
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].lastTransitionTime
      - name: ValidReleaseImage-reason
        path: .status.conditions[?(@.type=="ValidReleaseImage")].reason
      - name: ValidReleaseImage-status
        path: .status.conditions[?(@.type=="ValidReleaseImage")].status
      - name: ValidReleaseImage-message
        path: .status.conditions[?(@.type=="ValidReleaseImage")].message
      - name: ValidReleaseImage-observedGeneration
        path: .status.conditions[?(@.type=="ValidReleaseImage")].observedGeneration
      - name: ValidReleaseImage-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidReleaseImage")].lastTransitionTime
      - name: ValidAMI-reason
        path: .status.conditions[?(@.type=="ValidAMI")].reason
      - name: ValidAMI-status
        path: .status.conditions[?(@.type=="ValidAMI")].status
      - name: ValidAMI-message
        path: .status.conditions[?(@.type=="ValidAMI")].message
      - name: ValidAMI-observedGeneration
        path: .status.conditions[?(@.type=="ValidAMI")].observedGeneration
      - name: ValidAMI-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidAMI")].lastTransitionTime
      - name: ValidMachineConfig-reason
        path: .status.conditions[?(@.type=="ValidMachineConfig")].reason
      - name: ValidMachineConfig-status
        path: .status.conditions[?(@.type=="ValidMachineConfig")].status
      - name: ValidMachineConfig-message
        path: .status.conditions[?(@.type=="ValidMachineConfig")].message
      - name: ValidMachineConfig-observedGeneration
        path: .status.conditions[?(@.type=="ValidMachineConfig")].observedGeneration
      - name: ValidMachineConfig-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidMachineConfig")].lastTransitionTime
      - name: AutorepairEnabled-reason
        path: .status.conditions[?(@.type=="AutorepairEnabled")].reason
      - name: AutorepairEnabled-status
        path: .status.conditions[?(@.type=="AutorepairEnabled")].status
      - name: AutorepairEnabled-message
        path: .status.conditions[?(@.type=="AutorepairEnabled")].message
      - name: AutorepairEnabled-observedGeneration
        path: .status.conditions[?(@.type=="AutorepairEnabled")].observedGeneration
      - name: AutorepairEnabled-lastTransitionTime
        path: .status.conditions[?(@.type=="AutorepairEnabled")].lastTransitionTime
      - name: Ready-reason
        path: .status.conditions[?(@.type=="Ready")].reason
      - name: Ready-status
        path: .status.conditions[?(@.type=="Ready")].status
      - name: Ready-message
        path: .status.conditions[?(@.type=="Ready")].message
      - name: Ready-observedGeneration
        path: .status.conditions[?(@.type=="Ready")].observedGeneration
      - name: Ready-lastTransitionTime
        path: .status.conditions[?(@.type=="Ready")].lastTransitionTime
      type: JSONPaths
    resourceIdentifier:
      group: hypershift.openshift.io
      name: my-hosted-cluster-nodepool-2
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
            endpointAccess: Public
            region: us-east-1
            rolesRef:
              controlPlaneOperatorARN: arn:aws:iam::987654321:role/my-hosted-cluster-12345-control-plane-operator
              imageRegistryARN: arn:aws:iam::987654321:role/my-hosted-cluster-12345-openshift-image-registry
              ingressARN: arn:aws:iam::987654321:role/my-hosted-cluster-12345-openshift-ingress
              kubeCloudControllerARN: arn:aws:iam::987654321:role/my-hosted-cluster-12345-cloud-controller
              networkARN: arn:aws:iam::987654321:role/my-hosted-cluster-12345-cloud-network-config-controller
              nodePoolManagementARN: arn:aws:iam::987654321:role/my-hosted-cluster-12345-node-pool
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
        name: my-hosted-cluster-nodepool-1
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
    - apiVersion: hypershift.openshift.io/v1alpha1
      kind: NodePool
      metadata:
        name: my-hosted-cluster-nodepool-2
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
            instanceType: t3.large
            rootVolume:
              size: 35
              type: gp3
            securityGroups:
            - id: sg-13247589643
            subnet:
              id: subnet-5353628644
          type: AWS
        release:
          image: quay.io/openshift-release-dev/ocp-release:4.11.0-rc.5-x86_64
        replicas: 2
    - apiVersion: v1
      data:
        .dockerconfigjson: docker-config-json-content
      kind: Secret
      metadata:
        name: my-hosted-cluster-pull-secret
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

The payload resources are under `spec.workload.manifests`. Once these resources are created on the hosting cluster, the hypershift operator reconciles with these resources to create the specified hosted cluster. This manifestwork CR is the delivery or placement mechanism for a hosted cluster.

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

Under the manifestwork's `spec.manifestConfigs`, you can specify feedback rules to periodically get the latest states of the resources. This example configures manifestwork to collect the entire status of the hosted cluster and both nodepools from the hosting cluster.

```YAML
  - feedbackRules:
    - jsonPaths:
      - name: ReconciliationSucceeded-reason
        path: .status.conditions[?(@.type=="ReconciliationSucceeded")].reason
      - name: ReconciliationSucceeded-status
        path: .status.conditions[?(@.type=="ReconciliationSucceeded")].status
      - name: ReconciliationSucceeded-message
        path: .status.conditions[?(@.type=="ReconciliationSucceeded")].message
      - name: ReconciliationSucceeded-lastTransitionTime
        path: .status.conditions[?(@.type=="ReconciliationSucceeded")].lastTransitionTime
      - name: ReconciliationSucceeded-observedGeneration
        path: .status.conditions[?(@.type=="ReconciliationSucceeded")].observedGeneration
      - name: ClusterVersionSucceeding-reason
        path: .status.conditions[?(@.type=="ClusterVersionSucceeding")].reason
      - name: ClusterVersionSucceeding-status
        path: .status.conditions[?(@.type=="ClusterVersionSucceeding")].status
      - name: ClusterVersionSucceeding-message
        path: .status.conditions[?(@.type=="ClusterVersionSucceeding")].message
      - name: ClusterVersionSucceeding-lastTransitionTime
        path: .status.conditions[?(@.type=="ClusterVersionSucceeding")].lastTransitionTime
      - name: ClusterVersionSucceeding-observedGeneration
        path: .status.conditions[?(@.type=="ClusterVersionSucceeding")].observedGeneration
      - name: ClusterVersionUpgradeable-reason
        path: .status.conditions[?(@.type=="ClusterVersionUpgradeable")].reason
      - name: ClusterVersionUpgradeable-status
        path: .status.conditions[?(@.type=="ClusterVersionUpgradeable")].status
      - name: ClusterVersionUpgradeable-message
        path: .status.conditions[?(@.type=="ClusterVersionUpgradeable")].message
      - name: ClusterVersionUpgradeable-lastTransitionTime
        path: .status.conditions[?(@.type=="ClusterVersionUpgradeable")].lastTransitionTime
      - name: ClusterVersionUpgradeable-observedGeneration
        path: .status.conditions[?(@.type=="ClusterVersionUpgradeable")].observedGeneration
      - name: Available-reason
        path: .status.conditions[?(@.type=="Available")].reason
      - name: Available-status
        path: .status.conditions[?(@.type=="Available")].status
      - name: Available-message
        path: .status.conditions[?(@.type=="Available")].message
      - name: Available-lastTransitionTime
        path: .status.conditions[?(@.type=="Available")].lastTransitionTime
      - name: Available-observedGeneration
        path: .status.conditions[?(@.type=="Available")].observedGeneration
      - name: ValidConfiguration-reason
        path: .status.conditions[?(@.type=="ValidConfiguration")].reason
      - name: ValidConfiguration-status
        path: .status.conditions[?(@.type=="ValidConfiguration")].status
      - name: ValidConfiguration-message
        path: .status.conditions[?(@.type=="ValidConfiguration")].message
      - name: ValidConfiguration-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidConfiguration")].lastTransitionTime
      - name: ValidConfiguration-observedGeneration
        path: .status.conditions[?(@.type=="ValidConfiguration")].observedGeneration
      - name: SupportedHostedCluster-reason
        path: .status.conditions[?(@.type=="SupportedHostedCluster")].reason
      - name: SupportedHostedCluster-status
        path: .status.conditions[?(@.type=="SupportedHostedCluster")].status
      - name: SupportedHostedCluster-message
        path: .status.conditions[?(@.type=="SupportedHostedCluster")].message
      - name: SupportedHostedCluster-lastTransitionTime
        path: .status.conditions[?(@.type=="SupportedHostedCluster")].lastTransitionTime
      - name: SupportedHostedCluster-observedGeneration
        path: .status.conditions[?(@.type=="SupportedHostedCluster")].observedGeneration
      - name: ValidHostedControlPlaneConfiguration-reason
        path: .status.conditions[?(@.type=="ValidHostedControlPlaneConfiguration")].reason
      - name: ValidHostedControlPlaneConfiguration-status
        path: .status.conditions[?(@.type=="ValidHostedControlPlaneConfiguration")].status
      - name: ValidHostedControlPlaneConfiguration-message
        path: .status.conditions[?(@.type=="ValidHostedControlPlaneConfiguration")].message
      - name: ValidHostedControlPlaneConfiguration-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidHostedControlPlaneConfiguration")].lastTransitionTime
      - name: ValidHostedControlPlaneConfiguration-observedGeneration
        path: .status.conditions[?(@.type=="ValidHostedControlPlaneConfiguration")].observedGeneration
      - name: IgnitionEndpointAvailable-reason
        path: .status.conditions[?(@.type=="IgnitionEndpointAvailable")].reason
      - name: IgnitionEndpointAvailable-status
        path: .status.conditions[?(@.type=="IgnitionEndpointAvailable")].status
      - name: IgnitionEndpointAvailable-message
        path: .status.conditions[?(@.type=="IgnitionEndpointAvailable")].message
      - name: IgnitionEndpointAvailable-lastTransitionTime
        path: .status.conditions[?(@.type=="IgnitionEndpointAvailable")].lastTransitionTime
      - name: IgnitionEndpointAvailable-observedGeneration
        path: .status.conditions[?(@.type=="IgnitionEndpointAvailable")].observedGeneration
      - name: ReconciliationActive-reason
        path: .status.conditions[?(@.type=="ReconciliationActive")].reason
      - name: ReconciliationActive-status
        path: .status.conditions[?(@.type=="ReconciliationActive")].status
      - name: ReconciliationActive-message
        path: .status.conditions[?(@.type=="ReconciliationActive")].message
      - name: ReconciliationActive-lastTransitionTime
        path: .status.conditions[?(@.type=="ReconciliationActive")].lastTransitionTime
      - name: ReconciliationActive-observedGeneration
        path: .status.conditions[?(@.type=="ReconciliationActive")].observedGeneration
      - name: ValidReleaseImage-reason
        path: .status.conditions[?(@.type=="ValidReleaseImage")].reason
      - name: ValidReleaseImage-status
        path: .status.conditions[?(@.type=="ValidReleaseImage")].status
      - name: ValidReleaseImage-message
        path: .status.conditions[?(@.type=="ValidReleaseImage")].message
      - name: ValidReleaseImage-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidReleaseImage")].lastTransitionTime
      - name: ValidReleaseImage-observedGeneration
        path: .status.conditions[?(@.type=="ValidReleaseImage")].observedGeneration
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
      - name: AutoscalingEnabled-reason
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].reason
      - name: AutoscalingEnabled-status
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].status
      - name: AutoscalingEnabled-message
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].message
      - name: AutoscalingEnabled-observedGeneration
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].observedGeneration
      - name: AutoscalingEnabled-lastTransitionTime
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].lastTransitionTime
      - name: UpdateManagementEnabled-reason
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].reason
      - name: UpdateManagementEnabled-status
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].status
      - name: UpdateManagementEnabled-message
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].message
      - name: UpdateManagementEnabled-observedGeneration
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].observedGeneration
      - name: UpdateManagementEnabled-lastTransitionTime
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].lastTransitionTime
      - name: ValidReleaseImage-reason
        path: .status.conditions[?(@.type=="ValidReleaseImage")].reason
      - name: ValidReleaseImage-status
        path: .status.conditions[?(@.type=="ValidReleaseImage")].status
      - name: ValidReleaseImage-message
        path: .status.conditions[?(@.type=="ValidReleaseImage")].message
      - name: ValidReleaseImage-observedGeneration
        path: .status.conditions[?(@.type=="ValidReleaseImage")].observedGeneration
      - name: ValidReleaseImage-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidReleaseImage")].lastTransitionTime
      - name: ValidAMI-reason
        path: .status.conditions[?(@.type=="ValidAMI")].reason
      - name: ValidAMI-status
        path: .status.conditions[?(@.type=="ValidAMI")].status
      - name: ValidAMI-message
        path: .status.conditions[?(@.type=="ValidAMI")].message
      - name: ValidAMI-observedGeneration
        path: .status.conditions[?(@.type=="ValidAMI")].observedGeneration
      - name: ValidAMI-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidAMI")].lastTransitionTime
      - name: ValidMachineConfig-reason
        path: .status.conditions[?(@.type=="ValidMachineConfig")].reason
      - name: ValidMachineConfig-status
        path: .status.conditions[?(@.type=="ValidMachineConfig")].status
      - name: ValidMachineConfig-message
        path: .status.conditions[?(@.type=="ValidMachineConfig")].message
      - name: ValidMachineConfig-observedGeneration
        path: .status.conditions[?(@.type=="ValidMachineConfig")].observedGeneration
      - name: ValidMachineConfig-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidMachineConfig")].lastTransitionTime
      - name: AutorepairEnabled-reason
        path: .status.conditions[?(@.type=="AutorepairEnabled")].reason
      - name: AutorepairEnabled-status
        path: .status.conditions[?(@.type=="AutorepairEnabled")].status
      - name: AutorepairEnabled-message
        path: .status.conditions[?(@.type=="AutorepairEnabled")].message
      - name: AutorepairEnabled-observedGeneration
        path: .status.conditions[?(@.type=="AutorepairEnabled")].observedGeneration
      - name: AutorepairEnabled-lastTransitionTime
        path: .status.conditions[?(@.type=="AutorepairEnabled")].lastTransitionTime
      - name: Ready-reason
        path: .status.conditions[?(@.type=="Ready")].reason
      - name: Ready-status
        path: .status.conditions[?(@.type=="Ready")].status
      - name: Ready-message
        path: .status.conditions[?(@.type=="Ready")].message
      - name: Ready-observedGeneration
        path: .status.conditions[?(@.type=="Ready")].observedGeneration
      - name: Ready-lastTransitionTime
        path: .status.conditions[?(@.type=="Ready")].lastTransitionTime
      type: JSONPaths
    resourceIdentifier:
      group: hypershift.openshift.io
      name: my-hosted-cluster-nodepool-1
      namespace: clusters
      resource: nodepools
  - feedbackRules:
    - jsonPaths:
      - name: AutoscalingEnabled-reason
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].reason
      - name: AutoscalingEnabled-status
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].status
      - name: AutoscalingEnabled-message
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].message
      - name: AutoscalingEnabled-observedGeneration
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].observedGeneration
      - name: AutoscalingEnabled-lastTransitionTime
        path: .status.conditions[?(@.type=="AutoscalingEnabled")].lastTransitionTime
      - name: UpdateManagementEnabled-reason
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].reason
      - name: UpdateManagementEnabled-status
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].status
      - name: UpdateManagementEnabled-message
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].message
      - name: UpdateManagementEnabled-observedGeneration
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].observedGeneration
      - name: UpdateManagementEnabled-lastTransitionTime
        path: .status.conditions[?(@.type=="UpdateManagementEnabled")].lastTransitionTime
      - name: ValidReleaseImage-reason
        path: .status.conditions[?(@.type=="ValidReleaseImage")].reason
      - name: ValidReleaseImage-status
        path: .status.conditions[?(@.type=="ValidReleaseImage")].status
      - name: ValidReleaseImage-message
        path: .status.conditions[?(@.type=="ValidReleaseImage")].message
      - name: ValidReleaseImage-observedGeneration
        path: .status.conditions[?(@.type=="ValidReleaseImage")].observedGeneration
      - name: ValidReleaseImage-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidReleaseImage")].lastTransitionTime
      - name: ValidAMI-reason
        path: .status.conditions[?(@.type=="ValidAMI")].reason
      - name: ValidAMI-status
        path: .status.conditions[?(@.type=="ValidAMI")].status
      - name: ValidAMI-message
        path: .status.conditions[?(@.type=="ValidAMI")].message
      - name: ValidAMI-observedGeneration
        path: .status.conditions[?(@.type=="ValidAMI")].observedGeneration
      - name: ValidAMI-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidAMI")].lastTransitionTime
      - name: ValidMachineConfig-reason
        path: .status.conditions[?(@.type=="ValidMachineConfig")].reason
      - name: ValidMachineConfig-status
        path: .status.conditions[?(@.type=="ValidMachineConfig")].status
      - name: ValidMachineConfig-message
        path: .status.conditions[?(@.type=="ValidMachineConfig")].message
      - name: ValidMachineConfig-observedGeneration
        path: .status.conditions[?(@.type=="ValidMachineConfig")].observedGeneration
      - name: ValidMachineConfig-lastTransitionTime
        path: .status.conditions[?(@.type=="ValidMachineConfig")].lastTransitionTime
      - name: AutorepairEnabled-reason
        path: .status.conditions[?(@.type=="AutorepairEnabled")].reason
      - name: AutorepairEnabled-status
        path: .status.conditions[?(@.type=="AutorepairEnabled")].status
      - name: AutorepairEnabled-message
        path: .status.conditions[?(@.type=="AutorepairEnabled")].message
      - name: AutorepairEnabled-observedGeneration
        path: .status.conditions[?(@.type=="AutorepairEnabled")].observedGeneration
      - name: AutorepairEnabled-lastTransitionTime
        path: .status.conditions[?(@.type=="AutorepairEnabled")].lastTransitionTime
      - name: Ready-reason
        path: .status.conditions[?(@.type=="Ready")].reason
      - name: Ready-status
        path: .status.conditions[?(@.type=="Ready")].status
      - name: Ready-message
        path: .status.conditions[?(@.type=="Ready")].message
      - name: Ready-observedGeneration
        path: .status.conditions[?(@.type=="Ready")].observedGeneration
      - name: Ready-lastTransitionTime
        path: .status.conditions[?(@.type=="Ready")].lastTransitionTime
      type: JSONPaths
    resourceIdentifier:
      group: hypershift.openshift.io
      name: my-hosted-cluster-nodepool-2
      namespace: clusters
      resource: nodepools
```

The `resourceIdentifier` specifies which resource you want feedback from and `jsonPaths` specifies the resources fields you are interested in. Above feedback rules, you can see the entire status of hosted cluster and node pools in the status section of the manifestwork on ACM hub cluster. You can also specify more rules to collect other data about the resources from the hosting cluster. In this example, there are two nodepools `my-hosted-cluster-nodepool-1` and `my-hosted-cluster-nodepool-2` associated with the hosted cluster so the feedbackRules are specified for each nodepool to collect the status information from both nodepools.

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
            string: ReconciliatonSucceeded
            type: String
          name: ReconciliatonSucceeded-reason
        - fieldValue:
            string: "True"
            type: String
          name: ReconciliatonSucceeded-status
        - fieldValue:
            string: ""
            type: String
          name: ReconciliatonSucceeded-message
        - fieldValue:
            string: "2022-07-28T16:32:58Z"
            type: String
          name: ReconciliatonSucceeded-lastTransitionTime
        - fieldValue:
            string: "24"
            type: String
          name: ReconciliatonSucceeded-observedGeneration
        - fieldValue:
            string: ClusterVersionSucceeding
            type: String
          name: ClusterVersionSucceeding-reason
        - fieldValue:
            string: "True"
            type: String
          name: ClusterVersionSucceeding-status
        - fieldValue:
            string: ""
            type: String
          name: ClusterVersionSucceeding-message
        - fieldValue:
            string: "2022-07-28T16:32:58Z"
            type: String
          name: ClusterVersionSucceeding-lastTransitionTime
        - fieldValue:
            string: "24"
            type: String
          name: ClusterVersionSucceeding-observedGeneration
        - fieldValue:
            string: ClusterVersionStatusUnknown
            type: String
          name: ClusterVersionUpgradeable-reason
        - fieldValue:
            string: "Unknown"
            type: String
          name: ClusterVersionUpgradeable-status
        - fieldValue:
            string: ""
            type: String
          name: ClusterVersionUpgradeable-message
        - fieldValue:
            string: "2022-07-28T16:32:58Z"
            type: String
          name: ClusterVersionUpgradeable-lastTransitionTime
        - fieldValue:
            string: "24"
            type: String
          name: ClusterVersionUpgradeable-observedGeneration
        - fieldValue:
            string: Available
            type: String
          name: Available-reason
        - fieldValue:
            string: "True"
            type: String
          name: Available-status
        - fieldValue:
            string: ""
            type: String
          name: Available-message
        - fieldValue:
            string: "2022-07-28T16:32:58Z"
            type: String
          name: Available-lastTransitionTime
        - fieldValue:
            string: "24"
            type: String
          name: Available-observedGeneration
        - fieldValue:
            string: ValidConfiguration
            type: String
          name: ValidConfiguration-reason
        - fieldValue:
            string: "True"
            type: String
          name: ValidConfiguration-status
        - fieldValue:
            string: "Configuration passes validation"
            type: String
          name: ValidConfiguration-message
        - fieldValue:
            string: "2022-07-28T16:32:58Z"
            type: String
          name: ValidConfiguration-lastTransitionTime
        - fieldValue:
            string: "24"
            type: String
          name: ValidConfiguration-observedGeneration
        - fieldValue:
            string: HostedClusterAsExpected
            type: String
          name: SupportedHostedCluster-reason
        - fieldValue:
            string: "True"
            type: String
          name: SupportedHostedCluster-status
        - fieldValue:
            string: "HostedCluster is support by operator configuration"
            type: String
          name: SupportedHostedCluster-message
        - fieldValue:
            string: "2022-07-28T16:32:58Z"
            type: String
          name: SupportedHostedCluster-lastTransitionTime
        - fieldValue:
            string: "24"
            type: String
          name: SupportedHostedCluster-observedGeneration
        - fieldValue:
            string: HostedClusterAsExpected
            type: String
          name: ValidHostedControlPlaneConfiguration-reason
        - fieldValue:
            string: "True"
            type: String
          name: ValidHostedControlPlaneConfiguration-status
        - fieldValue:
            string: "Configuration passes validation"
            type: String
          name: ValidHostedControlPlaneConfiguration-message
        - fieldValue:
            string: "2022-07-28T16:32:58Z"
            type: String
          name: ValidHostedControlPlaneConfiguration-lastTransitionTime
        - fieldValue:
            string: "24"
            type: String
          name: ValidHostedControlPlaneConfiguration-observedGeneration
        - fieldValue:
            string: IgnitionServerDeploymentAsExpected
            type: String
          name: IgnitionEndpointAvailable-reason
        - fieldValue:
            string: "True"
            type: String
          name: IgnitionEndpointAvailable-status
        - fieldValue:
            string: ""
            type: String
          name: IgnitionEndpointAvailable-message
        - fieldValue:
            string: "2022-07-28T16:32:58Z"
            type: String
          name: IgnitionEndpointAvailable-lastTransitionTime
        - fieldValue:
            string: "24"
            type: String
          name: IgnitionEndpointAvailable-observedGeneration
        - fieldValue:
            string: ReconciliationActive
            type: String
          name: ReconciliationActive-reason
        - fieldValue:
            string: "True"
            type: String
          name: ReconciliationActive-status
        - fieldValue:
            string: "Reconciliation active on resource"
            type: String
          name: ReconciliationActive-message
        - fieldValue:
            string: "2022-07-28T16:32:58Z"
            type: String
          name: ReconciliationActive-lastTransitionTime
        - fieldValue:
            string: "24"
            type: String
          name: ReconciliationActive-observedGeneration
        - fieldValue:
            string: AsExpected
            type: String
          name: ValidReleaseImage-reason
        - fieldValue:
            string: "True"
            type: String
          name: ValidReleaseImage-status
        - fieldValue:
            string: "Release image is valid"
            type: String
          name: ValidReleaseImage-message
        - fieldValue:
            string: "2022-07-28T16:32:58Z"
            type: String
          name: ValidReleaseImage-lastTransitionTime
        - fieldValue:
            string: "24"
            type: String
          name: ValidReleaseImage-observedGeneration
        - fieldValue:
            string: Completed
            type: String
          name: progress
```

```YAML
      resourceMeta:
        group: hypershift.openshift.io
        kind: NodePool
        name: my-hosted-cluster-nodepool-1
        namespace: clusters
        ordinal: 2
        resource: nodepools
        version: v1alpha1
      statusFeedback:
        values:
        - fieldValue:
            string: AsExpected
            type: String
          name: AutoscalingEnabled-reason
        - fieldValue:
            string: "False"
            type: String
          name: AutoscalingEnabled-status
        - fieldValue:
            string: ""
            type: String
          name: AutoscalingEnabled-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: AutoscalingEnabled-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: AutoscalingEnabled-observedGeneration
        - fieldValue:
            string: AsExpected
            type: String
          name: UpdateManagementEnabled-reason
        - fieldValue:
            string: "True"
            type: String
          name: UpdateManagementEnabled-status
        - fieldValue:
            string: ""
            type: String
          name: UpdateManagementEnabled-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: AutoscaliUpdateManagementEnabledngEnabled-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: UpdateManagementEnabled-observedGeneration
        - fieldValue:
            string: AsExpected
            type: String
          name: ValidReleaseImage-reason
        - fieldValue:
            string: "True"
            type: String
          name: ValidReleaseImage-status
        - fieldValue:
            string: "Using release image: quay.io/openshift-release-dev/ocp-release:4.10.15-x86_64"
            type: String
          name: ValidReleaseImage-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: ValidReleaseImage-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: ValidReleaseImage-observedGeneration
        - fieldValue:
            string: AsExpected
            type: String
          name: ValidAMI-reason
        - fieldValue:
            string: "True"
            type: String
          name: ValidAMI-status
        - fieldValue:
            string: Bootstrap AMI is "ami-0efc96a4e17e7b048"
            type: String
          name: ValidAMI-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: ValidAMI-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: ValidAMI-observedGeneration
        - fieldValue:
            string: AsExpected
            type: String
          name: ValidMachineConfig-reason
        - fieldValue:
            string: "True"
            type: String
          name: ValidMachineConfig-status
        - fieldValue:
            string: ""
            type: String
          name: ValidMachineConfig-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: ValidMachineConfig-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: ValidMachineConfig-observedGeneration
        - fieldValue:
            string: AsExpected
            type: String
          name: AutorepairEnabled-reason
        - fieldValue:
            string: "False"
            type: String
          name: AutorepairEnabled-status
        - fieldValue:
            string: ""
            type: String
          name: AutorepairEnabled-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: AutorepairEnabled-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: AutorepairEnabled-observedGeneration
        - fieldValue:
            string: AsExpected
            type: String
          name: Ready-reason
        - fieldValue:
            string: "True"
            type: String
          name: Ready-status
        - fieldValue:
            string: ""
            type: String
          name: Ready-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: Ready-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: Ready-observedGeneration

      resourceMeta:
        group: hypershift.openshift.io
        kind: NodePool
        name: my-hosted-cluster-nodepool-2
        namespace: clusters
        ordinal: 2
        resource: nodepools
        version: v1alpha1
      statusFeedback:
        values:
        - fieldValue:
            string: AsExpected
            type: String
          name: AutoscalingEnabled-reason
        - fieldValue:
            string: "False"
            type: String
          name: AutoscalingEnabled-status
        - fieldValue:
            string: ""
            type: String
          name: AutoscalingEnabled-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: AutoscalingEnabled-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: AutoscalingEnabled-observedGeneration
        - fieldValue:
            string: AsExpected
            type: String
          name: UpdateManagementEnabled-reason
        - fieldValue:
            string: "True"
            type: String
          name: UpdateManagementEnabled-status
        - fieldValue:
            string: ""
            type: String
          name: UpdateManagementEnabled-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: AutoscaliUpdateManagementEnabledngEnabled-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: UpdateManagementEnabled-observedGeneration
        - fieldValue:
            string: AsExpected
            type: String
          name: ValidReleaseImage-reason
        - fieldValue:
            string: "True"
            type: String
          name: ValidReleaseImage-status
        - fieldValue:
            string: "Using release image: quay.io/openshift-release-dev/ocp-release:4.10.15-x86_64"
            type: String
          name: ValidReleaseImage-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: ValidReleaseImage-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: ValidReleaseImage-observedGeneration
        - fieldValue:
            string: AsExpected
            type: String
          name: ValidAMI-reason
        - fieldValue:
            string: "True"
            type: String
          name: ValidAMI-status
        - fieldValue:
            string: Bootstrap AMI is "ami-0efc96a4e17e7b048"
            type: String
          name: ValidAMI-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: ValidAMI-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: ValidAMI-observedGeneration
        - fieldValue:
            string: AsExpected
            type: String
          name: ValidMachineConfig-reason
        - fieldValue:
            string: "True"
            type: String
          name: ValidMachineConfig-status
        - fieldValue:
            string: ""
            type: String
          name: ValidMachineConfig-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: ValidMachineConfig-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: ValidMachineConfig-observedGeneration
        - fieldValue:
            string: AsExpected
            type: String
          name: AutorepairEnabled-reason
        - fieldValue:
            string: "False"
            type: String
          name: AutorepairEnabled-status
        - fieldValue:
            string: ""
            type: String
          name: AutorepairEnabled-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: AutorepairEnabled-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: AutorepairEnabled-observedGeneration
        - fieldValue:
            string: AsExpected
            type: String
          name: Ready-reason
        - fieldValue:
            string: "True"
            type: String
          name: Ready-status
        - fieldValue:
            string: ""
            type: String
          name: Ready-message
        - fieldValue:
            string: "2022-07-28T16:31:05Z"
            type: String
          name: Ready-lastTransitionTime
        - fieldValue:
            string: "1"
            type: String
          name: Ready-observedGeneration
```