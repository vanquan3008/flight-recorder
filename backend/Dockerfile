# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS build

WORKDIR /src

ENV GOTOOLCHAIN=local

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd/tracepilot ./cmd/tracepilot
COPY internal ./internal
COPY db ./db

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
    -trimpath -buildvcs=false -mod=readonly \
    -ldflags="-s -w \
    -X main.version=${VERSION} \
    -X main.commit=${COMMIT} \
    -X main.buildTime=${BUILD_TIME}" \
    -o /out/tracepilot \
    ./cmd/tracepilot

FROM gcr.io/distroless/static-debian12:nonroot AS server

WORKDIR /

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

LABEL org.opencontainers.image.title="TracePilot" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_TIME}"

COPY --from=build /out/tracepilot /tracepilot

USER nonroot:nonroot

EXPOSE 8080 4317 4318

HEALTHCHECK \
    --interval=30s \
    --timeout=5s \
    --start-period=30s \
    --start-interval=3s \
    --retries=3 \
    CMD ["/tracepilot", "healthcheck"]

ENTRYPOINT ["/tracepilot"]

CMD ["serve"]
