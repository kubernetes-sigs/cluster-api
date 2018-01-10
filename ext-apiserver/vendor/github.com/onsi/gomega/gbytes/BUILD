load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "buffer.go",
        "io_wrappers.go",
        "say_matcher.go",
    ],
    importpath = "github.com/onsi/gomega/gbytes",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/onsi/gomega/format:go_default_library"],
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "buffer_test.go",
        "gbuffer_suite_test.go",
        "io_wrappers_test.go",
        "say_matcher_test.go",
    ],
    importpath = "github.com/onsi/gomega/gbytes_test",
    deps = [
        ":go_default_library",
        "//vendor/github.com/onsi/ginkgo:go_default_library",
        "//vendor/github.com/onsi/gomega:go_default_library",
    ],
)
