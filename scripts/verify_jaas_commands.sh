#!/bin/bash
set -euo pipefail

# The following script verifies that all commands available to the jaas CLI tool
# are explicitly listed in the jaas snapcraft.yaml file as symlinks.
# It's necessary to have symlinks for the commands to be available to the Juju
# CLI as top-level commands e.g. `juju <command>`, otherwise they can only be 
# accessed via `juju jaas <command>`.

# The YAML file to check
YAML_FILE="snaps/jaas/snapcraft.yaml"
JAAS="./jaas"

# Extract the list of commands from `./jaas help commands`
commands=$($JAAS help commands | cut -d ' ' -f 1)

missing=0

# Commands to skip
skip_commands=(
  "documentation"
  "help"
)

for cmd in $commands; do
    # Transform "add-group" -> "juju-add-group" (as per the YAML symlink naming)
    symlink_name="juju-$cmd"

    # Check if the command is in the skip list
    skip=false
    for skip_cmd in "${skip_commands[@]}"; do
        if [[ "$skip_cmd" == "$cmd" ]]; then
            skip=true
            break
        fi
    done
    if $skip; then
        continue
    fi

    # Check if the YAML contains a line with "ln -sf jaas bin/juju-<command>"
    if ! grep -q "ln -sf jaas bin/$symlink_name" "$YAML_FILE"; then
        echo "Missing symlink for command: $cmd"
        missing=1
    fi
done

if [ "$missing" -eq 0 ]; then
    echo "All commands have corresponding symlinks."
else
    echo "Some commands are missing symlinks."
    exit 1
fi
