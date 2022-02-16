# -*- mode: Python -*-
# set defaults
version_settings(True, ">=0.22.2")

settings = {
    "deploy_cert_manager": True,
    "preload_images_for_kind": True,
    "enable_providers": ["docker"],
    "kind_cluster_name": "kind",
    "debug": {},
}

# global settings
settings.update(read_json(
    "tilt-settings.json",
    default = {},
))

allow_k8s_contexts(settings.get("allowed_contexts"))

os_name = str(local("go env GOOS")).rstrip("\n")
os_arch = str(local("go env GOARCH")).rstrip("\n")

if settings.get("trigger_mode") == "manual":
    trigger_mode(TRIGGER_MODE_MANUAL)

default_registry(settings.get("default_registry"))

always_enable_providers = ["core"]
extra_args = settings.get("extra_args", {})

providers = {
    "core": {
        "context": ".",
        "image": "gcr.io/k8s-staging-cluster-api/cluster-api-controller",
        "live_reload_deps": [
            "main.go",
            "go.mod",
            "go.sum",
            "api",
            "cmd",
            "controllers",
            "errors",
            "exp",
            "feature",
            "internal",
            "third_party",
            "util",
            "webhooks",
        ],
        "label": "CAPI",
    },
    "kubeadm-bootstrap": {
        "context": "bootstrap/kubeadm",
        "image": "gcr.io/k8s-staging-cluster-api/kubeadm-bootstrap-controller",
        "live_reload_deps": [
            "main.go",
            "api",
            "controllers",
            "internal",
            "types",
            "../../go.mod",
            "../../go.sum",
        ],
        "label": "CABPK",
    },
    "kubeadm-control-plane": {
        "context": "controlplane/kubeadm",
        "image": "gcr.io/k8s-staging-cluster-api/kubeadm-control-plane-controller",
        "live_reload_deps": [
            "main.go",
            "api",
            "controllers",
            "internal",
            "../../go.mod",
            "../../go.sum",
        ],
        "label": "KCP",
    },
    "docker": {
        "context": "test/infrastructure/docker",
        "image": "gcr.io/k8s-staging-cluster-api/capd-manager",
        "live_reload_deps": [
            "main.go",
            "../../go.mod",
            "../../go.sum",
            "../container",
            "api",
            "cloudinit",
            "controllers",
            "docker",
            "exp",
            "internal",
            "third_party",
        ],
        "additional_docker_helper_commands": "RUN curl -LO https://dl.k8s.io/release/v1.23.3/bin/linux/{ARCH}/kubectl && chmod +x ./kubectl && mv ./kubectl /usr/bin/kubectl".format(
            ARCH = os_arch,
        ),
        "additional_docker_build_commands": """
COPY --from=tilt-helper /usr/bin/kubectl /usr/bin/kubectl
""",
        "label": "CAPD",
    },
}

# Reads a provider's tilt-provider.json file and merges it into the providers map.
# A list of dictionaries is also supported by enclosing it in brackets []
# An example file looks like this:
# {
#     "name": "aws",
#     "config": {
#         "image": "gcr.io/k8s-staging-cluster-api-aws/cluster-api-aws-controller",
#         "live_reload_deps": [
#             "main.go", "go.mod", "go.sum", "api", "cmd", "controllers", "pkg"
#         ]
#     }
# }
def load_provider_tiltfiles():
    provider_repos = settings.get("provider_repos", [])

    for repo in provider_repos:
        file = repo + "/tilt-provider.json"
        provider_details = read_json(file, default = {})
        if type(provider_details) != type([]):
            provider_details = [provider_details]
        for item in provider_details:
            provider_name = item["name"]
            provider_config = item["config"]
            if "context" in provider_config:
                provider_config["context"] = repo + "/" + provider_config["context"]
            else:
                provider_config["context"] = repo
            if "kustomize_config" not in provider_config:
                provider_config["kustomize_config"] = True
            if "go_main" not in provider_config:
                provider_config["go_main"] = "main.go"
            providers[provider_name] = provider_config

tilt_helper_dockerfile_header = """
# Tilt image
FROM golang:1.17.3 as tilt-helper
# Support live reloading with Tilt
RUN go get github.com/go-delve/delve/cmd/dlv
RUN wget --output-document /restart.sh --quiet https://raw.githubusercontent.com/windmilleng/rerun-process-wrapper/master/restart.sh  && \
    wget --output-document /start.sh --quiet https://raw.githubusercontent.com/windmilleng/rerun-process-wrapper/master/start.sh && \
    chmod +x /start.sh && chmod +x /restart.sh && chmod +x /go/bin/dlv
"""

tilt_dockerfile_header = """
FROM gcr.io/distroless/base:debug as tilt
WORKDIR /
COPY --from=tilt-helper /start.sh .
COPY --from=tilt-helper /restart.sh .
COPY --from=tilt-helper /go/bin/dlv .
COPY manager .
"""

