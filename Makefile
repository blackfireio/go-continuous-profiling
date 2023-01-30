SHELL=/bin/bash -euo pipefail

# Mostly taken from https://github.com/mitchellh/packer which is licensed under the MPL2 license
NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
export GOOS=$(shell go env GOOS)
export GOARCH=$(shell go env GOARCH)
export GO111MODULE=on

.PHONY: update-deps
update-deps:
	go get -u
	$(MAKE) mod-tidy

.PHONY: mod-tidy
mod-tidy:
	go mod tidy -compat=1.19

.PHONY: format
format:
	@go fmt ./...

.PHONY: test-setup
test-setup:
	@go clean -testcache

.PHONY: test
test:
ifdef CI
	@echo "+++ [make test] $(OK_COLOR)Testing Go continuous profiler code$(NO_COLOR)"
endif
	go test . -race -v | sed 's/--- /-+- /g'

.PHONY: bench
bench:
	go test -bench . -benchmem 
