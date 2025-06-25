#!/bin/bash

# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

cd "$1"
chmod +rw . -R
chmod +rwx ./hack -R
make kind-up
make gardener-up
