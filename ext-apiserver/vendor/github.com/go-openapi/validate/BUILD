load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "formats.go",
        "object_validator.go",
        "result.go",
        "schema.go",
        "schema_props.go",
        "slice_validator.go",
        "spec.go",
        "type.go",
        "validator.go",
        "values.go",
    ],
    importpath = "github.com/go-openapi/validate",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/go-openapi/analysis:go_default_library",
        "//vendor/github.com/go-openapi/errors:go_default_library",
        "//vendor/github.com/go-openapi/jsonpointer:go_default_library",
        "//vendor/github.com/go-openapi/loads:go_default_library",
        "//vendor/github.com/go-openapi/runtime:go_default_library",
        "//vendor/github.com/go-openapi/spec:go_default_library",
        "//vendor/github.com/go-openapi/strfmt:go_default_library",
        "//vendor/github.com/go-openapi/swag:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "items_validator_test.go",
        "parameter_validator_test.go",
        "spec_test.go",
        "type_test.go",
        "values_test.go",
    ],
    importpath = "github.com/go-openapi/validate",
    library = ":go_default_library",
    deps = [
        "//vendor/github.com/go-openapi/analysis:go_default_library",
        "//vendor/github.com/go-openapi/errors:go_default_library",
        "//vendor/github.com/go-openapi/loads:go_default_library",
        "//vendor/github.com/go-openapi/loads/fmts:go_default_library",
        "//vendor/github.com/go-openapi/spec:go_default_library",
        "//vendor/github.com/go-openapi/strfmt:go_default_library",
        "//vendor/github.com/go-openapi/swag:go_default_library",
        "//vendor/github.com/stretchr/testify/assert:go_default_library",
    ],
)
