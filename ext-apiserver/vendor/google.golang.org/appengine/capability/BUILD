load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["capability.go"],
    importpath = "google.golang.org/appengine/capability",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/google.golang.org/appengine/internal:go_default_library",
        "//vendor/google.golang.org/appengine/internal/capability:go_default_library",
        "//vendor/google.golang.org/appengine/log:go_default_library",
    ],
)
