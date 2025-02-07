package bertrpc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"reflect"
)

var ErrRange = errors.New("value out of range")

func Decode(r io.Reader, term interface{}) error {
	byte1 := make([]byte, 1)
	_, err := r.Read(byte1)
	if err != nil {
		return err
	}

	// Read Erlang Term Format "magic byte"
	if byte1[0] != byte(TagETFVersion) {
		// Bad Version tag (aka 'magic number')
		return fmt.Errorf("incorrect Erlang Term version tag: %d", byte1[0])
	}

	return decodeData(r, term)
}

func decodeData(r io.Reader, term interface{}) error {
	// Resolve pointers
	val := reflect.ValueOf(term)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	switch val.Kind() {

	case reflect.Int8:
		return ErrRange
	case reflect.Int, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := decodeInt(r)
		if err == nil {
			val.SetInt(i)
		}
		return err
	case reflect.String:
		s, err := decodeString(r)
		if err == nil {
			val.SetString(s)
		}
		return err
	case reflect.Struct:
		// Wrapper for basic types
		if val.Type().Name() == "String" {
			return decodeBertString(r, val)
		}
		return decodeStruct(r, val)

	default:
		return fmt.Errorf("unhandled decoding target: %s", val.Kind())
	}
}

// ============================================================================
// Decode basic types

// TODO: Pass bitsize here to trigger overflow operations errors
func decodeInt(r io.Reader) (int64, error) {
	// Read Tag
	byte1 := make([]byte, 1)
	_, err := r.Read(byte1)
	if err != nil {
		return 0, err
	}

	// Compare expected type
	switch int(byte1[0]) {

	case TagSmallInteger:
		_, err := r.Read(byte1)
		if err != nil && err != io.EOF {
			return 0, err
		}
		return int64(byte1[0]), nil

	case TagInteger:
		byte4 := make([]byte, 4)
		n, err := r.Read(byte4)
		if err != nil && err != io.EOF {
			return 0, err
		}
		if n < 4 {
			return 0, fmt.Errorf("cannot decode integer, only %d bytes read", n)
		}
		var32 := int32(binary.BigEndian.Uint32(byte4))
		return int64(var32), nil
	case TagBigInteger:
		byteN := make([]byte, 1)
		byteSign := make([]byte, 1)
		_, err := r.Read(byteN)
		if err != nil {
			return 0, err
		}
		_, err = r.Read(byteSign)
		if err != nil {
			return 0, err
		}
		N := int(byteN[0])
		Sign := int(byteSign[0])
		byteD := make([]byte, N)
		n, err := r.Read(byteD)
		if err != nil && err != io.EOF {
			return 0, err
		}
		if n != N {
			return 0, errors.New("parse big integer error")
		}
		var value int64
		var B int64
		B = 1
		for idx := 0; idx < N; idx++ {
			d64 := int64(byteD[idx])
			value += int64(d64 * B)
			B *= 256
		}
		if Sign == 1 {
			value = -value
		}
		return value, nil
	}

	return 0, fmt.Errorf("incorrect type")
}

// We can decode several Erlang types in a string: Atom (Deprecated), AtomUTF8, Binary, CharList.
func decodeString(r io.Reader) (string, error) {
	// Read Tag
	byte1 := make([]byte, 1)
	_, err := r.Read(byte1)
	if err != nil {
		return "", err
	}

	// Compare expected type
	dataType := int(byte1[0])
	switch dataType {

	case TagSmallAtomUTF8:
		data, err := decodeString1(r)
		return string(data), err

	case TagDeprecatedAtom, TagAtomUTF8, TagString:
		data, err := decodeString2(r)
		return string(data), err

	case TagBinary:
		data, err := decodeString4(r)
		return string(data), err

	case TagList:
		data, err := decodeCharList(r)
		return string(data), err
	}

	return "", fmt.Errorf("incorrect type: %d", dataType)
}

func decodeString1(r io.Reader) ([]byte, error) {
	// Length:
	byte1 := make([]byte, 1)
	_, err := r.Read(byte1)
	if err != nil {
		return []byte{}, err
	}
	length := int(byte1[0])

	// Content:
	data := make([]byte, length)
	n, err := r.Read(data)
	if err != nil && err != io.EOF {
		return []byte{}, err
	}
	if n < length {
		return []byte{}, fmt.Errorf("truncated data")
	}
	return data, nil

}

