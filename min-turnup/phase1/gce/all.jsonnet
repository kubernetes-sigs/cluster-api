local cfg = import "../../.config.json";
{
  "gce.tf": (import "lib/gce.jsonnet")(cfg),
}
