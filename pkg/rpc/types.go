package rpc

type RPCErrorType struct {
	Name string
}

var (
	// RPCUnknownError represents an unexpected error, which is likely a bug in the aRPC library.
	// However, it's possible that the error is caused by network issues or other external factors.
	RPCUnknownError = RPCErrorType{Name: "unknown"}
	// RPCFailError represents an error correctly handled by the aRPC library, such as
	// (1) service/method not found
	// (2) RPC rejected by element policy
	RPCFailError = RPCErrorType{Name: "fail"}
)

type RPCError struct {
	Type   RPCErrorType
	Reason string
}

func (e *RPCError) Error() string {
	return e.Reason
}
