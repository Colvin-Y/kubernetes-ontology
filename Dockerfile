# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /out/ \
      ./cmd/kubernetes-ontology \
      ./cmd/kubernetes-ontologyd \
      ./cmd/kubernetes-ontology-viewer

FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="kubernetes-ontology" \
      org.opencontainers.image.description="Read-only Kubernetes ontology server, CLI, and dependency-free viewer" \
      org.opencontainers.image.source="https://github.com/Colvin-Y/kubernetes-ontology" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=build /out/kubernetes-ontology /kubernetes-ontology
COPY --from=build /out/kubernetes-ontologyd /kubernetes-ontologyd
COPY --from=build /out/kubernetes-ontology-viewer /kubernetes-ontology-viewer

USER nonroot:nonroot
EXPOSE 18080 8765
ENTRYPOINT ["/kubernetes-ontologyd"]
