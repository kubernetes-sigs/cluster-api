load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "log_writer.go",
        "run_result.go",
        "test_runner.go",
    ],
    importpath = "github.com/onsi/ginkgo/ginkgo/testrunner",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/onsi/ginkgo/config:go_default_library",
        "//vendor/github.com/onsi/ginkgo/ginkgo/testsuite:go_default_library",
        "//vendor/github.com/onsi/ginkgo/internal/remote:go_default_library",
        "//vendor/github.com/onsi/ginkgo/reporters/stenographer:go_default_library",
        "//vendor/github.com/onsi/ginkgo/types:go_default_library",
    ],
)

go_test(
    name = "go_default_xtest",
    srcs = ["test_runner_test.go"],
    importpath = "github.com/onsi/ginkgo/ginkgo/testrunner_test",
    deps = [
        ":go_default_library",
        "//vendor/github.com/onsi/ginkgo:go_default_library",
        "//vendor/github.com/onsi/ginkgo/ginkgo/testsuite:go_default_library",
        "//vendor/github.com/onsi/gomega:go_default_library",
    ],
)
