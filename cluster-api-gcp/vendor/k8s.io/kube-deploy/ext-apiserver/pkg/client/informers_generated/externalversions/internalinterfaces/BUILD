load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["factory_interfaces.go"],
    importpath = "k8s.io/kube-deploy/ext-apiserver/pkg/client/informers_generated/externalversions/internalinterfaces",
    visibility = ["//visibility:public"],
    deps = [
        "//pkg/client/clientset_generated/clientset:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/runtime:go_default_library",
        "//vendor/k8s.io/client-go/tools/cache:go_default_library",
    ],
)