# Configures a provider by doing the following:
#
# 1. Enables a local_resource go build of the provider's manager binary
# 2. Configures a docker build for the provider, with live updating of the manager binary
# 3. Runs kustomize for the provider's config/default and applies it
def enable_provider(name, debug):
    p = providers.get(name)
    context = p.get("context")
    go_main = p.get("go_main", "main.go")
    label = p.get("label", name)
    debug_port = int(debug.get("port", 0))

    # Prefix each live reload dependency with context. For example, for if the context is
    # test/infra/docker and main.go is listed as a dep, the result is test/infra/docker/main.go. This adjustment is
    # needed so Tilt can watch the correct paths for changes.
    live_reload_deps = []
    for d in p.get("live_reload_deps", []):
        live_reload_deps.append(context + "/" + d)

    # Set up a local_resource build of the provider's manager binary. The provider is expected to have a main.go in
    # manager_build_path or the main.go must be provided via go_main option. The binary is written to .tiltbuild/bin/manager.
    # TODO @randomvariable: Race detector mode only currently works on x86-64 Linux.
    # Need to switch to building inside Docker when architecture is mismatched
    race_detector_enabled = debug.get("race_detector", False)
    if race_detector_enabled:
        if os_name != "linux" or os_arch != "amd64":
            fail("race_detector is only supported on Linux x86-64")
        cgo_enabled = "1"
        build_options = "-race"
        ldflags = "-linkmode external -extldflags \"-static\""
    else:
        cgo_enabled = "0"
        build_options = ""
        ldflags = "-extldflags \"-static\""

    if debug_port != 0:
        # disable optimisations and include line numbers when debugging
        gcflags = "all=-N -l"
    else:
        gcflags = ""

    build_env = "CGO_ENABLED={cgo_enabled} GOOS=linux GOARCH={arch}".format(
        cgo_enabled = cgo_enabled,
        arch = os_arch,
    )
    build_cmd = "{build_env} go build {build_options} -gcflags '{gcflags}' -ldflags '{ldflags}' -o .tiltbuild/bin/manager {go_main}".format(
        build_env = build_env,
        build_options = build_options,
        gcflags = gcflags,
        go_main = go_main,
        ldflags = ldflags,
    )

    local_resource(
        label.lower() + "_binary",
        cmd = "cd {context};mkdir -p .tiltbuild/bin;{build_cmd}".format(
            context = context,
            build_cmd = build_cmd,
        ),
        deps = live_reload_deps,
        labels = [label, "ALL.binaries"],
    )

    additional_docker_helper_commands = p.get("additional_docker_helper_commands", "")
    additional_docker_build_commands = p.get("additional_docker_build_commands", "")

    dockerfile_contents = "\n".join([
        tilt_helper_dockerfile_header,
        additional_docker_helper_commands,
        tilt_dockerfile_header,
        additional_docker_build_commands,
    ])

    port_forwards = []
    links = []

    if debug_port != 0:
        # Add delve when debugging. Delve will always listen on the pod side on port 30000.
        entrypoint = ["sh", "/start.sh", "/dlv", "--listen=:" + str(30000), "--accept-multiclient", "--api-version=2", "--headless=true", "exec", "--", "/manager"]
        port_forwards.append(port_forward(debug_port, 30000))
        if debug.get("continue", True):
            entrypoint.insert(8, "--continue")
    else:
        entrypoint = ["sh", "/start.sh", "/manager"]

    metrics_port = int(debug.get("metrics_port", 0))
    profiler_port = int(debug.get("profiler_port", 0))
    if metrics_port != 0:
        port_forwards.append(port_forward(metrics_port, 8080))
        links.append(link("http://localhost:" + str(metrics_port) + "/metrics", "metrics"))

    if profiler_port != 0:
        port_forwards.append(port_forward(profiler_port, 6060))
        entrypoint.extend(["--profiler-address", ":6060"])
        links.append(link("http://localhost:" + str(profiler_port) + "/debug/pprof", "profiler"))

    # Set up an image build for the provider. The live update configuration syncs the output from the local_resource
    # build into the container.
    provider_args = extra_args.get(name)
    if provider_args:
        entrypoint.extend(provider_args)

    docker_build(
        ref = p.get("image"),
        context = context + "/.tiltbuild/bin/",
        dockerfile_contents = dockerfile_contents,
        target = "tilt",
        entrypoint = entrypoint,
        only = "manager",
        live_update = [
            sync(context + "/.tiltbuild/bin/manager", "/manager"),
            run("sh /restart.sh"),
        ],
    )

    if p.get("kustomize_config", True):
        yaml = read_file("./.tiltbuild/yaml/{}.provider.yaml".format(name))
        k8s_yaml(yaml)
        objs = decode_yaml_stream(yaml)
        k8s_resource(
            workload = find_object_name(objs, "Deployment"),
            objects = [find_object_qualified_name(objs, "Provider")],
            new_name = label.lower() + "_controller",
            labels = [label, "ALL.controllers"],
            port_forwards = port_forwards,
            links = links,
            resource_deps = ["provider_crd"],
        )

