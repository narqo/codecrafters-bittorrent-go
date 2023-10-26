package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
	// bencode "github.com/jackpal/bencode-go" // Available if you need it!
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
	default:
		fmt.Println("Unknown command: " + cmd)
		os.Exit(1)
	}
}
