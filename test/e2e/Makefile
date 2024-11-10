include ../../common.mk

KIND_TEST_CLUSTER ?= vahkane-test
KINDEST_NODE_VERSION ?= 1.31.0

DISCORD_APPLICATION_PUBLIC_KEY ?=

.PHONY: test
test: setup
	$(MAKE) clean
	$(MAKE) create-cluster
	go test . -v -ginkgo.v

.PHONY: clean
clean: setup
	kind delete cluster --name $(KIND_TEST_CLUSTER) || true

.PHONY: create-cluster
create-cluster: setup
	$(MAKE) -C ../.. generate
	kind create cluster --name $(KIND_TEST_CLUSTER) --image kindest/node:v$(KINDEST_NODE_VERSION)
	$(MAKE) -C ../.. docker-build
	kind load docker-image $(IMG) --name $(KIND_TEST_CLUSTER)
	$(MAKE) apply-manifests

.PHONY: port-forward-discord-webhook
port-forward-discord-webhook: setup
	kubectl -n e2e port-forward svc/vahkane-discord-webhook 38000

.PHONY: reload-controller
reload-controller:
	$(MAKE) -C ../.. docker-build
	kind load docker-image $(IMG) --name $(KIND_TEST_CLUSTER)
	$(MAKE) apply-manifests
	kubectl rollout restart deploy/vahkane-controller -n e2e

.PHONY: apply-manifests
apply-manifests:
	cat testdata/manifests-template.yaml | \
		sed \
			-e "s/{{DISCORD_APPLICATION_PUBLIC_KEY}}/$(DISCORD_APPLICATION_PUBLIC_KEY)/" \
			-e "s/{{DISCORD_APPLICATION_ID}}/$(DISCORD_APPLICATION_ID)/" \
			-e "s/{{DISCORD_TOKEN}}/$(DISCORD_TOKEN)/" \
			| \
		kubectl apply -f -
	kubectl apply -f ../../config/crd/bases/vahkane.anqou.net_discordinteractions.yaml
	cat ../../config/rbac/role.yaml | yq '.kind = "Role"' | yq '.metadata.namespace = "e2e"' | kubectl apply -f -
