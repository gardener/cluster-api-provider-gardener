#!/bin/bash

# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

REPO_ROOT="$1"
TMP_DIR=${REPO_ROOT}/tmp
EXPORT_FILE=${REPO_ROOT}/schemas/gardener/apiexport-controlplane.cluster.x-k8s.io.yaml

echo "⚙️ Generating APIResouceSchemas for gardener"
mkdir -p ${TMP_DIR}
apigen --input-dir ${REPO_ROOT}/config/crd/bases --output-dir ${TMP_DIR}

new_schema_files=$(find ${TMP_DIR} -type f | grep -E 'apiresourceschema.*\.yaml$')

for schema_file in ${new_schema_files}; do
  yq eval '.metadata.name |= sub("^v[0-9]+-[a-f0-9]+\.", "")' "${schema_file}" -i -P
done
echo "🔄 Replacing old APIResourceSchemas"
cp ${new_schema_files} ${REPO_ROOT}/schemas/gardener
