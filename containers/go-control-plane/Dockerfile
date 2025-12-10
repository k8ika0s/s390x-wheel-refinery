# Build stage
FROM golang:1.22 AS builder
WORKDIR /app
COPY go-control-plane/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /go-control-plane ./cmd/api

# Runtime stage
FROM gcr.io/distroless/base-debian12
COPY --from=builder /go-control-plane /usr/local/bin/control-plane
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/control-plane"]
