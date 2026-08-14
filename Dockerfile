# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY examples ./examples
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /out/flight ./cmd/flight
RUN CGO_ENABLED=0 go build -trimpath -o /out/demo-service ./examples/demo-service

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/flight /flight
COPY --from=build /out/demo-service /demo-service
COPY migrations /migrations
ENV FLIGHT_MIGRATIONS_PATH=/migrations
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD ["/flight", "healthcheck"]
ENTRYPOINT ["/flight"]
CMD ["serve"]
