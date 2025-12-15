# Build stage
FROM golang:1.22 AS builder
WORKDIR /app
COPY go-worker/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /go-worker ./cmd/worker

# Runtime stage
FROM gcr.io/distroless/base-debian12
COPY --from=builder /go-worker /usr/local/bin/worker
EXPOSE 9000
ENTRYPOINT ["/usr/local/bin/worker"]
