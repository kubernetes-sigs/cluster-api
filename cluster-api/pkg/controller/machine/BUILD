load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "controller.go",
        "zz_generated.api.register.go",
    ],
    importpath = "k8s.io/kube-deploy/ext-apiserver/pkg/controller/machine",
    visibility = ["//visibility:public"],
    deps = [
        "//pkg/apis/cluster/v1alpha1:go_default_library",
        "//pkg/client/listers_generated/cluster/v1alpha1:go_default_library",
        "//pkg/controller/sharedinformers:go_default_library",
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/kubernetes-incubator/apiserver-builder/pkg/builders:go_default_library",
        "//vendor/github.com/kubernetes-incubator/apiserver-builder/pkg/controller:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/api/errors:go_default_library",
        "//vendor/k8s.io/client-go/rest:go_default_library",
        "//vendor/k8s.io/client-go/tools/cache:go_default_library",
        "//vendor/k8s.io/client-go/util/workqueue:go_default_library",
    ],
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "controller_test.go",
        "machine_suite_test.go",
    ],
    importpath = "k8s.io/kube-deploy/ext-apiserver/pkg/controller/machine_test",
    deps = [
        ":go_default_library",
        "//pkg/apis:go_default_library",
        "//pkg/apis/cluster/v1alpha1:go_default_library",
        "//pkg/client/clientset_generated/clientset:go_default_library",
        "//pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1:go_default_library",
        "//pkg/controller/sharedinformers:go_default_library",
        "//pkg/openapi:go_default_library",
        "//vendor/github.com/kubernetes-incubator/apiserver-builder/pkg/test:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/apis/meta/v1:go_default_library",
        "//vendor/k8s.io/client-go/rest:go_default_library",
    ],
)
