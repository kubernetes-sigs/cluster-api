load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "interrupt_handler.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:darwin_amd64": [
            "sigquit_swallower_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_amd64": [
            "sigquit_swallower_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:windows_amd64": [
            "sigquit_swallower_windows.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "github.com/onsi/ginkgo/ginkgo/interrupthandler",
    visibility = ["//visibility:public"],
)
