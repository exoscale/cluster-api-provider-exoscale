# Image URL to use all building/pushing image targets
PREFIX = exoscale
NAME = cluster-api-provider-exoscale-controller
TAG ?= latest
IMG = ${PREFIX}/${NAME}:${TAG}


.PHONy: all
all: test manager bin/clusterctl

.PHONY: clean
clean:
	rm bin/clusterctl
	rm config/crds/*.yaml
	rm config/rbac/*.yaml
	rm provider-components.yaml

# Run tests
test: generate fmt vet manifests
	go test -v -coverprofile cover.out \
		./pkg/... \
		./cmd/...

# Build clusterctl binary
bin/clusterctl: generate fmt vet
	go build -o bin/clusterctl sigs.k8s.io/cluster-api-provider-exoscale/cmd/clusterctl

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager sigs.k8s.io/cluster-api-provider-exoscale/cmd/manager

# Run against the configured Kubernetes cluster in ~/.kube/config
run: bin/clusterctl provider-components.yaml
	bin/clusterctl create cluster -v 9 \
		--provider exoscale \
		-m cmd/clusterctl/examples/exoscale/machine.yaml \
		-c cmd/clusterctl/examples/exoscale/cluster.yaml \
		-p provider-components.yaml \
		--bootstrap-type kind

# Generate manifests e.g. CRD, RBAC etc.
provider-components.yaml:
	kustomize build config > $@
	echo "---" >> $@
	kustomize build vendor/sigs.k8s.io/cluster-api/config/default >> $@

.PHONY: manifests
manifests: provider-components.yaml

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Generate code
generate:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go crd
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go rbac
	go generate ./pkg/... ./cmd/...

# Build the docker image
docker-build: fmt vet
	docker build . -t ${IMG}
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/patch/manager_image.yaml

# Push the docker image
docker-push:
	docker push ${IMG}
