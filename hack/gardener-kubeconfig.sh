#!/bin/bash

# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

# Check if the CLI argument for GARDENER_KUBECONFIG is provided
if [ -z "$1" ]; then
  echo "Usage: $0 <path_to_kubeconfig>"
  exit 1
fi

GARDENER_KUBECONFIG=$1

# Check if the kubeconfig file exists
if [ ! -f "$GARDENER_KUBECONFIG" ]; then
  echo "File $GARDENER_KUBECONFIG does not exist."
  exit 1
fi

# Base64 encode the kubeconfig file, whilst replacing the localhost api-server with in cluster api access and print it
B64_GARDENER_KUBECONFIG=$(cat "$GARDENER_KUBECONFIG" | sed -E 's|https://127\.0\.0\.1:[0-9]+|https://kubernetes.default.svc:443|g' | base64 -w0)

# Print the base64 encoded kubeconfig
echo $B64_GARDENER_KUBECONFIG
