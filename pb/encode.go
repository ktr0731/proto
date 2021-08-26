package pb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Encode encodes `msg`
func Encode(msg proto.Message) ([]byte, error) {
	var enc encoder
	if err := enc.encodeMessage(msg.ProtoReflect()); err != nil {
		return nil, err
	}
	if err := enc.w.err; err != nil {
		return nil, err
	}

	return enc.w.Bytes(), nil
}

type writer struct {
	bytes.Buffer
	err error
}

func (w *writer) write(b []byte) {
	if w.err != nil {
		return
	}

	_, w.err = w.Write(b)
}

type encoder struct {
	w writer
}

func (e *encoder) encodeMessage(msg protoreflect.Message) error {
	var err error
	msg.Range(func(desc protoreflect.FieldDescriptor, val protoreflect.Value) bool {
		switch {
		case desc.IsList():
			if err = e.encodeList(desc, val); err != nil {
				return false
			}
		case desc.IsMap():
			if err = e.encodeMap(desc, val); err != nil {
				return false
			}
		default:
			var b []byte
			_, b, err = encodeField(desc, val)
			if err != nil {
				return false
			}

			e.w.write(b)
			return true
		}
		return true
	})

	return err
}

func (e *encoder) encodeList(desc protoreflect.FieldDescriptor, val protoreflect.Value) error {
	if desc.IsPacked() {
		_, b := encodeTag(protoreflect.BytesKind, desc.Number())
		e.w.write(b)

		var (
			total int
			buf   bytes.Buffer
		)
		for i := 0; i < val.List().Len(); i++ {
			n, b, err := encodeValue(desc, val.List().Get(i))
			if err != nil {
				return err
			}

			if _, err := buf.Write(b); err != nil {
				return err
			}
			total += n
		}

		_, b = encodeVarint(uint64(total))
		e.w.write(b)

		e.w.write(buf.Bytes())

		return nil
	}

	for i := 0; i < val.List().Len(); i++ {
		_, b, err := encodeField(desc, val.List().Get(i))
		if err != nil {
			return err
		}

		e.w.write(b)
	}

	return nil
}

func (e *encoder) encodeMap(desc protoreflect.FieldDescriptor, val protoreflect.Value) error {
	var err error
	keyDesc, valDesc := desc.MapKey(), desc.MapValue()
	val.Map().Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
		_, b := encodeTag(protoreflect.BytesKind, desc.Number())
		e.w.write(b)

		var (
			kn, vn int
			kb, vb []byte
		)
		kn, kb, err = encodeField(keyDesc, k.Value())
		if err != nil {
			return false
		}
		vn, vb, err = encodeField(valDesc, v)
		if err != nil {
			return false
		}

		_, length := encodeVarint(uint64(kn + vn))
		e.w.write(length)

		e.w.write(kb)
		e.w.write(vb)

		return true
	})
	return err
}

func encodeField(desc protoreflect.FieldDescriptor, val protoreflect.Value) (int, []byte, error) {
	tn, tb := encodeTag(desc.Kind(), desc.Number())

	vn, vb, err := encodeValue(desc, val)
	if err != nil {
		return 0, nil, err
	}

	return tn + vn, append(tb, vb...), nil
}

func encodeTag(kind protoreflect.Kind, fieldNumber protoreflect.FieldNumber) (int, []byte) {
	wireType := wireTypeByKind[kind]
	tag := int(fieldNumber)<<3 | wireType

	return encodeVarint(uint64(tag))
}

func encodeValue(desc protoreflect.FieldDescriptor, val protoreflect.Value) (int, []byte, error) {
	var (
		n int
		b []byte
	)
	switch desc.Kind() {
	case protoreflect.Int32Kind:
		n, b = encodeVarint(uint64(int32(val.Int())))
	case protoreflect.Int64Kind:
		n, b = encodeVarint(uint64(val.Int()))
	case protoreflect.Uint32Kind:
		n, b = encodeVarint(val.Uint())
	case protoreflect.Uint64Kind:
		n, b = encodeVarint(val.Uint())
	case protoreflect.Sint32Kind:
		n, b = encodeZigZag(val.Int())
	case protoreflect.Sint64Kind:
		n, b = encodeZigZag(val.Int())
	case protoreflect.BoolKind:
		var v uint64
		if val.Bool() {
			v = 1
		}
		n, b = encodeVarint(v)
	case protoreflect.EnumKind:
		n, b = encodeVarint(uint64(val.Enum()))
	case protoreflect.Fixed64Kind:
		n, b = encodeFixed64(val.Uint())
	case protoreflect.Sfixed64Kind:
		n, b = encodeFixed64(uint64(val.Int()))
	case protoreflect.DoubleKind:
		n, b = encodeFixed64(math.Float64bits(val.Float()))
	case protoreflect.BytesKind:
		n, b = encodeBytes(val.Bytes())
	case protoreflect.StringKind:
		n, b = encodeBytes([]byte(val.String()))
	case protoreflect.MessageKind:
		msg, err := Encode(val.Message().Interface())
		if err != nil {
			return 0, nil, err
		}

		n, b = encodeBytes(msg)
	case protoreflect.Fixed32Kind:
		n, b = encodeFixed32(uint32(val.Uint()))
	case protoreflect.Sfixed32Kind:
		n, b = encodeFixed32(uint32(val.Int()))
	case protoreflect.FloatKind:
		v := math.Float32bits(float32(val.Float()))
		n, b = encodeFixed32(v)
	default:
		return 0, nil, errors.New("unreachable")
	}

	return n, b, nil
}

func encodeVarint(val uint64) (int, []byte) {
	length := 1
	for v := val; v >= 1<<7; v >>= 7 {
		length++
	}

	b := make([]byte, 0, length)
	for i := 0; i < length; i++ {
		v := val >> (7 * i) & 0x7f
		if i+1 != length {
			v |= 0x80
		}

		b = append(b, byte(v))
	}

	return len(b), b
}

func encodeZigZag(n int64) (int, []byte) {
	v := uint64(n<<1) ^ uint64(n>>63)
	return encodeVarint(v)
}

func encodeFixed64(n uint64) (int, []byte) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, n)
	return 8, b
}

func encodeBytes(b []byte) (int, []byte) {
	vn, vb := encodeVarint(uint64(len(b)))
	return vn + len(b), append(vb, b...)
}

func encodeFixed32(n uint32) (int, []byte) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, n)
	return 4, b
}
