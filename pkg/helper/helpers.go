package helper

import (
	"fmt"

	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	hydclient "github.com/stolostron/hypershift-deployment-controller/pkg/client"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetHostingCluster(hyd *hypdeployment.HypershiftDeployment) string {
	if len(hyd.Spec.HostingCluster) == 0 {
		return hyd.GetNamespace()
	}

	return hyd.Spec.HostingCluster
}

func GetHostingClusterKey(hyd *hypdeployment.HypershiftDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name: GetHostingCluster(hyd),
	}
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

func GetClusterSetName(managedCluster clusterv1.ManagedCluster) string {
	labels := managedCluster.GetLabels()
	if len(labels) == 0 {
		return "default"
	}

	if clusterSetName := labels[clusterv1beta1.ClusterSetLabel]; len(clusterSetName) > 0 {
		return clusterSetName
	}

	return "default"
}

func GetClusterSetNames(client client.Client,
	cluster *clusterv1.ManagedCluster) ([]string, error) {
	csg := hydclient.ClusterSetsGetter{
		Client: client,
	}

	clusterSets, err := clusterv1beta1.GetClusterSetsOfCluster(cluster, csg)
	if err != nil {
		return nil, err
	}

	clusterSetNames := sets.NewString()

	for _, clusterSet := range clusterSets {
		clusterSetNames.Insert(clusterSet.Name)
	}

	return clusterSetNames.List(), nil
}

func IsClusterInClusterSet(client client.Client,
	managedCluster *clusterv1.ManagedCluster,
	clusterSets []string) (bool, error) {
	if len(clusterSets) == 0 {
		return false, nil
	}

	clusterSetNames, err := GetClusterSetNames(client, managedCluster)
	if err != nil {
		return false, err
	}

	for _, clusterSetName := range clusterSetNames {
		for _, clusterSet := range clusterSets {
			if clusterSetName == clusterSet {
				return true, nil
			}
		}
	}

	return false, nil
}
