---
myst:
  html_meta:
    description: "Enable distributed tracing for JAAS/JIMM by integrating with Tempo through the Canonical Observability Stack (COS)."
---

(enable-tracing)=
# Enable tracing

This guide walks through enabling distributed tracing for your JAAS deployment.

JAAS ships a **JAAS Traces** Grafana dashboard that lists recent JIMM traces and lets you open any trace to inspect its span waterfall. Traces are collected by Tempo, so the dashboard is provisioned once JIMM is integrated with a Tempo instance.

## What is traced

JIMM instruments the request path end to end. A single Juju API call typically produces a trace with a `jimm.facade` root span and nested child spans:

| Span name                  | Created for                                                                                   |
| -------------------------- | --------------------------------------------------------------------------------------------- |
| `jimm.facade`              | Every Juju API facade call received over the Juju websocket RPC connection (root span)        |
| `jimm.db`                  | Every database operation (create, query, update, delete, row, raw)                            |
| `jimm.openfga`             | Every OpenFGA authorization check (with an `openfga.operation` attribute)                     |
| `jimm.juju-dial`           | Establishing a connection to a managed Juju controller                                        |
| `jimm.juju-websocket-dial` | Opening the websocket to a managed Juju controller                                            |
| `jimm.juju-rpc`            | RPC calls forwarded to a managed Juju controller (with facade, version and method attributes) |
| `jimm.model-proxy`         | RPC messages proxied through to a managed model (with trace propagation to the controller)    |

This means traces appear for Juju API activity — `juju` CLI commands against the JAAS controller, dashboard sessions issuing Juju API calls, and any Juju client interaction — but not for plain HTTP requests such as the OAuth login redirect.

```{note}
On an idle deployment no traces are produced. Traces appear as soon as JIMM handles Juju API calls.
```

## Prerequisites

- A running JAAS deployment (see {ref}`Manage your JAAS deployment <manage-your-jaas-deployment>`)
- A deployed COS stack including Tempo (coordinator and worker) with a configured storage backend. See the [COS documentation](https://documentation.ubuntu.com/observability/) for deployment options.

## Integrate with COS

JIMM exposes a tracing integration endpoint:

| JIMM endpoint | Interface | Provides                                                 |
| ------------- | --------- | -------------------------------------------------------- |
| `tracing`     | `tracing` | OTLP trace export endpoint and the JAAS Traces dashboard |

Relate the endpoint to the Tempo coordinator in your COS stack. For example, with COS deployed in the same model as JIMM:

```text
juju relate jimm:tracing tempo-coordinator
```

Once the relation settles:

- JIMM receives the OTLP endpoint over the relation and starts exporting traces to Tempo.
- Grafana receives the Tempo datasource and the **JAAS Traces** dashboard.

```{note}
If your COS stack lives in a different model, deploy a `grafana-agent` subordinate next to JIMM and relate it to JIMM's `cos-agent` endpoint instead. The agent forwards traces to the remote Tempo.
```

## Sampling

JIMM samples traces according to the `tracing-sample-ratio` charm configuration, which defaults to `0.1` (10% of traces). To capture every trace — for example while testing or debugging — set the ratio to `1.0`:

```text
juju config jimm tracing-sample-ratio=1.0
```

## Verify

Open the Grafana UI of your COS deployment and log in. In the Grafana UI, open **Dashboards** and select **JAAS Traces**. The dashboard lists the most recent JIMM traces:

| Column     | Content                          |
| ---------- | -------------------------------- |
| Trace ID   | The trace identifier (clickable) |
| Start time | When the trace started           |
| Name       | Service and root span name       |
| Duration   | Total trace duration             |

Click a **Trace ID** to open the trace in Grafana Explore, where the full span waterfall is rendered.

If the list is empty, generate some Juju API activity (for example, run a `juju` command against the JAAS controller, or use the JAAS dashboard to interact with a model) and refresh the dashboard.

## Troubleshooting

- **The dashboard is missing**: check that the `tracing` relation is active (`juju status`) and that Grafana received the Tempo datasource.
- **The dashboard is present but the trace list is empty**: confirm there has been Juju API activity, that `tracing-sample-ratio` is not `0`, and that the time range (top right of the dashboard) covers the activity.
- **Traces are missing spans**: only Juju API facade calls and database operations are traced — see [What is traced](#what-is-traced). Plain HTTP requests do not produce traces.

For the up-to-date definition of the dashboard, see the [Grafana dashboards in the JIMM charm source](https://github.com/canonical/jimm-k8s-operator/tree/main/src/grafana_dashboards).
