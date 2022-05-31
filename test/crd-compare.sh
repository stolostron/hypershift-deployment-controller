#!/bin/bash

tmpCrd=$(mktemp)

printf "*****\n  Checking HostedCluster CRD\n"

curl --silent https://raw.githubusercontent.com/openshift/hypershift/main/cmd/install/assets/hypershift-operator/hypershift.openshift.io_hostedclusters.yaml --output ${tmpCrd}
if [ $? -ne 0 ]; then
  echo Failed to retreive the hostedCluster CRD from openshift/hypershift
  exit 1
fi

diff config/crd/hypershift.openshift.io_hostedclusters.yaml ${tmpCrd}
if [ $? -ne 0 ]; then
  echo CRDs did not match, a change has occured and the hypershift-deployment-controller and hypershift-addon-operator
  echo repositories need a go.mod update
  exit 1
fi
printf "  Done.\n*****\n"

printf "  Checking NodePools CRD\n"

curl --silent https://raw.githubusercontent.com/openshift/hypershift/main/cmd/install/assets/hypershift-operator/hypershift.openshift.io_nodepools.yaml --output ${tmpCrd}
if [ $? -ne 0 ]; then
  echo Failed to retreive the hostedCluster CRD from openshift/hypershift
  exit 1
fi

diff config/crd/hypershift.openshift.io_nodepools.yaml ${tmpCrd}
if [ $? -ne 0 ]; then
  echo CRDs did not match, a change has occured and the hypershift-deployment-controller and hypershift-addon-operator
  echo repositories need a go.mod update
  exit 1
fi
printf "  Done.\n*****\n"

rm ${tmpCrd}