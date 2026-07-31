.PHONY: frontend build dev-frontend dev-backend dev-proxy dev-transcode lint clean jellyfin-web migrate-continuum-check verify-local-paths install-hooks migrate-create migrate-validate migrate-status migrate-up migrate-down-to test-coverage check-coverage settings-bindings verify-settings-bindings verify-settings-bindings-web verify-settings-bindings-all

GIT_COMMON_DIR := $(strip $(shell git rev-parse --git-common-dir 2>/dev/null))
MAIN_CHECKOUT_ROOT := $(if $(GIT_COMMON_DIR),$(abspath $(GIT_COMMON_DIR)/..))
SHARED_MAKEFILE_LOCAL := $(if $(GIT_COMMON_DIR),$(abspath $(GIT_COMMON_DIR)/../Makefile.local))
DEFAULT_PLUGIN_SDK_DIR := $(abspath ../prairie-plugin-sdk)
SHARED_PLUGIN_SDK_DIR := $(if $(MAIN_CHECKOUT_ROOT),$(abspath $(MAIN_CHECKOUT_ROOT)/../prairie-plugin-sdk))
GOOSE := go run github.com/pressly/goose/v3/cmd/goose@v3.27.1
GOOSE_DIR := migrations/sql
ENV_FILE ?= .env
COVER_MIN ?= 75
COVER_PROFILE ?= coverage.out
COVER_EXCLUDE_FILE_REGEX ?= (^|/)store\.go$

ifneq ($(wildcard $(DEFAULT_PLUGIN_SDK_DIR)),)
DEV_PLUGIN_SDK_DIR ?= $(DEFAULT_PLUGIN_SDK_DIR)
else ifneq ($(wildcard $(SHARED_PLUGIN_SDK_DIR)),)
DEV_PLUGIN_SDK_DIR ?= $(SHARED_PLUGIN_SDK_DIR)
endif

JELLYFIN_WEB_INSTALL_DIR ?= .local/compat/jellyfin-web
JELLYFIN_WEB_VERSION ?= 10.11.6

# Build version stamping: inject the git revision so the admin Build panel shows a
# version even when Go's VCS metadata isn't embedded (mirrors the Dockerfile ldflags).
# BUILD_VERSION is the optional marketing semver (e.g. from a release tag).
BUILDINFO_PKG := github.com/prairie-server/prairie-server/internal/buildinfo
BUILD_REVISION ?= $(shell git rev-parse HEAD 2>/dev/null)
BUILD_DIRTY ?= $(shell test -n "$$(git status --porcelain 2>/dev/null)" && echo true || echo false)
BUILD_VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null)
GO_LDFLAGS := -X $(BUILDINFO_PKG).revisionOverride=$(BUILD_REVISION) -X $(BUILDINFO_PKG).dirtyOverride=$(BUILD_DIRTY) -X $(BUILDINFO_PKG).versionOverride=$(BUILD_VERSION)

# Build the frontend (requires pnpm)
frontend:
	cd web && pnpm install --frozen-lockfile && pnpm run build

# Build the Go binary (depends on frontend)
build: frontend
	go build -ldflags "$(GO_LDFLAGS)" -o prairie ./cmd/prairie/

# Run frontend dev server (proxies API to localhost:8080)
dev-frontend:
	cd web && pnpm run dev

# Run the Go backend (integrated mode)
dev-backend:
	go run ./cmd/prairie/

# Run a proxy node (stateless stream proxy, no DB required)
dev-proxy:
	go run ./cmd/prairie/ --mode=proxy

# Run a transcode node (HLS transcode worker, no DB required)
dev-transcode:
	go run ./cmd/prairie/ --mode=transcode

# Lint Go and frontend code
lint:
	golangci-lint run
	cd web && pnpm run lint

# Run Go unit tests with a coverprofile, then Vitest coverage for scoped web modules.
test-coverage:
	go test ./... -count=1 -covermode=atomic -coverprofile=$(COVER_PROFILE)
	cd web && pnpm exec vitest run --coverage

