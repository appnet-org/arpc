package serializer

import "google.golang.org/protobuf/proto"

type ProtoSerializer struct{}

func (p *ProtoSerializer) Marshal(msg any) ([]byte, error) {
	return proto.Marshal(msg.(proto.Message))
}

func (p *ProtoSerializer) Unmarshal(data []byte, out any) error {
	return proto.Unmarshal(data, out.(proto.Message))
}
