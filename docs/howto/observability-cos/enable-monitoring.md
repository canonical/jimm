---
myst:
  html_meta:
    description: "Enable Grafana dashboards for JAAS/JIMM by integrating with the Canonical Observability Stack (COS)."
---

(enable-monitoring)=
# Enable monitoring

This guide walks through enabling Grafana dashboards for your JAAS deployment.

JAAS ships three Grafana dashboards — **JAAS Metrics**, **JAAS Logs**, and **JAAS Traces** — which are provisioned automatically once JIMM is integrated with a Grafana instance. Metrics are collected by Prometheus, logs are collected by Loki, and traces are collected by Tempo. The monitoring dashboards are described below; for the tracing dashboard see {ref}`Enable tracing <enable-tracing>`.

## Prerequisites

- A running JAAS deployment (see {ref}`Manage your JAAS deployment <manage-your-jaas-deployment>`)
- A deployed COS stack. How you deploy and expose COS is up to you — see the [COS documentation](https://documentation.ubuntu.com/observability/) for deployment options.

## Integrate with COS

JIMM exposes three COS integration endpoints:

| JIMM endpoint       | Interface           | Provides                               |
| ------------------- | ------------------- | -------------------------------------- |
| `grafana-dashboard` | `grafana_dashboard` | The JAAS dashboards                    |
| `metrics-endpoint`  | `prometheus_scrape` | Scraping of JIMM's `/metrics` endpoint |
| `logging`           | `loki_push_api`     | Forwarding of JIMM's workload logs     |

Relate each endpoint to the corresponding application in your COS stack. For example, with a COS stack deployed in the same model as JIMM:

```text
juju relate jimm:grafana-dashboard grafana
juju relate jimm:metrics-endpoint prometheus
juju relate jimm:logging loki
```

```{note}
If your COS stack lives in a different model, deploy a `grafana-agent` subordinate next to JIMM and relate it to JIMM's `cos-agent` endpoint instead. The agent forwards dashboards, metrics and logs to the remote COS applications.
```

Once the relations settle, Grafana automatically provisions the JAAS dashboards — no manual import is needed.

## Verify

Open the Grafana UI of your COS deployment and log in. In the Grafana UI, open **Dashboards**. You should see the JAAS dashboards listed alongside the operator overview dashboards:

| Dashboard    | Description                                       |
| ------------ | ------------------------------------------------- |
| JAAS Metrics | JIMM operational metrics collected via Prometheus |
| JAAS Logs    | JIMM workload logs collected via Loki             |

### What to expect in the JAAS Metrics dashboard

The dashboard is organised in rows, each covering an area of JIMM:

| Row              | Panels                                                                                                                  | Data source                                         |
| ---------------- | ----------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------- |
| JIMM             | Active connections, managed controllers, managed models, Go runtime statistics (memory, goroutines, garbage collection) | `jimm_websocket_*`, `jimm_system_*`, `go_*` metrics |
| Juju Controllers | Ping duration, call durations and rates, error rate per controller and method                                           | `jimm_juju_*` metrics                               |
| Auth             | Authentication failure rate per method                                                                                  | `jimm_auth_*` metrics                               |
| Vault            | Vault call durations and rates                                                                                          | `jimm_vault_*` metrics                              |
| OpenFGA          | OpenFGA call durations and rates                                                                                        | `jimm_openfga_*` metrics                            |
| Database         | Database query durations and rates                                                                                      | `jimm_db_*` metrics                                 |

For the up-to-date definition of the dashboards, see the [Grafana dashboards in the JIMM charm source](https://github.com/canonical/jimm-k8s-operator/tree/main/src/grafana_dashboards).

```{note}
Prometheus only creates a time series the first time a counter is incremented. On an idle deployment — no connected controllers, no user activity — most panels will show no data. This is expected: the panels populate as JIMM handles authentication attempts, Juju API calls, and so on. The Go runtime and database panels show data as soon as JIMM is running.
```

### What to expect in the JAAS Logs dashboard

The dashboard shows JIMM's workload logs with:

- A **log volume histogram** broken down by severity level (`debug`, `info`, `warn`, `error`)
- A **log stream** panel with the parsed log fields (level, caller, message)

Use the **level** dropdown to filter by severity and the **search** box for full-text, case-insensitive filtering.

## Troubleshooting

- **All panels show no data and the variable dropdowns are empty**: check that Grafana can reach the Prometheus and Loki datasources. Their URLs are derived from the ingress, so a broken or misconfigured ingress (for example, a TLS certificate that does not cover the datasource hostnames) makes the topology variables fail silently.
- **The JAAS Metrics dashboard shows data only in the database and Go runtime panels**: this is expected on an idle deployment — see the note above.
