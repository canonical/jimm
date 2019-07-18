#!/bin/sh
# lxd-build-test.sh - run JIMM tests in a clean LXD environment

image=${image:-ubuntu:16.04}
container=${container:-jimm-test-`uuidgen`}
packages="build-essential bzr git make mongodb"

lxc launch -e ubuntu:16.04 $container
trap "lxc delete --force $container" EXIT

lxc exec $container -- sh -c 'while [ ! -f /var/lib/cloud/instance/boot-finished ]; do sleep 0.1; done'
lxc exec --env http_proxy=${http_proxy:-} --env no_proxy=${no_proxy:-} $container -- apt-get update -y
lxc exec --env http_proxy=${http_proxy:-} --env no_proxy=${no_proxy:-} $container -- apt-get install -y $packages
lxc exec --env http_proxy=${http_proxy:-} --env no_proxy=${no_proxy:-} $container -- snap install go --classic

lxc file push --uid 1000 --gid 1000 --mode 600 ${NETRC:-$HOME/.netrc} $container/home/ubuntu/.netrc
lxc exec --cwd /home/ubuntu/ --user 1000 --group 1000 $container -- mkdir -p /home/ubuntu/src
tar c . | lxc exec --cwd /home/ubuntu/src/ --user 1000 --group 1000 $container -- tar x
lxc exec \
	--env HOME=/home/ubuntu \
	--env http_proxy=${http_proxy:-} \
	--env no_proxy=${no_proxy:-} \
	--cwd /home/ubuntu/src/ \
	--user 1000 \
	--group 1000 \
	$container -- make check
