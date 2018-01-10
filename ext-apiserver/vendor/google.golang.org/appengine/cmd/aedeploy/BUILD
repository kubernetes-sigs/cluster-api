load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["aedeploy.go"],
    importpath = "google.golang.org/appengine/cmd/aedeploy",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "aedeploy",
    importpath = "google.golang.org/appengine/cmd/aedeploy",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)
