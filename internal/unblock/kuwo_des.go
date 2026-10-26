package unblock

// SPDX-License-Identifier: LGPL-3.0-only
//
// Kuwo's client encrypts the query of a convert_url2 request with a DES
// variant of its own: bit-level permutations over 64-bit blocks instead of
// the standard byte order, and a little-endian key. This is a modified Go
// port, made on 2026-10-26, of the tables and rounds that
// UnblockNeteaseMusic's server (github.com/UnblockNeteaseMusic/server)
// decoded from that client's JavaScript; that code is LGPL-3.0, and the
// license is in COPYING.LESSER beside this file.

import (
	"encoding/base64"
	"encoding/binary"
)

var arrayE = []int{
	31, 0, 1, 2, 3, 4, -1, -1, 3, 4, 5, 6, 7, 8, -1, -1,
	7, 8, 9, 10, 11, 12, -1, -1, 11, 12, 13, 14, 15, 16, -1, -1,
	15, 16, 17, 18, 19, 20, -1, -1, 19, 20, 21, 22, 23, 24, -1, -1,
	23, 24, 25, 26, 27, 28, -1, -1, 27, 28, 29, 30, 31, 30, -1, -1,
}

var arrayIP = []int{
	57, 49, 41, 33, 25, 17, 9, 1, 59, 51, 43, 35, 27, 19, 11, 3,
	61, 53, 45, 37, 29, 21, 13, 5, 63, 55, 47, 39, 31, 23, 15, 7,
	56, 48, 40, 32, 24, 16, 8, 0, 58, 50, 42, 34, 26, 18, 10, 2,
	60, 52, 44, 36, 28, 20, 12, 4, 62, 54, 46, 38, 30, 22, 14, 6,
}

var arrayIP_1 = []int{
	39, 7, 47, 15, 55, 23, 63, 31, 38, 6, 46, 14, 54, 22, 62, 30,
	37, 5, 45, 13, 53, 21, 61, 29, 36, 4, 44, 12, 52, 20, 60, 28,
	35, 3, 43, 11, 51, 19, 59, 27, 34, 2, 42, 10, 50, 18, 58, 26,
	33, 1, 41, 9, 49, 17, 57, 25, 32, 0, 40, 8, 48, 16, 56, 24,
}

var arrayP = []int{
	15, 6, 19, 20, 28, 11, 27, 16, 0, 14, 22, 25, 4, 17, 30, 9,
	1, 7, 23, 13, 31, 26, 2, 8, 18, 12, 29, 5, 21, 10, 3, 24,
}

var arrayPC_1 = []int{
	56, 48, 40, 32, 24, 16, 8, 0, 57, 49, 41, 33, 25, 17, 9, 1,
	58, 50, 42, 34, 26, 18, 10, 2, 59, 51, 43, 35, 62, 54, 46, 38,
	30, 22, 14, 6, 61, 53, 45, 37, 29, 21, 13, 5, 60, 52, 44, 36,
	28, 20, 12, 4, 27, 19, 11, 3,
}

var arrayPC_2 = []int{
	13, 16, 10, 23, 0, 4, -1, -1, 2, 27, 14, 5, 20, 9, -1, -1,
	22, 18, 11, 3, 25, 7, -1, -1, 15, 6, 26, 19, 12, 1, -1, -1,
	40, 51, 30, 36, 46, 54, -1, -1, 29, 39, 50, 44, 32, 47, -1, -1,
	43, 48, 38, 55, 33, 52, -1, -1, 45, 41, 49, 35, 28, 31, -1, -1,
}

