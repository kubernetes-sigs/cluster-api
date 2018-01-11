load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")
load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["urlfetch_service.pb.go"],
    importpath = "google.golang.org/appengine/internal/urlfetch",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
    deps = ["//vendor/github.com/golang/protobuf/proto:go_default_library"],
)

proto_library(
    name = "urlfetch_proto",
    srcs = ["urlfetch_service.proto"],
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_proto_library(
    name = "urlfetch_go_proto",
    importpath = "google.golang.org/appengine/internal/urlfetch",
    proto = ":urlfetch_proto",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)
