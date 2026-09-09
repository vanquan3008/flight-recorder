FROM golang:1.26.6-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /out/tracepilot ./cmd/tracepilot \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tracepilot-agent ./cmd/tracepilot-agent

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tracepilot /tracepilot
COPY --from=build /out/tracepilot-agent /tracepilot-agent
COPY db/migrations /db/migrations
ENV TRACEPILOT_MIGRATIONS_PATH=/db/migrations
EXPOSE 8080 4317 4318 6514
USER nonroot:nonroot
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD ["/tracepilot", "healthcheck"]
ENTRYPOINT ["/tracepilot"]
CMD ["serve"]