# Enforce Go coverage for packages listed in .github/coverage-packages.txt.
# PgStore DB code in store.go is excluded (same as CI).
# Always regenerate coverage.out so the gate never reads a stale profile.
check-coverage:
	$(MAKE) $(COVER_PROFILE)
	@pkgs=$$(grep -vE '^\s*(#|$$)' .github/coverage-packages.txt | tr '\n' ' '); \
	COVER_PACKAGES="$$pkgs" COVER_EXCLUDE_FILE_REGEX='$(COVER_EXCLUDE_FILE_REGEX)' \
		./scripts/check-go-coverage.sh $(COVER_PROFILE) $(COVER_MIN)

.PHONY: $(COVER_PROFILE)
$(COVER_PROFILE):
	go test ./... -count=1 -covermode=atomic -coverprofile=$(COVER_PROFILE)

# Regenerate the settings-contract bindings for every language.
#
# The client repos are siblings of this one (see AGENTS.md); a missing checkout
# is skipped rather than failing, so a server-only developer can still run this.
#
# The conformance fixture (contracts/settings/v1/conformance.json) travels with
# the bindings: the vendored copy in web/src/lib is what the web runner reads.
# The Kotlin and Swift copies land together with their runners in the client
# repos, which will pick their own test-resource paths.
PRAIRIE_ANDROID_DIR ?= $(abspath ../prairie-android)
PRAIRIE_APPLE_DIR ?= $(abspath ../prairie-apple)

settings-bindings:
	@mkdir -p internal/settingskeys
	go run ./cmd/settingsgen -lang go -out internal/settingskeys/keys.go
	gofmt -w internal/settingskeys/keys.go
	go run ./cmd/settingsgen -lang ts -out web/src/lib/settingsContract.ts
	@cd web && pnpm exec oxfmt --write src/lib/settingsContract.ts >/dev/null
	cp contracts/settings/v1/conformance.json web/src/lib/settingsConformance.json
	@if [ -d "$(PRAIRIE_ANDROID_DIR)" ]; then \
		go run ./cmd/settingsgen -lang kotlin \
			-package org.prairieserver.prairie.model.settings \
			-out "$(PRAIRIE_ANDROID_DIR)/shared/src/commonMain/kotlin/org/prairieserver/prairie/model/settings/SettingKeys.kt"; \
		echo "wrote Kotlin bindings to $(PRAIRIE_ANDROID_DIR)"; \
	else \
		echo "skipping Kotlin: $(PRAIRIE_ANDROID_DIR) not checked out"; \
	fi
	@if [ -d "$(PRAIRIE_APPLE_DIR)" ]; then \
		go run ./cmd/settingsgen -lang swift \
			-out "$(PRAIRIE_APPLE_DIR)/iosApp/iosApp/Networking/SettingKeys.generated.swift"; \
		echo "wrote Swift bindings to $(PRAIRIE_APPLE_DIR)"; \
	else \
		echo "skipping Swift: $(PRAIRIE_APPLE_DIR) not checked out"; \
	fi

# Fail when the committed bindings disagree with the manifest, so a manifest
# change cannot merge without regenerating what every client reads.
#
# Split in two because the generated TypeScript is compared after formatting, and
# only the Web CI job has pnpm: the Go job runs this target, the Web job runs
# verify-settings-bindings-web. Locally, `verify-settings-bindings-all` is both.
verify-settings-bindings:
	@CHECK_DIR=$$(mktemp -d) && trap 'rm -rf "$$CHECK_DIR"' EXIT && \
	go run ./cmd/settingsgen -lang go | gofmt > "$$CHECK_DIR/keys.go" && \
	diff -u internal/settingskeys/keys.go "$$CHECK_DIR/keys.go" \
		|| { echo "::error::internal/settingskeys/keys.go is stale; run make settings-bindings"; exit 1; }
	@diff -u web/src/lib/settingsConformance.json contracts/settings/v1/conformance.json \
		|| { echo "::error::web/src/lib/settingsConformance.json is stale; run make settings-bindings"; exit 1; }
	@echo "settings bindings are current"

