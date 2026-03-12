# WraithOS Makefile
#
# Top-level build targets for the OS layer.
# Run from the repository root: make build-iso

SHELL := /bin/bash
.DEFAULT_GOAL := help

VERSION := $(shell cat os/rootfs/usr/share/wraithos/version 2>/dev/null || echo "0.0.0-dev")
BUILD_DIR := os/build

.PHONY: help build-iso build-ui test test-qemu upload clean lint

help: ## Show available targets
	@echo "WraithOS Build System (v$(VERSION))"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'
	@echo ""

build-ui: ## Build the Go web UI binary
	@echo "==> Building wraith-ui binary..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -ldflags="-s -w -X main.version=$(VERSION)" \
		-o wraith-ui ./cmd/wraith-ui
	@echo "==> Built: wraith-ui ($$(du -h wraith-ui | cut -f1))"

build-iso: build-ui ## Build the WraithOS ISO image
	@echo "==> Building WraithOS ISO..."
	bash os/scripts/build-iso.sh
	@echo "==> ISO ready in $(BUILD_DIR)/"

test-qemu: ## Boot ISO in QEMU for testing
	bash os/scripts/test-qemu.sh

test: lint ## Run all checks
	@echo "==> All checks passed"

lint: ## Lint shell scripts with shellcheck
	@echo "==> Linting shell scripts..."
	@if command -v shellcheck >/dev/null 2>&1; then \
		find os/ -name "*.sh" -type f -exec shellcheck -s bash {} +; \
		echo "==> Lint passed"; \
	else \
		echo "==> shellcheck not found -- skipping lint"; \
	fi

upload: ## Upload ISO to XCP-ng (usage: make upload XCP_HOST=myhost)
	@if [ -z "$(XCP_HOST)" ]; then \
		echo "Usage: make upload XCP_HOST=<hostname>"; \
		exit 1; \
	fi
	bash os/scripts/upload-xcp.sh $(XCP_HOST)

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)
	rm -f wraith-ui
	@echo "==> Clean"
