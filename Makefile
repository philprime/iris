# ============================================================================
# IRIS MAKEFILE
# ============================================================================
# Front door for building, testing, and developing the Iris controller, relay,
# and reloader. Run 'make help' to see all available commands.
# ============================================================================

# Default target - show help when running 'make' without arguments
.DEFAULT_GOAL := help

# ----------------------------------------------------------------------------
# Configuration
# ----------------------------------------------------------------------------

# Local tool/binary output (envtest assets, etc.). Git-ignored.
LOCALBIN ?= $(shell pwd)/bin

# Kubernetes API version used to download envtest assets.
ENVTEST_K8S_VERSION ?= 1.31.0

# Local kind cluster name.
KIND_CLUSTER ?= iris

# Container image coordinates. Override IMAGE_REGISTRY / IMAGE_TAG to publish.
IMAGE_REGISTRY ?= ghcr.io/philprime
IMAGE_TAG ?= dev
CONTROLLER_IMG ?= $(IMAGE_REGISTRY)/iris-controller:$(IMAGE_TAG)
RELAY_IMG ?= $(IMAGE_REGISTRY)/iris-relay:$(IMAGE_TAG)
POSTFIX_IMG ?= $(IMAGE_REGISTRY)/iris-postfix:$(IMAGE_TAG)
E2E_STUB_IMG ?= $(IMAGE_REGISTRY)/iris-e2e-stub:$(IMAGE_TAG)

# End-to-end deployment coordinates. E2E_FULLNAME is the chart fullname for the
# release (<release>-<chart name>), which prefixes the rendered resource names.
E2E_RELEASE ?= iris
E2E_NAMESPACE ?= iris-system
E2E_FULLNAME ?= $(E2E_RELEASE)-iris

# Tools pinned via the go.mod `tool` directive, invoked with `go tool`.
CONTROLLER_GEN ?= go tool controller-gen
SETUP_ENVTEST ?= go tool setup-envtest
CRD_REF_DOCS ?= go tool crd-ref-docs

# Shell prelude that sources the CI logging helpers (begin_group / end_group /
# log_info / log_notice / …). Each recipe runs in its own shell, so source it
# per recipe: `@set -eu; $(LOG); begin_group "..."; ...; end_group`. The helpers
# emit GitHub Actions workflow commands in CI and plain text locally.
LOG := . ./scripts/log.sh

# ============================================================================
# SETUP & INSTALLATION
# ============================================================================

## Initialize the project for development (installs all dependencies)
#
# Detects the platform and installs system dependencies, tidies Go modules, and
# installs the pre-commit git hooks. Run this once after cloning the repository.
.PHONY: init
init:
	@if [ "$$(uname)" = "Darwin" ]; then \
		echo "Darwin detected."; \
		$(MAKE) init-darwin; \
	elif [ "$$(uname)" = "Linux" ]; then \
		echo "Linux detected."; \
		$(MAKE) init-linux; \
	else \
		echo "Not running on Darwin or Linux."; \
		exit 1; \
	fi
	$(MAKE) install
	$(MAKE) setup-hooks

.PHONY: init-darwin
init-darwin:
	@if ! command -v brew >/dev/null 2>&1; then \
		echo "Homebrew not detected. Skipping system dependency installation."; \
		exit 1; \
	fi
	echo "Homebrew detected. Installing system dependencies..."; \
	brew bundle

.PHONY: init-linux
init-linux:
	@if ! command -v dprint >/dev/null 2>&1; then \
		echo "dprint not detected. Install it using: curl -fsSL https://dprint.dev/install.sh | sh"; \
		exit 1; \
	fi

## Install and tidy Go module dependencies
#
# Downloads module dependencies and removes unused ones. Safe to re-run.
.PHONY: install
install:
	@set -eu; $(LOG); \
	begin_group "go mod tidy"; \
	go mod tidy; \
	end_group

## Install the pre-commit git hooks into .git/hooks
#
# Registers the hooks from .pre-commit-config.yaml so they run on `git commit`.
# Warns and skips (rather than failing) if pre-commit is not installed.
.PHONY: setup-hooks
setup-hooks:
	@set -eu; $(LOG); \
	begin_group "Setup git hooks"; \
	if command -v pre-commit >/dev/null 2>&1; then \
		pre-commit install; \
	else \
		log_warning "pre-commit not found; skipping git hook install. Install it (macOS: 'brew bundle'; otherwise 'pipx install pre-commit') then run 'make setup-hooks'."; \
	fi; \
	end_group

