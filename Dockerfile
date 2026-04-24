FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS deps
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

FROM deps AS test-runner
COPY . .
ENV CGO_ENABLED=0
CMD ["go", "test", "./..."]

FROM deps AS build
COPY . .
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN set -eux; \
    mkdir -p /out/data; \
    if [ "$TARGETARCH" = "arm" ]; then export GOARM="${TARGETVARIANT#v}"; fi; \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/tockr ./cmd/app

FROM alpine:3.22
WORKDIR /app
COPY --from=build /out/tockr /app/tockr
COPY web/static /app/web/static
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh
COPY --from=build --chown=65532:65532 /out/data /app/data
USER 65532:65532
EXPOSE 8080
ENV TOCKR_ADDR=:8080 TOCKR_DB_PATH=/app/data/tockr.db TOCKR_DATA_DIR=/app/data
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/entrypoint.sh"]
