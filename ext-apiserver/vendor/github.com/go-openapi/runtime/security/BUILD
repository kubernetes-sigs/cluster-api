load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["authenticator.go"],
    importpath = "github.com/go-openapi/runtime/security",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/go-openapi/errors:go_default_library",
        "//vendor/github.com/go-openapi/runtime:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "apikey_auth_test.go",
        "basic_auth_test.go",
        "bearer_auth_test.go",
    ],
    importpath = "github.com/go-openapi/runtime/security",
    library = ":go_default_library",
    deps = [
        "//vendor/github.com/go-openapi/errors:go_default_library",
        "//vendor/github.com/stretchr/testify/assert:go_default_library",
    ],
)
