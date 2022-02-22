
# E2e log sync

## Prerequisites

Start the Tilt development environment via `tilt up`.

*Notes*:
* If you only want to see imported logs, disable promtail.
* If you want to drop all logs from Loki, just delete the Loki Pod in the `observability` namespace.

## Import logs

Imports logs into Loki:
```bash
go run ./hack/tools/e2e-log-sync --log-bucket=kubernetes-jenkins --log-controller-folder=pr-logs/pull/kubernetes-sigs_cluster-api/6150/pull-cluster-api-e2e-main/1496099075710259200/artifacts/clusters/bootstrap/controllers
```

## View logs

Now the logs are available:
* via Grafana: `http://localhost:3001/explore`
* via the Loki `logcli`:
  ```bash
  logcli query '{app="capd-controller-manager"}' --timezone=UTC --from="2022-02-22T10:00:00Z"
  ```

## Caveats

* Make sure you query the correct time range.
* The logs are currently uploaded with now as timestamp, because otherwise it would 
  take a few minutes until the logs show up in Loki.
