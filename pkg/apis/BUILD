load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "zz_generated.api.register.go",
    ],
    importpath = "sigs.k8s.io/cluster-api/pkg/apis",
    visibility = ["//visibility:public"],
    deps = [
        "//pkg/apis/cluster:go_default_library",
        "//pkg/apis/cluster/v1alpha1:go_default_library",
        "//vendor/github.com/kubernetes-incubator/apiserver-builder/pkg/builders:go_default_library",
    ],
)
