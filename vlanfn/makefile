VERSION ?= latest
REGISTRY ?= europe-docker.pkg.dev/srlinux/eu.gcr.io
IMG ?= $(REGISTRY)/vlan-fn:${VERSION}

ROOTDIR=$(abspath $(CURDIR)/..)

.PHONY: all
all: test 

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

#test: fmt vet ## Run tests.
#	go test ./...

docker-build:  ## Build docker images.
	docker buildx build --load --tag  ${IMG} -f ./Dockerfile ${ROOTDIR}

docker-push: ## Build docker images.
	docker buildx build --push --tag  ${IMG} -f ./Dockerfile ${ROOTDIR}