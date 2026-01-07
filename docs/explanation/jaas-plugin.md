(jaas-plugin)=
# `jaas` plugin

The `jaas` plugin is a CLI tool that acts as a plugin for the `juju` CLI and
provides extra functionality in a JAAS system.


This document explains how to install the `jaas` plugin and how it works.

## Installation

The `jaas` plugin is distributed as a [Snap](https://snapcraft.io/jaas).

```text
sudo snap install jaas --channel=3/stable
```

## How it works

When you install both the Juju and JAAS snaps, they automatically connect via snap's
[content-interface](https://snapcraft.io/docs/content-interface) enabling new commands on the `juju` CLI.

To view a list of all the newly available commands run `juju jaas -h`.

Plugin commands can either be executed with the `jaas` subcommand or directly, as follows:

```text
$ juju jaas <command>
```
or

```text
$ juju <command>
```

The second form is preferred and used throughout our documentation.

These commands are intended to extend Juju's capabilities but note that some commands require elevated permissions.
