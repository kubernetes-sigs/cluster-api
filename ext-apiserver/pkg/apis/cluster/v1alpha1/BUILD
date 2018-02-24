load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "cluster_types.go",
        "doc.go",
        "machine_types.go",
        "zz_generated.api.register.go",
        "zz_generated.conversion.go",
        "zz_generated.deepcopy.go",
        "zz_generated.defaults.go",
    ],
    importpath = "k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/v1alpha1",
    visibility = ["//visibility:public"],
    deps = [
        "//pkg/apis/cluster:go_default_library",
        "//vendor/github.com/kubernetes-incubator/apiserver-builder/pkg/builders:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/apis/meta/v1:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/conversion:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/runtime:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/runtime/schema:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/util/validation/field:go_default_library",
        "//vendor/k8s.io/apiserver/pkg/endpoints/request:go_default_library",
    ],
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "cluster_types_test.go",
        "machine_types_test.go",
        "v1alpha1_suite_test.go",
    ],
    importpath = "k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/v1alpha1_test",
    deps = [
        ":go_default_library",
        "//pkg/apis:go_default_library",
        "//pkg/client/clientset_generated/clientset:go_default_library",
        "//pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1:go_default_library",
        "//pkg/openapi:go_default_library",
        "//vendor/github.com/kubernetes-incubator/apiserver-builder/pkg/test:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/apis/meta/v1:go_default_library",
        "//vendor/k8s.io/client-go/rest:go_default_library",
    ],
)
