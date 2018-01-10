load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["nodot.go"],
    importpath = "github.com/onsi/ginkgo/ginkgo/nodot",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "nodot_suite_test.go",
        "nodot_test.go",
    ],
    importpath = "github.com/onsi/ginkgo/ginkgo/nodot_test",
    deps = [
        ":go_default_library",
        "//vendor/github.com/onsi/ginkgo:go_default_library",
        "//vendor/github.com/onsi/gomega:go_default_library",
    ],
)
