# -*- mode: Python -*-
# set defaults
load("ext://cert_manager", "deploy_cert_manager")
load("ext://local_output", "local_output")

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

envsubst_cmd = "./hack/tools/bin/envsubst"
kustomize_cmd = "./hack/tools/bin/kustomize"
yq_cmd = "./hack/tools/bin/yq"
os_name = local_output("go env GOOS")
os_arch = local_output("go env GOARCH")

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
            "third_party",
            "util",
            "exp",
            "feature",
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
            "api",
            "cloudinit",
            "controllers",
            "docker",
            "exp",
            "third_party",
        ],
        "additional_docker_helper_commands": "RUN wget -qO- https://dl.k8s.io/v1.21.2/kubernetes-client-linux-{arch}.tar.gz | tar xvz".format(
            arch = os_arch,
        ),
        "additional_docker_build_commands": """
COPY --from=tilt-helper /go/kubernetes/client/bin/kubectl /usr/bin/kubectl
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
    # manager_build_path or the main.go must be provided via go_main option. The binary is written to .tiltbuild/manager.
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
    build_cmd = "{build_env} go build {build_options} -gcflags '{gcflags}' -ldflags '{ldflags}' -o .tiltbuild/manager {go_main}".format(
        build_env = build_env,
        build_options = build_options,
        gcflags = gcflags,
        go_main = go_main,
        ldflags = ldflags,
    )

    local_resource(
        label.lower() + "_binary",
        cmd = "cd " + context + ";mkdir -p .tiltbuild;" + build_cmd,
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
        context = context + "/.tiltbuild/",
        dockerfile_contents = dockerfile_contents,
        target = "tilt",
        entrypoint = entrypoint,
        only = "manager",
        live_update = [
            sync(context + "/.tiltbuild/manager", "/manager"),
            run("sh /restart.sh"),
        ],
    )

    if p.get("kustomize_config", True):
        # Copy all the substitutions from the user's tilt-settings.json into the environment. Otherwise, the substitutions
        # are not available and their placeholders will be replaced with the empty string when we call kustomize +
        # envsubst below.
        substitutions = settings.get("kustomize_substitutions", {})
        os.environ.update(substitutions)

        # Apply the kustomized yaml for this provider
        if debug_port == 0:
            yaml = str(kustomize_with_envsubst(context + "/config/default", False))
        else:
            yaml = str(kustomize_with_envsubst(context + "/config/default", True))
        k8s_yaml(blob(yaml))

        manager_name = find_manager(yaml)
        print("manager: " + manager_name)

        k8s_resource(
            workload = manager_name,
            new_name = label.lower() + "_controller",
            labels = [label, "ALL.controllers"],
            port_forwards = port_forwards,
            links = links,
        )

def find_manager(yaml):
    manifests = decode_yaml_stream(yaml)
    for m in manifests:
        if m["kind"] == "Deployment":
            return m["metadata"]["name"]
    return ""

# Users may define their own Tilt customizations in tilt.d. This directory is excluded from git and these files will
# not be checked in to version control.
def include_user_tilt_files():
    user_tiltfiles = listdir("tilt.d")
    for f in user_tiltfiles:
        include(f)

# Enable core cluster-api plus everything listed in 'enable_providers' in tilt-settings.json
def enable_providers():
    local("make kustomize envsubst")
    user_enable_providers = settings.get("enable_providers", [])
    union_enable_providers = {k: "" for k in user_enable_providers + always_enable_providers}.keys()
    for name in union_enable_providers:
        enable_provider(name, settings.get("debug").get(name, {}))

def kustomize_with_envsubst(path, enable_debug = False):
    # we need to ditch the readiness and liveness probes when debugging, otherwise K8s will restart the pod whenever execution
    # has paused.
    if enable_debug:
        yq_cmd_line = "| {} eval 'del(.. | select(has\"livenessProbe\")).livenessProbe | del(.. | select(has\"readinessProbe\")).readinessProbe' -".format(yq_cmd)
    else:
        yq_cmd_line = ""
    return str(local("{} build {} | {} {}".format(kustomize_cmd, path, envsubst_cmd, yq_cmd_line), quiet = True))

def ensure_yq():
    if not os.path.exists(yq_cmd):
        local("make {}".format(yq_cmd))

def ensure_envsubst():
    if not os.path.exists(envsubst_cmd):
        local("make {}".format(os.path.abspath(envsubst_cmd)))

def ensure_kustomize():
    if not os.path.exists(kustomize_cmd):
        local("make {}".format(os.path.abspath(kustomize_cmd)))

def deploy_observability():
    k8s_yaml(blob(str(local("{} build {}".format(kustomize_cmd, "./hack/observability/"), quiet = True))))

    k8s_resource(workload = "promtail", extra_pod_selectors = [{"app": "promtail"}], labels = ["observability"])
    k8s_resource(workload = "loki", extra_pod_selectors = [{"app": "loki"}], labels = ["observability"])
    k8s_resource(workload = "grafana", port_forwards = "3000", extra_pod_selectors = [{"app": "grafana"}], labels = ["observability"])

##############################
# Actual work happens here
##############################

ensure_yq()
ensure_envsubst()
ensure_kustomize()

include_user_tilt_files()

load_provider_tiltfiles()

if settings.get("deploy_cert_manager"):
    deploy_cert_manager(version = "v1.5.3")

if settings.get("deploy_observability"):
    deploy_observability()

enable_providers()
