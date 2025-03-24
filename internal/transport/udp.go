package transport

import (
	"net"
	"time"
)

type UDPTransport struct {
	conn *net.UDPConn
}

func NewUDPTransport(address string) (*UDPTransport, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	return &UDPTransport{conn: conn}, nil
}

func (t *UDPTransport) Send(addr string, data []byte) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	_, err = t.conn.WriteToUDP(data, udpAddr)
	return err
}

func (t *UDPTransport) Receive(bufferSize int) ([]byte, *net.UDPAddr, error) {
	buffer := make([]byte, bufferSize)
	n, addr, err := t.conn.ReadFromUDP(buffer)
	if err != nil {
		return nil, nil, err
	}

	return buffer[:n], addr, nil
}

func (t *UDPTransport) Close() error {
	return t.conn.Close()
}