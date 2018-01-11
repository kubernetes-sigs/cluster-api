load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["aebundler.go"],
    importpath = "google.golang.org/appengine/cmd/aebundler",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "aebundler",
    importpath = "google.golang.org/appengine/cmd/aebundler",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)
