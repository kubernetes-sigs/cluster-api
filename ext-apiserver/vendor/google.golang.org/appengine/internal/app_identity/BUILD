load("@io_bazel_rules_go//proto:def.bzl", "go_grpc_library")
load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["app_identity_service.pb.go"],
    importpath = "google.golang.org/appengine/internal/app_identity",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
    deps = ["//vendor/github.com/golang/protobuf/proto:go_default_library"],
)

proto_library(
    name = "app_identity_proto",
    srcs = ["app_identity_service.proto"],
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_grpc_library(
    name = "app_identity_go_proto",
    importpath = "google.golang.org/appengine/internal/app_identity",
    proto = ":app_identity_proto",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)
