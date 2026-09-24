---
myst:
  html_meta:
    description: "Enable Prometheus and Loki alert rules for JAAS/JIMM by integrating with the Canonical Observability Stack (COS)."
---

(enable-alert-rules)=
# Enable alert rules

This guide walks through enabling Prometheus and Loki alert rules for your JAAS deployment.

JIMM ships a set of alert rules that are provisioned automatically to Prometheus through the same relation used for monitoring. The rules travel over the `metrics-endpoint` relation, so no integration is required beyond the ones described in {ref}`Enable monitoring <enable-monitoring>`.

The current alert rules cover:

| Alert                     | Fires when                                                      | Severity |
| ------------------------- | --------------------------------------------------------------- | -------- |
| `JimmDown`                | JIMM's `/metrics` endpoint has been unreachable for 5 minutes   | Critical |
| `JimmHighAuthFailureRate` | Authentication failures exceed ~12 per minute for 5 minutes     | High     |
| `JimmJujuApiErrors`       | JIMM experiences sustained errors calling the Juju API          | Warning  |
| `JimmSlowDatabaseQueries` | The p95 database query latency is above 1 second for 10 minutes | Warning  |
| `JimmVaultErrors`         | JIMM experiences sustained errors calling Vault                 | Warning  |
| `JimmOpenFGAErrors`       | JIMM experiences sustained errors calling OpenFGA               | Warning  |

For the up-to-date list of rules and their exact expressions, see the [alert rules in the JIMM charm source](https://github.com/canonical/jimm-k8s-operator/tree/main/src/prometheus_alert_rules).

## Prerequisites

- A running JAAS deployment (see {ref}`Manage your JAAS deployment <manage-your-jaas-deployment>`)
- A deployed COS stack, integrated with JIMM as described in {ref}`Enable monitoring <enable-monitoring>`

## Integrate with COS

The alert rules are transferred over the monitoring relations:

| JIMM endpoint      | Interface           | Alert rules                                      |
| ------------------ | ------------------- | ------------------------------------------------ |
| `metrics-endpoint` | `prometheus_scrape` | Prometheus alert rules based on `jimm_*` metrics |

Relate the endpoint to the corresponding application in your COS stack — the same relation described in {ref}`Enable monitoring <enable-monitoring>`:

```text
juju relate jimm:metrics-endpoint prometheus
```

To be notified when an alert fires, also integrate the Prometheus application of your COS stack with Alertmanager.

## Verify

To check which alert rules are loaded in Prometheus, open the Prometheus web UI of your COS deployment and check **Status > Rules**. Rules sourced from each related charm appear as groups named `<model>_<model-uuid>_<application>_<rule-group>`; the JIMM groups are named after the alerts listed above.

Firing alerts are visible in the Alertmanager UI of your COS deployment, and in the **Alertmanager Operator Overview** dashboard in Grafana.

```{note}
Alert rules based on counters (for example, authentication failures or Juju API errors) only evaluate once the corresponding time series exist. On an idle deployment most JIMM alerts would remain in an inactive state until JIMM starts handling traffic.
```
