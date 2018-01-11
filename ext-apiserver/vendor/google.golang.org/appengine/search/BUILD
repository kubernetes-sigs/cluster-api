load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "field.go",
        "search.go",
        "struct.go",
    ],
    importpath = "google.golang.org/appengine/search",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/golang/protobuf/proto:go_default_library",
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/google.golang.org/appengine:go_default_library",
        "//vendor/google.golang.org/appengine/internal:go_default_library",
        "//vendor/google.golang.org/appengine/internal/search:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "search_test.go",
        "struct_test.go",
    ],
    importpath = "google.golang.org/appengine/search",
    library = ":go_default_library",
    deps = [
        "//vendor/github.com/golang/protobuf/proto:go_default_library",
        "//vendor/google.golang.org/appengine:go_default_library",
        "//vendor/google.golang.org/appengine/internal/aetesting:go_default_library",
        "//vendor/google.golang.org/appengine/internal/search:go_default_library",
    ],
)
