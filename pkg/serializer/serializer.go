package serializer

type Serializer interface {
	Marshal(msg any) ([]byte, error)
	Unmarshal(data []byte, out any) error
}
