package main

import (
	"context"
	"fmt"
	"os"

	echo "github.com/appnet-org/arpc/examples/echo_symphony/symphony"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/serializer"
	"go.uber.org/zap"
)

// EchoService implementation
type echoServer struct{}

func (s *echoServer) Echo(ctx context.Context, req *echo.EchoRequest) (*echo.EchoResponse, context.Context, error) {

	logging.Debug("Server received request", zap.String("content", req.GetContent()))

	resp := &echo.EchoResponse{
		Id:       req.GetId(),
		Score:    req.GetScore(),
		Username: req.GetUsername(),
		Content:  "Echo " + req.GetContent(),
	}

	logging.Debug("Server sending response", zap.String("content", resp.Content))
	return resp, context.Background(), nil
}

// getLoggingConfig reads logging configuration from environment variables with defaults
func getLoggingConfig() *logging.Config {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "debug"
	}

	format := os.Getenv("LOG_FORMAT")
	if format == "" {
		format = "console"
	}

	return &logging.Config{
		Level:  level,
		Format: format,
	}
}

func main() {
	// Initialize logging with configuration from environment variables
	err := logging.Init(getLoggingConfig())
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logging: %v", err))
	}

	serializer := &serializer.SymphonySerializer{}
	server, err := rpc.NewServer(":11000", serializer, nil)
	if err != nil {
		logging.Fatal("Failed to start server", zap.Error(err))
	}

	echo.RegisterEchoServiceServer(server, &echoServer{})
	logging.Info("Server starting on :11000")
	server.Start()
}
