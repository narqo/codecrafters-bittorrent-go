package main

import (
	"encoding/binary"
	"io"
	"unsafe"
)

const protocolStrLen = 19

type handshake struct {
	// Tag is the length of the protocol string, always 19
	Tag byte
	// Proto is the protocol string "BitTorrent protocol"
	Proto [protocolStrLen]byte
	// Reserved is reserved bytes, which are all set to zero
	Reserved [8]byte
	// InfoHash is the info hash of the torrent
	InfoHash [20]byte
	// PeerID is the id of the peer
	PeerID [20]byte
}

func newHandshake(infoHash []byte) handshake {
	return handshake{
		Tag:      protocolStrLen,
		Proto:    [protocolStrLen]byte([]byte("BitTorrent protocol")),
		InfoHash: [20]byte(infoHash),
		// hard-coded for test implementation
		PeerID: [20]byte([]byte("00112233445566778899")),
	}
}

func (m handshake) WriteTo(w io.Writer) (int64, error) {
	err := binary.Write(w, binary.BigEndian, m)
	if err != nil {
		return 0, err
	}
	return int64(unsafe.Sizeof(m)), nil
}

func (m *handshake) ReadFrom(r io.Reader) (int64, error) {
	err := binary.Read(r, binary.BigEndian, m)
	if err != nil {
		return 0, err
	}
	return int64(unsafe.Sizeof(m)), nil
}
