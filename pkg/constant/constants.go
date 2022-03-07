package constant

const (
	AnnoHypershiftDeployment = "hypershift.open-cluster-management.io/hypershiftdeployemnt"

	DefaultAddonInstallNamespace = "open-cluster-management-agent-addon"
)

var (
	// InstallAddons list addons that will be install to the hypershift hosted cluster
	InstallAddons = []string{
		"cert-policy-controller",
		"config-policy-controller",
		"governance-policy-framework",
		"iam-policy-controller",
	}
)
