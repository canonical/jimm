---
myst:
  html_meta:
    description: "JIMM-specific Prometheus metrics, including names, types, labels and descriptions."
---

(jimm-metrics)=
# JIMM metrics

JIMM exposes Prometheus metrics at `/metrics`.
This reference lists JIMM-specific metrics declared in
`internal/servermon/monitoring.go`; Go runtime, process and HTTP handler
instrumentation metrics are not included.

Labelled metrics appear only after the corresponding label values are used.
Histogram names below are metric families: Prometheus exposes their
`_bucket` (with an additional `le` label), `_sum` and `_count` series.

| Metric | Type | Labels | Description |
| ------ | ---- | ------ | ----------- |
| `jimm_auth_failure_total` | counter | `method` | The number of failed authentications. |
| `jimm_auth_success_total` | counter | `method` | The number of successful authentications. |
| `jimm_db_error_total` | counter | `method` | The number of database errors. |
| `jimm_db_query_duration_seconds` | histogram | `method` | Histogram of database query time in seconds |
| `jimm_http_request_duration_seconds` | histogram | `route`, `method`, `status_code` | The duration of handling an HTTP request in seconds. |
| `jimm_jimm_op_duration_seconds` | histogram | `method` | Histogram of jimm operations time in seconds |
| `jimm_juju_call_duration_seconds` | histogram | `facade`, `method`, `controller` | Histogram of juju call time in seconds |
| `jimm_juju_error_total` | counter | `facade`, `method`, `controller` | The number of juju call errors. |
| `jimm_juju_ping_duration_seconds` | histogram | `controller` | Histogram of juju ping time in seconds |
| `jimm_monitor_errors_total` | counter | `controller` | The number of monitoring errors found. |
| `jimm_openfga_call_duration_seconds` | histogram | `method` | Histogram of openfga call time in seconds |
| `jimm_openfga_error_total` | counter | `method` | The number of openfga call errors. |
| `jimm_system_controller` | gauge | - | The number of controllers managed by JIMM. |
| `jimm_system_model` | gauge | `controller` | The number of models managed per controller attached to JIMM. |
| `jimm_vault_call_duration_seconds` | histogram | `method` | Histogram of vault call time in seconds |
| `jimm_vault_error_total` | counter | `method` | The number of vault call errors. |
| `jimm_websocket_concurrent_connections` | gauge | - | The number of concurrent websocket connections |
| `jimm_websocket_models_created_fail_total` | counter | - | The number of fails attempting to create models. |
| `jimm_websocket_models_created_total` | counter | - | The number of models created. |
| `jimm_websocket_request_duration_seconds` | histogram | `type`, `action` | The duration of a websocket request in seconds. |
