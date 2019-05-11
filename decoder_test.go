package bert_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/processone/bert"
)

// Small Erlang Term type is Uint8. It cannot fit into an int8
func TestDecodeInt8(t *testing.T) {
	var i int8
	buf := bytes.NewBuffer([]byte{131, 97, 255})
	if err := bert.Decode(buf, &i); err != bert.ErrRange {
		t.Errorf("Decoding an Erlang small integer into int8 should fail")
	}
}

func TestDecodeInt(t *testing.T) {
	tests := []struct {
		input []byte
		want  int64
	}{
		{input: []byte{131, 97, 42}, want: 42},
		{input: []byte{131, 97, 255}, want: 255},
		{input: []byte{131, 98, 255, 255, 255, 0}, want: -256},
		{input: []byte{131, 98, 0, 0, 1, 0}, want: 256},
		{input: []byte{131, 98, 128, 0, 0, 0}, want: -2147483648},
		{input: []byte{131, 98, 127, 255, 255, 255}, want: 2147483647},
	}

	for _, tc := range tests {
		var i int
		buf := bytes.NewBuffer(tc.input)
		if err := bert.Decode(buf, &i); err != nil {
			t.Errorf("cannot decode Erlang term: %s", err)
			return
		}

		if int64(i) != tc.want {
			t.Errorf("incorrect decoded value: %d. expected: %d", i, tc.want)
		}
	}
}

// TODO: Implement decode same types to []byte and bert.Atom
func TestDecodeToString(t *testing.T) {
	longUTF8 := strings.Repeat("🖖", 64)
	tests := []struct {
		input []byte
		want  string
	}{
		{input: []byte{131, 100, 0, 0}, want: ""},
		{input: []byte{131, 100, 0, 2, 111, 107}, want: "ok"},
		{input: []byte{131, 119, 4, 240, 159, 150, 150}, want: "🖖"},
		{input: append([]byte{131, 118, 1, 0}, []byte(longUTF8)...), want: longUTF8},
		{input: []byte{131, 107, 0, 5, 72, 101, 108, 108, 111}, want: "Hello"},
		{input: []byte{131, 109, 0, 0, 0, 5, 72, 101, 108, 108, 111}, want: "Hello"},
		{input: []byte{131, 109, 0, 0, 0, 10, 240, 159, 150, 150, 32, 72, 101, 108, 108, 111}, want: "🖖 Hello"},
		{input: []byte{131, 108, 0, 0, 0, 3, 98, 0, 1, 245, 150, 97, 72, 97, 105, 106}, want: "🖖Hi"},
	}

	for _, tc := range tests {
		var a string
		buf := bytes.NewBuffer(tc.input)
		if err := bert.Decode(buf, &a); err != nil {
			t.Errorf("cannot decode Erlang term: %s", err)
			return
		}

		if a != tc.want {
			t.Errorf("incorrect decoded value: %#v. expected: %#v", a, tc.want)
		}
	}
}

func TestDecodeEmptyTuple(t *testing.T) {
	input := []byte{131, 104, 0}
	want := struct{}{}

	var tuple struct{}
	buf := bytes.NewBuffer(input)
	if err := bert.Decode(buf, &tuple); err != nil {
		t.Errorf("cannot decode Erlang term: %s", err)
		return
	}

	if tuple != want {
		t.Errorf("cannot decode empty tuple: %v", tuple)
	}
}

func TestDecodeTuple2(t *testing.T) {
	input := []byte{131, 104, 2, 100, 0, 5, 101, 114, 114, 111, 114, 100, 0, 9, 110, 111,
		116, 95, 102, 111, 117, 110, 100}
	want := struct {
		Result string
		Reason string
	}{"error", "not_found"}

	var tuple struct {
		Result string
		Reason string
	}
	buf := bytes.NewBuffer(input)
	if err := bert.Decode(buf, &tuple); err != nil {
		t.Errorf("cannot decode Erlang term: %s", err)
		return
	}

	if tuple != want {
		t.Errorf("cannot decode empty tuple: %v", tuple)
	}
}

func TestFailOnLengthMismatch(t *testing.T) {
	input := []byte{131, 104, 2, 100, 0, 5, 101, 114, 114, 111, 114, 100, 0, 9, 110, 111,
		116, 95, 102, 111, 117, 110, 100}

	var tuple struct {
		Result string
		Reason string
		Extra  string
	}
	buf := bytes.NewBuffer(input)
	if err := bert.Decode(buf, &tuple); err == nil {
		t.Errorf("decoding tuple into struct with different number of field should fail")
	}
}
