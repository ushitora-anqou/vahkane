LOCALBIN ?= $(dir $(abspath $(lastword $(MAKEFILE_LIST))))/bin

ENVTEST ?= $(LOCALBIN)/setup-envtest
ENVTEST_VERSION ?= release-0.19
ENVTEST_K8S_VERSION = 1.30.2

MOCKGEN ?= $(LOCALBIN)/mockgen
MOCKGEN_VERSION ?= v0.5.0

IMG ?= controller:latest

.PHONY: setup
setup: envtest mockgen
	@command -v aqua > /dev/null || { \
		echo "Install aqua. See https://aquaproj.github.io/docs/install" ;\
		exit 1 ;\
	}
	aqua i

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: mockgen
mockgen: $(MOCKGEN)
$(MOCKGEN): $(LOCALBIN)
	$(call go-install-tool,$(MOCKGEN),go.uber.org/mock/mockgen,$(MOCKGEN_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
