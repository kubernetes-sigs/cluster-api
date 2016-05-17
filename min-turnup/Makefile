
SHELL=/bin/bash
.SHELLFLAGS="-O extglob -o errexit -o pipefail -o nounset -c"

.PHONY: config echo-config


# sorry windows and non amd64
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	OS = linux
endif
ifeq ($(UNAME_S),Darwin)
	OS = darwin
endif

CONF_TOOL_VERSION = 4.6

default:
	$(MAKE) config

.tmp/conf:
	mkdir -p .tmp; curl -sSL --fail -o "$@" \
		"https://storage.googleapis.com/public-mikedanese-k8s/kconfig/$(CONF_TOOL_VERSION)/$(OS)/conf"; \
	chmod +x "$@"

config: .tmp/conf
	git update-index --assume-unchanged .config
	CONFIG_="" .tmp/conf Kconfig

.config: config

echo-config:
	util/conig_to_json .config


clean:
	rm -rf .tmp

fmt:
	for f in $$(find . -name '*.jsonnet'); do jsonnet fmt -i -n 2 $${f}; done;
