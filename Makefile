# CouponKill 本地开发 / 构建入口（Linux / macOS；Windows 用 build.ps1，目标名相同）。
# 发布职责不在这里：集群 CD 以 Jenkinsfile 为准（docs/CICD-SOURCE-OF-TRUTH.md）。
#
# 两组目标：
#   * 五个 Go 服务：build test vet fmt lint chart-test images push up down smoke deploy-chart*
#   * kubebuilder Operator（couponkill-operator 是独立 Go 模块，仓库根不是 Go 模块，所以它们都 cd 进去）：
#       manifests generate install uninstall deploy undeploy operator-*
#     重新生成 CRD / deepcopy：编辑 couponkill-operator/api/v1 后执行 make manifests generate。

# ---- 五个 Go 服务 --------------------------------------------------------------
GO_SERVICES := gateway user coupon order connector
PORT_gateway := 8088
PORT_user := 8081
PORT_coupon := 8080
PORT_order := 8082
PORT_connector := 8085

# 竞态检测需要 cgo（gcc）；没有 gcc 的机器用 `make test RACE=`
RACE ?= -race

# 镜像：<IMAGE_REGISTRY>/couponkill-<svc>:<IMAGE_TAG>（与 build/Dockerfile、Jenkinsfile、charts/couponkill 一致）
IMAGE_REGISTRY ?= crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker
IMAGE_TAG ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
VCS_REF ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)

# 部署
HELM_RELEASE ?= couponkill
HELM_NAMESPACE ?= couponkill
HELM_CHART ?= ./charts/couponkill

# 冒烟 / 端到端：对运行中的栈（make up + scripts/dev-up.ps1，或 docker compose --profile app up）
E2E_BASE_URL ?= http://127.0.0.1:8088

# ---- Operator（kubebuilder）-----------------------------------------------------
# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29.0
# Operator 的 Go 模块目录（仓库根不是 Go 模块，kubebuilder 目标都在该目录内执行）
OPERATOR_DIR ?= couponkill-operator

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
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Go services

.PHONY: build
build: ## Build the five service binaries into ./bin
	@mkdir -p bin
	@for svc in $(GO_SERVICES); do \
		echo "go build ./cmd/$$svc"; \
		CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/$$svc ./cmd/$$svc; \
	done

.PHONY: test
test: ## Run Go tests (race detector on; RACE= to disable). Set CK_TEST_PG_DSN for the PostgreSQL tests.
	go test $(RACE) -count=1 ./cmd/... ./internal/...

.PHONY: vet
vet: ## Run go vet against the services.
	go vet ./cmd/... ./internal/...

.PHONY: fmt
fmt: ## Run gofmt against the services.
	gofmt -w cmd internal

.PHONY: lint
lint: vet ## gofmt check + go vet + helm lint
	@unformatted="$$(gofmt -l cmd internal 2>/dev/null)"; \
	if [ -n "$$unformatted" ]; then echo "未 gofmt 的文件:"; echo "$$unformatted"; exit 1; fi
	helm lint $(HELM_CHART)
	helm lint $(HELM_CHART) -f $(HELM_CHART)/values-prod.yaml --set-string image.tag=lint
	helm lint $(HELM_CHART) -f $(HELM_CHART)/values.canary-keda.yaml

.PHONY: chart-test
chart-test: ## Run the helm template render assertions (needs helm on PATH)
	cd charts/couponkill/tests/render && go test -count=1 ./...

.PHONY: images
images: ## Build the five service images (couponkill-<svc>:$(IMAGE_TAG))
	$(foreach svc,$(GO_SERVICES),$(CONTAINER_TOOL) build -f build/Dockerfile --build-arg SERVICE=$(svc) --build-arg PORT=$(PORT_$(svc)) --build-arg VCS_REF=$(VCS_REF) -t couponkill-$(svc):$(IMAGE_TAG) -t $(IMAGE_REGISTRY)/couponkill-$(svc):$(IMAGE_TAG) . &&) true

.PHONY: push
push: images ## Build and push the five service images to $(IMAGE_REGISTRY)
	@for svc in $(GO_SERVICES); do \
		$(CONTAINER_TOOL) push $(IMAGE_REGISTRY)/couponkill-$$svc:$(IMAGE_TAG) || exit 1; \
	done

