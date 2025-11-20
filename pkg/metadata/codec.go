package metadata

import (
	"encoding/binary"
	"errors"
	"strings"

	"github.com/appnet-org/arpc/pkg/common"
)

type MetadataCodec struct{}

// EncodeHeaders serializes metadata to wire format: [count][kLen][key][vLen][value]...
// If pool is provided, it will be used to allocate the buffer; otherwise, a new buffer is allocated.
func (MetadataCodec) EncodeHeaders(md Metadata, pool *common.BufferPool) ([]byte, error) {
	// First compute total size
	totalSize := 2 // for count
	for k, v := range md {
		kb := []byte(strings.ToLower(k))
		vb := []byte(v)
		totalSize += 2 + len(kb) + 2 + len(vb)
	}

	var buf []byte
	if pool != nil {
		buf = pool.GetSize(totalSize)
	} else {
		buf = make([]byte, totalSize)
	}
	binary.LittleEndian.PutUint16(buf[0:2], uint16(len(md)))

	// Write pairs
	offset := 2
	for k, v := range md {
		kb := []byte(strings.ToLower(k))
		vb := []byte(v)

		binary.LittleEndian.PutUint16(buf[offset:offset+2], uint16(len(kb)))
		offset += 2
		copy(buf[offset:], kb)
		offset += len(kb)

		binary.LittleEndian.PutUint16(buf[offset:offset+2], uint16(len(vb)))
		offset += 2
		copy(buf[offset:], vb)
		offset += len(vb)
	}

	return buf, nil
}

// DecodeHeaders parses [count][kLen][key][vLen][value]... into Metadata
func (MetadataCodec) DecodeHeaders(data []byte) (Metadata, error) {
	if len(data) < 2 {
		return nil, errors.New("data too short for header count")
	}
	count := binary.LittleEndian.Uint16(data[0:2])

	md := make(Metadata, count)

	offset := 2
	for i := 0; i < int(count); i++ {
		if offset+2 > len(data) {
			return nil, errors.New("truncated before key length")
		}
		kLen := int(binary.LittleEndian.Uint16(data[offset:]))
		offset += 2

		if offset+kLen > len(data) {
			return nil, errors.New("truncated in key")
		}
		k := strings.ToLower(string(data[offset : offset+kLen]))
		offset += kLen

		if offset+2 > len(data) {
			return nil, errors.New("truncated before value length")
		}
		vLen := int(binary.LittleEndian.Uint16(data[offset:]))
		offset += 2

		if offset+vLen > len(data) {
			return nil, errors.New("truncated in value")
		}
		v := string(data[offset : offset+vLen])
		offset += vLen

		md[k] = v
	}

	return md, nil
}
