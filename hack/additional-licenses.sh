#!/bin/sh
# Copy licenses from dependencies that go-licenses doesn't pickup"
# This script may be updated or removed on next release.
go mod vendor
mkdir -p licenses/github.com/stretchr/testify
cp vendor/github.com/stretchr/testify/LICENSE licenses/github.com/stretchr/testify/LICENSE
mkdir -p licenses/github.com/stretchr/objx
cp vendor/github.com/stretchr/objx/LICENSE licenses/github.com/stretchr/objx/LICENSE
mkdir -p licenses/github.com/pmezard/go-difflib
cp vendor/github.com/pmezard/go-difflib/LICENSE licenses/github.com/pmezard/go-difflib/LICENSE
