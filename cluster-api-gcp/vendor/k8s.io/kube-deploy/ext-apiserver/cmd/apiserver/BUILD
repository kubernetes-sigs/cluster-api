load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "k8s.io/kube-deploy/ext-apiserver/cmd/apiserver",
    visibility = ["//visibility:private"],
    deps = [
        "//pkg/apis:go_default_library",
        "//pkg/openapi:go_default_library",
        "//vendor/github.com/go-openapi/loads:go_default_library",
        "//vendor/github.com/kubernetes-incubator/apiserver-builder/pkg/cmd/server:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/apis/meta/v1:go_default_library",
        "//vendor/k8s.io/client-go/plugin/pkg/client/auth:go_default_library",
    ],
)

go_binary(
    name = "apiserver",
    importpath = "k8s.io/kube-deploy/ext-apiserver/cmd/apiserver",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)
