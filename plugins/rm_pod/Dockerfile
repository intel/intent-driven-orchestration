# Copyright (c) 2022 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

FROM golang:1.24.6 AS builder

WORKDIR /plugins

COPY . ./

RUN make prepare-build build-plugins \
    && go run github.com/google/go-licenses@v1.6.0 save "./..." --save_path licenses \
    && hack/additional-licenses.sh

FROM scratch

WORKDIR /plugins

COPY --from=builder /plugins/bin/plugins/rm_pod /plugins/bin/plugins/rm_pod
COPY --from=builder /plugins/licenses ./licenses

USER nonroot:nonroot
EXPOSE 33334

ENTRYPOINT ["/plugins/bin/plugins/rm_pod"]
