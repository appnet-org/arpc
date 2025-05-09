# Use the official Go image as the base image
FROM golang:1.22.1-bullseye AS builder

ENV CGO_ENABLED=0

# Set the working directory
WORKDIR /app

# Copy the entire arpc folder from parent directory
COPY ../.. /app/arpc/

# Set working directory to the arpc folder
WORKDIR /app/arpc

# Build the frontend
RUN go build -trimpath -ldflags="-s -w" -o frontend frontend.go

# Build the server
RUN go build -trimpath -ldflags="-s -w" -o server server.go

# Use a minimal alpine image for the final stage
FROM alpine:latest

WORKDIR /app

# Copy the binaries from the builder stage
COPY --from=builder /app/arpc/frontend /app/frontend
COPY --from=builder /app/arpc/server /app/server

RUN chmod +x /app/frontend /app/server