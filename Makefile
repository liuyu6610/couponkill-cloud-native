# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# 阿里云镜像仓库配置
REGISTRY ?= crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker/my-docker
CANARY_REGISTRY ?= crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker/canary-keda-dev

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
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
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

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
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./... -coverprofile cover.out

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	$(CONTROLLER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:latest PLATFORMS=linux/arm64,linux/amd64).
# To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To conveniently create a local registry, you can use: make create-registry
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: test ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' couponkill-operator/Dockerfile > Dockerfile.cross
	- $(CONTROLLER_TOOL) buildx create --name project-v3-builder
	$(CONTROLLER_TOOL) buildx use project-v3-builder
	- $(CONTROLLER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- rm Dockerfile.cross

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd couponkill-operator/config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build couponkill-operator/config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build couponkill-operator/config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Project Specific

.PHONY: deploy-chart
deploy-chart: ## Deploy the entire CouponKill system using Helm chart
	helm upgrade --install couponkill ./charts/couponkill --namespace couponkill --create-namespace

.PHONY: deploy-chart-prod
deploy-chart-prod: ## Deploy the entire CouponKill system using Helm chart in production mode
	helm upgrade --install couponkill ./charts/couponkill --namespace couponkill --create-namespace -f ./charts/couponkill/values-prod.yaml

.PHONY: deploy-chart-canary
deploy-chart-canary: ## Deploy the entire CouponKill system using Helm chart with canary release
	helm upgrade --install couponkill ./charts/couponkill --namespace couponkill --create-namespace -f ./charts/couponkill/values.canary-keda.yaml

.PHONY: build-all-images
build-all-images: ## Build all Docker images for the project
	docker build -t gateway:latest -f couponkill-gateway/Dockerfile .
	docker build -t coupon:latest -f couponkill-coupon-service/Dockerfile .
	docker build -t order:latest -f couponkill-order-service/Dockerfile .
	docker build -t user:latest -f couponkill-user-service/Dockerfile .
	docker build -t seckill-go:latest -f couponkill-go-service/Dockerfile .
	docker build -t operator:latest -f couponkill-operator/Dockerfile .

.PHONY: build-all-images-registry
build-all-images-registry: ## Build and tag all Docker images for pushing to registry
	docker build -t gateway:latest -f couponkill-gateway/Dockerfile .
	docker build -t coupon:latest -f couponkill-coupon-service/Dockerfile .
	docker build -t order:latest -f couponkill-order-service/Dockerfile .
	docker build -t user:latest -f couponkill-user-service/Dockerfile .
	docker build -t seckill-go:latest -f couponkill-go-service/Dockerfile .
	docker build -t operator:latest -f couponkill-operator/Dockerfile .
	
	# Tag images for registry (阿里云个人仓库使用标签区分镜像)
	docker tag gateway:latest ${REGISTRY}:gateway
	docker tag coupon:latest ${REGISTRY}:coupon
	docker tag order:latest ${REGISTRY}:order
	docker tag user:latest ${REGISTRY}:user
	docker tag seckill-go:latest ${REGISTRY}:seckill-go
	docker tag operator:latest ${REGISTRY}:operator

.PHONY: push-all-images
push-all-images: ## Push all Docker images to registry
	docker push ${REGISTRY}:gateway
	docker push ${REGISTRY}:coupon
	docker push ${REGISTRY}:order
	docker push ${REGISTRY}:user
	docker push ${REGISTRY}:seckill-go
	docker push ${REGISTRY}:operator

.PHONY: build-and-push-all
build-and-push-all: build-all-images-registry push-all-images ## Build and push all images to registry

.PHONY: build-all-images-canary
build-all-images-canary: ## Build and tag all Docker images for canary release
	docker build -t gateway:latest -f couponkill-gateway/Dockerfile .
	docker build -t coupon:latest -f couponkill-coupon-service/Dockerfile .
	docker build -t order:latest -f couponkill-order-service/Dockerfile .
	docker build -t user:latest -f couponkill-user-service/Dockerfile .
	docker build -t seckill-go:latest -f couponkill-go-service/Dockerfile .
	docker build -t operator:latest -f couponkill-operator/Dockerfile .
	
	# Tag images for canary registry (阿里云个人仓库使用标签区分镜像)
	docker tag gateway:latest ${CANARY_REGISTRY}:gateway
	docker tag coupon:latest ${CANARY_REGISTRY}:coupon
	docker tag order:latest ${CANARY_REGISTRY}:order
	docker tag user:latest ${CANARY_REGISTRY}:user
	docker tag seckill-go:latest ${CANARY_REGISTRY}:seckill-go
	docker tag operator:latest ${CANARY_REGISTRY}:operator

.PHONY: push-all-images-canary
push-all-images-canary: ## Push all Docker images for canary release to registry
	docker push ${CANARY_REGISTRY}:gateway
	docker push ${CANARY_REGISTRY}:coupon
	docker push ${CANARY_REGISTRY}:order
	docker push ${CANARY_REGISTRY}:user
	docker push ${CANARY_REGISTRY}:seckill-go
	docker push ${CANARY_REGISTRY}:operator

.PHONY: build-and-push-all-canary
build-and-push-all-canary: build-all-images-canary push-all-images-canary ## Build and push all canary images to registry

.PHONY: pull-dependency-images
pull-dependency-images: ## Pull all dependency images to local registry
	# Pull and retag MySQL image
	docker pull mysql:8.0
	docker tag mysql:8.0 ${REGISTRY}:mysql
	docker push ${REGISTRY}:mysql
	
	# Pull and retag Redis image
	docker pull redis:7.0
	docker tag redis:7.0 ${REGISTRY}:redis
	docker push ${REGISTRY}:redis
	
	# Pull and retag RocketMQ nameserver image
	docker pull apache/rocketmq:5.3.1
	docker tag apache/rocketmq:5.3.1 ${REGISTRY}:rocketmq-namesrv
	docker push ${REGISTRY}:rocketmq-namesrv
	
	# Pull and retag RocketMQ broker image
	docker pull apache/rocketmq:5.3.1
	docker tag apache/rocketmq:5.3.1 ${REGISTRY}:rocketmq-broker
	docker push ${REGISTRY}:rocketmq-broker
	
	# Pull and retag Nacos image
	docker pull nacos/nacos-server:v2.2.3
	docker tag nacos/nacos-server:v2.2.3 ${REGISTRY}:nacos-server
	docker push ${REGISTRY}:nacos-server
	
	# Pull and retag Sentinel image
	docker pull bladex/sentinel-dashboard:1.8.6
	docker tag bladex/sentinel-dashboard:1.8.6 ${REGISTRY}:sentinel-dashboard
	docker push ${REGISTRY}:sentinel-dashboard
	
	# Pull and retag Kafka image
	docker pull bitnami/kafka:3.4.0
	docker tag bitnami/kafka:3.4.0 ${REGISTRY}:kafka
	docker push ${REGISTRY}:kafka
	
	# Pull and retag Zookeeper image (for Kafka)
	docker pull bitnami/zookeeper:3.8.1
	docker tag bitnami/zookeeper:3.8.1 ${REGISTRY}:zookeeper
	docker push ${REGISTRY}:zookeeper

.PHONY: build-and-push-all-complete
build-and-push-all-complete: build-and-push-all build-and-push-all-canary pull-dependency-images ## Build and push all project and dependency images

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)

## Tool Versions
KUSTOMIZE_VERSION ?= v5.3.0
CONTROLLER_TOOLS_VERSION ?= v0.14.0
ENVTEST_VERSION ?= release-0.17

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if necessary.
# $1 - target path with name of binary (ideally with version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv "$$(echo "$(1)" | sed "s/-$(3)$$//")" $(1) ;\
}
endef