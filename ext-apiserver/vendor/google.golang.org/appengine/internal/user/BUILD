load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")
load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["user_service.pb.go"],
    importpath = "google.golang.org/appengine/internal/user",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
    deps = ["//vendor/github.com/golang/protobuf/proto:go_default_library"],
)

proto_library(
    name = "user_proto",
    srcs = ["user_service.proto"],
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_proto_library(
    name = "user_go_proto",
    importpath = "google.golang.org/appengine/internal/user",
    proto = ":user_proto",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)
