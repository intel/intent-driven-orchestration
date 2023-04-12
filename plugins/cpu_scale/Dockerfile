# Copyright (c) 2022 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

FROM golang:1.19 AS builder

WORKDIR /plugins

COPY . ./

RUN make prepare-build build-plugins \
    && go run github.com/google/go-licenses@v1.6.0 save "./..." --save_path licenses \
    && hack/additional-licenses.sh

FROM alpine:3.17

RUN apk add --upgrade --no-cache openssl=~3.0 && apk add --no-cache python3=~3.10 py3-matplotlib=~3.6 \
    py3-pip=~22.3 py3-scikit-learn=~1.1
RUN pip install --no-cache-dir pymongo~=4.3

WORKDIR /plugins

COPY --from=builder /plugins/bin/plugins/cpu_scale /plugins/bin/plugins/cpu_scale
COPY --from=builder /plugins/licenses ./licenses
COPY pkg/planner/actuators/scaling/analytics/cpu_rightsizing.py /plugins/pkg/planner/actuators/scaling/cpu_rightsizing.py

USER nonroot:nonroot
EXPOSE 33334

ENTRYPOINT ["/plugins/bin/plugins/cpu_scale"]

