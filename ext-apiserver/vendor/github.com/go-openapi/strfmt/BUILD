load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "date.go",
        "default.go",
        "doc.go",
        "duration.go",
        "format.go",
        "time.go",
    ],
    importpath = "github.com/go-openapi/strfmt",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/asaskevich/govalidator:go_default_library",
        "//vendor/github.com/go-openapi/errors:go_default_library",
        "//vendor/github.com/mailru/easyjson/jlexer:go_default_library",
        "//vendor/github.com/mailru/easyjson/jwriter:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "date_test.go",
        "default_test.go",
        "duration_test.go",
        "format_test.go",
        "time_test.go",
    ],
    importpath = "github.com/go-openapi/strfmt",
    library = ":go_default_library",
    deps = [
        "//vendor/github.com/pborman/uuid:go_default_library",
        "//vendor/github.com/stretchr/testify/assert:go_default_library",
    ],
)
