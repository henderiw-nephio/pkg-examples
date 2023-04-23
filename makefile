.PHONY: all
all: test

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

test: fmt vet ## Run tests.
	go test ./...

docker-build:  ## Build docker images.
	cd interfacefn; make docker-build
	cd nadfn; make docker-build

docker-push: ## Build docker images.
	cd interfacefn; make docker-push
	cd nadfn; make docker-push