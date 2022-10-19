#!/usr/bin/env bash

if ! [ -d "./vendor/k8s.io/code-generator/" ]; then
    echo "Please run 'go mod vendor'."
    exit 1
fi

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")

bash ./vendor/k8s.io/code-generator/generate-groups.sh "deepcopy,client,informer,lister" \
  github.com/intel/intent-driven-orchestration/pkg/generated github.com/intel/intent-driven-orchestration/pkg/api \
  intents:v1alpha1 \
  --output-base "$SCRIPT_DIR/../../../.." \
  --go-header-file "$SCRIPT_DIR/header.go.txt" \
  -v 2
