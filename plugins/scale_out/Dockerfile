# Copyright (c) 2022 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

FROM golang:1.19 AS builder

WORKDIR /plugins

COPY . ./

RUN make prepare-build build-plugins \
    && go run github.com/google/go-licenses@v1.3.1 save "./..." --save_path licenses \
    && hack/additional-licenses.sh

FROM alpine:3.16

RUN adduser -D nonroot

WORKDIR /plugins

COPY --from=builder /plugins/bin/plugins/scale_out /plugins/bin/plugins/scale_out
COPY --from=builder /plugins/licenses ./licenses
COPY pkg/planner/actuators/scaling/analytics.py /plugins/pkg/planner/actuators/scaling/analytics.py

RUN apk update && apk --no-cache add python3=~3.10 py3-matplotlib=~3.5  \
    py3-pip=~22.1 py3-scikit-learn=~1.1

RUN pip install --no-cache-dir pymongo==4.2

USER nonroot:nonroot

EXPOSE 33334
ENTRYPOINT ["/plugins/bin/plugins/scale_out"]
