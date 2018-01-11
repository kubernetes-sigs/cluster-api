load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "router.go",
        "server.go",
        "util.go",
    ],
    importpath = "github.com/go-openapi/runtime/middleware/denco",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "router_bench_test.go",
        "router_test.go",
        "server_test.go",
        "util_test.go",
    ],
    importpath = "github.com/go-openapi/runtime/middleware/denco_test",
    deps = [":go_default_library"],
)
