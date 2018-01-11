load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "ae.go",
        "fix.go",
        "main.go",
        "typecheck.go",
    ],
    importpath = "google.golang.org/appengine/cmd/aefix",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "aefix",
    importpath = "google.golang.org/appengine/cmd/aefix",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = [
        "ae_test.go",
        "main_test.go",
    ],
    importpath = "google.golang.org/appengine/cmd/aefix",
    library = ":go_default_library",
)
