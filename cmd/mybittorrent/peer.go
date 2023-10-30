package main

import (
	"fmt"
	"io"
	"sync"
)

type Peer struct {
	conn io.ReadWriter

	mu  sync.Mutex
	msg message
}

func NewPeer(conn io.ReadWriter) *Peer {
	p := &Peer{
		conn: conn,
	}
	p.msg.payload = p.msg.buf[:]
	return p
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

func (p *Peer) Recv(typ MessageType) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

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
		return p.msg.payload, nil
	}
}

func (p *Peer) Send(typ MessageType, m MessagePayload) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	msg := p.msg
	msg.typ = typ

	var n int
	if m != nil {
		var err error
		n, err = m.Read(msg.payload[:maxPayloadSize])
		if err != nil {
			return err
		}
	}
	msg.length = uint32(1 + n)

	_, err := msg.WriteTo(p.conn)
	return err
}
