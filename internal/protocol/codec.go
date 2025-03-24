package protocol

import (
	"bytes"
	"encoding/binary"
)


func EncodeMessage(msg *RPCMessage) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, msg.ID); err != nil {
		return nil, err
	}
	methodBytes := []byte(msg.Method)
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(methodBytes))); err != nil {
		return nil, err
	}
	if _, err := buf.Write(methodBytes); err != nil {
		return nil, err
	}
	if _, err := buf.Write(msg.Payload); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DecodeMessage(data []byte) (*RPCMessage, error) {
	buf := bytes.NewReader(data)
	msg := &RPCMessage{}
	if err := binary.Read(buf, binary.LittleEndian, &msg.ID); err != nil {
		return nil, err
	}
	var methodLen uint16
	if err := binary.Read(buf, binary.LittleEndian, &methodLen); err != nil {
		return nil, err
	}
	methodBytes := make([]byte, methodLen)
	if _, err := buf.Read(methodBytes); err != nil {
		return nil, err
	}
	msg.Method = string(methodBytes)
	msg.Payload = make([]byte, buf.Len())
	if _, err := buf.Read(msg.Payload); err != nil {
		return nil, err
	}
	return msg, nil
}