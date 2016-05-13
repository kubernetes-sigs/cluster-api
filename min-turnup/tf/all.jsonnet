local cfg = import "cfg.jsonnet";
{
  "gce.tf": (import "lib/gce.jsonnet")(cfg),
}
