load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["interface.go"],
    importpath = "k8s.io/kube-deploy/ext-apiserver/pkg/client/informers_generated/externalversions/cluster",
    visibility = ["//visibility:public"],
    deps = [
        "//pkg/client/informers_generated/externalversions/cluster/v1alpha1:go_default_library",
        "//pkg/client/informers_generated/externalversions/internalinterfaces:go_default_library",
    ],
)
