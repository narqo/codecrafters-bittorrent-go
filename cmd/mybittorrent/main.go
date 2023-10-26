package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	bencode "github.com/jackpal/bencode-go"
)

// Example:
// - 5:hello -> hello
// - 10:hello12345 -> hello12345
func decodeBencode(s string) (interface{}, string, error) {
	tag := s[0]
	switch {
	case unicode.IsDigit(rune(tag)):
		head, tail, ok := strings.Cut(s, ":")
		if !ok {
			return nil, "", fmt.Errorf("can't find colon in %q", s)
		}

		slen, err := strconv.Atoi(head)
		if err != nil {
			return "", "", err
		}
		return tail[:slen], tail[slen:], nil
	case tag == 'i':
		head, tail, ok := strings.Cut(s[1:], "e")
		if !ok {
			return nil, "", fmt.Errorf("can't find end of %q", s)
		}
		n, err := strconv.Atoi(head)
		return n, tail, err
	case tag == 'l':
		var list []interface{}
		s = s[1:]
		for {
			var (
				v   interface{}
				err error
			)
			v, s, err = decodeBencode(s)
			if err != nil {
				return nil, s, err
			}
			list = append(list, v)
			// consume the end of the list and exit
			if s[0] == 'e' {
				s = s[1:]
				break
			}
		}
		return list, s, nil
	case tag == 'd':
		dict := make(map[string]interface{}, 0)
		s = s[1:]
		for {
			var (
				v   interface{}
				err error
			)
			v, s, err = decodeBencode(s)
			if err != nil {
				return nil, s, err
			}
			k, ok := v.(string)
			if !ok {
				return nil, s, fmt.Errorf("key must be string, got %T (%v)", v, v)
			}
			v, s, err = decodeBencode(s)
			if err != nil {
				return nil, s, err
			}
			dict[k] = v
			// consume the end of the list and exit
			if s[0] == 'e' {
				s = s[1:]
				break
			}
		}
		return dict, s, nil
	default:
		return "", s, errors.ErrUnsupported
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

		var b bytes.Buffer
		err = bencode.Marshal(&b, t.Info)
		if err != nil {
			panic(err)
		}

		infoHash := sha1.Sum(b.Bytes())

		fmt.Printf("Tracker URL: %s\n", t.Announce)
		fmt.Printf("Length: %d\n", t.Info.Length)
		fmt.Printf("Info Hash: %x\n", infoHash)
		fmt.Printf("Piece Length: %d\n", t.Info.PieceLength)

		fmt.Printf("Piece Hashes:\n")
		for p := []byte(t.Info.Pieces); len(p) > 0; p = p[20:] {
			fmt.Printf("%x\n", p[:20])
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
