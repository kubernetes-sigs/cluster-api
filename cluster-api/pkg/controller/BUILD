load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "zz_generated.api.register.go",
    ],
    importpath = "k8s.io/kube-deploy/ext-apiserver/pkg/controller",
    visibility = ["//visibility:public"],
    deps = [
        "//pkg/controller/cluster:go_default_library",
        "//pkg/controller/machine:go_default_library",
        "//pkg/controller/sharedinformers:go_default_library",
        "//vendor/github.com/kubernetes-incubator/apiserver-builder/pkg/controller:go_default_library",
        "//vendor/k8s.io/client-go/rest:go_default_library",
    ],
)
