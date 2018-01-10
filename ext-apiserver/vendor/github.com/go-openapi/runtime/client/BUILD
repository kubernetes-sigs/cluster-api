load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "auth_info.go",
        "request.go",
        "response.go",
        "runtime.go",
    ],
    importpath = "github.com/go-openapi/runtime/client",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/go-openapi/runtime:go_default_library",
        "//vendor/github.com/go-openapi/strfmt:go_default_library",
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/golang.org/x/net/context/ctxhttp:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "auth_info_test.go",
        "request_test.go",
        "response_test.go",
        "runtime_test.go",
    ],
    importpath = "github.com/go-openapi/runtime/client",
    library = ":go_default_library",
    deps = [
        "//vendor/github.com/go-openapi/runtime:go_default_library",
        "//vendor/github.com/go-openapi/strfmt:go_default_library",
        "//vendor/github.com/stretchr/testify/assert:go_default_library",
    ],
)