// Decode a string with length on 16 bits.
func decodeString2(r io.Reader) ([]byte, error) {
	// Length:
	l := make([]byte, 2)
	_, err := r.Read(l)
	if err != nil {
		return []byte{}, err
	}
	length := int(binary.BigEndian.Uint16(l))

	// Content:
	data := make([]byte, length)
	n, err := r.Read(data)
	if err != nil && err != io.EOF {
		return []byte{}, err
	}
	if n < length {
		return []byte{}, fmt.Errorf("truncated data")
	}

	return data, nil
}

// Decode a string with length on 32 bits.
func decodeString4(r io.Reader) ([]byte, error) {
	// Length:
	l := make([]byte, 4)
	_, err := r.Read(l)
	if err != nil {
		return []byte{}, err
	}
	length := int(binary.BigEndian.Uint32(l))

	// Content:
	data := make([]byte, length)
	n, err := r.Read(data)
	if err != nil && err != io.EOF {
		return []byte{}, err
	}
	if n < length {
		return []byte{}, fmt.Errorf("truncated data")
	}

	return data, nil
}

// Decode a string with length on 32 bits.
func decodeCharList(r io.Reader) ([]rune, error) {
	// Count:
	byte4 := make([]byte, 4)
	n, err := r.Read(byte4)
	if err != nil {
		return []rune{}, err
	}
	if n < 4 {
		return []rune{}, fmt.Errorf("truncated List data")
	}
	count := int(binary.BigEndian.Uint32(byte4))

	s := []rune("")
	// Last element in list should be termination marker, so we loop (count - 1) times
	for i := 1; i <= count; i++ {
		// Assumption: We are decoding a into a string, so we expect all elements to be integers;
		// We can fail otherwise.
		char, err := decodeInt(r)
		if err != nil {
			return []rune{}, err
		}
		// Erlang does not encode utf8 charlist into a series of bytes, but use large integers.
		// We need to process the integer list as runes.
		s = append(s, rune(char))
	}
	// Check that we have the list termination mark
	if err := decodeNil(r); err != nil {
		return s, err
	}

	return s, nil
}

func decodeBertString(r io.Reader, val reflect.Value) error {
	// Read Tag
	byte1 := make([]byte, 1)
	_, err := r.Read(byte1)
	if err != nil {
		return err
	}

	var strValue string
	var strType int

	// Compare expected type
	dataType := int(byte1[0])
	switch dataType {

	case TagSmallAtomUTF8:
		data, err := decodeString1(r)
		if err != nil {
			return err
		}
		strValue = string(data)
		strType = StringTypeAtom

	case TagDeprecatedAtom, TagAtomUTF8:
		data, err := decodeString2(r)
		if err != nil {
			return err
		}
		strValue = string(data)
		strType = StringTypeAtom

	case TagString:
		data, err := decodeString2(r)
		if err != nil {
			return err
		}
		strValue = string(data)
		strType = StringTypeString

	case TagBinary:
		data, err := decodeString4(r)
		if err != nil {
			return err
		}
		strValue = string(data)
		strType = StringTypeString

	case TagList:
		data, err := decodeCharList(r)
		if err != nil {
			return err
		}
		strValue = string(data)
		strType = StringTypeString

	default:
		return fmt.Errorf("cannot decode %s to bert.String", tagName(dataType))
	}

	field := val.FieldByName("Value")
	field.SetString(strValue)
	field = val.FieldByName("ErlangType")
	field.SetInt(int64(strType))

	return nil
}

// Read a nil value and return error in case of unexpected value.
// Nil is expected as a marker for end of lists.
func decodeNil(r io.Reader) error {
	// Read Tag
	byte1 := make([]byte, 1)
	_, err := r.Read(byte1)
	if err != nil && err != io.EOF {
		return err
	}

	if byte1[0] != byte(TagNil) {
		return fmt.Errorf("could not find nil: %d", byte1[0])
	}

	return nil
}
