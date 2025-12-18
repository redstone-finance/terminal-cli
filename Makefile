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

build: | $(BASE)
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
.PHONY: build-all build-windows build-linux-amd64 build-darwin-arm64

build-all: build-windows build-linux-amd64 build-darwin-arm64

build-windows: | $(BASE)
	$Q cd $(BASE) && \
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build \
		$(GOFLAGS) \
		-tags "release,goexperiment.jsonv2" \
		-ldflags '$(LDFLAGS)' \
		-o $(OUTDIR)/windows_amd64/$(PACKAGE).exe main.go

build-linux-amd64: | $(BASE)
	$Q cd $(BASE) && \
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build \
		$(GOFLAGS) \
		-tags "release,goexperiment.jsonv2" \
		-ldflags '$(LDFLAGS)' \
		-o $(OUTDIR)/linux_amd64/$(PACKAGE) main.go

build-darwin-arm64: | $(BASE)
	$Q cd $(BASE) && \
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build \
		$(GOFLAGS) \
		-tags "release,goexperiment.jsonv2" \
		-ldflags '$(LDFLAGS)' \
		-o $(OUTDIR)/darwin_arm64/$(PACKAGE) main.go

.PHONY: lint
lint: $(GOLANGCILINT) | $(BASE) ; $(info $(M) running golangci-lint) @
	$Q GOEXPERIMENT=jsonv2 $(GOLANGCILINT) run

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