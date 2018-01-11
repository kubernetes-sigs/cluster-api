load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "test_suite.go",
        "vendor_check_go15.go",
        "vendor_check_go16.go",
    ],
    importpath = "github.com/onsi/ginkgo/ginkgo/testsuite",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "testsuite_suite_test.go",
        "testsuite_test.go",
        "vendor_check_go15_test.go",
    ],
    importpath = "github.com/onsi/ginkgo/ginkgo/testsuite_test",
    deps = [
        ":go_default_library",
        "//vendor/github.com/onsi/ginkgo:go_default_library",
        "//vendor/github.com/onsi/gomega:go_default_library",
    ],
)
