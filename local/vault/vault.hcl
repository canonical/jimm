
storage "postgresql" {
  connection_url = "postgres://jimm:jimm@db:5432/jimm"
}

# Reachable here: http://localhost:8200/ui/
ui = true

# We handle this via server flags, see init.sh.
// api_addr = "0.0.0.0:8200"
// listener "tcp" {
//     address     = "0.0.0.0:8200"
//     tls_disable = true
// }
