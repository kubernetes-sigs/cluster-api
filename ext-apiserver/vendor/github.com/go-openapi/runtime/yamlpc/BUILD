load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["yaml.go"],
    importpath = "github.com/go-openapi/runtime/yamlpc",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/go-openapi/runtime:go_default_library",
        "//vendor/gopkg.in/yaml.v2:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = ["yaml_test.go"],
    importpath = "github.com/go-openapi/runtime/yamlpc",
    library = ":go_default_library",
    deps = ["//vendor/github.com/stretchr/testify/assert:go_default_library"],
)
