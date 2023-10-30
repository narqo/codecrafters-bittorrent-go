package main

import (
	"encoding/binary"
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
	maxPayloadSize = maxBlockSize + 8 + 8
)

type MessagePayload interface {
	io.Reader
}

type ZeroPayload struct{}

func (p ZeroPayload) Read(b []byte) (int, error) {
	return 0, nil
}

type RawPayload []byte

func (p RawPayload) Read(b []byte) (int, error) {
	if len(b) < len([]byte(p)) {
		return 0, io.ErrShortBuffer
	}
	n := copy(b, []byte(p))
	return n, nil
}

type RequestPayload struct {
	Piece uint32
	Begin uint32
	BLen  uint32
}

func (p RequestPayload) Read(b []byte) (int, error) {
	if len(b) < 3*4 {
		return 0, io.ErrShortBuffer
	}
	packUint32(b, p.Piece, p.Begin, p.BLen)
	return 3 * 4, nil
}

func packUint32(buf []byte, v ...uint32) {
	for n, val := range v {
		binary.BigEndian.PutUint32(buf[n*4:], val)
	}
}

type message struct {
	length  uint32
	typ     MessageType
	payload []byte
	buf     [maxPayloadSize]byte
}

func (m *message) WriteTo(w io.Writer) (int64, error) {
	var buf [5]byte
	binary.BigEndian.PutUint32(buf[:], m.length)
	buf[4] = byte(m.typ)

	_, err := w.Write(buf[:])
	if err != nil {
		return 0, err
	}

	// fast exit if m doesn't have payload
	if m.length == 1 {
		return 5, nil
	}

	n, err := w.Write(m.payload[:m.length-1])
	if err != nil {
		return 0, err
	}
	return int64(5 + n), nil
}

func (m *message) ReadFrom(r io.Reader) (int64, error) {
	var buf [5]byte
	_, err := io.ReadFull(r, buf[:])
	if err != nil {
		return 0, err
	}
	m.length = binary.BigEndian.Uint32(buf[:])
	m.typ = MessageType(buf[4])

	m.payload = m.buf[:m.length-1]
	n, err := io.ReadFull(r, m.payload)
	if err != nil {
		return 0, err
	}
	return int64(len(buf) + n), nil
}
