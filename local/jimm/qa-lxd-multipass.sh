#!/bin/bash

# This script sets up a JIMM service within docker compose in multipass and adds an LXD controller.
# It uses the default branch of jimm for the deployment.
#
# It requires the following tools:
# - multipass
# - go
# - jq

VM_NAME="${1:-jimm}"

echo "Setting up VM $VM_NAME."
vm_exists=$(multipass list --format json | jq -r ".list[] | select(.name == \"$VM_NAME\") | .name")
if [ -n "$vm_exists" ]; then
  echo "Please delete $VM_NAME and try again."
  exit 1
fi

# Cleaning ./tmp. Permission issues can arise when mounting ./tmp.
sudo rm -rf ../../tmp

echo "VM does not exist, launching VM: $VM_NAME"
multipass launch --cpus 4 docker -n $VM_NAME

# Multipass detects the vm name is the same as parent working path dir and creates a fuse mount.
# We want a classic mount to reflect changes on the host to the VM.
echo "Setting up classic mount"
mount_name="$VM_NAME:jimm"
multipass umount $mount_name|| true
multipass mount --type=classic ../../ $mount_name || true

echo "Installing & setting up dependencies"
multipass exec $VM_NAME -- bash <<- 'EOF'
	sudo snap install juju
	sudo snap install go --classic
	sudo sudo apt-get -y install make
	sudo lxd init --auto
EOF

echo "Setting up JIMM"
multipass exec --working-directory /home/ubuntu/jimm $VM_NAME -- bash <<- 'EOF'
    make certs

    # Re-copy and update certs (workaround to keep the same generated certs but simply update the VM's certs only)
    sudo cp local/traefik/certs/ca.crt /usr/local/share/ca-certificates
    sudo update-ca-certificates

    make version/commit.txt
    make version/version.txt

    # TODO(ale8k): Have docker cache images somewhere that can be shared, the compose takes forever otherwise.
    docker compose --profile dev up --wait -d
EOF

echo "Building JAAS CLI"
GOOS="linux" go build ./cmd/jaas

echo "Setting up forwarding for keycloak login"

# Run initially to setup connection (may require user interaction for sudo)
./qa-lxd-multipass-forward.sh "$VM_NAME"

# Run again to background (shouldn't require user interaction for sudo)
./qa-lxd-multipass-forward.sh "$VM_NAME" --wait &
SSH_FORWARD_PID=$!

# Kill SSH forwarding on process exit
cleanup_ssh_forward() {
  kill $SSH_FORWARD_PID 2>/dev/null
}
trap cleanup_ssh_forward EXIT

echo
echo
echo
echo

echo "To continue the QA environment setup, please login to keycloak."
echo "The username is: \"jimm-test\" and the password is: \"password\""
multipass exec --working-directory /home/ubuntu/jimm $VM_NAME -- juju login jimm.localhost -c jimm-dev

cleanup_ssh_forward

echo
echo "Setting up LXD controller"

multipass exec --working-directory /home/ubuntu/jimm $VM_NAME -- bash <<- 'EOF'
	sudo iptables -I FORWARD -i lxdbr0 -j ACCEPT
	sudo iptables -I FORWARD -o lxdbr0 -j ACCEPT
	./local/jimm/setup-controller.sh
	./local/jimm/add-controller.sh
EOF
