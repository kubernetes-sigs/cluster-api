load("@io_bazel_rules_go//proto:def.bzl", "go_grpc_library")
load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["remote_api.pb.go"],
    importpath = "google.golang.org/appengine/internal/remote_api",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
    deps = ["//vendor/github.com/golang/protobuf/proto:go_default_library"],
)

proto_library(
    name = "remote_api_proto",
    srcs = ["remote_api.proto"],
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_grpc_library(
    name = "remote_api_go_proto",
    importpath = "google.golang.org/appengine/internal/remote_api",
    proto = ":remote_api_proto",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)