# ============================================================================
# CODE GENERATION
# ============================================================================

## Generate deepcopy code, CRD/RBAC/webhook manifests, and the CRD reference
#
# Runs controller-gen to (re)generate zz_generated.deepcopy.go from the API
# types and the CRD/RBAC/webhook manifests, then renders the CRD reference doc.
# Run after editing api/v1alpha1/*_types.go.
.PHONY: generate
generate: generate-deepcopy manifests generate-crd-docs
	@set -eu; $(LOG); log_notice "Code generation complete."

## Generate deepcopy methods (zz_generated.deepcopy.go)
.PHONY: generate-deepcopy
generate-deepcopy:
	@set -eu; $(LOG); \
	begin_group "controller-gen: deepcopy"; \
	log_info "Generating zz_generated.deepcopy.go from ./api/..."; \
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."; \
	end_group

## Generate CRD, RBAC, and webhook manifests
#
# Writes CRDs to config/crd/bases (kustomize base) and copies them into
# chart/iris/crds so the Helm chart stays in sync.
#
# IRIS_GEN_PATHS scopes controller-gen to our own source. We cannot use
# "./..." because controller-gen's directory loader descends into the
# gitignored reference clones under tmp/refs (cert-manager, cloudnative-pg,
# source-controller) and would emit their CRDs too. It is a space-separated
# list expanded into one paths= flag each. Extend it as packages carrying
# markers are added.
IRIS_GEN_PATHS ?= ./api/... ./internal/controller/... ./internal/webhook/...
.PHONY: manifests
manifests:
	@set -eu; $(LOG); \
	begin_group "controller-gen: manifests"; \
	log_info "Generating CRD, RBAC, and webhook manifests..."; \
	$(CONTROLLER_GEN) \
		rbac:roleName=iris-manager-role \
		crd \
		webhook \
		$(foreach p,$(IRIS_GEN_PATHS),paths="$(p)") \
		output:crd:artifacts:config=config/crd/bases \
		output:rbac:artifacts:config=config/rbac \
		output:webhook:artifacts:config=config/webhook; \
	log_info "Syncing CRDs into chart/iris/crds..."; \
	mkdir -p chart/iris/crds; \
	cp config/crd/bases/*.yaml chart/iris/crds/; \
	log_info "Syncing RBAC rules into chart/iris/rbac..."; \
	mkdir -p chart/iris/rbac; \
	cp config/rbac/role.yaml chart/iris/rbac/role.yaml; \
	end_group

## Generate CRD reference docs from api/v1alpha1
#
# Renders docs/crd-reference.md from the API types using crd-ref-docs, then runs
# the formatter so the generated manifests and docs land in their committed
# style. As the final step of `generate`, this keeps the whole codegen output
# clean in one pass.
.PHONY: generate-crd-docs
generate-crd-docs:
	@set -eu; $(LOG); \
	begin_group "crd-ref-docs"; \
	log_info "Rendering CRD reference → docs/crd-reference.md"; \
	$(CRD_REF_DOCS) \
		--source-path=./api/v1alpha1 \
		--config=hack/crd-ref-docs.yaml \
		--renderer=markdown \
		--output-path=docs/crd-reference.md; \
	end_group
	@$(MAKE) format

## Verify the generated files are up to date (regenerates, then fails on any diff)
#
# Runs the full codegen and fails if it changes any tracked generated artifact.
# Used by the pre-commit hook and suitable for CI. The diff is scoped to the
# generated set so a stale commit is reported as codegen drift, not formatting.
.PHONY: verify-generate
verify-generate: generate
	@set -eu; $(LOG); \
	begin_group "verify-generate"; \
	if git diff --exit-code -- \
		api/v1alpha1/zz_generated.deepcopy.go \
		config/crd config/rbac config/webhook \
		chart/iris/crds chart/iris/rbac docs/crd-reference.md; then \
		end_group; \
		log_notice "Generated files are up to date."; \
	else \
		log_error "Generated files are stale. Run 'make generate' and stage the result."; \
		end_group; \
		exit 1; \
	fi

# ============================================================================
# BUILDING
# ============================================================================

## Build the controller, relay, and reloader binaries into dist/
.PHONY: build
build: build-controller build-relay build-reloader
	@set -eu; $(LOG); log_notice "Built controller, relay, and reloader into dist/."

.PHONY: build-controller
build-controller:
	@set -eu; $(LOG); \
	begin_group "Build controller"; \
	log_info "Compiling ./cmd/controller → dist/controller"; \
	mkdir -p dist; \
	go build -o dist/controller ./cmd/controller; \
	end_group

.PHONY: build-relay
build-relay:
	@set -eu; $(LOG); \
	begin_group "Build relay"; \
	log_info "Compiling ./cmd/relay → dist/relay"; \
	mkdir -p dist; \
	go build -o dist/relay ./cmd/relay; \
	end_group

.PHONY: build-reloader
build-reloader:
	@set -eu; $(LOG); \
	begin_group "Build reloader"; \
	log_info "Compiling ./cmd/reloader → dist/reloader"; \
	mkdir -p dist; \
	go build -o dist/reloader ./cmd/reloader; \
	end_group

## Build the controller, relay, and postfix Docker images
.PHONY: build-docker
build-docker: build-docker-controller build-docker-relay build-docker-postfix
	@set -eu; $(LOG); log_notice "Built controller, relay, and postfix images."

.PHONY: build-docker-controller
build-docker-controller:
	@set -eu; $(LOG); \
	begin_group "Build image: $(CONTROLLER_IMG)"; \
	log_info "docker buildx build -f build/controller.Dockerfile"; \
	docker buildx build --platform linux/amd64 -f build/controller.Dockerfile -t $(CONTROLLER_IMG) --load .; \
	end_group

.PHONY: build-docker-relay
build-docker-relay:
	@set -eu; $(LOG); \
	begin_group "Build image: $(RELAY_IMG)"; \
	log_info "docker buildx build -f build/relay.Dockerfile"; \
	docker buildx build --platform linux/amd64 -f build/relay.Dockerfile -t $(RELAY_IMG) --load .; \
	end_group

.PHONY: build-docker-postfix
build-docker-postfix:
	@set -eu; $(LOG); \
	begin_group "Build image: $(POSTFIX_IMG)"; \
	log_info "docker buildx build -f build/postfix.Dockerfile"; \
	docker buildx build --platform linux/amd64 -f build/postfix.Dockerfile -t $(POSTFIX_IMG) --load .; \
	end_group

## Build the end-to-end stub destination image (used by make test-e2e)
.PHONY: build-docker-e2e-stub
build-docker-e2e-stub:
	@set -eu; $(LOG); \
	begin_group "Build image: $(E2E_STUB_IMG)"; \
	log_info "docker buildx build -f build/e2e-stub.Dockerfile"; \
	docker buildx build --platform linux/amd64 -f build/e2e-stub.Dockerfile -t $(E2E_STUB_IMG) --load .; \
	end_group

# ============================================================================
# DEVELOPMENT & RUNNING
# ============================================================================

## Run the controller with air hot-reload (.air.controller.toml)
.PHONY: dev-controller
dev-controller:
	go tool air -c .air.controller.toml

## Run the relay with air hot-reload (.air.relay.toml, hack/dev/relay.config.yaml)
.PHONY: dev-relay
dev-relay:
	go tool air -c .air.relay.toml

## Run the controller directly without hot reload
.PHONY: run
run:
	go run ./cmd/controller

# ============================================================================
# TESTING & QUALITY ASSURANCE
# ============================================================================

## Run unit tests and envtest integration tests (PKG=... RUN=... to focus)
#
# Defaults to the whole module. Narrow it for a fast TDD loop:
#   make test PKG=./internal/postfix/...
#   make test PKG=./internal/postfix/... RUN=TestRenderTransport
# RUN passes -run and adds -v; a focused run also disables the test cache
# (-count=1) so red-green cycles never see a stale pass.
.PHONY: test
test: setup-envtest
	@set -eu; $(LOG); \
	begin_group "Run tests (unit + envtest)"; \
	log_info "Resolving envtest assets for Kubernetes $(ENVTEST_K8S_VERSION)..."; \
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)"; \
	export KUBEBUILDER_ASSETS; \
	pkg="$(PKG)"; [ -n "$$pkg" ] || pkg="./..."; \
	args="$$pkg"; \
	if [ -n "$(RUN)" ]; then args="-run $(RUN) -v -count=1 $$pkg"; fi; \
	if [ -n "$(RACE)" ]; then args="-race $$args"; fi; \
	log_info "go test $$args"; \
	go test $$args; \
	end_group; \
	log_notice "Tests passed."

## Run tests with a coverage report (tmp/coverage.out)
.PHONY: test-coverage
test-coverage: setup-envtest
	@set -eu; $(LOG); \
	begin_group "Run tests with coverage"; \
	mkdir -p tmp; \
	log_info "Resolving envtest assets for Kubernetes $(ENVTEST_K8S_VERSION)..."; \
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)"; \
	export KUBEBUILDER_ASSETS; \
	log_info "go test ./... -coverprofile=tmp/coverage.out"; \
	go test ./... -coverprofile=tmp/coverage.out; \
	go tool cover -func=tmp/coverage.out; \
	end_group; \
	log_notice "Coverage report written to tmp/coverage.out."

## Run the kind-based end-to-end suite (test/e2e/)
.PHONY: test-e2e
test-e2e: build-docker build-docker-e2e-stub
	@set -eu; $(LOG); \
	begin_group "Provision kind cluster $(KIND_CLUSTER)"; \
	if ! kind get clusters 2>/dev/null | grep -qx "$(KIND_CLUSTER)"; then \
		kind create cluster --name $(KIND_CLUSTER); \
	fi; \
	kind export kubeconfig --name $(KIND_CLUSTER); \
	end_group; \
	begin_group "Load images into kind"; \
	kind load docker-image $(CONTROLLER_IMG) $(RELAY_IMG) $(POSTFIX_IMG) $(E2E_STUB_IMG) --name $(KIND_CLUSTER); \
	end_group; \
	begin_group "Deploy chart (release $(E2E_RELEASE) → $(E2E_NAMESPACE))"; \
	helm upgrade --install $(E2E_RELEASE) chart/iris --namespace $(E2E_NAMESPACE) --create-namespace \
		--set webhook.enabled=false \
		--set podDisruptionBudget.enabled=false \
		--set controller.replicas=1 --set postfix.replicas=1 \
		--set controller.image.tag=$(IMAGE_TAG) --set controller.image.pullPolicy=IfNotPresent \
		--set controller.relayImage.tag=$(IMAGE_TAG) \
		--set postfix.image.tag=$(IMAGE_TAG) --set postfix.image.pullPolicy=IfNotPresent; \
	end_group; \
	begin_group "Wait for control and data plane rollout"; \
	kubectl -n $(E2E_NAMESPACE) rollout status deploy/$(E2E_FULLNAME)-controller --timeout=180s; \
	kubectl -n $(E2E_NAMESPACE) rollout status deploy/$(E2E_FULLNAME)-postfix --timeout=180s; \
	end_group; \
	$(MAKE) test-e2e-run

## Run only the e2e suite against an already-deployed cluster (no build/deploy)
.PHONY: test-e2e-run
test-e2e-run:
	@set -eu; $(LOG); \
	begin_group "Run e2e suite"; \
	log_info "go test ./test/e2e/... -tags=e2e"; \
	E2E_NAMESPACE=$(E2E_NAMESPACE) E2E_POSTFIX_SERVICE=$(E2E_FULLNAME)-postfix E2E_STUB_IMAGE=$(E2E_STUB_IMG) \
		go test ./test/e2e/... -tags=e2e -v -count=1 -timeout 600s; \
	end_group; \
	log_notice "End-to-end suite passed."

## Download and locate the envtest API-server/etcd binaries
.PHONY: setup-envtest
setup-envtest:
	@set -eu; $(LOG); \
	begin_group "Setup envtest"; \
	log_info "Locating envtest binaries for Kubernetes $(ENVTEST_K8S_VERSION)..."; \
	mkdir -p $(LOCALBIN); \
	$(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN); \
	end_group

## Run static analysis (go vet, staticcheck, govulncheck)
.PHONY: analyze
analyze:
	@set -eu; $(LOG); \
	if [ -z "$$(go list ./... 2>/dev/null)" ]; then \
		log_warning "No Go packages found; skipping static analysis."; \
		exit 0; \
	fi; \
	begin_group "go vet"; \
	log_info "Examining source for suspicious constructs..."; \
	go vet ./...; \
	log_info "Type-checking the e2e suite (build tag e2e)..."; \
	go vet -tags=e2e ./test/e2e/...; \
	end_group; \
	begin_group "staticcheck"; \
	log_info "Running static analysis..."; \
	go tool staticcheck ./...; \
	end_group; \
	begin_group "govulncheck"; \
	log_info "Scanning imported packages for known vulnerabilities..."; \
	go tool govulncheck -scan=package ./...; \
	end_group; \
	log_notice "Static analysis passed."

## Format code (go mod tidy, go fmt, dprint fmt)
.PHONY: format
format:
	@set -eu; $(LOG); \
	begin_group "Format"; \
	log_info "go mod tidy"; \
	go mod tidy; \
	log_info "go fmt ./..."; \
	go fmt ./...; \
	log_info "dprint fmt"; \
	dprint fmt; \
	end_group; \
	log_notice "Formatting complete."

# ============================================================================
# LOCAL CLUSTER (kind)
# ============================================================================

## Create the local kind cluster
.PHONY: kind-up
kind-up:
	kind create cluster --name $(KIND_CLUSTER)

## Delete the local kind cluster
.PHONY: kind-down
kind-down:
	kind delete cluster --name $(KIND_CLUSTER)

## Load the locally built images into the kind cluster
.PHONY: kind-load
kind-load:
	kind load docker-image $(CONTROLLER_IMG) $(RELAY_IMG) $(POSTFIX_IMG) --name $(KIND_CLUSTER)

# ============================================================================
# DEPLOYMENT (Helm)
# ============================================================================

## Install the Helm chart into the current cluster
.PHONY: deploy
deploy:
	@set -eu; $(LOG); \
	begin_group "Deploy chart"; \
	log_info "helm upgrade --install iris → namespace iris-system"; \
	helm upgrade --install iris chart/iris --namespace iris-system --create-namespace; \
	end_group; \
	log_notice "Iris deployed to namespace iris-system."

## Remove the Helm chart from the current cluster
.PHONY: undeploy
undeploy:
	@set -eu; $(LOG); \
	begin_group "Undeploy chart"; \
	log_info "helm uninstall iris (namespace iris-system)"; \
	helm uninstall iris --namespace iris-system; \
	end_group

## Lint the Helm chart
.PHONY: chart-lint
chart-lint:
	@set -eu; $(LOG); \
	begin_group "helm lint"; \
	helm lint chart/iris; \
	end_group

## Render the Helm chart templates to stdout for inspection
## Optional: SET=key=value,key2=value2 to override values
.PHONY: chart-template
chart-template:
	@set -eu; $(LOG); \
	begin_group "helm template"; \
	args=""; [ -z "$(SET)" ] || args="--set $(SET)"; \
	helm template iris chart/iris $$args; \
	end_group

## Package the Helm chart into dist/
.PHONY: chart-package
chart-package:
	@set -eu; $(LOG); \
	begin_group "helm package"; \
	mkdir -p dist; \
	helm package chart/iris --destination dist; \
	end_group

# ============================================================================
# MAINTENANCE
# ============================================================================

## Update all dependencies to the latest compatible versions, then format
.PHONY: upgrade-deps
upgrade-deps:
	go get -u ./...
	$(MAKE) format

# ============================================================================
# HELP
# ============================================================================

## Show this help message with all available commands
.PHONY: help
help:
	@echo "=============================================="
	@echo "🌈 IRIS DEVELOPMENT COMMANDS"
	@echo "=============================================="
	@echo ""
	@awk 'BEGIN { desc = "" } \
	/^## / { desc = substr($$0, 4) } \
	/^\.PHONY: / && desc != "" { \
		printf "\033[36m%-22s\033[0m %s\n", $$2, desc; \
		desc = "" \
	}' $(MAKEFILE_LIST)
	@echo ""
	@echo "💡 Use 'make <command>' to run any command above."
	@echo ""
