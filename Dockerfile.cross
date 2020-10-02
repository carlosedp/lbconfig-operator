FROM quay.io/prometheus/busybox:latest
ARG TARGETOS
ARG TARGETARCH

WORKDIR /
COPY bin/manager-$TARGETOS-$TARGETARCH ./manager
USER nobody

ENTRYPOINT ["/manager"]