load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "table.go",
        "table_entry.go",
    ],
    importpath = "github.com/onsi/ginkgo/extensions/table",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/onsi/ginkgo:go_default_library"],
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "table_suite_test.go",
        "table_test.go",
    ],
    importpath = "github.com/onsi/ginkgo/extensions/table_test",
    deps = [
        ":go_default_library",
        "//vendor/github.com/onsi/ginkgo:go_default_library",
        "//vendor/github.com/onsi/gomega:go_default_library",
    ],
)
