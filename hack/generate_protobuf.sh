#!/usr/bin/env bash

MAIN_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )/../" &> /dev/null && pwd )

set -e
function protoc::ensure_installed {
    if [[ ! -x "$(command -v protoc)" || "$(protoc --version)" != "libprotoc 27."* ]]; then
        echo "Generating api requires an up-to-date protoc compiler. Please follow the instructions at"
        echo "  https://grpc.io/docs/protoc-installation/"
        exit 1
    fi
}

function protoc::generate_go {
    protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative "$1"
}
protoc::ensure_installed

cd "$MAIN_DIR"
find pkg/api/plugins -type f -name '*.proto' -print0 | while IFS= read -r -d '' file; do
    echo "Generating $file"
    protoc::generate_go "$file"
done
