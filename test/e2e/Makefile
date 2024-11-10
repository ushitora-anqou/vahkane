include ../../common.mk

KIND_TEST_CLUSTER ?= vahkane-test
KINDEST_NODE_VERSION ?= 1.31.0

.PHONY: test
test: setup
	$(MAKE) clean
	$(MAKE) -C ../.. docker-build
	$(MAKE) create-cluster
	go test . -v -ginkgo.v

.PHONY: clean
clean: setup
	kind delete cluster --name $(KIND_TEST_CLUSTER) || true

.PHONY: create-cluster
create-cluster: setup
	kind create cluster --name $(KIND_TEST_CLUSTER) --image kindest/node:v$(KINDEST_NODE_VERSION)
	kind load docker-image $(IMG) --name $(KIND_TEST_CLUSTER)
	kubectl create namespace e2e

