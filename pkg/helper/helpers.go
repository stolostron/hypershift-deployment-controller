package helper

import (
	"fmt"

	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
)

const (
	HostingManagedClusterMissing = "spec.hostingManagedCluster value is missing"
)

//TODO @ianzhang366 integrate with the clusterSet logic
func GetHostingManagedCluster(hyd *hypdeployment.HypershiftDeployment) string {
	if len(hyd.Spec.HostingManagedCluster) == 0 {
		return hyd.GetNamespace()
	}

	return hyd.Spec.HostingManagedCluster
}

func GetHostingNamespace(hyd *hypdeployment.HypershiftDeployment) string {
	if len(hyd.Spec.HostingNamespace) == 0 {
		hyd.Spec.HostingNamespace = hyd.GetNamespace()
	}
	return hyd.Spec.HostingNamespace
}

func ManagedClusterName(hyd *hypdeployment.HypershiftDeployment) string {
	return hyd.Spec.InfraID
}

// TODO(zhujian7) get this from hyd.Status.Kubeconfig
func HostedKubeconfigName(hyd *hypdeployment.HypershiftDeployment) string {
	return fmt.Sprintf("%s-%s-admin-kubeconfig", GetHostingNamespace(hyd), hyd.GetName())
}
