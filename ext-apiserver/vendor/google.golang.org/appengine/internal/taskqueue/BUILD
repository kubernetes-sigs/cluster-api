load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")
load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["taskqueue_service.pb.go"],
    importpath = "google.golang.org/appengine/internal/taskqueue",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
    deps = [
        "//vendor/github.com/golang/protobuf/proto:go_default_library",
        "//vendor/google.golang.org/appengine/internal/datastore:go_default_library",
    ],
)

proto_library(
    name = "taskqueue_proto",
    srcs = ["taskqueue_service.proto"],
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
    deps = ["//google.golang.org/appengine/internal/datastore:datastore_proto"],
)

go_proto_library(
    name = "taskqueue_go_proto",
    importpath = "google.golang.org/appengine/internal/taskqueue",
    proto = ":taskqueue_proto",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
    deps = ["//google.golang.org/appengine/internal/datastore:go_default_library"],
)
