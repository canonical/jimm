# Grafana dashboard definitions

In this directory you can put dashboard template files. Those files are provided
automatically to Grafana via the `grafana_dashboard` relation.

## How to build a Grafana dashboard?

The dashboard template file can be created by manually building a Grafana
dashboard using the Grafana web UI, then exporting it to a JSON file and
updating it by changing the "templating" section so that it only includes an
empty list:
```
  "templating": {
    "list": []
  },
```