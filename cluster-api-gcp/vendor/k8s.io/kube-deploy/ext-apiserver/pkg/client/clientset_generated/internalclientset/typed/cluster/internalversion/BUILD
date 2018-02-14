load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "cluster.go",
        "cluster_client.go",
        "doc.go",
        "generated_expansion.go",
        "machine.go",
    ],
    importpath = "k8s.io/kube-deploy/ext-apiserver/pkg/client/clientset_generated/internalclientset/typed/cluster/internalversion",
    visibility = ["//visibility:public"],
    deps = [
        "//pkg/apis/cluster:go_default_library",
        "//pkg/client/clientset_generated/internalclientset/scheme:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/apis/meta/v1:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/types:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/watch:go_default_library",
        "//vendor/k8s.io/client-go/rest:go_default_library",
    ],
)
