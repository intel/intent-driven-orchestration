#!/usr/bin/env bash

if ! [ -d "./vendor/k8s.io/code-generator/" ]; then
    echo "Please run 'go mod vendor'."
    exit 1
fi

REPO_DIR=$(git rev-parse --show-toplevel)
source "${REPO_DIR}/vendor/k8s.io/code-generator/kube_codegen.sh"

kube::codegen::gen_helpers \
    ${REPO_DIR}/pkg/api/ \
    --boilerplate "${REPO_DIR}/hack/header.go.txt"

kube::codegen::gen_client \
    ${REPO_DIR}/pkg/api/ \
    --with-watch \
    --output-dir "${REPO_DIR}"/pkg/generated \
    --output-pkg github.com/intel/intent-driven-orchestration/pkg/generated \
    --boilerplate "${REPO_DIR}/hack/header.go.txt"
