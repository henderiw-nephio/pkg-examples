.PHONY: all
all: test

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

test: fmt vet ## Run tests.
	go test ./...

docker-build:  ## Build docker images.
	##cd interfacefn; make docker-build
	##cd nadfn; make docker-build
	##cd dnnfn; make docker-build
	##cd nfdeployfn; make docker-build
	cd ipamfn; make docker-build
	cd vlanfn; make docker-build

docker-push: ## Build docker images.
	##cd interfacefn; make docker-push
	##cd nadfn; make docker-push
	##cd dnnfn; make docker-push
	##cd nfdeployfn; make docker-push
	cd ipamfn; make docker-push
	cd vlanfn; make docker-push