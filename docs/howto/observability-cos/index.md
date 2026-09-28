---
myst:
  html_meta:
    description: "Observability guides for JAAS/JIMM using the Canonical Observability Stack (COS), including monitoring with Grafana, alert rules with Prometheus and Loki, and tracing with Tempo."
---

(observability-cos)=
# Observability (COS)

The Canonical Observability Stack (COS) can be integrated with JAAS to facilitate collection and visualization of telemetry data.

See also: [Observability documentation | What is COS?](https://documentation.ubuntu.com/observability/latest/explanation/overview/what-is-cos/)

## Guides

JAAS supports the following COS applications:

| Function                                | Application      |
| --------------------------------------- | ---------------- |
| {ref}`Monitoring <enable-monitoring>`   | Grafana          |
| {ref}`Alert rules <enable-alert-rules>` | Prometheus, Loki |
| {ref}`Tracing <enable-tracing>`         | Tempo            |

```{toctree}
:maxdepth: 1
:hidden:

Enable monitoring <enable-monitoring>
Enable alert rules <enable-alert-rules>
Enable tracing <enable-tracing>
```
