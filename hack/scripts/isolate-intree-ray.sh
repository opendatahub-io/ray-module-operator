#!/usr/bin/env bash
# Isolate the in-tree Ray controller so the module operator can be tested standalone.
# Unlike scaling rhods-operator to 0, pausing the Subscription prevents OLM from
# restoring replicas. Run hack/scripts/restore-intree-ray.sh to undo.
set -euo pipefail

echo "Setting DSC Ray to Removed..."
oc patch dsc default-dsc --type=merge \
  -p '{"spec":{"components":{"ray":{"managementState":"Removed"}}}}'

echo "Waiting for in-tree kuberay-operator to terminate..."
until ! oc -n redhat-ods-applications get deploy kuberay-operator >/dev/null 2>&1; do
  sleep 5
done

echo "Pausing rhods-operator Subscription (prevents OLM restore)..."
SUB_NAME=$(oc -n redhat-ods-operator get subscription -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
if [ -n "$SUB_NAME" ]; then
  oc -n redhat-ods-operator patch subscription "$SUB_NAME" --type=merge \
    -p '{"spec":{"config":{"paused":true}}}'
  echo "Subscription '$SUB_NAME' paused."
else
  echo "No Subscription found — scaling rhods-operator to 0 as fallback."
  oc -n redhat-ods-operator scale deploy/rhods-operator --replicas=0
fi

echo "In-tree Ray isolated. Safe to deploy module operator."
