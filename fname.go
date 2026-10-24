package main

// How the engine stores names: "X_602" is kept as the text "X" with the number 603, while "X_001"
// (a leading zero) and "X_0602" stay whole. Every name in a package's name map also carries two
// 16-bit hashes of its text.

import (
	"strconv"
	"strings"
)

// splitFName splits a name the way FName does: (text, number), where number 0 means "no number".
func splitFName(s string) (string, int32) {
	i := strings.LastIndexByte(s, '_')
	if i <= 0 || i == len(s)-1 {
		return s, 0
	}
	tail := s[i+1:]
	if len(tail) > 10 {
		return s, 0
	}
	for _, c := range tail {
		if c < '0' || c > '9' {
			return s, 0
		}
	}
	if tail[0] == '0' && len(tail) > 1 {
		return s, 0
	}
	v, err := strconv.ParseInt(tail, 10, 64)
	if err != nil || v >= 0x7FFFFFFE {
		return s, 0
	}
	return s[:i], int32(v + 1)
}

// joinFName is the reverse of splitFName.
func joinFName(base string, num int32) string {
	if num == 0 {
		return base
	}
	return base + "_" + strconv.Itoa(int(num-1))
}

var crcMSB, crcLSB [256]uint32

func init() {
	for i := 0; i < 256; i++ {
		c := uint32(i) << 24
		for k := 0; k < 8; k++ {
			if c&0x80000000 != 0 {
				c = c<<1 ^ 0x04C11DB7
			} else {
				c <<= 1
			}
		}
		crcMSB[i] = c
		c = uint32(i)
		for k := 0; k < 8; k++ {
			if c&1 != 0 {
				c = c>>1 ^ 0xEDB88320
			} else {
				c >>= 1
			}
		}
		crcLSB[i] = c
	}
}

func upperRune(r rune) rune {
	if r < 128 {
		if r >= 'a' && r <= 'z' {
			return r - 32
		}
		return r
	}
	return []rune(strings.ToUpper(string(r)))[0]
}

// nameHashes gives the two hashes stored after a name in a package's name map.
func nameHashes(s string) [4]byte {
	wide := false
	for _, r := range s {
		if r > 127 {
			wide = true
		}
	}
	var h uint32
	for _, r := range s {
		c := uint32(upperRune(r))
		h = (h>>8)&0x00FFFFFF ^ crcMSB[(h^(c&0xFF))&0xFF]
		if wide {
			h = (h>>8)&0x00FFFFFF ^ crcMSB[(h^(c>>8))&0xFF]
		}
	}
	crc := uint32(0xFFFFFFFF)
	for _, r := range s {
		c := uint32(r)
		for k := 0; k < 4; k++ {
			crc = crc>>8 ^ crcLSB[(crc^c)&0xFF]
			c >>= 8
		}
	}
	crc ^= 0xFFFFFFFF
	return [4]byte{byte(h), byte(h >> 8), byte(crc), byte(crc >> 8)}
}
