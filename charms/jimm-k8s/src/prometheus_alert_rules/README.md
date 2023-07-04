# Prometheus alerting rules

Alert rules are automatically gathered by `MetricsEndpointProvider`,
as used in this charm, from this directory.

Each file in this directory is assumed to be in one of two formats:
- the official prometheus alert rule format, conforming to the
  [Prometheus docs](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/)
- a single rule format, which is a simplified subset of the official format,
  comprising a single alert rule per file, using the same YAML fields.

The file name must have one of the following extensions:
- `.rule`
- `.rules`
- `.yml`
- `.yaml`

An example of the contents of such a file in the custom single rule
format is shown below:

```
alert: HighRequestLatency
expr: job:request_latency_seconds:mean5m{my_key=my_value} > 0.5
for: 10m
labels:
  severity: Medium
  type: HighLatency
annotations:
  summary: High request latency for {{ $labels.instance }}.
```