# Image URL to use all building/pushing image targets
PREFIX = exoscale
NAME = cluster-api-provider-exoscale-controller
TAG ?= latest
IMG = ${PREFIX}/${NAME}:${TAG}


all: test manager clusterctl

# Run tests
test: generate fmt vet manifests
	go test -v -coverprofile cover.out \
		./pkg/... \
		./cmd/...

# Build clusterctl binary
clusterctl: generate fmt vet
	go build -o bin/clusterctl sigs.k8s.io/cluster-api-provider-exoscale/cmd/clusterctl

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager sigs.k8s.io/cluster-api-provider-exoscale/cmd/manager

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/manager/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl apply -f config/crds
	cat provider-components.yaml | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests:
	#go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go crd
	kustomize build config/default/ > provider-components.yaml
	echo "---" >> provider-components.yaml
	kustomize build vendor/sigs.k8s.io/cluster-api/config/default/ >> provider-components.yaml

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Generate code
generate:
	go generate ./pkg/... ./cmd/...

# Build the docker image
docker-build:
	docker build . -t ${IMG}
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push ${IMG}
