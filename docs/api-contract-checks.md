# API contract checks

Pull requests targeting **`main`** run static CRD contract checks from
[`red-hat-data-services/api-contract-tests`](https://github.com/red-hat-data-services/api-contract-tests)
([RHOAIENG-103171](https://redhat.atlassian.net/browse/RHOAIENG-103171)). No cluster is required.
Every job uses **`mode: enforce`**.

## What runs on each pull request

| Job | What it checks |
|-----|----------------|
| `crd-contract` | `config/crd/bases/*.yaml` (`Ray` module CRD) vs baseline **main** |
| `api-contract-coverage` | Every CRD manifest is in the check glob or a recorded exclusion |

There are no HTTP API contracts in this repository. `.rhoai/contracts.yaml` declares `contracts: []`.

Bundled **ray.io** CRDs under `opt/manifests/*/kuberay/` are owned by
[opendatahub-io/kuberay](https://github.com/opendatahub-io/kuberay), not this operator.

## Baseline

`97a6b5a8cbeaf30107ec267ab7be4ecde8101637` (`main` when onboarded). No GA release tag existed in
this repository at onboarding time. Use full 40-character SHAs only when moving the baseline.

## Run locally

```sh
git clone https://github.com/red-hat-data-services/api-contract-tests.git /tmp/api-contract-tests
git -C /tmp/api-contract-tests fetch --tags && git -C /tmp/api-contract-tests checkout latest

CT=/tmp/api-contract-tests
BASELINE=97a6b5a8cbeaf30107ec267ab7be4ecde8101637

python3 "$CT/cmd/contract-tests/cli.py" crd-check --repo . \
  --crd-glob 'config/crd/bases/*.yaml' --baseline-ref "$BASELINE" \
  --output-dir /tmp/contract-report-crd

python3 "$CT/cmd/contract-tests/cli.py" coverage --repo . \
  --contract-set .rhoai/contracts.yaml --yq-version 4.45.1 \
  --crd-glob 'config/crd/bases/*.yaml' \
  --crd-exclude 'charts/ray-module-operator/files/*.yaml' \
  --crd-exclude 'dist/*.yaml' \
  --crd-exclude 'opt/manifests/odh/kuberay/crd/bases/*.yaml' \
  --crd-exclude 'opt/manifests/rhoai/kuberay/crd/bases/*.yaml'
```

Downstream-only: keep `.rhoai/` and `.github/workflows/api-contract.yml` when vendoring upstream changes.
