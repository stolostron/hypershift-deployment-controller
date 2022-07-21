#!/bin/bash

curCrd=$(mktemp)
tmpCrd=$(mktemp)


COMMIT_SHA=`cat go.mod | grep github.com/openshift/hypershift | sed -En 's/.* v.*-//p'`
echo Using SHA ${COMMIT_SHA}

printf "*****\n  Checking HostedCluster CRD\n"

curl --silent https://raw.githubusercontent.com/openshift/hypershift/${COMMIT_SHA}/cmd/install/assets/hypershift-operator/hypershift.openshift.io_hostedclusters.yaml --output ${curCrd}
if [ $? -ne 0 ]; then
  echo Failed to retreive the hostedCluster CRD from openshift/hypershift@${COMMIT_SHA}
  exit 1
fi

diff config/crd/hypershift.openshift.io_hostedclusters.yaml ${curCrd}
if [ $? -ne 0 ]; then
  echo CRDs did not match, a change has occured and the hypershift-deployment-controller and hypershift-addon-operator
  echo repositories need a go.mod update
  exit 1
fi
printf "  Done.\n*****\n"

printf "  Checking NodePools CRD\n"

curl --silent https://raw.githubusercontent.com/openshift/hypershift/${COMMIT_SHA}/cmd/install/assets/hypershift-operator/hypershift.openshift.io_nodepools.yaml --output ${curCrd}
if [ $? -ne 0 ]; then
  echo Failed to retreive the hostedCluster CRD from openshift/hypershift@${COMMIT_SHA}
  exit 1
fi

diff config/crd/hypershift.openshift.io_nodepools.yaml ${curCrd}
if [ $? -ne 0 ]; then
  echo CRDs did not match, a change has occured and the hypershift-deployment-controller and hypershift-addon-operator
  echo repositories need a go.mod update
  exit 1
fi
printf "  Done.\n*****\n"

rm ${tmpCrd} ${curCrd}