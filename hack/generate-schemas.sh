#!/bin/bash

# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

REPO_ROOT="$1"
TMP_DIR=${REPO_ROOT}/tmp
EXPORT_FILE=${REPO_ROOT}/schemas/gardener/apiexport-controlplane.cluster.x-k8s.io.yaml

echo "⚙️ Generating APIResouceSchemas for gardener"
apigen --input-dir ${REPO_ROOT}/config/crd/bases --output-dir ${TMP_DIR}

new_schema_files=$(find ${TMP_DIR} -type f | grep -E 'apiresourceschema.*\.yaml$')

old_schema_names=$(cat ${EXPORT_FILE} | yq .spec.latestResourceSchemas | grep -E 'controlplane|infrastructure' | sed 's/^- //' )
new_schema_names=()

for schema_file in ${new_schema_files}; do
  new_schema_names+=($(yq .metadata.name ${schema_file}))
done

echo "🔄 Replacing old APIResourceSchemas"
cp ${new_schema_files} ${REPO_ROOT}/schemas/gardener

echo "📝 Updating APIExport"
# Remove old schemas from list
for old_schema in ${old_schema_names}; do
  yq -i "del(.spec.latestResourceSchemas[] | select(. == \"${old_schema}\"))" ${EXPORT_FILE}
done

# Prepend new schemas to list
for new_schema in "${new_schema_names[@]}"; do
  if ! yq '.spec.latestResourceSchemas[]' "${EXPORT_FILE}" | grep -qx "${new_schema}"; then
    yq -i ".spec.latestResourceSchemas = [\"${new_schema}\"] + .spec.latestResourceSchemas" "${EXPORT_FILE}"
  fi
done
