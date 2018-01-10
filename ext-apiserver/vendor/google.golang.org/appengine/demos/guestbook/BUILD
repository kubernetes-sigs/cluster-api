load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["guestbook.go"],
    importpath = "google.golang.org/appengine/demos/guestbook",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/google.golang.org/appengine:go_default_library",
        "//vendor/google.golang.org/appengine/datastore:go_default_library",
        "//vendor/google.golang.org/appengine/log:go_default_library",
        "//vendor/google.golang.org/appengine/user:go_default_library",
    ],
)

go_binary(
    name = "guestbook",
    importpath = "google.golang.org/appengine/demos/guestbook",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)
