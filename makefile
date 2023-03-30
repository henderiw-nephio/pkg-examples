VERSION ?= latest
REGISTRY ?= europe-docker.pkg.dev/srlinux/eu.gcr.io
IMG ?= $(REGISTRY)/pkg-examples:${VERSION}

.PHONY: all
all: test

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

test: fmt vet ## Run tests.
	go test ./...

docker-build:  ## Build docker images.
	docker build -t ${IMG} .

docker-push: ## Build docker images.
	docker push ${IMG}