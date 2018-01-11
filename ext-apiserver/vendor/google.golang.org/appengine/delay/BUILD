load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "delay.go",
        "delay_go17.go",
        "delay_pre17.go",
    ],
    importpath = "google.golang.org/appengine/delay",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/google.golang.org/appengine:go_default_library",
        "//vendor/google.golang.org/appengine/log:go_default_library",
        "//vendor/google.golang.org/appengine/taskqueue:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "delay_go17_test.go",
        "delay_test.go",
    ],
    importpath = "google.golang.org/appengine/delay",
    library = ":go_default_library",
    deps = [
        "//vendor/github.com/golang/protobuf/proto:go_default_library",
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/google.golang.org/appengine/internal:go_default_library",
        "//vendor/google.golang.org/appengine/taskqueue:go_default_library",
    ],
)
