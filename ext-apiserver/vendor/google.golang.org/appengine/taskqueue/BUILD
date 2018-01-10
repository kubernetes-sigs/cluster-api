load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["taskqueue.go"],
    importpath = "google.golang.org/appengine/taskqueue",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/golang/protobuf/proto:go_default_library",
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/google.golang.org/appengine:go_default_library",
        "//vendor/google.golang.org/appengine/internal:go_default_library",
        "//vendor/google.golang.org/appengine/internal/datastore:go_default_library",
        "//vendor/google.golang.org/appengine/internal/taskqueue:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = ["taskqueue_test.go"],
    importpath = "google.golang.org/appengine/taskqueue",
    library = ":go_default_library",
    deps = [
        "//vendor/google.golang.org/appengine:go_default_library",
        "//vendor/google.golang.org/appengine/internal:go_default_library",
        "//vendor/google.golang.org/appengine/internal/aetesting:go_default_library",
        "//vendor/google.golang.org/appengine/internal/taskqueue:go_default_library",
    ],
)
