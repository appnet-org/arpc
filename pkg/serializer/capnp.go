package serializer

import (
	"capnproto.org/go/capnp/v3"
)

type CapnpSerializer struct{}

func (c *CapnpSerializer) Marshal(msg any) ([]byte, error) {
	payload, err := msg.(*capnp.Message).Marshal()
	return payload, err
}

func (c *CapnpSerializer) Unmarshal(data []byte, out any) error {
	msg, err := capnp.Unmarshal(data)
	if err != nil {
		return err
	}
	*out.(**capnp.Message) = msg
	return err
}