.PHONY: up
up: ## Start PostgreSQL / Redis / Kafka via docker compose (add PROFILE=app for the five services too)
	docker compose $(if $(PROFILE),--profile $(PROFILE)) up -d $(if $(filter app,$(PROFILE)),--build)

.PHONY: down
down: ## Stop the local stack (keep volumes; make down VOLUMES=1 drops them)
	docker compose --profile app down $(if $(VOLUMES),-v)

.PHONY: smoke
smoke: ## Run the end-to-end test against a running stack (E2E_BASE_URL=$(E2E_BASE_URL))
	CK_E2E_BASE_URL=$(E2E_BASE_URL) go test -count=1 ./test/e2e/...

##@ Helm

.PHONY: deploy-chart
deploy-chart: ## Deploy the entire CouponKill system using Helm chart (dev defaults: chart generates secrets)
	helm upgrade --install $(HELM_RELEASE) $(HELM_CHART) --namespace $(HELM_NAMESPACE) --create-namespace --set-string image.tag=$(IMAGE_TAG)

.PHONY: deploy-chart-prod
deploy-chart-prod: ## Deploy with values-prod.yaml (create the app Secret first; see charts/couponkill/README.md)
	helm upgrade --install $(HELM_RELEASE) $(HELM_CHART) --namespace $(HELM_NAMESPACE) --create-namespace -f $(HELM_CHART)/values-prod.yaml --set-string image.tag=$(IMAGE_TAG)

.PHONY: deploy-chart-canary
deploy-chart-canary: ## Canary: STABLE_TAG stays stable, coupon/order canary pods run IMAGE_TAG
	@test -n "$(STABLE_TAG)" || { echo "make deploy-chart-canary STABLE_TAG=<稳定版镜像标签>"; exit 1; }
	helm upgrade --install $(HELM_RELEASE) $(HELM_CHART) --namespace $(HELM_NAMESPACE) --create-namespace \
		-f $(HELM_CHART)/values-prod.yaml -f $(HELM_CHART)/values.canary-keda.yaml \
		--set-string image.tag=$(STABLE_TAG) --set-string canary.coupon.tag=$(IMAGE_TAG) --set-string canary.order.tag=$(IMAGE_TAG)

##@ Operator (kubebuilder)

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	cd $(OPERATOR_DIR) && $(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	cd $(OPERATOR_DIR) && $(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: operator-fmt
operator-fmt: ## Run go fmt against the operator code.
	cd $(OPERATOR_DIR) && go fmt ./...

.PHONY: operator-vet
operator-vet: ## Run go vet against the operator code.
	cd $(OPERATOR_DIR) && go vet ./...

.PHONY: operator-test
operator-test: manifests generate operator-fmt operator-vet envtest ## Run the operator tests (envtest).
	cd $(OPERATOR_DIR) && KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./... -coverprofile cover.out

.PHONY: operator-build
operator-build: manifests generate operator-fmt operator-vet ## Build the operator manager binary.
	cd $(OPERATOR_DIR) && go build -o bin/manager main.go

.PHONY: operator-run
operator-run: manifests generate operator-fmt operator-vet ## Run the operator controller from your host.
	cd $(OPERATOR_DIR) && go run ./main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: operator-docker-build
operator-docker-build: operator-test ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} -f $(OPERATOR_DIR)/Dockerfile .

.PHONY: operator-docker-push
operator-docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make operator-docker-buildx IMG=myregistry/mypoperator:latest PLATFORMS=linux/arm64,linux/amd64).
# To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To conveniently create a local registry, you can use: make create-registry
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: operator-docker-buildx
operator-docker-buildx: operator-test ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' $(OPERATOR_DIR)/Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- rm Dockerfile.cross

##@ Operator deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build $(OPERATOR_DIR)/config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build $(OPERATOR_DIR)/config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy the operator controller to the K8s cluster specified in ~/.kube/config.
	cd $(OPERATOR_DIR)/config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build $(OPERATOR_DIR)/config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy the operator controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build $(OPERATOR_DIR)/config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

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
# controller-gen v0.14 依赖的旧 x/tools 无法用 Go 1.27 编译；setup-envtest release-0.17 依赖已下线的 GCS 存储桶
CONTROLLER_TOOLS_VERSION ?= v0.23.0
ENVTEST_VERSION ?= release-0.22

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