var matrixNSBox = [8][64]byte{
	{
		14, 4, 3, 15, 2, 13, 5, 3, 13, 14, 6, 9, 11, 2, 0, 5,
		4, 1, 10, 12, 15, 6, 9, 10, 1, 8, 12, 7, 8, 11, 7, 0,
		0, 15, 10, 5, 14, 4, 9, 10, 7, 8, 12, 3, 13, 1, 3, 6,
		15, 12, 6, 11, 2, 9, 5, 0, 4, 2, 11, 14, 1, 7, 8, 13,
	},
	{
		15, 0, 9, 5, 6, 10, 12, 9, 8, 7, 2, 12, 3, 13, 5, 2,
		1, 14, 7, 8, 11, 4, 0, 3, 14, 11, 13, 6, 4, 1, 10, 15,
		3, 13, 12, 11, 15, 3, 6, 0, 4, 10, 1, 7, 8, 4, 11, 14,
		13, 8, 0, 6, 2, 15, 9, 5, 7, 1, 10, 12, 14, 2, 5, 9,
	},
	{
		10, 13, 1, 11, 6, 8, 11, 5, 9, 4, 12, 2, 15, 3, 2, 14,
		0, 6, 13, 1, 3, 15, 4, 10, 14, 9, 7, 12, 5, 0, 8, 7,
		13, 1, 2, 4, 3, 6, 12, 11, 0, 13, 5, 14, 6, 8, 15, 2,
		7, 10, 8, 15, 4, 9, 11, 5, 9, 0, 14, 3, 10, 7, 1, 12,
	},
	{
		7, 10, 1, 15, 0, 12, 11, 5, 14, 9, 8, 3, 9, 7, 4, 8,
		13, 6, 2, 1, 6, 11, 12, 2, 3, 0, 5, 14, 10, 13, 15, 4,
		13, 3, 4, 9, 6, 10, 1, 12, 11, 0, 2, 5, 0, 13, 14, 2,
		8, 15, 7, 4, 15, 1, 10, 7, 5, 6, 12, 11, 3, 8, 9, 14,
	},
	{
		2, 4, 8, 15, 7, 10, 13, 6, 4, 1, 3, 12, 11, 7, 14, 0,
		12, 2, 5, 9, 10, 13, 0, 3, 1, 11, 15, 5, 6, 8, 9, 14,
		14, 11, 5, 6, 4, 1, 3, 10, 2, 12, 15, 0, 13, 2, 8, 5,
		11, 8, 0, 15, 7, 14, 9, 4, 12, 7, 10, 9, 1, 13, 6, 3,
	},
	{
		12, 9, 0, 7, 9, 2, 14, 1, 10, 15, 3, 4, 6, 12, 5, 11,
		1, 14, 13, 0, 2, 8, 7, 13, 15, 5, 4, 10, 8, 3, 11, 6,
		10, 4, 6, 11, 7, 9, 0, 6, 4, 2, 13, 1, 9, 15, 3, 8,
		15, 3, 1, 14, 12, 5, 11, 0, 2, 12, 14, 7, 5, 10, 8, 13,
	},
	{
		4, 1, 3, 10, 15, 12, 5, 0, 2, 11, 9, 6, 8, 7, 6, 9,
		11, 4, 12, 15, 0, 3, 10, 5, 14, 13, 7, 8, 13, 14, 1, 2,
		13, 6, 14, 9, 4, 1, 2, 14, 11, 13, 5, 0, 1, 10, 8, 3,
		0, 11, 3, 5, 9, 4, 15, 2, 7, 8, 12, 15, 10, 7, 6, 12,
	},
	{
		13, 7, 10, 0, 6, 9, 5, 15, 8, 4, 3, 10, 11, 14, 12, 5,
		2, 11, 9, 6, 15, 12, 0, 3, 4, 1, 14, 13, 1, 2, 7, 8,
		1, 2, 12, 15, 10, 4, 0, 3, 13, 14, 6, 9, 7, 8, 9, 6,
		15, 1, 5, 12, 3, 10, 14, 5, 8, 7, 11, 0, 4, 13, 2, 11,
	},
}

// The key schedule's rotations, and the masks that keep a 28-bit half
// from spilling into the other one.
var (
	arrayLs     = [16]int{1, 1, 2, 2, 2, 2, 2, 2, 1, 2, 2, 2, 2, 2, 2, 1}
	arrayLsMask = [3]uint64{0, 0x100001, 0x300003}
	kuwoKey     = []byte("ylzsxkwm")
)

// bitTransform gathers the bits of l named by arr, table order becoming
// bit order, and skips the negative entries, which are the padding in
// Kuwo's tables.
func bitTransform(arr []int, l uint64) uint64 {
	var out uint64
	for i, at := range arr {
		if at < 0 {
			continue
		}
		if l&(1<<uint(at)) != 0 {
			out |= 1 << uint(i)
		}
	}
	return out
}

// des64 encrypts one block with the subkeys. The halves swap every round,
// so the last permutation is handed left<<32|right rather than the usual
// order: getting that backwards still round-trips, but produces a
// different ciphertext than the client's.
func des64(sub *[16]uint64, l uint64) uint64 {
	out := bitTransform(arrayIP, l)
	left := out & 0xffffffff
	right := out >> 32
	for i := range 16 {
		r := bitTransform(arrayE, right) ^ sub[i]
		var s uint64
		for b := 7; b >= 0; b-- {
			s = s<<4 | uint64(matrixNSBox[b][(r>>uint(8*b))&0xff])
		}
		p := bitTransform(arrayP, s)
		left, right = right, left^p
	}
	return bitTransform(arrayIP_1, left<<32|right)
}

func subKeys(key uint64) (sub [16]uint64) {
	l := bitTransform(arrayPC_1, key)
	for i := 0; i < 16; i++ {
		ls := arrayLs[i]
		mask := arrayLsMask[ls]
		l = (l&mask)<<uint(28-ls) | (l&^mask)>>uint(ls)
		sub[i] = bitTransform(arrayPC_2, l)
	}
	return
}

// kuwoEncryptQuery encrypts a request's query the way the client does:
// little-endian key and blocks, zero padding to a block boundary, one
// extra empty block, then base64.
func kuwoEncryptQuery(q string) string {
	msg := []byte(q)
	buf := make([]byte, (len(msg)+7)/8*8+8)
	copy(buf, msg)
	var key uint64
	for i, b := range kuwoKey {
		key |= uint64(b) << uint(8*i)
	}
	sub := subKeys(key)
	for i := 0; i < len(buf); i += 8 {
		binary.LittleEndian.PutUint64(buf[i:], des64(&sub, binary.LittleEndian.Uint64(buf[i:])))
	}
	return base64.StdEncoding.EncodeToString(buf)
}
