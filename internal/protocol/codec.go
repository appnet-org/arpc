package protocol

import (
	"bytes"
	"encoding/binary"
)

// EncodeMessage serializes an RPCMessage into a byte slice for transmission over UDP.
// The format is:
// - 8 bytes: Message ID (uint64, Little Endian)
// - 2 bytes: Method name length (uint16, Little Endian)
// - N bytes: Method name (raw bytes)
// - M bytes: Payload (remaining data)
func EncodeMessage(msg *RPCMessage) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write message ID (8 bytes)
	if err := binary.Write(buf, binary.LittleEndian, msg.ID); err != nil {
		return nil, err
	}

	// Convert method name to bytes and write its length (2 bytes)
	methodBytes := []byte(msg.Method)
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(methodBytes))); err != nil {
		return nil, err
	}

	// Write method name bytes
	if _, err := buf.Write(methodBytes); err != nil {
		return nil, err
	}

	// Write payload bytes
	if _, err := buf.Write(msg.Payload); err != nil {
		return nil, err
	}

	// Return the serialized message
	return buf.Bytes(), nil
}

// DecodeMessage deserializes a byte slice back into an RPCMessage.
// It follows the same format as EncodeMessage.
func DecodeMessage(data []byte) (*RPCMessage, error) {
	buf := bytes.NewReader(data)
	msg := &RPCMessage{}

	// Read message ID (8 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &msg.ID); err != nil {
		return nil, err
	}

	// Read method name length (2 bytes)
	var methodLen uint16
	if err := binary.Read(buf, binary.LittleEndian, &methodLen); err != nil {
		return nil, err
	}

	// Read method name bytes
	methodBytes := make([]byte, methodLen)
	if _, err := buf.Read(methodBytes); err != nil {
		return nil, err
	}
	msg.Method = string(methodBytes)

	// Read remaining payload bytes
	msg.Payload = make([]byte, buf.Len())
	if _, err := buf.Read(msg.Payload); err != nil {
		return nil, err
	}

	// Return the decoded message
	return msg, nil
}
