package main

import (
	"encoding/binary"
	"fmt"
	"io"
)

type MessageType uint8

const (
	Choke         MessageType = 0
	Unchoke       MessageType = 1
	Interested    MessageType = 2
	NotInterested MessageType = 3
	Have          MessageType = 4
	Bitfield      MessageType = 5
	Request       MessageType = 6
	Piece         MessageType = 7
	Cancel        MessageType = 8
)

const (
	maxBlockSize   = 1 << 14
	maxMessageSize = maxBlockSize + 8 + 8
)

type message struct {
	length uint32
	typ    MessageType
	buf    [maxMessageSize]byte

	Payload []byte
}

func (m *message) Recv(r io.Reader, typ MessageType) error {
	for {
		_, err := m.ReadFrom(r)
		if err == io.EOF {
			continue
		}
		if err != nil {
			return err
		}
		if m.typ != typ {
			return fmt.Errorf("recv: expected type %v, got %v", typ, m.typ)
		}
		return nil
	}
}

func (m *message) Send(w io.Writer, typ MessageType, payload []byte) error {
	m.length = uint32(1 + len(payload))
	m.typ = typ
	m.Payload = payload
	_, err := m.WriteTo(w)
	return err
}

func (m *message) WriteTo(w io.Writer) (int64, error) {
	binary.BigEndian.PutUint32(m.buf[:4], m.length)
	m.buf[4] = byte(m.typ)

	_, err := w.Write(m.buf[:5])
	if err != nil {
		return 0, err
	}

	if m.Payload == nil {
		return 5, nil
	}

	n, err := w.Write(m.Payload)
	if err != nil {
		return 0, err
	}
	return int64(5 + n), nil
}

func (m *message) packUint32(v ...uint32) []byte {
	for n, val := range v {
		// first 5 bytes in buf are used for message header
		binary.BigEndian.PutUint32(m.buf[5+n*4:], val)
	}
	return m.buf[5 : 5+len(v)*4]
}

func (m *message) ReadFrom(r io.Reader) (int64, error) {
	_, err := io.ReadFull(r, m.buf[:5])
	if err != nil {
		return 0, err
	}
	m.length = binary.BigEndian.Uint32(m.buf[:4])
	m.typ = MessageType(m.buf[4])

	m.Payload = m.buf[:m.length-1]
	n, err := io.ReadFull(r, m.Payload)
	if err != nil {
		return 0, err
	}
	return int64(5 + n), nil
}
