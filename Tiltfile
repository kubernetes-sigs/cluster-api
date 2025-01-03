# -*- mode: Python -*-

envsubst_cmd = "./hack/tools/bin/envsubst"
clusterctl_cmd = "./bin/clusterctl"
kubectl_cmd = "kubectl"
kubernetes_version = "v1.31.2"

load("ext://uibutton", "cmd_button", "location", "text_input")

# set defaults
version_settings(True, ">=0.30.8")

settings = {
    "enable_providers": ["docker"],
    "kind_cluster_name": os.getenv("CAPI_KIND_CLUSTER_NAME", "capi-test"),
    "debug": {},
    "build_engine": "docker",
}

# global settings
tilt_file = "./tilt-settings.yaml" if os.path.exists("./tilt-settings.yaml") else "./tilt-settings.json"
settings.update(read_yaml(
    tilt_file,
    default = {},
))

os.putenv("CAPI_KIND_CLUSTER_NAME", settings.get("kind_cluster_name"))

allow_k8s_contexts(settings.get("allowed_contexts"))

if str(local("command -v " + kubectl_cmd + " || true", quiet = True)) == "":
    fail("Required command '" + kubectl_cmd + "' not found in PATH")

# detect if docker images should be built using podman
if "Podman Engine" in str(local("docker version || podman version", quiet = True)):
    settings["build_engine"] = "podman"

os_name = str(local("go env GOOS")).rstrip("\n")
os_arch = str(local("go env GOARCH")).rstrip("\n")

if settings.get("trigger_mode") == "manual":
    trigger_mode(TRIGGER_MODE_MANUAL)

usingLocalRegistry = str(local(kubectl_cmd + " get cm -n kube-public local-registry-hosting || true", quiet = True))
if not usingLocalRegistry:
    if settings.get("default_registry", "") == "":
        fail("default_registry is required when not using a local registry, please add it to your tilt-settings.yaml/json")

    protectedRegistries = ["gcr.io/k8s-staging-cluster-api"]
    if settings.get("default_registry") in protectedRegistries:
        fail("current default_registry '{}' is protected, tilt cannot push images to it. Please select another default_registry in your tilt-settings.yaml/json".format(settings.get("default_registry")))

if settings.get("default_registry", "") != "":
    default_registry(settings.get("default_registry"))

always_enable_providers = ["core"]

