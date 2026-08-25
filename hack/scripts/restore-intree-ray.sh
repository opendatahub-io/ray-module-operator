#!/usr/bin/env bash
# Restore the in-tree Ray controller after standalone module operator testing.
# Undoes hack/scripts/isolate-intree-ray.sh.
set -euo pipefail

echo "Unpausing rhods-operator Subscription..."
SUB_NAME=$(oc -n redhat-ods-operator get subscription -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
if [ -n "$SUB_NAME" ]; then
  oc -n redhat-ods-operator patch subscription "$SUB_NAME" --type=merge \
    -p '{"spec":{"config":{"paused":false}}}'
  echo "Subscription '$SUB_NAME' unpaused."
else
  oc -n redhat-ods-operator scale deploy/rhods-operator --replicas=1
fi

echo "Setting DSC Ray to Managed..."
oc patch dsc default-dsc --type=merge \
  -p '{"spec":{"components":{"ray":{"managementState":"Managed"}}}}'

echo "Waiting for in-tree kuberay-operator..."
until oc -n redhat-ods-applications get deploy kuberay-operator >/dev/null 2>&1; do
  sleep 5
done
oc -n redhat-ods-applications rollout status deploy/kuberay-operator --timeout=120s

echo "In-tree Ray restored."
