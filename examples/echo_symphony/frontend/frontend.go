package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	echo "github.com/appnet-org/arpc/examples/echo_symphony/symphony"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/rpc/element"
	"github.com/appnet-org/arpc/pkg/serializer"
	"go.uber.org/zap"
)

var (
	echoClient   echo.EchoServiceClient
	elementTable map[string]func() element.RPCElement = map[string]func() element.RPCElement{
		"metrics":  NewMetricsElement,
		"firewall": NewFirewallElement,
	}
)

type RPCElementError struct {
	reason string
}

func (e *RPCElementError) Error() string {
	return e.reason
}

func handler(w http.ResponseWriter, r *http.Request) {
	message := r.URL.Query().Get("key")
	logging.Debug("Received HTTP request", zap.String("key", message))

	req := &echo.EchoRequest{
		Id:       42,
		Score:    100,
		Username: "alice",
		Content:  message,
	}
	resp, err := echoClient.Echo(context.Background(), req)

	if err != nil {
		if _, ok := err.(*RPCElementError); !ok {
			logging.Error("RPC call failed", zap.Error(err))
		}
		http.Error(w, fmt.Sprintf("RPC call failed: %v", err), http.StatusInternalServerError)
		return
	}

	logging.Debug("Got RPC response", zap.String("content", resp.Content))
	fmt.Fprintf(w, "Response from RPC: %s\n", resp.Content)
}

// getLoggingConfig reads logging configuration from environment variables with defaults
func getLoggingConfig() *logging.Config {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "info"
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

	// Create RPC client
	serializer := &serializer.SymphonySerializer{}

	var elementStr string
	var elements []string
	var rpcElements []element.RPCElement
	flag.StringVar(&elementStr, "element", "", "comma separated list of elements")
	flag.Parse()
	if elementStr == "" {
		elements = []string{}
	} else {
		elements = strings.Split(elementStr, ",")
	}
	for _, element := range elements {
		if _, ok := elementTable[element]; !ok {
			logging.Warn("Unrecognized element, skipped", zap.String("element", element))
			continue
		}
		rpcElements = append(rpcElements, elementTable[element]())
	}

	client, err := rpc.NewClient(serializer, ":11000", rpcElements) // TODO: change to your server's address fully qualified domain name
	if err != nil {
		logging.Fatal("Failed to create RPC client", zap.Error(err))
	}

	// Create EchoService client
	echoClient = echo.NewEchoServiceClient(client)

	// Set up HTTP server
	http.HandleFunc("/", handler)
	logging.Info("HTTP server listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logging.Fatal("HTTP server failed", zap.Error(err))
	}
}
