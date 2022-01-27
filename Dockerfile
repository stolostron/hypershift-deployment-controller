# Build the manager binary
FROM registry.ci.openshift.org/open-cluster-management/builder:go1.17-linux as builder

WORKDIR /go/src/github.com/stolostron/hypershift-deployment-controller
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY pkg/main.go pkg/main.go
COPY api/ api/
COPY pkg/controllers/ pkg/controllers/
#COPY vendor vendor                     # Developer Note: Needs to be retreived for every build
COPY Makefile Makefile
COPY hack hack

# Build
RUN make build

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

ENV USER_UID=1001

# Add the binaries
COPY --from=builder /go/src/github.com/stolostron/hypershift-deployment-controller/bin/manager .
 
USER ${USER_UID}

ENTRYPOINT ["/manager"]
