# Set Go image
FROM golang:1.24.0-bullseye AS builder

ENV CGO_ENABLED=0
ENV GO111MODULE=on

# Create workspace
WORKDIR /app

# Copy the entire repo, assuming your context is ~/arpc
COPY . .

# Go to echo_proto subdir
WORKDIR /app/examples/echo_proto

RUN go mod tidy

# Build binaries
RUN go build -trimpath -ldflags="-s -w" -o frontend ./frontend
RUN go build -trimpath -ldflags="-s -w" -o server ./server

# Final image
FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/examples/echo_proto/frontend /app/frontend
COPY --from=builder /app/examples/echo_proto/server /app/server
RUN chmod +x /app/frontend /app/server
