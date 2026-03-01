# Copyright 2026 NVIDIA CORPORATION & AFFILIATES
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# SPDX-License-Identifier: Apache-2.0

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

ARG TARGETPLATFORM
ARG TARGETARCH
ARG TARGETOS

COPY . /usr/src/rdma-cni
WORKDIR /usr/src/rdma-cni

ENV HTTP_PROXY=$http_proxy
ENV HTTPS_PROXY=$https_proxy

RUN apk add --no-cache build-base=~0.5 \
    && make clean && make build TARGET_OS=$TARGETOS TARGET_ARCH=$TARGETARCH

FROM alpine:3
COPY --from=builder /usr/src/rdma-cni/build/rdma /usr/bin/
COPY ./images/entrypoint.sh /

WORKDIR /
LABEL io.k8s.display-name="RDMA CNI"
ENTRYPOINT ["/entrypoint.sh"]
