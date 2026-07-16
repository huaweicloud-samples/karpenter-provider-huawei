#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." &>/dev/null && pwd)"

HELM_BIN="${HELM:-helm}"
CHART_DIR="${CHART_DIR:-${REPO_ROOT}/charts/karpenter-provider-huawei}"
CHART_REPO_DIR="${CHART_REPO_DIR:-${REPO_ROOT}/charts}"
HELM_REPO_URL="${HELM_REPO_URL:-}"

if [[ "${CHART_DIR}" != /* ]]; then
  CHART_DIR="${REPO_ROOT}/${CHART_DIR}"
fi
if [[ "${CHART_REPO_DIR}" != /* ]]; then
  CHART_REPO_DIR="${REPO_ROOT}/${CHART_REPO_DIR}"
fi

if ! command -v "${HELM_BIN}" &>/dev/null; then
  echo "helm executable not found: ${HELM_BIN}" >&2
  exit 1
fi

[[ -d "${CHART_DIR}" ]] || { echo "chart directory not found: ${CHART_DIR}" >&2; exit 1; }
mkdir -p "${CHART_REPO_DIR}"

echo "Linting Helm chart: ${CHART_DIR}"
"${HELM_BIN}" lint "${CHART_DIR}"

echo "Packaging Helm chart into: ${CHART_REPO_DIR}"
"${HELM_BIN}" package "${CHART_DIR}" --destination "${CHART_REPO_DIR}"

index_args=("${CHART_REPO_DIR}")
if [[ -n "${HELM_REPO_URL}" ]]; then
  index_args+=(--url "${HELM_REPO_URL}")
fi

echo "Generating Helm repository index: ${CHART_REPO_DIR}/index.yaml"
"${HELM_BIN}" repo index "${index_args[@]}"
