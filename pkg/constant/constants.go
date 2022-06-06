package constant

const (
	AnnoHypershiftDeployment = "cluster.open-cluster-management.io/hypershiftdeployment"

	NamespaceNameSeperator = "/"

	ManagedClusterCleanupFinalizer = "hypershiftdeployment.cluster.open-cluster-management.io/managedcluster-cleanup"

	ReleaseImage = "quay.io/openshift-release-dev/ocp-release:4.10.9-x86_64"

	// AnnoReleaseImage is an annotation used to specify the release image
	AnnoReleaseImage = "hypershiftdeployment.cluster.open-cluster-management.io/release-image"

	// DestroyFinalizer makes sure infrastructure is cleaned up before it is removed
	DestroyFinalizer = "hypershiftdeployment.cluster.open-cluster-management.io/finalizer"

	// HostedClusterFinalizer makes sure that the hostedcluster is gone before removing
	HostedClusterFinalizer = "hypershift.openshift.io/used-by-hostedcluster"

	// AutoInfraLabelName identifies that a resource was created by the hypershift-deployment-controller
	AutoInfraLabelName = "hypershift.openshift.io/auto-created-for-infra"

	// InfraLabelName Tracks the infrastructure-id for easy HypershiftDeployment list filtering
	InfraLabelName = "hypershift.openshift.io/infra-id"

	// HostingClusterMissing message
	HostingClusterMissing = "spec.hostingCluster value is missing"

	// CreatedByHypershiftDeployment is an annotation that is used to show ownership via infra-ids
	CreatedByHypershiftDeployment = "hypershift-deployment.open-cluster-management.io/created-by"

	// CCredsSuffix Cloud Credential Suffix
	CCredsSuffix = "-cloud-credentials" // #nosec G101

	// HypershiftBucketSecretName is the secret name used to work with the AWS s3 credential
	HypershiftBucketSecretName = "hypershift-operator-oidc-provider-s3-credentials"
)
