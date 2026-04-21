FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/tockr ./cmd/app

FROM alpine:3.22
RUN addgroup -S tockr && adduser -S -G tockr tockr
WORKDIR /app
COPY --from=build /out/tockr /app/tockr
COPY web/static /app/web/static
RUN mkdir -p /app/data && chown -R tockr:tockr /app
USER tockr
EXPOSE 8080
ENV TOCKR_ADDR=:8080 TOCKR_DB_PATH=/app/data/tockr.db TOCKR_DATA_DIR=/app/data
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/tockr"]

