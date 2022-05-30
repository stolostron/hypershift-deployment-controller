# Repo hosting the image with path
REPO ?= "quay.io/stolostron/"

# Image URL to use all building/pushing image targets
IMG ?= $(REPO)hypershift-deployment-controller:latest

KUBECONFIG ?= ${HOME}/.kube/config
S3_CREDS ?= ${HOME}/.aws/credentials
BUCKET_REGION ?= ""
BUCKET_NAME ?= ""
CLOUD_PROVIDER_SECRET_NAME ?= ""
CLOUD_PROVIDER_SECRET_NAMESPACE ?= "default"
INFRA_REGION ?= "us-east-1"
HYPERSHIFT_DEPLOYMENT_NAME ?= "hypershift-test"

KUBECTL ?= kubectl --kubeconfig=$(KUBECONFIG)

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.22

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen get-hypershift-crds## Generate ClusterRole and CustomResourceDefinition objects. rm -f config/crd/*.yaml
	$(CONTROLLER_GEN) rbac:roleName=hypershfit-deployment-controller crd paths="./..." output:crd:artifacts:config=config/crd

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	XDG_CACHE_HOME="/tmp" KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

##@ Build
.PHONY: vendor
vendor:
	go mod tidy -compat=1.18
	go mod vendor

.PHONY: build
build: fmt vet ## Build manager binary.
	GOFLAGS="" go build -o bin/manager pkg/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./pkg/main.go

.PHONY: docker-build
docker-build: #test ## Build docker image with the manager.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

.PHONY: docker
docker: docker-build docker-push ## Build and Psh the docker image

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/deployment && $(KUSTOMIZE) edit set image quay.io/stolostron/hypershift-deployment-controller:latest=${IMG}
	$(KUSTOMIZE) build config/deployment | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/deployment | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
CONTROLLER_GEN_PACKAGE ?= sigs.k8s.io/controller-tools/cmd/controller-gen@v0.7.0
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),$(CONTROLLER_GEN_PACKAGE))

KUSTOMIZE = $(shell pwd)/bin/kustomize
KUSTOMIZE_PACKAGE ?= sigs.k8s.io/kustomize/kustomize/v3@v3.8.7
.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),$(KUSTOMIZE_PACKAGE))

ENVTEST = $(shell pwd)/bin/setup-envtest
ENVTEST_PACKAGE ?= sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
.PHONY: envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),$(ENVTEST_PACKAGE))

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
	$(call go-get-tool-internal,$(1),$(2),$(firstword $(subst @, ,$(2))))
endef

define go-get-tool-internal
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get -d $(2) ;\
GOBIN=$(PROJECT_DIR)/bin go install $(3) ;\
rm -rf $$TMP_DIR ;\
}
endef

.PHONY: install-hypershift-addon
install-hypershift-addon:
	S3_CREDS=${S3_CREDS} BUCKET_REGION=${BUCKET_REGION} BUCKET_NAME=${BUCKET_NAME} samples/quickstart/start.sh

.PHONY: create-a-hosted-cluster
create-a-hosted-cluster:
	CLOUD_PROVIDER_SECRET_NAMESPACE=${CLOUD_PROVIDER_SECRET_NAMESPACE} CLOUD_PROVIDER_SECRET_NAME=${CLOUD_PROVIDER_SECRET_NAME} INFRA_REGION=${INFRA_REGION} HYPERSHIFT_DEPLOYMENT_NAME=${HYPERSHIFT_DEPLOYMENT_NAME} samples/quickstart/create-aws-hosted-cluster.sh

.PHONY: create-a-policy
create-a-policy:
	HYPERSHIFT_DEPLOYMENT_NAME=${HYPERSHIFT_DEPLOYMENT_NAME} samples/quickstart/create-policy.sh

.PHONY: test-sd
test-sd: install-hypershift-addon create-a-hosted-cluster create-a-policy

.PHONY: get-hypershift-crds
get-hypershift-crds:
	export COMMIT_SHA=`cat go.mod | grep github.com/openshift/hypershift | sed -En 's/.* v.*-//p'`; \
	curl https://raw.githubusercontent.com/openshift/hypershift/${COMMIT_SHA}/cmd/install/assets/hypershift-operator/hypershift.openshift.io_hostedclusters.yaml > ./config/crd/hypershift.openshift.io_hostedclusters.yaml; \
	curl https://raw.githubusercontent.com/openshift/hypershift/${COMMIT_SHA}/cmd/install/assets/hypershift-operator/hypershift.openshift.io_nodepools.yaml > ./config/crd/hypershift.openshift.io_nodepools.yaml; \