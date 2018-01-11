load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["memcache.go"],
    importpath = "google.golang.org/appengine/memcache",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/golang/protobuf/proto:go_default_library",
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/google.golang.org/appengine:go_default_library",
        "//vendor/google.golang.org/appengine/internal:go_default_library",
        "//vendor/google.golang.org/appengine/internal/memcache:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = ["memcache_test.go"],
    importpath = "google.golang.org/appengine/memcache",
    library = ":go_default_library",
    deps = [
        "//vendor/google.golang.org/appengine:go_default_library",
        "//vendor/google.golang.org/appengine/internal/aetesting:go_default_library",
        "//vendor/google.golang.org/appengine/internal/memcache:go_default_library",
    ],
)
