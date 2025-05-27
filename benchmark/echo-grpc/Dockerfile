# Use the official Go image as the base image
FROM golang:1.23.9-bullseye AS builder

ENV CGO_ENABLED=0

# Set the working directory
WORKDIR /app

# Copy the entire arpc folder from parent directory
COPY . /app/echo-grpc/

# Set working directory to the arpc folder
WORKDIR /app/echo-grpc

# Build the frontend
RUN go build -trimpath -ldflags="-s -w" -o frontend frontend/frontend.go 

# Build the server
RUN go build -trimpath -ldflags="-s -w" -o server server/server.go

# Use a minimal alpine image for the final stage
FROM alpine:latest

WORKDIR /app

# Copy the binaries from the builder stage
COPY --from=builder /app/echo-grpc/frontend /app/frontend
COPY --from=builder /app/echo-grpc/server /app/server

RUN chmod +x /app/frontend /app/server