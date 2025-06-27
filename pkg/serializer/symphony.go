package serializer

type SymphonyMessage interface {
	MarshalSymphony() ([]byte, error)
	UnmarshalSymphony([]byte) error
}

type SymphonySerializer struct{}

func (s *SymphonySerializer) Marshal(msg any) ([]byte, error) {
	return msg.(SymphonyMessage).MarshalSymphony()
}

func (s *SymphonySerializer) Unmarshal(data []byte, out any) error {
	return out.(SymphonyMessage).UnmarshalSymphony(data)
}
