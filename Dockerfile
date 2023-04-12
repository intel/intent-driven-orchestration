# Copyright (c) 2022 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

FROM golang:1.19 AS build

WORKDIR /app

COPY . ./

RUN make prepare-build build \
    && go run github.com/google/go-licenses@v1.6.0 save "./..." --save_path licenses \
    && hack/additional-licenses.sh

FROM scratch

WORKDIR /app

COPY --from=build /app/bin/planner /app/bin/planner
COPY --from=build /app/licenses ./licenses

USER nonroot:nonroot

EXPOSE 33333
ENTRYPOINT ["/app/bin/planner"]
