#!/bin/bash

# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

command="$1"
path="$2"

# Resolve the pkg/apis module cache path from the project root before cd-ing
# into the Gardener directory (whose go.mod would cause self-resolution).
script_dir="$(cd "$(dirname "$0")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
apis_dir="$(cd "$repo_root" && go list -m -f '{{.Dir}}' github.com/gardener/gardener/pkg/apis)"

cd "$path"
chmod +rw . -R
chmod +rwx ./hack -R
chmod +rwx ./dev-setup -R
rm -f go.work go.work.sum

# pkg/apis is a separate Go submodule not included in the main module cache;
# copy it into the expected location for skaffold/ko builds.
if [ ! -d ./pkg/apis ]; then
  cp -r "$apis_dir" ./pkg/apis
fi

case $command in
  up)
  make kind-up
  make gardener-up
  ;;
  down)
  make kind-down
  ;;
esac
