package helper

import (
	"fmt"
	"strings"

	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
)

//TODO @ianzhang366 integrate with the clusterSet logic
func GetTargetManagedCluster(hyd *hypdeployment.HypershiftDeployment) string {
	if len(hyd.Spec.TargetManagedCluster) == 0 {
		return hyd.GetNamespace()
	}

	return hyd.Spec.TargetManagedCluster
}

func GetTargetNamespace(hyd *hypdeployment.HypershiftDeployment) string {
	if len(hyd.Spec.TargetNamespace) == 0 {
		return hyd.GetNamespace()
	}

	return hyd.Spec.TargetNamespace
}

func ManagedClusterName(hyd *hypdeployment.HypershiftDeployment) string {
	if strings.HasPrefix(hyd.Spec.InfraID, hyd.GetName()) {
		return hyd.Spec.InfraID
	}
	return fmt.Sprintf("%s-%s", hyd.GetName(), hyd.Spec.InfraID)
}

// TODO(zhujian7) get this from hyd.Status.Kubeconfig
func HostedKubeconfigName(hyd *hypdeployment.HypershiftDeployment) string {
	return fmt.Sprintf("%s-%s-admin-kubeconfig", GetTargetNamespace(hyd), hyd.GetName())
}
