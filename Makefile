OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
ALL_PLATFORM = linux/amd64

# Image URL to use all building/pushing image targets
REGISTRY ?= sh4d1
CONTROLLER_IMG ?= scaleway-k8s-vpc
NODE_IMG ?= scaleway-k8s-vpc-node
CONTROLLER_FULL_IMG ?= $(REGISTRY)/$(CONTROLLER_IMG)
NODE_FULL_IMG ?= $(REGISTRY)/$(NODE_IMG)

IMAGE_TAG ?= $(shell git rev-parse HEAD)

DOCKER_CLI_EXPERIMENTAL ?= enabled

CRD_OPTIONS ?= "crd:crdVersions=v1"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: controller node

# Run tests
test: generate fmt vet manifests
	go test ./... -coverprofile cover.out

# Build controller binary
controller: generate fmt vet
	go build -o bin/controller ./cmd/controller/

# Build node binary
node: generate fmt vet
	go build -o bin/node ./cmd/node/

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./cmd/controller/controller.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/controller && kustomize edit set image controller=${CONTROLLER_FULL_IMG}
	cd config/node && kustomize edit set image node=${NODE_FULL_IMG}
	kustomize build config/default | kubectl apply -f -

rbac: controller-gen
	$(CONTROLLER_GEN) rbac:roleName=controller-role paths="./controllers/privatenetwork_controller.go" output:stdout > config/rbac/controller-role.yaml
	$(CONTROLLER_GEN) rbac:roleName=node-role paths="./controllers/networkinterface_controller.go" output:stdout > config/rbac/node-role.yaml

# Generate manifests e.g. CRD, RBAC etc.
manifests: rbac controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
docker-build: test
	docker build --platform=linux/$(ARCH) -f Dockerfile.controller . -t ${CONTROLLER_FULL_IMG}
	docker build --platform=linux/$(ARCH) -f Dockerfile.node . -t ${NODE_FULL_IMG}

# Push the docker image
docker-push:
	docker push ${CONTROLLER_FULL_IMG}
	docker push ${NODE_FULL_IMG}

docker-buildx-all:
	@echo "Making release for tag $(IMAGE_TAG)"
	docker buildx build --platform=$(ALL_PLATFORM) -f Dockerfile.controller --push -t $(CONTROLLER_FULL_IMG):$(IMAGE_TAG) .
	docker buildx build --platform=$(ALL_PLATFORM) -f Dockerfile.node --push -t $(NODE_FULL_IMG):$(IMAGE_TAG) .

release: docker-buildx-all

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.5.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif
