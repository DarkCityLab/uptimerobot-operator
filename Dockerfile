# Build the manager binary.
# Pin the builder to the native BUILD platform and cross-compile to the target
# arch via GOARCH. This avoids running the Go toolchain under QEMU emulation
# (which is 5-10x slower) when producing arm64 images on an amd64 host.
FROM --platform=${BUILDPLATFORM} golang:1.27@sha256:205c7636d74e0ffbe3d007162df7d74f859f5a8c8322dfb9f907977686c126cf AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the Go source (relies on .dockerignore to filter)
COPY . .

# Cross-compile for the requested target architecture. GOOS/GOARCH come from the
# docker buildx TARGET* args. No -a: reuse the precompiled stdlib and build cache.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary.
# Pinned by digest (Debian 13 variant) per the org's SHA-pinning policy.
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static-debian13:nonroot@sha256:8ce6dd87e9823c5b48dcfb2c8604714ccb351ce59607327bcc4957448da279b5
# Auto-links the GHCR package to the source repository on push, and provides the
# package description. For multi-arch images GHCR reads the description from the
# index annotation (set in the release workflow); this LABEL covers single-arch
# pulls and other tooling.
LABEL org.opencontainers.image.source=https://github.com/DarkCityLab/uptimerobot-operator
LABEL org.opencontainers.image.description="Kubernetes operator that creates and deletes UptimeRobot (v3) monitors for services exposed in your cluster"
LABEL org.opencontainers.image.licenses=Apache-2.0
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
