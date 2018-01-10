load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")
load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "protobuf.go",
        "simple_message.pb.go",
    ],
    importpath = "github.com/onsi/gomega/ghttp/protobuf",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/golang/protobuf/proto:go_default_library"],
)

proto_library(
    name = "protobuf_proto",
    srcs = ["simple_message.proto"],
    visibility = ["//visibility:public"],
)

go_proto_library(
    name = "protobuf_go_proto",
    importpath = "github.com/onsi/gomega/ghttp/protobuf",
    proto = ":protobuf_proto",
    visibility = ["//visibility:public"],
)
