
# Image URL to use all building/pushing image targets
IMG ?= controller:latest

all: test manager clusterctl

# Run tests
test: generate fmt vet manifests
	go test ./pkg/... ./cmd/... -coverprofile cover.out

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
	cat provider-components.yaml | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests:
	#go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go crd
	kustomize build config/default/ > provider-components.yaml

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Generate code
generate:
	go install sigs.k8s.io/cluster-api-provider-exoscale/vendor/k8s.io/code-generator/cmd/deepcopy-gen
	go generate ./pkg/... ./cmd/...
	deepcopy-gen \
		-i ./pkg/cloud/exoscale/providerconfig,./pkg/cloud/exoscale/providerconfig/v1alpha1 \
		-O zz_generated.deepcopy \
		-h hack/boilerplate.go.txt

# Build the docker image
docker-build: test
	docker build . -t ${IMG}
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push ${IMG}
