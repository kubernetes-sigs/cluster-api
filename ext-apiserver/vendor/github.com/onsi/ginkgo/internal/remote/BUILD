load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "aggregator.go",
        "forwarding_reporter.go",
        "output_interceptor.go",
        "server.go",
        "syscall_dup_unix.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:darwin_amd64": [
            "output_interceptor_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_amd64": [
            "output_interceptor_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:windows_amd64": [
            "output_interceptor_win.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "github.com/onsi/ginkgo/internal/remote",
    visibility = ["//vendor/github.com/onsi/ginkgo:__subpackages__"],
    deps = [
        "//vendor/github.com/onsi/ginkgo/config:go_default_library",
        "//vendor/github.com/onsi/ginkgo/internal/spec_iterator:go_default_library",
        "//vendor/github.com/onsi/ginkgo/reporters:go_default_library",
        "//vendor/github.com/onsi/ginkgo/reporters/stenographer:go_default_library",
        "//vendor/github.com/onsi/ginkgo/types:go_default_library",
    ],
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "aggregator_test.go",
        "fake_output_interceptor_test.go",
        "fake_poster_test.go",
        "forwarding_reporter_test.go",
        "remote_suite_test.go",
        "server_test.go",
    ],
    importpath = "github.com/onsi/ginkgo/internal/remote_test",
    deps = [
        ":go_default_library",
        "//vendor/github.com/onsi/ginkgo:go_default_library",
        "//vendor/github.com/onsi/ginkgo/config:go_default_library",
        "//vendor/github.com/onsi/ginkgo/reporters:go_default_library",
        "//vendor/github.com/onsi/ginkgo/reporters/stenographer:go_default_library",
        "//vendor/github.com/onsi/ginkgo/types:go_default_library",
        "//vendor/github.com/onsi/gomega:go_default_library",
    ],
)
