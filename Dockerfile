# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
# Pinned to a digest shipping ca-certificates 20250419+deb13u1, which drops the
# compromised eMudhra/Sectigo roots (see issue #515).
FROM gcr.io/distroless/static:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99
ARG TARGETARCH
ARG TARGETOS

WORKDIR /
COPY output/manager-$TARGETOS-$TARGETARCH /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
