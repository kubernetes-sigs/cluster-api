load("@io_bazel_rules_go//proto:def.bzl", "go_grpc_library")
load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["capability_service.pb.go"],
    importpath = "google.golang.org/appengine/internal/capability",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
    deps = ["//vendor/github.com/golang/protobuf/proto:go_default_library"],
)

proto_library(
    name = "capability_proto",
    srcs = ["capability_service.proto"],
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_grpc_library(
    name = "capability_go_proto",
    importpath = "google.golang.org/appengine/internal/capability",
    proto = ":capability_proto",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)
