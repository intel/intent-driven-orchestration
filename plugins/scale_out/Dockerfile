# Copyright (c) 2022 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

FROM golang:1.24.2 AS builder

WORKDIR /plugins

COPY . ./

RUN make prepare-build build-plugins \
    && go run github.com/google/go-licenses@v1.6.0 save "./..." --save_path licenses \
    && hack/additional-licenses.sh

FROM alpine:3.20

RUN adduser -D nonroot
RUN apk add --upgrade --no-cache openssl=~3.3 && apk add --no-cache python3=~3.12 py3-matplotlib=~3.7 \
    py3-pip=~24.0 py3-scikit-learn=~1.3
RUN pip install --break-system-packages --no-cache-dir pymongo~=4.6

WORKDIR /plugins

COPY --from=builder /plugins/bin/plugins/scale_out /plugins/bin/plugins/scale_out
COPY --from=builder /plugins/licenses ./licenses
COPY pkg/planner/actuators/scaling/analytics/horizontal_scaling.py /plugins/pkg/planner/actuators/scaling/horizontal_scaling.py

USER nonroot:nonroot
EXPOSE 33334

ENTRYPOINT ["/plugins/bin/plugins/scale_out"]