def find_object_name(objs, kind):
    for o in objs:
        if o["kind"] == kind:
            return o["metadata"]["name"]
    return ""

def find_object_qualified_name(objs, kind):
    for o in objs:
        if o["kind"] == kind:
            return "{}:{}:{}".format(o["metadata"]["name"], kind, o["metadata"]["namespace"])
    return ""

# Users may define their own Tilt customizations in tilt.d. This directory is excluded from git and these files will
# not be checked in to version control.
def include_user_tilt_files():
    user_tiltfiles = listdir("tilt.d")
    for f in user_tiltfiles:
        include(f)

# Enable core cluster-api plus everything listed in 'enable_providers' in tilt-settings.json
def enable_providers():
    for name in get_providers():
        enable_provider(name, settings.get("debug").get(name, {}))

def get_providers():
    user_enable_providers = settings.get("enable_providers", [])
    return {k: "" for k in user_enable_providers + always_enable_providers}.keys()

def deploy_provider_crds():
    # NOTE: we are applying raw yaml for clusterctl resources (vs delegating this to clusterctl methods) because
    # it is required to control precedence between creating this CRDs and creating providers.
    k8s_yaml(read_file("./.tiltbuild/yaml/clusterctl.crd.yaml"))
    k8s_resource(
        objects = ["providers.clusterctl.cluster.x-k8s.io:CustomResourceDefinition:default"],
        new_name = "provider_crd",
    )

def deploy_observability():
    if "promtail" in settings.get("deploy_observability", []):
        k8s_yaml(read_file("./.tiltbuild/yaml/promtail.observability.yaml"), allow_duplicates = True)
        k8s_resource(workload = "promtail", extra_pod_selectors = [{"app": "promtail"}], labels = ["observability"], resource_deps = ["loki"])

    if "loki" in settings.get("deploy_observability", []):
        k8s_yaml(read_file("./.tiltbuild/yaml/loki.observability.yaml"), allow_duplicates = True)
        k8s_resource(workload = "loki", extra_pod_selectors = [{"app": "loki"}], labels = ["observability"])

    if "grafana" in settings.get("deploy_observability", []):
        k8s_yaml(read_file("./.tiltbuild/yaml/grafana.observability.yaml"), allow_duplicates = True)
        k8s_resource(workload = "grafana", port_forwards = "3001:3000", extra_pod_selectors = [{"app": "grafana"}], labels = ["observability"])

def prepare_all():
    allow_k8s_arg = ""
    if settings.get("allowed_contexts"):
        if type(settings.get("allowed_contexts")) == "string":
            allow_k8s_arg = "--allow-k8s-contexts={} ".format(settings.get("allowed_contexts"))
        if type(settings.get("allowed_contexts")) == "list":
            for context in settings.get("allowed_contexts"):
                allow_k8s_arg = allow_k8s_arg + "--allow-k8s-contexts={} ".format(context)

    tools_arg = "--tools kustomize,envsubst "
    cert_manager_arg = ""
    if settings.get("deploy_cert_manager"):
        cert_manager_arg = "--cert-manager "

    # Note: we are creating clusterctl CRDs using kustomize (vs using clusterctl) because we want to create
    # a dependency between these resources and provider resources.
    kustomize_build_arg = "--kustomize-builds clusterctl.crd:./cmd/clusterctl/config/crd/ "
    for tool in settings.get("deploy_observability", []):
        kustomize_build_arg = kustomize_build_arg + "--kustomize-builds {tool}.observability:./hack/observability/{tool}/ ".format(tool = tool)
    providers_arg = ""
    for name in get_providers():
        p = providers.get(name)
        if p.get("kustomize_config", True):
            context = p.get("context")
            debug = ""
            if name in settings.get("debug"):
                debug = ":debug"
            providers_arg = providers_arg + "--providers {name}:{context}{debug} ".format(
                name = name,
                context = context,
                debug = debug,
            )

    cmd = "make -B tilt-prepare && ./hack/tools/bin/tilt-prepare {allow_k8s_arg}{tools_arg}{cert_manager_arg}{kustomize_build_arg}{providers_arg}".format(
        allow_k8s_arg = allow_k8s_arg,
        tools_arg = tools_arg,
        cert_manager_arg = cert_manager_arg,
        kustomize_build_arg = kustomize_build_arg,
        providers_arg = providers_arg,
    )
    local(cmd, env = settings.get("kustomize_substitutions", {}))

##############################
# Actual work happens here
##############################

include_user_tilt_files()

load_provider_tiltfiles()

prepare_all()

deploy_provider_crds()

deploy_observability()

enable_providers()
