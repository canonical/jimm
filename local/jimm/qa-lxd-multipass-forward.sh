#!/bin/bash

# This script forwards JIMM and Keycloaks ports to localhost from the QA VM.
#
# It can be used in a number of ways, as follows:
# ./qa-lxd-multipass-forward.sh jimm2 --linux --wait  - Run for a VM named jimm2, on linux, and wait after tunnels are established
# ./qa-lxd-multipass-forward.sh jimm2 --linux         - Run for a VM named jimm2, on linux, and exit immediately
# ./qa-lxd-multipass-forward.sh --linux               - Run for the default VM (jimm), on linux, and exit immediately
# ./qa-lxd-multipass-forward.sh jimm2 --mac --wait    - Run for a VM named jimm2, on mac, and wait after tunnels are established

KEY_PATH_LINUX="/var/snap/multipass/common/data/multipassd/ssh-keys/id_rsa"
KEY_PATH_MAC="/var/root/Library/Application Support/multipassd/ssh-keys/id_rsa"
VM_NAME="${VM_NAME:-jimm}"
OS=""
WAIT=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --linux)
      OS="linux"
      shift
      ;;
    --mac)
      OS="mac"
      shift
      ;;
    --wait)
      WAIT=true
      shift
      ;;
    *)
      # Assume positional argument VM_NAME or unknown
      VM_NAME="$1"
      shift
      ;;
  esac
done

if [[ "$OS" == "" ]]; then
  case "$(uname -s)" in
    Darwin)
      OS="mac"
      ;;
    *)
      OS="linux"
      ;;
  esac

  echo "Detected running on $OS"
fi

# Set the key path based on OS
if [[ "$OS" == "mac" ]]; then
  KEY_PATH="$KEY_PATH_MAC"
elif [[ "$OS" == "linux" ]]; then
  KEY_PATH="$KEY_PATH_LINUX"
else
  echo "Error: You must specify --linux or --mac"
  exit 1
fi

echo "Copying ssh key to local"
KEY_COPIED_PATH="../../local/vm/id_rsa"
if [ ! -f "$KEY_COPIED_PATH" ]; then
    mkdir -p $(dirname "$KEY_COPIED_PATH")
    sudo cp "$KEY_PATH" "$KEY_COPIED_PATH"
    sudo chown $USER:$USER "$KEY_COPIED_PATH"
    chmod 600 "$KEY_COPIED_PATH"
fi

echo "Retrieving VM address"
VM_ADDR=$(multipass info "$VM_NAME" --format json | jq -r ".info.\"$VM_NAME\".ipv4 | .[0]")

echo "OS detected: $OS"
echo "Using SSH key: $KEY_PATH"
echo "VM Name: $VM_NAME"
echo "VM Address: $VM_ADDR"

# Test SSH connection first (non-blocking, just exits if failure, sudo to prompt user)
sudo ssh -i "$KEY_COPIED_PATH" -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=accept-new ubuntu@$VM_ADDR "exit"
if [ $? -ne 0 ]; then
  echo "SSH connection test failed. Exiting."
  exit 1
fi

# Connect keycloak
sudo ssh -i "$KEY_COPIED_PATH" -N -L 8082:localhost:8082 -L 443:localhost:443 ubuntu@$VM_ADDR > /dev/null 2>&1 &
SSH_PID=$!
echo "SSH PID: $SSH_PID"
echo "JIMM is available at https://jimm.localhost"
echo "Keycloak is available at http://keycloak.localhost:8082"

cleanup() {
    echo "Cleaning up SSH tunnels..."
    kill $SSH_PID 2>/dev/null
}

trap cleanup EXIT

if $WAIT; then
    echo "Waiting for tunnels..."
    wait $SSH_PID
fi
