/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package client provides helper functions for API clients used by the
// Hypershift Deployment controller
package client

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/labels"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

type ClusterSetsGetter struct {
	Client client.Client
}

type ClusterSetBindingsGetter struct {
	Client client.Client
}

func (cbg ClusterSetBindingsGetter) List(namespace string,
	selector labels.Selector) ([]*clusterv1beta1.ManagedClusterSetBinding, error) {
	clusterSetBindingList := clusterv1beta1.ManagedClusterSetBindingList{}
	err := cbg.Client.List(context.Background(), &clusterSetBindingList,
		client.InNamespace(namespace), &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	var retClusterSetBindings []*clusterv1beta1.ManagedClusterSetBinding
	for i := range clusterSetBindingList.Items {
		retClusterSetBindings = append(retClusterSetBindings, &clusterSetBindingList.Items[i])
	}
	return retClusterSetBindings, nil
}

func (csg ClusterSetsGetter) List(selector labels.Selector) ([]*clusterv1beta1.ManagedClusterSet, error) {
	clusterSetList := clusterv1beta1.ManagedClusterSetList{}
	err := csg.Client.List(context.Background(), &clusterSetList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	var retClusterSets []*clusterv1beta1.ManagedClusterSet
	for i := range clusterSetList.Items {
		retClusterSets = append(retClusterSets, &clusterSetList.Items[i])
	}
	return retClusterSets, nil
}
