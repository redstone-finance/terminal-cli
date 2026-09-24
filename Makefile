PACKAGE  = terminal-cli
IMPORT   = github.com/redstone-finance/terminal-cli
GOROOT   = $(CURDIR)/.gopath~
GOPATH   = $(CURDIR)/.gopath~
BIN      = $(GOPATH)/bin
BASE     = $(GOPATH)/src/$(PACKAGE)
PATH    := bin:$(PATH)
GO       = go
VERSION ?= $(shell git rev-parse --short=8 HEAD)
DATE    ?= $(shell date +%FT%T%z)
SEMVER_REGEX := ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z\-\.]+)?(\+[0-9A-Za-z\-\.]+)?$

export GOPATH
export TF_ENABLE_ONEDNN_OPTS = 0

# Display utils
V = 0
Q = $(if $(filter 1,$V),,@)
M = $(shell printf "\033[34;1m▶\033[0m")
ESC     := \033
BOLD    := $(ESC)[1m
RESET   := $(ESC)[0m

LDFLAGS = -s -w -buildid=
GCFLAGS =
ASMFLAGS =
GOFLAGS = -trimpath -buildvcs=false
OUTDIR  = bin

build: | $(BASE) $(OUTDIR)
	$Q cd $(BASE) && CGO_ENABLED=0 $(GO) build \
		$(GOFLAGS) \
		-tags "release,goexperiment.jsonv2" \
		-ldflags '$(LDFLAGS)' \
		-o bin/$(PACKAGE) main.go

# Default target
.PHONY: all
all:  build lint | $(BASE); $(info $(M) built and lint everything!) @

# Setup
$(BASE): ; $(info $(M) setting GOPATH…)
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@
$(OUTDIR):
	@mkdir -p $@

# External tools 
$(BIN):
	@mkdir -p $@
$(BIN)/%: | $(BIN) ; $(info $(M) installing $(REPOSITORY)…)
	$Q tmp=$$(mktemp -d); \
	   env GO111MODULE=on GOPATH=$$tmp GOBIN=$(BIN) $(GO) install $(REPOSITORY) \
		|| ret=$$?; \
	   exit $$ret

GOLANGCILINT = $(BIN)/golangci-lint
$(BIN)/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH)/bin v2.6.0

# Build targets
PLATFORMS     = linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 windows-arm64
BUILD_TARGETS = $(addprefix build-,$(PLATFORMS))

.PHONY: build-all $(BUILD_TARGETS)

build-all: $(BUILD_TARGETS)

# Outputs bin/terminal-cli-<os>-<arch>[.exe]
$(BUILD_TARGETS): build-%: | $(BASE) $(OUTDIR)
	$Q cd $(BASE) && \
	GOOS=$(word 1,$(subst -, ,$*)) GOARCH=$(word 2,$(subst -, ,$*)) CGO_ENABLED=0 $(GO) build \
		$(GOFLAGS) \
		-tags "release,goexperiment.jsonv2" \
		-ldflags '$(LDFLAGS)' \
		-o $(OUTDIR)/$(PACKAGE)-$*$(if $(filter windows-%,$*),.exe) main.go

.PHONY: lint
lint: $(GOLANGCILINT) | $(BASE) ; $(info $(M) running golangci-lint) @
	$Q GOEXPERIMENT=jsonv2 $(GOLANGCILINT) run $(LINT_FLAGS)

.PHONY: lint-fix
lint-fix: $(GOLANGCILINT) | $(BASE) ; $(info $(M) running golangci-lint with auto-fix) @
	$Q GOEXPERIMENT=jsonv2 $(GOLANGCILINT) run --fix

.PHONY: run
run: build-race | ; $(info $(M) starting app with default params…)
	@ARGS="$(filter-out $@,$(MAKECMDGOALS))"; \
	if [ -z "$$ARGS" ]; then \
	  ARGS="local-gateway"; \
	fi; \
	echo && \
	echo "$(M) Using from configuration: $(BOLD).env $(RESET)" && \
	echo "$(M) Using from configuration: $(BOLD).env-$$ARGS $(RESET)" && \
	echo "$(M) Using from configuration: $(BOLD)configs/dev/$$ARGS.yaml $(RESET)" && \
	echo && \
	bin/$(PACKAGE) sync --config ./configs/dev/$$ARGS.yaml --custom-env .env-$$ARGS