# The half that needs pnpm: regenerate the web binding, format it the way the
# bindings target does, and compare. Without this a manifest change could merge
# with a stale settingsContract.ts, which is what every web control renders from.
verify-settings-bindings-web:
	@CHECK_DIR=$$(mktemp -d) && trap 'rm -rf "$$CHECK_DIR"' EXIT && \
	go run ./cmd/settingsgen -lang ts -out "$$CHECK_DIR/settingsContract.ts" && \
	cd web && pnpm exec oxfmt --write "$$CHECK_DIR/settingsContract.ts" >/dev/null && cd .. && \
	diff -u web/src/lib/settingsContract.ts "$$CHECK_DIR/settingsContract.ts" \
		|| { echo "::error::web/src/lib/settingsContract.ts is stale; run make settings-bindings"; exit 1; }
	@echo "web settings binding is current"

verify-settings-bindings-all: verify-settings-bindings verify-settings-bindings-web

# Check committed content for local machine path leaks.
verify-local-paths:
	scripts/check-local-path-leaks.sh

# Create a timestamped Goose SQL migration. Usage: make migrate-create NAME=add_thing
migrate-create:
	@if [ -z "$(NAME)" ]; then echo "usage: make migrate-create NAME=add_thing"; exit 1; fi
	$(GOOSE) -dir $(GOOSE_DIR) create "$(NAME)" sql

# Validate Goose migration annotations and SQL parsing without touching a database.
migrate-validate:
	$(GOOSE) -dir $(GOOSE_DIR) validate

# Show Goose migration status through Prairie's bootstrapping runner.
migrate-status:
	go run ./cmd/prairie/ --env "$(ENV_FILE)" --migrate-status

# Roll back every migration newer than VERSION (the version to KEEP).
#
# Not a routine operation: it discards data. It exists because some migrations
# are Go rather than SQL — the settings backfill and the jellycompat
# DisplayPreferences move — and those are registered in-process, so the goose
# CLI above cannot see or reverse them.
#
# This is a RANGE, not a list: everything newer than VERSION comes off, including
# migrations belonging to other features that happen to sort in between. Check
# `make migrate-status` and read the down of each one you are about to revert.
# Take a backup first regardless; the per-user SQLite stores have no down path.
#
# Usage: make migrate-down-to VERSION=<timestamp from migrate-status>
migrate-down-to:
	@if [ -z "$(VERSION)" ]; then echo "usage: make migrate-down-to VERSION=<timestamp from make migrate-status>"; exit 1; fi
	go run ./cmd/prairie/ --env "$(ENV_FILE)" --migrate-down-to "$(VERSION)"

# Apply pending Goose migrations through Prairie's bootstrapping runner.
migrate-up:
	go run ./cmd/prairie/ --env "$(ENV_FILE)" --migrate-only

# Install repo-local git hooks for this checkout/worktree.
install-hooks:
	@existing="$$(git config --local core.hooksPath 2>/dev/null || true)"; \
	if [ -n "$$existing" ] && [ "$$existing" != ".githooks" ]; then \
		echo "warning: overwriting existing local core.hooksPath ($$existing) with .githooks"; \
	fi
	git config core.hooksPath .githooks

# Fetch and build the pinned Jellyfin Web component into a gitignored local cache.
jellyfin-web:
	go run ./cmd/prairie/ compat-web install --dir "$(JELLYFIN_WEB_INSTALL_DIR)" --version "$(JELLYFIN_WEB_VERSION)"

# Read-only preflight for Continuum Docker installs moving to Prairie.
migrate-continuum-check:
	scripts/migrate-continuum-docker.sh check

# Clean build artifacts
clean:
	rm -rf web/dist web/node_modules prairie

# Include developer-specific targets (gitignored, optional).
# In Git worktrees, fall back to the main checkout's Makefile.local so custom
# targets like dev-deploy work without per-worktree symlinks or copies.
ifneq ($(wildcard Makefile.local),)
include Makefile.local
else ifneq ($(wildcard $(SHARED_MAKEFILE_LOCAL)),)
include $(SHARED_MAKEFILE_LOCAL)
endif
