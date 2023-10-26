package main

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"

	bencode "github.com/jackpal/bencode-go"
)

// Example:
// - 5:hello -> hello
// - 10:hello12345 -> hello12345
func decodeBencode(str string) (interface{}, string, error) {
	tag := str[0]
	switch {
	case unicode.IsDigit(rune(tag)):
		head, tail, ok := strings.Cut(str, ":")
		if !ok {
			return nil, "", fmt.Errorf("can't find colon in %q", str)
		}

		slen, err := strconv.Atoi(head)
		if err != nil {
			return "", "", err
		}
		return tail[:slen], tail[slen:], nil
	case tag == 'i':
		head, tail, ok := strings.Cut(str[1:], "e")
		if !ok {
			return nil, "", fmt.Errorf("can't find end of %q", str)
		}
		n, err := strconv.Atoi(head)
		return n, tail, err
	case tag == 'l':
		var list []interface{}
		str = str[1:]
		for {
			var (
				v   interface{}
				err error
			)
			v, str, err = decodeBencode(str)
			if err != nil {
				return nil, str, err
			}
			list = append(list, v)
			// consume the end of the list and exit
			if str[0] == 'e' {
				str = str[1:]
				break
			}
		}
		return list, str, nil
	case tag == 'd':
		dict := make(map[string]interface{}, 0)
		str = str[1:]
		for {
			var (
				v   interface{}
				err error
			)
			v, str, err = decodeBencode(str)
			if err != nil {
				return nil, str, err
			}
			k, ok := v.(string)
			if !ok {
				return nil, str, fmt.Errorf("key must be string, got %T (%v)", v, v)
			}
			v, str, err = decodeBencode(str)
			if err != nil {
				return nil, str, err
			}
			dict[k] = v
			// consume the end of the list and exit
			if str[0] == 'e' {
				str = str[1:]
				break
			}
		}
		return dict, str, nil
	default:
		return "", str, errors.ErrUnsupported
	}
}

func main() {
	switch cmd := os.Args[1]; cmd {
	case "decode":
		input := os.Args[2]
		val, tail, err := decodeBencode(input)
		if err != nil {
			fmt.Printf("%s: %s", tail, err)
			return
		}
		if tail != "" {
			fmt.Printf("didn't consume the whole input: tail %q", tail)
			return
		}

		out, _ := json.Marshal(val)
		fmt.Println(string(out))
	case "info":
		filePath := os.Args[2]
		f, err := os.Open(filePath)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		var t Tracker
		err = bencode.Unmarshal(f, &t)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Tracker URL: %s\n", t.Announce)
		fmt.Printf("Length: %d\n", t.Info.Length)

		infoHash, err := t.InfoHash()
		if err != nil {
			panic(err)
		}

		fmt.Printf("Info Hash: %x\n", infoHash)
		fmt.Printf("Piece Length: %d\n", t.Info.PieceLength)

		fmt.Printf("Piece Hashes:\n")
		for p := []byte(t.Info.Pieces); len(p) > 0; p = p[20:] {
			fmt.Printf("%x\n", p[:20])
		}
	case "peers":
		filePath := os.Args[2]
		f, err := os.Open(filePath)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		var t Tracker
		err = bencode.Unmarshal(f, &t)
		if err != nil {
			panic(err)
		}

		turl, err := trackerURLFrom(t)
		if err != nil {
			panic(err)
		}

		httpResp, err := http.Get(turl)
		if err != nil {
			panic(err)
		}
		defer httpResp.Body.Close()

		var trackerResp trackerResponse
		err = bencode.Unmarshal(httpResp.Body, &trackerResp)
		if err != nil {
			panic(err)
		}

		for _, peer := range trackerResp.Peers() {
			fmt.Println(peer.String())
		}
	default:
		fmt.Println("Unknown command: " + cmd)
		os.Exit(1)
	}
}

type Tracker struct {
	// Announce is a URL to a tracker.
	Announce string
	// Info contains metainfo of a tracker.
	Info TrackerInfo
}

type TrackerInfo struct {
	// Name is a suggested name to save the file or directory as.
	Name string `bencode:"name"`
	// PieceLength is the number of bytes in each piece the file is split into.
	PieceLength int64 `bencode:"piece length"`
	// Pieces is a string of multiple of 20. It is to be subdivided into strings of length 20,
	// each of which is the SHA1 hash of the piece at the corresponding index.
	Pieces string `bencode:"pieces"`
	// Length is the size of the file in bytes, for single-file torrents
	Length uint64 `bencode:"length"`
}

func (t Tracker) InfoHash() ([]byte, error) {
	h := sha1.New()
	err := bencode.Marshal(h, t.Info)
	if err != nil {
		return nil, err
	}
	ret := h.Sum(nil)
	return ret[:], nil
}

func trackerURLFrom(t Tracker) (string, error) {
	u, err := url.Parse(t.Announce)
	if err != nil {
		return "", nil
	}

	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", nil
	}
	// the info hash of the torrent file
	infoHash, err := t.InfoHash()
	if err != nil {
		return "", nil
	}
	q.Add("info_hash", string(infoHash))
	// a unique identifier for your client
	q.Add("peer_id", "00112233445566778899")
	// the port your client is listening on
	q.Add("port", "6881")
	// the total amount uploaded so far (always 0)
	q.Add("uploaded", "0")
	// the total amount downloaded (always 0)
	q.Add("downloaded", "0")
	// the number of bytes left to download
	left := strconv.FormatUint(t.Info.Length, 10)
	q.Add("left", left)
	// whether the peer list should use the compact representation (always 1)
	q.Add("compact", "1")

	u.RawQuery = q.Encode()

	return u.String(), nil
}

type trackerResponse struct {
	Interval uint64 `bencode:"interval"`
	RawPeers string `bencode:"peers"`
}

func (tr trackerResponse) Peers() []netip.AddrPort {
	rawPeers := []byte(tr.RawPeers)
	peers := make([]netip.AddrPort, 0, len(rawPeers)/6)
	for len(rawPeers) > 0 {
		addr, _ := netip.AddrFromSlice(rawPeers[:4])
		port := binary.BigEndian.Uint16(rawPeers[4:])
		peers = append(peers, netip.AddrPortFrom(addr, port))
		rawPeers = rawPeers[6:]
	}
	return peers
}
