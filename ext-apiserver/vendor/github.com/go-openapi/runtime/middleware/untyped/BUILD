load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["api.go"],
    importpath = "github.com/go-openapi/runtime/middleware/untyped",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/go-openapi/analysis:go_default_library",
        "//vendor/github.com/go-openapi/errors:go_default_library",
        "//vendor/github.com/go-openapi/loads:go_default_library",
        "//vendor/github.com/go-openapi/runtime:go_default_library",
        "//vendor/github.com/go-openapi/spec:go_default_library",
        "//vendor/github.com/go-openapi/strfmt:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = ["api_test.go"],
    importpath = "github.com/go-openapi/runtime/middleware/untyped",
    library = ":go_default_library",
    deps = [
        "//vendor/github.com/go-openapi/analysis:go_default_library",
        "//vendor/github.com/go-openapi/errors:go_default_library",
        "//vendor/github.com/go-openapi/loads:go_default_library",
        "//vendor/github.com/go-openapi/runtime:go_default_library",
        "//vendor/github.com/go-openapi/spec:go_default_library",
        "//vendor/github.com/stretchr/testify/assert:go_default_library",
    ],
)
