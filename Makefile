# Deploy target is read from .env (DEPLOY_HOST=sanspie@akarpov.ru).
# Only DEPLOY_HOST is pulled from .env — the file isn't `include`d wholesale
# because it contains values with characters that would break make parsing.
DEPLOY_HOST := $(shell grep -E '^DEPLOY_HOST=' .env 2>/dev/null | cut -d= -f2-)

# Native install locations + systemd units on the server.
PREVIEW_DIR     := /home/sanspie/about
PREVIEW_SERVICE := about.service
PROD_DIR        := /home/sanspie/about-prod
PROD_SERVICE    := about-prod.service

BUILD_ENV := CGO_ENABLED=0 GOOS=linux GOARCH=amd64
BUILD_FLAGS := -trimpath -ldflags="-s -w"

.PHONY: help build build-linux cli cli-bike deploy deploy-prod _require-host

help:
	@echo "make build        - build ./main for the local OS"
	@echo "make build-linux  - build ./main for linux/amd64 (server target)"
	@echo "make cli          - build ./projects (the projects CLI)"
	@echo "make cli-bike     - build ./bike (the bike rides / GPX-reprocess CLI)"
	@echo "make deploy       - build + ship ./main to PREVIEW ($(PREVIEW_DIR)) and restart $(PREVIEW_SERVICE)"
	@echo "make deploy-prod  - build + ship ./main to PROD ($(PROD_DIR)) and restart $(PROD_SERVICE)"
	@echo "DEPLOY_HOST = $(DEPLOY_HOST)"

build:
	go build $(BUILD_FLAGS) -o main .

build-linux:
	$(BUILD_ENV) go build $(BUILD_FLAGS) -o main .

cli:
	go build $(BUILD_FLAGS) -o projects ./cmd/projects

cli-bike:
	go build $(BUILD_FLAGS) -o bike ./cmd/bike

_require-host:
	@test -n "$(DEPLOY_HOST)" || { echo "DEPLOY_HOST not set in .env"; exit 1; }

# Ship the freshly built binary to <dir>/main and restart <service>.
# Copies to a temp name then renames (can't overwrite a running binary in place),
# and uses ssh -t so sudo can prompt for the password.
deploy: build-linux _require-host
	@echo ">> deploying PREVIEW to $(DEPLOY_HOST):$(PREVIEW_DIR)"
	scp main $(DEPLOY_HOST):$(PREVIEW_DIR)/main.new
	ssh -t $(DEPLOY_HOST) 'mv $(PREVIEW_DIR)/main.new $(PREVIEW_DIR)/main && sudo systemctl restart $(PREVIEW_SERVICE) && sleep 2 && systemctl is-active $(PREVIEW_SERVICE)'
	@echo ">> preview deployed."

deploy-prod: build-linux _require-host
	@echo ">> deploying PROD to $(DEPLOY_HOST):$(PROD_DIR)"
	scp main $(DEPLOY_HOST):$(PROD_DIR)/main.new
	ssh -t $(DEPLOY_HOST) 'mv $(PROD_DIR)/main.new $(PROD_DIR)/main && sudo systemctl restart $(PROD_SERVICE) && sleep 2 && systemctl is-active $(PROD_SERVICE)'
	@echo ">> prod deployed."
