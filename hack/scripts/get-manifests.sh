#!/usr/bin/env bash
# Follows ODH-operator /opt/manifests pattern (download at build, not committed).
set -euo pipefail

GITHUB_URL="https://github.com"
DST_MANIFESTS_DIR="${DST_MANIFESTS_DIR:-./opt/manifests}"

# COMPONENT_MANIFESTS entries are in the format:
#   "repo-org:repo-name:ref-name:source-folder"
#
# ref-name supports:
#   1. "branch"          - tracks latest commit on branch
#   2. "tag"             - immutable reference (e.g., v1.4.2)
#   3. "branch@sha"      - fetch branch, reset to specific commit (preferred for pinning)
#   4. "tag@sha"         - fetch tag, reset to specific commit

declare -A ODH_COMPONENT_MANIFESTS=(
    ["kuberay"]="opendatahub-io:kuberay:dev@ad425f7febc4039f2378747f2a0ea5dcf5a2263f:ray-operator/config"
)

# FIXME: Bump to rhoai-3.6@<sha> once the GA branch is cut
declare -A RHOAI_COMPONENT_MANIFESTS=(
    ["kuberay"]="red-hat-data-services:kuberay:rhoai-3.5@d1972fbd2cb7efa5c8c4b527f5156a499cfb9173:ray-operator/config"
)

# Select manifests based on platform type
if [ "${ODH_PLATFORM_TYPE:-OpenDataHub}" = "OpenDataHub" ]; then
    echo "Downloading manifests for ODH"
    declare -A COMPONENT_MANIFESTS=()
    for key in "${!ODH_COMPONENT_MANIFESTS[@]}"; do
        COMPONENT_MANIFESTS["$key"]="${ODH_COMPONENT_MANIFESTS[$key]}"
    done
else
    echo "Downloading manifests for RHOAI"
    declare -A COMPONENT_MANIFESTS=()
    for key in "${!RHOAI_COMPONENT_MANIFESTS[@]}"; do
        COMPONENT_MANIFESTS["$key"]="${RHOAI_COMPONENT_MANIFESTS[$key]}"
    done
fi

TMP_DIR=$(mktemp -d -t "odh-kuberay-manifests.XXXXXXXXXX")
trap '{ rm -rf -- "${TMP_DIR}"; }' EXIT

function try_fetch_ref()
{
    local repo=$1
    local ref_type=$2  # "tags" or "heads"
    local ref=$3

    local git_ref="refs/${ref_type}/${ref}"
    local ref_name
    ref_name=$([[ ${ref_type} == "tags" ]] && echo "tag" || echo "branch")

    if git ls-remote --exit-code "${repo}" "${git_ref}" &>/dev/null; then
        if git fetch -q --depth 1 "${repo}" "${git_ref}" && git reset -q --hard FETCH_HEAD; then
            return 0
        else
            echo "ERROR: Failed to fetch ${ref_name} ${ref} from ${repo}"
            return 1
        fi
    fi
    return 1
}

function git_fetch_ref()
{
    local repo=$1
    local ref=$2
    local dir=$3

    mkdir -p "${dir}"
    pushd "${dir}" &>/dev/null
    git init -q

    # branch@sha or tag@sha format: fetch the specific commit directly
    if [[ ${ref} =~ ^([a-zA-Z0-9_./-]+)@([a-f0-9]{7,40})$ ]]; then
        local commit_sha="${BASH_REMATCH[2]}"

        git remote add origin "${repo}"
        if ! git fetch --depth 1 -q origin "${commit_sha}"; then
            echo "ERROR: Failed to fetch commit ${commit_sha} from ${repo}"
            popd &>/dev/null
            return 1
        fi
        if ! git reset -q --hard "${commit_sha}" 2>/dev/null; then
            echo "ERROR: Commit SHA ${commit_sha} not found in ${repo}"
            popd &>/dev/null
            return 1
        fi
    else
        # Plain branch, tag, or bare SHA: try tag then branch
        if try_fetch_ref "${repo}" "tags" "${ref}" || try_fetch_ref "${repo}" "heads" "${ref}"; then
            :  # success
        else
            echo "ERROR: '${ref}' is not a valid branch, tag, or commit SHA in ${repo}"
            popd &>/dev/null
            return 1
        fi
    fi

    popd &>/dev/null
}

download_manifest() {
    local key=$1
    local repo_info=$2
    echo -e "\033[32mDownloading \033[33m${key}\033[32m:\033[0m ${repo_info}"
    IFS=':' read -r -a parts <<< "${repo_info}"

    local repo_org="${parts[0]}"
    local repo_name="${parts[1]}"
    local repo_ref="${parts[2]}"
    local source_path="${parts[3]}"

    local repo_url="${GITHUB_URL}/${repo_org}/${repo_name}"
    local repo_dir="${TMP_DIR}/${key}"

    # USE_LOCAL: copy from adjacent checkout instead of cloning (host-only, not used in Dockerfile)
    if [[ "${USE_LOCAL:-}" == "true" ]] && [[ -e "../${repo_name}" ]]; then
        echo "Copying from adjacent checkout ..."
        rm -rf "${DST_MANIFESTS_DIR:?}/${key}"
        mkdir -p "${DST_MANIFESTS_DIR}/${key}"
        cp -rf "../${repo_name}/${source_path}"/* "${DST_MANIFESTS_DIR}/${key}"
        return
    fi

    if ! git_fetch_ref "${repo_url}" "${repo_ref}" "${repo_dir}"; then
        echo "ERROR: Failed to fetch ref '${repo_ref}' from '${repo_url}' for component '${key}'"
        return 1
    fi

    rm -rf "${DST_MANIFESTS_DIR:?}/${key}"
    mkdir -p "${DST_MANIFESTS_DIR}/${key}"
    cp -rf "${repo_dir}/${source_path}"/* "${DST_MANIFESTS_DIR}/${key}"
}

declare -a pids=()
for key in "${!COMPONENT_MANIFESTS[@]}"; do
    download_manifest "${key}" "${COMPONENT_MANIFESTS[$key]}" &
    pids+=("$!")
done

failed=0
for pid in "${pids[@]}"; do
    if ! wait "${pid}"; then
        failed=1
    fi
done

if [ "${failed}" -eq 1 ]; then
    echo "One or more downloads failed"
    exit 1
fi

# Remove test/sample/scorecard artifacts not needed at runtime
rm -rf "${DST_MANIFESTS_DIR}"/*/e2e "${DST_MANIFESTS_DIR}"/*/scorecard \
       "${DST_MANIFESTS_DIR}"/*/test "${DST_MANIFESTS_DIR}"/*/samples \
       "${DST_MANIFESTS_DIR}"/*/example-*
find "${DST_MANIFESTS_DIR}" -name "README.md" -delete 2>/dev/null || true

# Strip TLS metrics patch: the vendored manifests add --metrics-cert-path but
# no published kuberay image supports it yet. Remove the kustomize reference
# so the operator deploys without cert-manager TLS on the metrics endpoint.
for kust in "${DST_MANIFESTS_DIR}"/*/openshift/kustomization.yaml; do
    if [ -f "$kust" ]; then
        python3 -c "
import re, sys
text = open(sys.argv[1]).read()
text = re.sub(r'- path: tls-metrics-patch\.yaml\n(?:  [^\n]*\n)*', '', text)
open(sys.argv[1], 'w').write(text)
" "$kust"
    fi
done

echo "Manifests downloaded to ${DST_MANIFESTS_DIR}"
