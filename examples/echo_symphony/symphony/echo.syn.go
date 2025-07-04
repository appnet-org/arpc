// Code generated by protoc-gen-symphony. DO NOT EDIT.
package echo

import (
	"bytes"
	"encoding/binary"
)

func (m *EchoRequest) MarshalSymphony() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(0x00) // layout header
	buf.Write([]byte{1, 2, 3, 4})
	offset := 0
	offset += 4
	offset += 4
	binary.Write(&buf, binary.LittleEndian, byte(3))
	binary.Write(&buf, binary.LittleEndian, uint16(offset)) // offset of Username
	binary.Write(&buf, binary.LittleEndian, uint16(len(m.Username)))
	offset += len(m.Username)
	binary.Write(&buf, binary.LittleEndian, byte(4))
	binary.Write(&buf, binary.LittleEndian, uint16(offset)) // offset of Content
	binary.Write(&buf, binary.LittleEndian, uint16(len(m.Content)))
	offset += len(m.Content)
	binary.Write(&buf, binary.LittleEndian, m.Id)
	binary.Write(&buf, binary.LittleEndian, m.Score)
	buf.Write([]byte(m.Username))
	buf.Write([]byte(m.Content))
	return buf.Bytes(), nil
}

func (m *EchoRequest) UnmarshalSymphony(data []byte) error {
	reader := bytes.NewReader(data)
	var header byte
	if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
		return err
	}
	fieldOrder := make([]byte, 4)
	if _, err := reader.Read(fieldOrder); err != nil {
		return err
	}
	type offsetEntry struct{ offset, length uint16 }
	offsets := map[byte]offsetEntry{}
	for i := 0; i < 2; i++ {
		var fieldID byte
		var off, len uint16
		if err := binary.Read(reader, binary.LittleEndian, &fieldID); err != nil {
			return err
		}
		if err := binary.Read(reader, binary.LittleEndian, &off); err != nil {
			return err
		}
		if err := binary.Read(reader, binary.LittleEndian, &len); err != nil {
			return err
		}
		offsets[fieldID] = offsetEntry{off, len}
	}
	dataRegion := data[len(data)-reader.Len():]
	if err := binary.Read(bytes.NewReader(dataRegion[0:4]), binary.LittleEndian, &m.Id); err != nil {
		return err
	}
	if err := binary.Read(bytes.NewReader(dataRegion[4:8]), binary.LittleEndian, &m.Score); err != nil {
		return err
	}
	if entry, ok := offsets[3]; ok {
		m.Username = string(dataRegion[entry.offset : entry.offset+entry.length])
	}
	if entry, ok := offsets[4]; ok {
		m.Content = string(dataRegion[entry.offset : entry.offset+entry.length])
	}
	return nil
}

func (m *EchoResponse) MarshalSymphony() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(0x00) // layout header
	buf.Write([]byte{1, 2, 3, 4})
	offset := 0
	offset += 4
	offset += 4
	binary.Write(&buf, binary.LittleEndian, byte(3))
	binary.Write(&buf, binary.LittleEndian, uint16(offset)) // offset of Username
	binary.Write(&buf, binary.LittleEndian, uint16(len(m.Username)))
	offset += len(m.Username)
	binary.Write(&buf, binary.LittleEndian, byte(4))
	binary.Write(&buf, binary.LittleEndian, uint16(offset)) // offset of Content
	binary.Write(&buf, binary.LittleEndian, uint16(len(m.Content)))
	offset += len(m.Content)
	binary.Write(&buf, binary.LittleEndian, m.Id)
	binary.Write(&buf, binary.LittleEndian, m.Score)
	buf.Write([]byte(m.Username))
	buf.Write([]byte(m.Content))
	return buf.Bytes(), nil
}

func (m *EchoResponse) UnmarshalSymphony(data []byte) error {
	reader := bytes.NewReader(data)
	var header byte
	if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
		return err
	}
	fieldOrder := make([]byte, 4)
	if _, err := reader.Read(fieldOrder); err != nil {
		return err
	}
	type offsetEntry struct{ offset, length uint16 }
	offsets := map[byte]offsetEntry{}
	for i := 0; i < 2; i++ {
		var fieldID byte
		var off, len uint16
		if err := binary.Read(reader, binary.LittleEndian, &fieldID); err != nil {
			return err
		}
		if err := binary.Read(reader, binary.LittleEndian, &off); err != nil {
			return err
		}
		if err := binary.Read(reader, binary.LittleEndian, &len); err != nil {
			return err
		}
		offsets[fieldID] = offsetEntry{off, len}
	}
	dataRegion := data[len(data)-reader.Len():]
	if err := binary.Read(bytes.NewReader(dataRegion[0:4]), binary.LittleEndian, &m.Id); err != nil {
		return err
	}
	if err := binary.Read(bytes.NewReader(dataRegion[4:8]), binary.LittleEndian, &m.Score); err != nil {
		return err
	}
	if entry, ok := offsets[3]; ok {
		m.Username = string(dataRegion[entry.offset : entry.offset+entry.length])
	}
	if entry, ok := offsets[4]; ok {
		m.Content = string(dataRegion[entry.offset : entry.offset+entry.length])
	}
	return nil
}
