load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "build.go",
        "exit_matcher.go",
        "prefixed_writer.go",
        "session.go",
    ],
    importpath = "github.com/onsi/gomega/gexec",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/onsi/gomega:go_default_library",
        "//vendor/github.com/onsi/gomega/format:go_default_library",
        "//vendor/github.com/onsi/gomega/gbytes:go_default_library",
    ],
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "build_test.go",
        "exit_matcher_test.go",
        "gexec_suite_test.go",
        "prefixed_writer_test.go",
        "session_test.go",
    ],
    importpath = "github.com/onsi/gomega/gexec_test",
    deps = [
        ":go_default_library",
        "//vendor/github.com/onsi/ginkgo:go_default_library",
        "//vendor/github.com/onsi/gomega:go_default_library",
        "//vendor/github.com/onsi/gomega/gbytes:go_default_library",
    ],
)
