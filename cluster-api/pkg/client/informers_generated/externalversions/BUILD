load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "factory.go",
        "generic.go",
    ],
    importpath = "k8s.io/kube-deploy/ext-apiserver/pkg/client/informers_generated/externalversions",
    visibility = ["//visibility:public"],
    deps = [
        "//pkg/apis/cluster/v1alpha1:go_default_library",
        "//pkg/client/clientset_generated/clientset:go_default_library",
        "//pkg/client/informers_generated/externalversions/cluster:go_default_library",
        "//pkg/client/informers_generated/externalversions/internalinterfaces:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/runtime:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/runtime/schema:go_default_library",
        "//vendor/k8s.io/client-go/tools/cache:go_default_library",
    ],
)
