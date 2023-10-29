package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
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
