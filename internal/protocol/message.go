package protocol

type RPCMessage struct {
	ID      uint64
	Method  string
	Payload []byte
}