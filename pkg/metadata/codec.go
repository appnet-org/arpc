package metadata

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

type MetadataCodec struct{}

// EncodeHeaders serializes metadata to the wire format: [count][kLen][key][vLen][value]...
func (MetadataCodec) EncodeHeaders(md Metadata) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write number of headers
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(md))); err != nil {
		return nil, err
	}

	// Write each key-value pair
	for k, v := range md {
		kb := []byte(strings.ToLower(k))
		vb := []byte(v)

		if err := binary.Write(buf, binary.LittleEndian, uint16(len(kb))); err != nil {
			return nil, err
		}
		if _, err := buf.Write(kb); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, uint16(len(vb))); err != nil {
			return nil, err
		}
		if _, err := buf.Write(vb); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// DecodeHeaders parses the [count][kLen][key][vLen][value]... wire format into Metadata
func (MetadataCodec) DecodeHeaders(data []byte) (Metadata, error) {
	md := Metadata{}
	buf := bytes.NewReader(data)

	var count uint16
	if err := binary.Read(buf, binary.LittleEndian, &count); err != nil {
		return nil, fmt.Errorf("failed to read header count: %w", err)
	}

	for i := 0; i < int(count); i++ {
		var kLen uint16
		if err := binary.Read(buf, binary.LittleEndian, &kLen); err != nil {
			return nil, err
		}
		k := make([]byte, kLen)
		if _, err := buf.Read(k); err != nil {
			return nil, err
		}

		var vLen uint16
		if err := binary.Read(buf, binary.LittleEndian, &vLen); err != nil {
			return nil, err
		}
		v := make([]byte, vLen)
		if _, err := buf.Read(v); err != nil {
			return nil, err
		}

		md[strings.ToLower(string(k))] = string(v)
	}

	return md, nil
}
