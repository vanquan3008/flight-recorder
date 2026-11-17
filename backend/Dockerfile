FROM golang:1.27.0-alpine AS build-base
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY db ./db

FROM build-base AS build-server
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /out/tracepilot ./cmd/tracepilot

FROM gcr.io/distroless/static-debian12:nonroot AS runtime

FROM runtime AS server
COPY --from=build-server /out/tracepilot /tracepilot
EXPOSE 8080 4317 4318
USER nonroot:nonroot
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD ["/tracepilot", "healthcheck"]
ENTRYPOINT ["/tracepilot"]
CMD ["serve"]
