package main

import (
	"encoding/json"
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
func decodeBencode(s string) (interface{}, error) {
	tag := s[0]
	switch {
	case unicode.IsDigit(rune(tag)):
		head, tail, ok := strings.Cut(s, ":")
		if !ok {
			return nil, fmt.Errorf("can't find colon in %q", s)
		}

		slen, err := strconv.Atoi(head)
		if err != nil {
			return "", err
		}

		return tail[:slen], nil
	case tag == 'i':
		head, _, ok := strings.Cut(s[1:], "e")
		if !ok {
			return nil, fmt.Errorf("can't find end of %q", s)
		}
		return strconv.Atoi(head)
	default:
		return "", fmt.Errorf("only strings are supported at the moment")
	}
}

func main() {
	command := os.Args[1]

	if command == "decode" {
		s := os.Args[2]
		val, err := decodeBencode(s)
		if err != nil {
			fmt.Println(err)
			return
		}

		out, _ := json.Marshal(val)
		fmt.Println(string(out))
	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}
