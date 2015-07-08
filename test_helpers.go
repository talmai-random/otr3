package otr3

import (
	"encoding/hex"
	"io"
	"reflect"
	"testing"
)

func assertEquals(t *testing.T, actual, expected interface{}) {
	if actual != expected {
		t.Errorf("Expected %v to equal %v", actual, expected)
	}
}

func assertDeepEquals(t *testing.T, actual, expected interface{}) {
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Expected %v to equal %v", actual, expected)
	}
}

func hexToByte(s string) []byte {
	plainBytes, _ := hex.DecodeString(s)
	return plainBytes
}

type fixedRandReader struct {
	data []string
	at   int
}

func fixedRand(data []string) io.Reader {
	return &fixedRandReader{data, 0}
}

func (frr *fixedRandReader) Read(p []byte) (n int, err error) {
	if frr.at < len(frr.data) {
		plainBytes := hexToByte(frr.data[frr.at])
		frr.at++
		n = copy(p, plainBytes)
		return
	}
	return 0, io.EOF
}