providers = {
    "core": {
        "context": ".",  # NOTE: this should be kept in sync with corresponding setting in tilt-prepare
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
            "util",
            "webhooks",
        ],
        "label": "CAPI",
    },
    "kubeadm-bootstrap": {
        "context": "bootstrap/kubeadm",  # NOTE: this should be kept in sync with corresponding setting in tilt-prepare
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
        "context": "controlplane/kubeadm",  # NOTE: this should be kept in sync with corresponding setting in tilt-prepare
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
        "context": "test/infrastructure/docker",  # NOTE: this should be kept in sync with corresponding setting in tilt-prepare
        "image": "gcr.io/k8s-staging-cluster-api/capd-manager",
        "live_reload_deps": [
            "main.go",
            "../../go.mod",
            "../../go.sum",
            "../container",
            "api",
            "controllers",
            "docker",
            "exp",
            "internal",
        ],
        "label": "CAPD",
    },
    "in-memory": {
        "context": "test/infrastructure/inmemory",  # NOTE: this should be kept in sync with corresponding setting in tilt-prepare
        "image": "gcr.io/k8s-staging-cluster-api/capim-manager",
        "live_reload_deps": [
            "main.go",
            "../../go.mod",
            "../../go.sum",
            "api",
            "controllers",
            "internal",
        ],
        "label": "CAPIM",
    },
    "test-extension": {
        "context": "test/extension",  # NOTE: this should be kept in sync with corresponding setting in tilt-prepare
        "image": "gcr.io/k8s-staging-cluster-api/test-extension",
        "live_reload_deps": [
            "main.go",
            "handlers",
        ],
        "label": "test-extension",
        # Add the ExtensionConfig for this Runtime extension; given that the ExtensionConfig can be installed only when capi_controller
        # are up and running, it is required to set a resource_deps to ensure proper install order.
        "additional_resources": [
            "config/tilt/extensionconfig.yaml",
        ],
        "resource_deps": ["capi_controller"],
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
        file = repo + "/tilt-provider.yaml" if os.path.exists(repo + "/tilt-provider.yaml") else repo + "/tilt-provider.json"
        if not os.path.exists(file):
            fail("Failed to load provider. No tilt-provider.{yaml|json} file found in " + repo)
        provider_details = read_yaml(file, default = {})
        if type(provider_details) != type([]):
            provider_details = [provider_details]
        for item in provider_details:
            provider_name = item["name"]
            provider_config = item["config"]
            if "context" in provider_config:
                provider_config["context"] = repo + "/" + provider_config["context"]
            else:
                provider_config["context"] = repo
            if "go_main" not in provider_config:
                provider_config["go_main"] = "main.go"
            providers[provider_name] = provider_config

tilt_helper_dockerfile_header = """
# Tilt image
FROM golang:1.23.0 as tilt-helper
# Install delve. Note this should be kept in step with the Go release minor version.
RUN go install github.com/go-delve/delve/cmd/dlv@v1.23
# Support live reloading with Tilt
RUN wget --output-document /restart.sh --quiet https://raw.githubusercontent.com/tilt-dev/rerun-process-wrapper/master/restart.sh  && \
    wget --output-document /start.sh --quiet https://raw.githubusercontent.com/tilt-dev/rerun-process-wrapper/master/start.sh && \
    chmod +x /start.sh && chmod +x /restart.sh && chmod +x /go/bin/dlv && \
    touch /process.txt && chmod 0777 /process.txt `# pre-create PID file to allow even non-root users to run the image`
"""

tilt_dockerfile_header = """
FROM golang:1.23.0 as tilt
WORKDIR /
COPY --from=tilt-helper /process.txt .
COPY --from=tilt-helper /start.sh .
COPY --from=tilt-helper /restart.sh .
COPY --from=tilt-helper /go/bin/dlv .
COPY $binary_name .
"""

def build_go_binary(context, reload_deps, debug, go_main, binary_name, label):
    # Set up a local_resource build of a go binary. The target repo is expected to have a main.go in
    # the context path or the main.go must be provided via go_main option. The binary is written to .tiltbuild/bin/{$binary_name}.
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

    debug_port = int(debug.get("port", 0))
    if debug_port != 0:
        # disable optimisations and include line numbers when debugging
        gcflags = "all=-N -l"
    else:
        gcflags = ""

    build_env = "CGO_ENABLED={cgo_enabled} GOOS=linux GOARCH={arch}".format(
        cgo_enabled = cgo_enabled,
        arch = os_arch,
    )

    build_cmd = "{build_env} go build {build_options} -gcflags '{gcflags}' -ldflags '{ldflags}' -o .tiltbuild/bin/{binary_name} {go_main}".format(
        build_env = build_env,
        build_options = build_options,
        gcflags = gcflags,
        go_main = go_main,
        ldflags = ldflags,
        binary_name = binary_name,
    )

    # Prefix each live reload dependency with context. For example, for if the context is
    # test/infra/docker and main.go is listed as a dep, the result is test/infra/docker/main.go. This adjustment is
    # needed so Tilt can watch the correct paths for changes.
    live_reload_deps = []
    for d in reload_deps:
        live_reload_deps.append(context + "/" + d)

    # Ensure the {context}/.tiltbuild/bin directory before any other resources
    # `local` is evaluated immediately, other resources are executed later in the startup/when triggered
    local("mkdir -p {context}/.tiltbuild/bin".format(context = shlex.quote(context)), quiet = True)

    # Build the go binary
    local_resource(
        label.lower() + "_binary",
        cmd = "cd {context};{build_cmd}".format(
            context = context,
            build_cmd = build_cmd,
        ),
        deps = live_reload_deps,
        labels = [label, "ALL.binaries"],
    )

def build_docker_image(image, context, binary_name, additional_docker_build_commands, additional_docker_helper_commands, port_forwards):
    links = []

    dockerfile_contents = "\n".join([
        tilt_helper_dockerfile_header,
        additional_docker_helper_commands,
        tilt_dockerfile_header,
        additional_docker_build_commands,
    ])

    # Set up an image build for the provider. The live update configuration syncs the output from the local_resource
    # build into the container.
    if settings.get("build_engine") == "podman":
        bin_context = context + "/.tiltbuild/bin/"

        # Write dockerfile_contents to a Dockerfile as custom_build doesn't support dockerfile_contents nor stdin.
        # The Dockerfile is in the context path to simplify the below podman command.
        local("tee %s/Dockerfile" % (shlex.quote(bin_context)), quiet = True, stdin = dockerfile_contents)

        custom_build(
            ref = image,
            command = (
                "set -ex\n" +
                "podman build -t $EXPECTED_REF --build-arg binary_name=%s --target tilt %s\n" +
                "podman push --format=docker $EXPECTED_REF\n"
            ) % (binary_name, shlex.quote(bin_context)),
            deps = [bin_context],
            skips_local_docker = True,
            live_update = [
                sync(bin_context + binary_name, "/" + binary_name),
                run("sh /restart.sh"),
            ],
        )
    else:
        docker_build(
            ref = image,
            context = context + "/.tiltbuild/bin/",
            dockerfile_contents = dockerfile_contents,
            build_args = {"binary_name": binary_name},
            target = "tilt",
            only = binary_name,
            live_update = [
                sync(context + "/.tiltbuild/bin/" + binary_name, "/" + binary_name),
                run("sh /restart.sh"),
            ],
        )

def get_port_forwards(debug):
    port_forwards = []
    links = []

    debug_port = int(debug.get("port", 0))
    if debug_port != 0:
        port_forwards.append(port_forward(debug_port, 30000))

    metrics_port = int(debug.get("metrics_port", 0))
    profiler_port = int(debug.get("profiler_port", 0))
    if metrics_port != 0:
        port_forwards.append(port_forward(metrics_port, 8080))
        links.append(link("http://localhost:" + str(metrics_port) + "/metrics", "metrics"))

    if profiler_port != 0:
        port_forwards.append(port_forward(profiler_port, 6060))
        links.append(link("http://localhost:" + str(profiler_port) + "/debug/pprof", "profiler"))

    return port_forwards, links

# Configures a provider by doing the following:
#
# 1. Enables a local_resource go build of the provider's manager binary
# 2. Configures a docker build for the provider, with live updating of the manager binary
# 3. Runs kustomize for the provider's config/default and applies it
def enable_provider(name, debug):
    p = providers.get(name)
    label = p.get("label")

    port_forwards, links = get_port_forwards(debug)

    if p.get("image"):
        build_go_binary(
            context = p.get("context"),
            reload_deps = p.get("live_reload_deps"),
            debug = debug,
            go_main = p.get("go_main", "main.go"),
            binary_name = "manager",
            label = label,
        )

        build_docker_image(
            image = p.get("image"),
            context = p.get("context"),
            binary_name = "manager",
            additional_docker_helper_commands = p.get("additional_docker_helper_commands", ""),
            additional_docker_build_commands = p.get("additional_docker_build_commands", ""),
            port_forwards = port_forwards,
        )

    additional_objs = []
    p_resources = p.get("additional_resources", [])
    for resource in p_resources:
        k8s_yaml(p.get("context") + "/" + resource)
        additional_objs = additional_objs + decode_yaml_stream(read_file(p.get("context") + "/" + resource))

    if p.get("apply_provider_yaml", True):
        yaml = read_file("./.tiltbuild/yaml/{}.provider.yaml".format(name))
        k8s_yaml(yaml, allow_duplicates = True)
        objs = decode_yaml_stream(yaml)
        k8s_resource(
            workload = find_object_name(objs, "Deployment"),
            objects = [find_object_qualified_name(objs, "Provider")] + find_all_objects_names(additional_objs),
            new_name = label.lower() + "_controller",
            labels = [label, "ALL.controllers"],
            port_forwards = port_forwards,
            links = links,
            resource_deps = ["provider_crd"] + p.get("resource_deps", []),
        )

def find_object_name(objs, kind):
    for o in objs:
        # Ignore objects that are not part of the provider, e.g. the ASO Deployment in CAPZ.
        if o["kind"] == kind and "cluster.x-k8s.io/provider" in o["metadata"]["labels"]:
            return o["metadata"]["name"]
    return ""

def find_object_qualified_name(objs, kind):
    for o in objs:
        if o["kind"] == kind:
            return "{}:{}:{}".format(o["metadata"]["name"], kind, o["metadata"]["namespace"])
    return ""

def find_all_objects_names(objs):
    qualified_names = []
    for o in objs:
        if "namespace" in o["metadata"] and o["metadata"]["namespace"] != "":
            qualified_names = qualified_names + ["{}:{}:{}".format(o["metadata"]["name"], o["kind"], o["metadata"]["namespace"])]
        else:
            qualified_names = qualified_names + ["{}:{}".format(o["metadata"]["name"], o["kind"])]
    return qualified_names

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
        k8s_resource(workload = "promtail", extra_pod_selectors = [{"app": "promtail"}], labels = ["observability"], resource_deps = ["loki"], objects = ["promtail:serviceaccount"])

    if "loki" in settings.get("deploy_observability", []):
        k8s_yaml(read_file("./.tiltbuild/yaml/loki.observability.yaml"), allow_duplicates = True)
        k8s_resource(workload = "loki", port_forwards = "3100", extra_pod_selectors = [{"app": "loki"}], labels = ["observability"], objects = ["loki:serviceaccount"])

        cmd_button(
            "loki:import logs",
            argv = ["sh", "-c", "cd ./hack/tools/internal/log-push && go run ./main.go --log-path=$LOG_PATH"],
            resource = "loki",
            icon_name = "import_export",
            text = "Import logs",
            inputs = [
                text_input("LOG_PATH", label = "Log path, one of: GCS path, ProwJob URL or local folder"),
            ],
        )

    if "tempo" in settings.get("deploy_observability", []):
        k8s_yaml(read_file("./.tiltbuild/yaml/tempo.observability.yaml"), allow_duplicates = True)

        # Port-forward the tracing port to localhost, so we can also send traces from local.
        k8s_resource(workload = "tempo", port_forwards = "4317:4317", extra_pod_selectors = [{"app": "tempo"}], labels = ["observability"])

    if "grafana" in settings.get("deploy_observability", []):
        k8s_yaml(read_file("./.tiltbuild/yaml/grafana.observability.yaml"), allow_duplicates = True)
        k8s_resource(workload = "grafana", port_forwards = "3001:3000", extra_pod_selectors = [{"app": "grafana"}], labels = ["observability"], objects = ["grafana:serviceaccount"])

    if "prometheus" in settings.get("deploy_observability", []):
        k8s_yaml(read_file("./.tiltbuild/yaml/prometheus.observability.yaml"), allow_duplicates = True)
        k8s_resource(workload = "prometheus-server", new_name = "prometheus", port_forwards = "9090", extra_pod_selectors = [{"app": "prometheus"}], labels = ["observability"], objects = ["prometheus-server:serviceaccount"])

    if "kube-state-metrics" in settings.get("deploy_observability", []):
        k8s_yaml(read_file("./.tiltbuild/yaml/kube-state-metrics.observability.yaml"), allow_duplicates = True)
        k8s_resource(workload = "kube-state-metrics", new_name = "kube-state-metrics", extra_pod_selectors = [{"app": "kube-state-metrics"}], labels = ["observability"], objects = ["kube-state-metrics:serviceaccount"])

    if "parca" in settings.get("deploy_observability", []):
        k8s_yaml(read_file("./.tiltbuild/yaml/parca.observability.yaml"), allow_duplicates = True)
        k8s_resource(workload = "parca", new_name = "parca", port_forwards = "7070", extra_pod_selectors = [{"app": "parca"}], labels = ["observability"], objects = ["parca:serviceaccount"])

    if "metrics-server" in settings.get("deploy_observability", []):
        k8s_yaml(read_file("./.tiltbuild/yaml/metrics-server.observability.yaml"), allow_duplicates = True)
        k8s_resource(workload = "metrics-server", new_name = "metrics-server", extra_pod_selectors = [{"app": "metrics-server"}], labels = ["observability"], objects = ["metrics-server:serviceaccount"])

    if "visualizer" in settings.get("deploy_observability", []):
        k8s_yaml(read_file("./.tiltbuild/yaml/visualizer.observability.yaml"), allow_duplicates = True)
        k8s_resource(
            workload = "capi-visualizer",
            new_name = "visualizer",
            port_forwards = [port_forward(local_port = 8000, container_port = 8081, name = "View visualization")],
            labels = ["observability"],
            objects = ["capi-visualizer:serviceaccount"],
        )

def deploy_additional_kustomizations():
    for name in settings.get("additional_kustomizations", []):
        yaml = read_file("./.tiltbuild/yaml/{}.kustomization.yaml".format(name))
        k8s_yaml(yaml)
        objs = decode_yaml_stream(yaml)
        print("objects")
        print(find_all_objects_names(objs))
        k8s_resource(
            new_name = name,
            objects = find_all_objects_names(objs),
            labels = ["kustomization"],
        )

def prepare_all():
    tools_arg = "--tools kustomize,envsubst,clusterctl "
    tilt_settings_file_arg = "--tilt-settings-file " + tilt_file

    cmd = "make -B tilt-prepare && ./hack/tools/bin/tilt-prepare {tools_arg}{tilt_settings_file_arg}".format(
        tools_arg = tools_arg,
        tilt_settings_file_arg = tilt_settings_file_arg,
    )
    local(cmd, env = settings.get("kustomize_substitutions", {}))

# create cluster template resources from cluster-template files in the templates directory
def cluster_templates():
    substitutions = settings.get("kustomize_substitutions", {})

    # Ensure we have default values for a small set of well-known variables
    substitutions["NAMESPACE"] = substitutions.get("NAMESPACE", "default")
    substitutions["KUBERNETES_VERSION"] = substitutions.get("KUBERNETES_VERSION", kubernetes_version)
    substitutions["CONTROL_PLANE_MACHINE_COUNT"] = substitutions.get("CONTROL_PLANE_MACHINE_COUNT", "1")
    substitutions["WORKER_MACHINE_COUNT"] = substitutions.get("WORKER_MACHINE_COUNT", "1")

    template_dirs = settings.get("template_dirs", {
        "docker": ["./test/infrastructure/docker/templates"],
        "in-memory": ["./test/infrastructure/inmemory/templates"],
    })

    for provider, provider_dirs in template_dirs.items():
        if provider not in get_providers():
            continue

        p = providers.get(provider)
        label = p.get("label", provider)

        for template_dir in provider_dirs:
            template_list = [filename for filename in listdir(template_dir) if os.path.basename(filename).endswith("yaml")]
            for filename in template_list:
                deploy_templates(filename, label, substitutions)

def deploy_templates(filename, label, substitutions):
    # validate filename exists
    if not os.path.exists(filename):
        fail(filename + " not found")

    basename = os.path.basename(filename)
    if basename.endswith(".yaml"):
        if basename.startswith("clusterclass-"):
            clusterclass_name = basename.replace("clusterclass-", "").replace(".yaml", "")
            deploy_clusterclass(clusterclass_name, label, filename, substitutions)
        elif basename.startswith("cluster-template-"):
            template_name = basename.replace("cluster-template-", "").replace(".yaml", "")
            deploy_cluster_template(template_name, label, filename, substitutions)
        elif basename == "cluster-template.yaml":
            template_name = "default"
            deploy_cluster_template(template_name, label, filename, substitutions)

def deploy_clusterclass(clusterclass_name, label, filename, substitutions):
    apply_clusterclass_cmd = "cat " + filename + " | " + envsubst_cmd + " | " + kubectl_cmd + " apply --namespace=$NAMESPACE -f - && echo \"ClusterClass created from\'" + filename + "\', don't forget to delete\n\""
    delete_clusterclass_cmd = kubectl_cmd + " --namespace=$NAMESPACE delete clusterclass " + clusterclass_name + ' --ignore-not-found=true; echo "\n"'

    local_resource(
        name = clusterclass_name,
        cmd = ["bash", "-c", apply_clusterclass_cmd],
        env = substitutions,
        auto_init = False,
        trigger_mode = TRIGGER_MODE_MANUAL,
        labels = [label + ".clusterclasses"],
    )

    cmd_button(
        clusterclass_name + ":apply",
        argv = ["bash", "-c", apply_clusterclass_cmd],
        env = dictonary_to_list_of_string(substitutions),
        resource = clusterclass_name,
        icon_name = "note_add",
        text = "Apply `" + clusterclass_name + "` ClusterClass",
        inputs = [
            text_input("NAMESPACE", default = substitutions.get("NAMESPACE")),
        ],
    )

    cmd_button(
        clusterclass_name + ":delete",
        argv = ["bash", "-c", delete_clusterclass_cmd],
        env = dictonary_to_list_of_string(substitutions),
        resource = clusterclass_name,
        icon_name = "delete_forever",
        text = "Delete `" + clusterclass_name + "` ClusterClass",
        inputs = [
            text_input("NAMESPACE", default = substitutions.get("NAMESPACE")),
        ],
    )

def deploy_cluster_template(template_name, label, filename, substitutions):
    apply_cluster_template_cmd = "CLUSTER_NAME=" + template_name + "-$RANDOM;" + clusterctl_cmd + " generate cluster -n $NAMESPACE $CLUSTER_NAME --from " + filename + " | " + kubectl_cmd + " apply -f - && echo \"Cluster '$CLUSTER_NAME' created, don't forget to delete\n\""
    delete_clusters_cmd = 'DELETED=$(echo "$(bash -c "' + kubectl_cmd + ' --namespace=$NAMESPACE get clusters -A --no-headers -o custom-columns=":metadata.name"")" | grep -E "^' + template_name + '-[[:digit:]]{1,5}$"); if [ -z "$DELETED" ]; then echo "Nothing to delete for cluster template ' + template_name + '"; else echo "Deleting clusters:\n$DELETED\n"; echo $DELETED | xargs -L1 ' + kubectl_cmd + ' delete cluster; fi; echo "\n"'

    local_resource(
        name = template_name,
        cmd = ["bash", "-c", apply_cluster_template_cmd],
        env = substitutions,
        auto_init = False,
        trigger_mode = TRIGGER_MODE_MANUAL,
        labels = [label + ".templates"],
    )

    cmd_button(
        template_name + ":apply",
        argv = ["bash", "-c", apply_cluster_template_cmd],
        env = dictonary_to_list_of_string(substitutions),
        resource = template_name,
        icon_name = "add_box",
        text = "Create `" + template_name + "` cluster",
        inputs = [
            text_input("NAMESPACE", default = substitutions.get("NAMESPACE")),
            text_input("KUBERNETES_VERSION", default = substitutions.get("KUBERNETES_VERSION")),
            text_input("CONTROL_PLANE_MACHINE_COUNT", default = substitutions.get("CONTROL_PLANE_MACHINE_COUNT")),
            text_input("WORKER_MACHINE_COUNT", default = substitutions.get("WORKER_MACHINE_COUNT")),
        ],
    )

    cmd_button(
        template_name + ":delete",
        argv = ["bash", "-c", delete_clusters_cmd],
        env = dictonary_to_list_of_string(substitutions),
        resource = template_name,
        icon_name = "delete_forever",
        text = "Delete `" + template_name + "` clusters",
        inputs = [
            text_input("NAMESPACE", default = substitutions.get("NAMESPACE")),
        ],
    )

    cmd_button(
        template_name + ":delete-all",
        argv = ["bash", "-c", kubectl_cmd + " delete clusters --all --wait=false"],
        env = dictonary_to_list_of_string(substitutions),
        resource = template_name,
        icon_name = "delete_sweep",
        text = "Delete all workload clusters",
    )

# A function to convert dictonary to list of strings in a format of "name=value"
def dictonary_to_list_of_string(substitutions):
    substitutions_list = []
    for name, value in substitutions.items():
        substitutions_list.append(name + "=" + value)
    return substitutions_list

##############################
# Actual work happens here
##############################

include_user_tilt_files()

load_provider_tiltfiles()

prepare_all()

deploy_provider_crds()

deploy_observability()

deploy_additional_kustomizations()

enable_providers()

cluster_templates()
