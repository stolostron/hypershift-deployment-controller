package client

import (
	"os"
	"reflect"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	cliScheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

var (
	scheme = runtime.NewScheme()
)

var existingClusterSetBindings = []*clusterv1beta1.ManagedClusterSetBinding{
	{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dev",
			Namespace: "default",
		},
		Spec: clusterv1beta1.ManagedClusterSetBindingSpec{
			ClusterSet: "dev",
		},
		Status: clusterv1beta1.ManagedClusterSetBindingStatus{
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1beta1.ClusterSetBindingBoundType,
					Status: metav1.ConditionTrue,
				},
			},
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "global",
			Namespace: "default",
		},
		Spec: clusterv1beta1.ManagedClusterSetBindingSpec{
			ClusterSet: "global",
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-such-cluster-set",
			Namespace: "kube-system",
		},
		Spec: clusterv1beta1.ManagedClusterSetBindingSpec{
			ClusterSet: "no-such-cluster-set",
		},
	},
}
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

func convertClusterSetToSet(clustersets []*clusterv1beta1.ManagedClusterSet) sets.String {
	if len(clustersets) == 0 {
		return nil
	}
	retSet := sets.NewString()
	for _, clusterset := range clustersets {
		retSet.Insert(clusterset.Name)
	}
	return retSet
}

func convertClusterSetBindingsToSet(clusterSetBindings []*clusterv1beta1.ManagedClusterSetBinding) sets.String {
	if len(clusterSetBindings) == 0 {
		return nil
	}
	retSet := sets.NewString()
	for _, clusterSetBinding := range clusterSetBindings {
		retSet.Insert(clusterSetBinding.Name)
	}
	return retSet
}

func TestMain(m *testing.M) {
	clusterv1beta1.AddToScheme(cliScheme.Scheme)

	if err := clusterv1beta1.AddToScheme(scheme); err != nil {
		os.Exit(1)
	}

	exitVal := m.Run()
	os.Exit(exitVal)
}

func TestClusterSetsGetterList(t *testing.T) {
	tests := []struct {
		name                  string
		selector              labels.Selector
		expectClusterSetNames sets.String
		expectError           bool
	}{
		{
			name:                  "test list label everything",
			selector:              labels.Everything(),
			expectClusterSetNames: sets.NewString("dev", "global", "openshift"),
		},
	}

	var existingObjs []client.Object
	for _, clusterSet := range existingClusterSets {
		existingObjs = append(existingObjs, clusterSet)
	}
	csg := ClusterSetsGetter{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingObjs...).Build(),
	}

	for _, test := range tests {
		clusterSets, err := csg.List(test.selector)
		if err != nil {
			if test.expectError {
				continue
			}
			t.Errorf("Case: %v, Failed to run List with selector: %v", test.name, test.selector)
			return
		}
		returnClusterSets := convertClusterSetToSet(clusterSets)
		if !reflect.DeepEqual(returnClusterSets, test.expectClusterSetNames) {
			t.Errorf("Case: %v, Failed to run List. Expect clusterSets: %v, return clusterSets: %v", test.name, test.expectClusterSetNames, returnClusterSets)
			return
		}
	}
}

func TestClusterSetBindingsGetterList(t *testing.T) {
	tests := []struct {
		name                         string
		namespace                    string
		selector                     labels.Selector
		expectClusterSetBindingNames sets.String
		expectError                  bool
	}{
		{
			name:                         "test list default namespace label everything",
			namespace:                    "default",
			selector:                     labels.Everything(),
			expectClusterSetBindingNames: sets.NewString("dev", "global"),
		},
	}

	var existingObjs []client.Object
	for _, clusterSetBinding := range existingClusterSetBindings {
		existingObjs = append(existingObjs, clusterSetBinding)
	}
	cbg := ClusterSetBindingsGetter{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingObjs...).Build(),
	}

	for _, test := range tests {
		clusterSetBindings, err := cbg.List(test.namespace, test.selector)
		if err != nil {
			if test.expectError {
				continue
			}
			t.Errorf("Case: %v, Failed to run List with namespace: %v selector: %v", test.name, test.namespace, test.selector)
			return
		}
		returnClusterSetBindings := convertClusterSetBindingsToSet(clusterSetBindings)
		if !reflect.DeepEqual(returnClusterSetBindings, test.expectClusterSetBindingNames) {
			t.Errorf("Case: %v, Failed to run List. Expect clusterSetBindings: %v, return clusterSetBindings: %v", test.name, test.expectClusterSetBindingNames, returnClusterSetBindings)
			return
		}
	}
}
