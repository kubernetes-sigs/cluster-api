load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "k8s.io/kube-deploy/ext-apiserver/cmd/controller-manager",
    visibility = ["//visibility:private"],
    deps = [
        "//pkg/controller:go_default_library",
        "//vendor/github.com/kubernetes-incubator/apiserver-builder/pkg/controller:go_default_library",
    ],
)

go_binary(
    name = "controller-manager",
    importpath = "k8s.io/kube-deploy/ext-apiserver/cmd/controller-manager",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)
