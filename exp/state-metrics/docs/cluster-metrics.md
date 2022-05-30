# Cluster Metrics

| Metric name                   | Metric type | Additional Labels/tags                                                                                                                                                                 |
|-------------------------------|-------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| capi_cluster_created          | Gauge       | `cluster`=&lt;cluster-name&gt; <br> `namespace`=&lt;cluster-namespace&gt; <br> `uid`=&lt;uid&gt;                                                                                       |
| capi_cluster_labels           | Gauge       | `cluster`=&lt;cluster-name&gt; <br> `namespace`=&lt;cluster-namespace&gt; <br> `uid`=&lt;uid&gt; <br> `label_CLUSTER_LABEL`=&lt;CLUSTER_LABEL&gt;                                      |
| capi_cluster_paused           | Gauge       | `cluster`=&lt;cluster-name&gt; <br> `namespace`=&lt;cluster-namespace&gt; <br> `uid`=&lt;uid&gt;                                                                                       |
| capi_cluster_status_condition | Gauge       | `cluster`=&lt;cluster-name&gt; <br> `namespace`=&lt;cluster-namespace&gt; <br> `uid`=&lt;uid&gt; <br> `condition`=&lt;cluster-condition&gt; <br> `status`=&lt;true\|false\|unknown&gt; |
| capi_cluster_status_phase     | Gauge       | `cluster`=&lt;cluster-name&gt; <br> `namespace`=&lt;cluster-namespace&gt; <br> `uid`=&lt;uid&gt; <br> `phase`=&lt;Deleting\|Failed\|Pending\|Provisioned\|Provisioning\|Unknown&gt;    |
