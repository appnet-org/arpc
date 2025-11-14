package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	kv "github.com/appnet-org/arpc/benchmark/kv-store-symphony-transport/symphony"
	"github.com/appnet-org/arpc/pkg/custom/congestion"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/serializer"
	"github.com/appnet-org/arpc/pkg/transport"
	"go.uber.org/zap"
)

// Deterministic random string generator from key_id and desired length
func generateDeterministicStringCC(keyID string, length int) string {
	hash := sha256.Sum256([]byte(keyID))
	repeatCount := (length + len(hash)*2 - 1) / (len(hash) * 2)
	hexStr := strings.Repeat(hex.EncodeToString(hash[:]), repeatCount)
	return hexStr[:length]
}

var kvClientCC kv.KVServiceClient

// getLoggingConfig reads logging configuration from environment variables with defaults
func getLoggingConfigCC() *logging.Config {
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

func handlerCC(w http.ResponseWriter, r *http.Request) {
	op := strings.ToLower(r.URL.Query().Get("op"))
	keyID := r.URL.Query().Get("key")
	keySizeStr := r.URL.Query().Get("key_size")
	valueSizeStr := r.URL.Query().Get("value_size")

	keySize, _ := strconv.Atoi(keySizeStr)
	valueSize, _ := strconv.Atoi(valueSizeStr)

	if keyID == "" {
		http.Error(w, "key parameter is required", http.StatusBadRequest)
		return
	}

	// Generate deterministic key/value strings
	keyStr := generateDeterministicStringCC(keyID+"-key", keySize)
	valueStr := generateDeterministicStringCC(keyID+"-value", valueSize)

	logging.Debug("Received HTTP request",
		zap.String("op", op),
		zap.String("key_id", keyID),
		zap.Int("key_size", keySize),
		zap.Int("value_size", valueSize),
	)

	switch op {
	case "get":
		req := &kv.GetRequest{Key: keyStr}
		resp, err := kvClientCC.Get(context.Background(), req)
		if err != nil {
			logging.Error("Get RPC call failed", zap.Error(err))
			http.Error(w, fmt.Sprintf("Get RPC failed: %v", err), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Value for key_id '%s' (key='%s'): %s\n", keyID, keyStr, resp.Value)

	case "set":
		req := &kv.SetRequest{Key: keyStr, Value: valueStr}
		resp, err := kvClientCC.Set(context.Background(), req)
		if err != nil {
			logging.Error("Set RPC call failed", zap.Error(err))
			http.Error(w, fmt.Sprintf("Set RPC failed: %v", err), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Set key_id '%s' (key='%s') to value='%s'. Response: %s\n",
			keyID, keyStr, valueStr, resp.Value)

	default:
		http.Error(w, "Invalid operation. Use op=GET or op=SET", http.StatusBadRequest)
	}
}

func main() {
	err := logging.Init(getLoggingConfigCC())
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logging: %v", err))
	}

	// Create RPC client (creates UDP transport internally)
	serializer := &serializer.SymphonySerializer{}
	// client, err := rpc.NewClient(serializer, "localhost:11000", nil)
	client, err := rpc.NewClient(serializer, "kvstore.default.svc.cluster.local:11000", nil)
	if err != nil {
		logging.Fatal("Failed to create RPC client", zap.Error(err))
	}

	// Get the UDP transport from the client
	udpTransport := client.Transport()
	defer udpTransport.Close()

	// Register CCFeedback packet type
	ccFeedbackPacketType, err := udpTransport.RegisterPacketType(congestion.CCFeedbackPacketName, &congestion.CCFeedbackCodec{})
	if err != nil {
		logging.Fatal("Failed to register CCFeedback packet type", zap.Error(err))
	}

	// Create congestion control client handler
	clientHandler := congestion.NewCCClientHandler(
		udpTransport,
		udpTransport.GetTimerManager(),
	)
	defer clientHandler.Cleanup()

	// Register for REQUEST packets (OnSend)
	requestChain, exists := udpTransport.GetHandlerRegistry().GetHandlerChain(
		packet.PacketTypeRequest.TypeID,
		transport.RoleClient,
	)
	if !exists {
		logging.Fatal("Failed to get REQUEST handler chain")
	}
	requestChain.AddHandler(clientHandler)

	// Register for RESPONSE packets (OnReceive)
	responseChain, exists := udpTransport.GetHandlerRegistry().GetHandlerChain(
		packet.PacketTypeResponse.TypeID,
		transport.RoleClient,
	)
	if !exists {
		logging.Fatal("Failed to get RESPONSE handler chain")
	}
	responseChain.AddHandler(clientHandler)

	// Register handler chain for CCFeedback packets
	// CCFeedback packets use the same handler chain for both send and receive on client
	ccFeedbackChain := transport.NewHandlerChain("ClientCCFeedbackHandlerChain", clientHandler)
	udpTransport.RegisterHandlerChain(ccFeedbackPacketType.TypeID, ccFeedbackChain, transport.RoleClient)

	kvClientCC = kv.NewKVServiceClient(client)

	http.HandleFunc("/", handlerCC)
	logging.Info("HTTP server listening (congestion control mode)", zap.String("addr", ":8080"))
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logging.Fatal("HTTP server failed", zap.Error(err))
	}
}
