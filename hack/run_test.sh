#!/usr/bin/env bash

set -e

go test -p 1 -count=1 -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
