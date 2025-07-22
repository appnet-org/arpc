# Set Go image
FROM golang:1.23.9-bullseye AS builder

ENV CGO_ENABLED=0
ENV GO111MODULE=on

# Create workspace
WORKDIR /app

COPY cmd/proxy/ ./
RUN go build -o proxy main.go

# Final image
FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/proxy /app/proxy
RUN chmod +x /app/proxy

USER proxyuser
