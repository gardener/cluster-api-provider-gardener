#!/bin/bash

# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

command="$1"
path="$2"

cd "$path"
chmod +rw . -R
chmod +rwx ./hack -R

case $command in
  up)
  make kind-up
  make gardener-up
  ;;
  down)
  make kind-down
  ;;
esac
