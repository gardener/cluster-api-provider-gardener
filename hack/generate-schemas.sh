#!/bin/bash

# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

REPO_ROOT="$1"
CAPI_DIR="$2"

TMP_DIR=${REPO_ROOT}/tmp
EXPORT_FILE=${REPO_ROOT}/schemas/gardener/apiexport-controlplane.cluster.x-k8s.io.yaml

echo "⚙️ Generating APIResouceSchemas for gardener"
mkdir -p ${TMP_DIR}
apigen --input-dir ${REPO_ROOT}/config/crd/bases --output-dir ${TMP_DIR}

new_schema_files=$(find ${TMP_DIR} -type f | grep -E 'apiresourceschema.*\.yaml$')

for schema_file in ${new_schema_files}; do
  yq eval '.metadata.name |= sub("^v[0-9]+-[a-f0-9]+\.", "generated.")' "${schema_file}" -i -P
done
echo "🔄 Replacing old Gardener APIResourceSchemas"
cp ${new_schema_files} ${REPO_ROOT}/schemas/gardener

echo "📂 Copying relevant CAPI CRDs"
mkdir -p ${TMP_DIR}/capi
cp ${CAPI_DIR}/config/crd/bases/cluster.x-k8s.io_clusters.yaml ${TMP_DIR}/capi -f
cp ${CAPI_DIR}/config/crd/bases/cluster.x-k8s.io_machinepools.yaml ${TMP_DIR}/capi -f

echo "🚫 Removing non stored / served CAPI CRD versions"
for capi_crd in ${TMP_DIR}/capi/*.yaml; do
  yq -i '.spec.versions |= map(select(.served == true and .storage == true))' "${capi_crd}"
done

echo "⚙️ Generating APIResouceSchemas for CAPI"
apigen --input-dir ${TMP_DIR}/capi --output-dir ${TMP_DIR}/capi

new_capi_schema_files=$(find ${TMP_DIR} -type f | grep -E 'apiresourceschema.*\.yaml$')
for schema_file in ${new_capi_schema_files}; do
  yq eval '.metadata.name |= sub("^v[0-9]+-[a-f0-9]+\.", "generated.")' "${schema_file}" -i -P
done

echo "🔄 Replacing old CAPI APIResourceSchemas"
cp ${new_capi_schema_files} ${REPO_ROOT}/schemas/gardener