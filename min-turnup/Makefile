
SHELL=/bin/bash
.SHELLFLAGS="-O extglob -o errexit -o pipefail -o nounset -c"

.PHONY: config phase1 phase1.5 phase2 phase3


# sorry windows and non amd64
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	OS = linux
endif
ifeq ($(UNAME_S),Darwin)
	OS = darwin
endif

CONF_TOOL = ".tmp/conf"
CONF_TOOL_VERSION = 4.6

default:
	$(MAKE) config

$(CONF_TOOL):
	mkdir -p .tmp
	curl -sSL --fail -o $(CONF_TOOL) \
		"https://storage.googleapis.com/public-mikedanese-k8s/kconfig/$(CONF_TOOL_VERSION)/$(OS)/conf"
	chmod +x $(CONF_TOOL)

config: $(CONF_TOOL)
	CONFIG_="" $(CONF_TOOL) Kconfig

clean:
	rm -rf .tmp

fmt:
	for f in $$(find . -name '*.jsonnet'); do jsonnet fmt -i -n 2 $${f}; done;
