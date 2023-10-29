package main

import (
	"fmt"
	"io"
)

type Peer struct {
	conn io.ReadWriter
	msg  message
}

func NewPeer(conn io.ReadWriter) *Peer {
	return &Peer{
		conn: conn,
	}
}

func (p *Peer) Handshake(infoHash []byte) ([]byte, error) {
	hsk := newHandshake(infoHash)

	_, err := hsk.WriteTo(p.conn)
	if err != nil {
		return nil, err
	}

	_, err = hsk.ReadFrom(p.conn)
	if err != nil {
		return nil, err
	}

	if hsk.Tag != protocolStrLen {
		return nil, fmt.Errorf("handshake: unexpected tag %v", hsk.Tag)
	}

	return hsk.PeerID[:], nil
}

func (p *Peer) Request(piece int, begin, blen uint32) error {
	payload := p.msg.packUint32(uint32(piece), begin, blen)
	return p.Send(Request, payload)
}

func (p *Peer) Recv(typ MessageType) ([]byte, error) {
	for {
		_, err := p.msg.ReadFrom(p.conn)
		if err == io.EOF {
			continue
		}
		if err != nil {
			return nil, err
		}
		if p.msg.typ != typ {
			return nil, fmt.Errorf("recv: expected type %v, got %v", typ, p.msg.typ)
		}
		return p.msg.Payload, nil
	}
}

func (p *Peer) Send(typ MessageType, payload []byte) error {
	m := p.msg
	m.length = uint32(1 + len(payload))
	m.typ = typ
	m.Payload = payload
	_, err := m.WriteTo(p.conn)
	return err
}
