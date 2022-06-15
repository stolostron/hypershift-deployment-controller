package helper

import (
	"os"
	"reflect"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	cliScheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

var (
	scheme = runtime.NewScheme()
)

var existingClusterSets = []*clusterv1beta1.ManagedClusterSet{
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dev",
		},
		Spec: clusterv1beta1.ManagedClusterSetSpec{},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "global",
		},
		Spec: clusterv1beta1.ManagedClusterSetSpec{
			ClusterSelector: clusterv1beta1.ManagedClusterSelector{
				SelectorType:  clusterv1beta1.LabelSelector,
				LabelSelector: &metav1.LabelSelector{},
			},
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openshift",
		},
		Spec: clusterv1beta1.ManagedClusterSetSpec{
			ClusterSelector: clusterv1beta1.ManagedClusterSelector{
				SelectorType: clusterv1beta1.LabelSelector,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"vendor": "openshift",
					},
				},
			},
		},
	},
}
var existingClusters = []*clusterv1.ManagedCluster{
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "c1",
			Labels: map[string]string{
				"vendor":                       "openshift",
				clusterv1beta1.ClusterSetLabel: "dev",
			},
		},
		Spec: clusterv1.ManagedClusterSpec{},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "c2",
			Labels: map[string]string{
				"cloud":                        "aws",
				"vendor":                       "openshift",
				clusterv1beta1.ClusterSetLabel: "dev",
			},
		},
		Spec: clusterv1.ManagedClusterSpec{},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "c3",
			Labels: map[string]string{
				"cloud": "aws",
			},
		},
		Spec: clusterv1.ManagedClusterSpec{},
	},
}

func TestMain(m *testing.M) {
	clusterv1.AddToScheme(cliScheme.Scheme)
	clusterv1beta1.AddToScheme(cliScheme.Scheme)

	if err := clusterv1.Install(scheme); err != nil {
		os.Exit(1)
	}
	if err := clusterv1beta1.AddToScheme(scheme); err != nil {
		os.Exit(1)
	}

	exitVal := m.Run()
	os.Exit(exitVal)
}

func TestGetClusterSetName(t *testing.T) {
	tests := []struct {
		name                 string
		cluster              clusterv1.ManagedCluster
		expectClusterSetName string
		expectError          bool
	}{
		{
			name: "test cluster with clusterset label",
			cluster: clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c1",
					Labels: map[string]string{
						clusterv1beta1.ClusterSetLabel: "c1set",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{},
			},
			expectClusterSetName: "c1set",
		},
		{
			name: "test cluster without clusterset label",
			cluster: clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c2",
				},
				Spec: clusterv1.ManagedClusterSpec{},
			},
			expectClusterSetName: "default",
		},
	}

	for _, test := range tests {
		returnClusterSetName := GetClusterSetName(test.cluster)

		if !reflect.DeepEqual(returnClusterSetName, test.expectClusterSetName) {
			t.Errorf("Case: %v, Failed to run TestGetClusterSetName. Expect clusterSetName: %v, return clusterSetName: %v", test.name, test.expectClusterSetName, returnClusterSetName)
			return
		}
	}
}

func TestGetClusterSetNames(t *testing.T) {
	tests := []struct {
		name                 string
		cluster              clusterv1.ManagedCluster
		expectClusterSetName []string
		expectError          bool
	}{
		{
			name: "test c1 cluster",
			cluster: clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c1",
					Labels: map[string]string{
						"vendor":                       "openshift",
						clusterv1beta1.ClusterSetLabel: "dev",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{},
			},
			expectClusterSetName: []string{"dev", "global", "openshift"},
		},
		{
			name: "test c2 cluster",
			cluster: clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c2",
					Labels: map[string]string{
						"cloud":                        "aws",
						"vendor":                       "openshift",
						clusterv1beta1.ClusterSetLabel: "dev",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{},
			},
			expectClusterSetName: []string{"dev", "global", "openshift"},
		},
		{
			name: "test c3 cluster",
			cluster: clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c2",
					Labels: map[string]string{
						"cloud": "aws",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{},
			},
			expectClusterSetName: []string{"global"},
		},
		{
			name: "test nonexist cluster in client",
			cluster: clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "doNotExistCluster",
					Labels: map[string]string{
						"cloud":  "aws",
						"vendor": "openshift",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{},
			},
			expectClusterSetName: []string{"global", "openshift"},
		},
	}

	var existingObjs []client.Object
	for _, cluster := range existingClusters {
		existingObjs = append(existingObjs, cluster)
	}
	for _, clusterset := range existingClusterSets {
		existingObjs = append(existingObjs, clusterset)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingObjs...).Build()

	for _, test := range tests {
		returnClusterSets, err := GetClusterSetNames(client, &test.cluster)

		if err != nil {
			if test.expectError {
				continue
			}
			t.Errorf("Case: %v, Failed to run GetClusterSetNames with cluster: %v", test.name, test.cluster)
			return
		}
		if !reflect.DeepEqual(returnClusterSets, test.expectClusterSetName) {
			t.Errorf("Case: %v, Failed to run GetClusterSetNames. Expect clusterSets: %v, return clusterSets: %v", test.name, test.expectClusterSetName, returnClusterSets)
			return
		}
	}
}

func TestIsClusterInClusterSet(t *testing.T) {
	tests := []struct {
		name         string
		cluster      clusterv1.ManagedCluster
		clusterSets  []string
		expectResult bool
		expectError  bool
	}{
		{
			name: "test c1 cluster",
			cluster: clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c1",
					Labels: map[string]string{
						"vendor":                       "openshift",
						clusterv1beta1.ClusterSetLabel: "dev",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{},
			},
			clusterSets:  []string{"dev", "global", "openshift"},
			expectResult: true,
		},
		{
			name: "test c2 cluster",
			cluster: clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c2",
					Labels: map[string]string{
						"cloud":                        "aws",
						"vendor":                       "openshift",
						clusterv1beta1.ClusterSetLabel: "dev",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{},
			},
			clusterSets:  []string{"dev", "global", "openshift"},
			expectResult: true,
		},
		{
			name: "test c3 cluster",
			cluster: clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c2",
					Labels: map[string]string{
						"cloud": "aws",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{},
			},
			clusterSets:  []string{"global"},
			expectResult: true,
		},
		{
			name: "test nonexist cluster in client",
			cluster: clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "doNotExistCluster",
					Labels: map[string]string{
						"doNotExist": "doNotExist",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{},
			},
			clusterSets:  []string{""},
			expectResult: false,
		},
	}

	var existingObjs []client.Object
	for _, cluster := range existingClusters {
		existingObjs = append(existingObjs, cluster)
	}
	for _, clusterset := range existingClusterSets {
		existingObjs = append(existingObjs, clusterset)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingObjs...).Build()

	for _, test := range tests {
		result, err := IsClusterInClusterSet(client, &test.cluster, test.clusterSets)

		if err != nil {
			if test.expectError {
				continue
			}
			t.Errorf("Case: %v, Failed to run IsClusterInClusterSet with cluster: %v and clusterSets %v", test.name, test.cluster, test.clusterSets)
			return
		}
		if result != test.expectResult {
			t.Errorf("Case: %v, Failed to run IsClusterInClusterSet. Expect: %v, return: %v", test.name, test.expectResult, result)
			return
		}
	}
}
