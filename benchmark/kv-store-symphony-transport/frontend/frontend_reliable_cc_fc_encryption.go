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
	"github.com/appnet-org/arpc/pkg/custom/flowcontrol"
	"github.com/appnet-org/arpc/pkg/custom/reliable"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/serializer"
	"github.com/appnet-org/arpc/pkg/transport"
	"go.uber.org/zap"
)

// Deterministic random string generator from key_id and desired length
func generateDeterministicStringReliableCCFCEncryption(keyID string, length int) string {
	hash := sha256.Sum256([]byte(keyID))
	repeatCount := (length + len(hash)*2 - 1) / (len(hash) * 2)
	hexStr := strings.Repeat(hex.EncodeToString(hash[:]), repeatCount)
	return hexStr[:length]
}

var kvClientReliableCCFCEncryption kv.KVServiceClient

// getLoggingConfig reads logging configuration from environment variables with defaults
func getLoggingConfigReliableCCFCEncryption() *logging.Config {
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

func handlerReliableCCFCEncryption(w http.ResponseWriter, r *http.Request) {
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
	keyStr := generateDeterministicStringReliableCCFCEncryption(keyID+"-key", keySize)
	valueStr := generateDeterministicStringReliableCCFCEncryption(keyID+"-value", valueSize)

	logging.Debug("Received HTTP request",
		zap.String("op", op),
		zap.String("key_id", keyID),
		zap.Int("key_size", keySize),
		zap.Int("value_size", valueSize),
	)

	switch op {
	case "get":
		req := &kv.GetRequest{Key: keyStr}
		resp, err := kvClientReliableCCFCEncryption.Get(context.Background(), req)
		if err != nil {
			logging.Error("Get RPC call failed", zap.Error(err))
			http.Error(w, fmt.Sprintf("Get RPC failed: %v", err), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Value for key_id '%s' (key='%s'): %s\n", keyID, keyStr, resp.Value)

	case "set":
		req := &kv.SetRequest{Key: keyStr, Value: valueStr}
		resp, err := kvClientReliableCCFCEncryption.Set(context.Background(), req)
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
	err := logging.Init(getLoggingConfigReliableCCFCEncryption())
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logging: %v", err))
	}

	// Log startup to verify correct binary is running
	logging.Info("Starting KV client with ENCRYPTION enabled", zap.String("variant", "reliable-cc-fc-encryption"))

	// Create RPC client (creates UDP transport internally)
	serializer := &serializer.SymphonySerializer{}
	// client, err := rpc.NewClient(serializer, ":11000", nil, true)
	enableEncryption := true
	logging.Info("Calling NewClient with encryption parameter", zap.Bool("enableEncryption", enableEncryption))
	client, err := rpc.NewClient(serializer, "kvstore.default.svc.cluster.local:11000", nil, enableEncryption)
	if err != nil {
		logging.Fatal("Failed to create RPC client", zap.Error(err))
	}

	// Get the UDP transport from the client
	udpTransport := client.Transport()
	defer udpTransport.Close()

	// Verify encryption is enabled (should have been enabled by NewClient with true parameter)
	logging.Info("KV client initialized",
		zap.String("clientType", "reliable-cc-fc-encryption"),
		zap.Bool("encryptionRequested", true),
		zap.Bool("encryptionEnabled", udpTransport.IsEncryptionEnabled()))

	// Register ACK packet type
	ackPacketType, err := udpTransport.RegisterPacketType(reliable.AckPacketName, &reliable.ACKPacketCodec{})
	if err != nil {
		logging.Fatal("Failed to register ACK packet type", zap.Error(err))
	}

	// Register CCFeedback packet type
	ccFeedbackPacketType, err := udpTransport.RegisterPacketType(congestion.CCFeedbackPacketName, &congestion.CCFeedbackCodec{})
	if err != nil {
		logging.Fatal("Failed to register CCFeedback packet type", zap.Error(err))
	}

	// Register FCFeedback packet type
	fcFeedbackPacketType, err := udpTransport.RegisterPacketType(flowcontrol.FCFeedbackPacketName, &flowcontrol.FCFeedbackCodec{})
	if err != nil {
		logging.Fatal("Failed to register FCFeedback packet type", zap.Error(err))
	}

	// Create reliable client handler
	reliableHandler := reliable.NewReliableClientHandler(
		udpTransport,
		udpTransport.GetTimerManager(),
	)
	defer reliableHandler.Cleanup()

	// Create congestion control client handler
	ccHandler := congestion.NewCCClientHandler(
		udpTransport,
		udpTransport.GetTimerManager(),
	)
	defer ccHandler.Cleanup()

	// Create flow control client handler
	fcHandler := flowcontrol.NewFCClientHandler(
		udpTransport,
		udpTransport.GetTimerManager(),
	)
	defer fcHandler.Cleanup()

	// Register for REQUEST packets (OnSend)
	// All handlers need to process REQUEST packets
	requestChain, exists := udpTransport.GetHandlerRegistry().GetHandlerChain(
		packet.PacketTypeRequest.TypeID,
		transport.RoleClient,
	)
	if !exists {
		logging.Fatal("Failed to get REQUEST handler chain")
	}
	// Add CC/FC handlers first (for congestion/flow control checks), then reliable handler
	requestChain.AddHandler(ccHandler)
	requestChain.AddHandler(fcHandler)
	requestChain.AddHandler(reliableHandler)

	// Register for RESPONSE packets (OnReceive)
	// All handlers need to process RESPONSE packets
	responseChain, exists := udpTransport.GetHandlerRegistry().GetHandlerChain(
		packet.PacketTypeResponse.TypeID,
		transport.RoleClient,
	)
	if !exists {
		logging.Fatal("Failed to get RESPONSE handler chain")
	}
	// Add reliable handler first (for ACK), then CC/FC handlers (for feedback)
	responseChain.AddHandler(reliableHandler)
	responseChain.AddHandler(ccHandler)
	responseChain.AddHandler(fcHandler)

	// Register handler chain for ACK packets
	// ACK packets use the same handler chain for both send and receive on client
	ackChain := transport.NewHandlerChain("ClientACKHandlerChain", reliableHandler)
	udpTransport.RegisterHandlerChain(ackPacketType.TypeID, ackChain, transport.RoleClient)

	// Register handler chain for CCFeedback packets
	// CCFeedback packets use the same handler chain for both send and receive on client
	ccFeedbackChain := transport.NewHandlerChain("ClientCCFeedbackHandlerChain", ccHandler)
	udpTransport.RegisterHandlerChain(ccFeedbackPacketType.TypeID, ccFeedbackChain, transport.RoleClient)

	// Register handler chain for FCFeedback packets
	// FCFeedback packets use the same handler chain for both send and receive on client
	fcFeedbackChain := transport.NewHandlerChain("ClientFCFeedbackHandlerChain", fcHandler)
	udpTransport.RegisterHandlerChain(fcFeedbackPacketType.TypeID, fcFeedbackChain, transport.RoleClient)

	kvClientReliableCCFCEncryption = kv.NewKVServiceClient(client)

	http.HandleFunc("/", handlerReliableCCFCEncryption)
	logging.Info("HTTP server listening (reliable + congestion control + flow control mode)", zap.String("addr", ":8080"))
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logging.Fatal("HTTP server failed", zap.Error(err))
	}
}
