package pb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

type Reader interface {
	io.ByteReader
	io.Reader
}

// Decode decodes `in` as a serialized message.
func Decode(msg proto.Message, in Reader) error {
	dec := &decoder{
		msg:  msg.ProtoReflect(),
		desc: msg.ProtoReflect().Descriptor(),
		in:   in,
	}
	if err := dec.decodeMessage(); err != nil {
		return err
	}

	return nil
}

// Wire types: https://developers.google.com/protocol-buffers/docs/encoding#structure
const (
	wireTypeVarint     = 0
	wireTypeFixed64bit = 1
	wireTypeBytesType  = 2
	wireTypeFixed32bit = 5
)

type decoder struct {
	msg  protoreflect.Message
	desc protoreflect.MessageDescriptor
	in   Reader
}

func (d *decoder) decodeMessage() error {
	for {
		tag, err := d.decodeTag()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		wireType := int(tag & 0x07)
		fieldNumber := protoreflect.FieldNumber(tag >> 3)

		fieldDesc := d.desc.Fields().ByNumber(fieldNumber)
		v, err := d.decodeField(fieldDesc, wireType, int(fieldNumber))
		if err != nil {
			return err
		}

		switch {
		case fieldDesc.IsList() && !fieldDesc.IsPacked():
			list := d.msg.Get(fieldDesc).List()
			if !list.IsValid() {
				list = d.msg.NewField(fieldDesc).List()
			}

			list.Append(v)
			v = protoreflect.ValueOfList(list)
		case fieldDesc.IsMap():
			m := d.msg.Get(fieldDesc).Map()
			if !m.IsValid() {
				m = d.msg.NewField(fieldDesc).Map()
			}

			m.Set(v.Message().Get(fieldDesc.MapKey()).MapKey(), v.Message().Get(fieldDesc.MapValue()))
			v = protoreflect.ValueOfMap(m)
		}

		d.msg.Set(fieldDesc, v)
	}

	return nil
}

func (d *decoder) decodeField(desc protoreflect.FieldDescriptor, wireType, fieldNumber int) (protoreflect.Value, error) {
	_, v, err := d.decodeValue(desc, wireType)
	return protoreflect.ValueOf(v), err
}

func (d *decoder) decodeValue(desc protoreflect.FieldDescriptor, wireType int) (int, interface{}, error) {
	switch wireType {
	case wireTypeVarint:
		n, v, err := d.decodeVarint()
		if err != nil {
			return 0, nil, err
		}

		switch desc.Kind() {
		case protoreflect.Int32Kind:
			return n, int32(v), nil
		case protoreflect.Int64Kind:
			return n, int64(v), nil
		case protoreflect.Uint32Kind:
			return n, uint32(v), nil
		case protoreflect.Uint64Kind:
			return n, v, nil
		case protoreflect.Sint32Kind:
			return n, int32(decodeZigZag(v)), nil
		case protoreflect.Sint64Kind:
			return n, decodeZigZag(v), nil
		case protoreflect.BoolKind:
			return n, v != 0, nil
		case protoreflect.EnumKind:
			return n, protoreflect.EnumNumber(v), nil
		}
	case wireTypeFixed64bit:
		n, err := d.decodeFixed64bit()
		if err != nil {
			return 8, nil, err
		}

		switch desc.Kind() {
		case protoreflect.Fixed64Kind:
			return 8, n, nil
		case protoreflect.Sfixed64Kind:
			return 8, int64(n), nil
		case protoreflect.DoubleKind:
			return 8, math.Float64frombits(n), nil
		}
	case wireTypeBytesType:
		switch desc.Kind() {
		case protoreflect.BytesKind:
			b, err := d.decodeBytes()
			if err != nil {
				return 0, nil, err
			}

			return len(b), b, nil
		case protoreflect.StringKind:
			b, err := d.decodeBytes()
			if err != nil {
				return 0, nil, err
			}

			return len(b), string(b), nil
		case protoreflect.MessageKind:
			b, err := d.decodeBytes()
			if err != nil {
				return 0, nil, err
			}

			var msg protoreflect.Message
			switch {
			case desc.IsMap():
				// Treat map as a repeated message.
				// See https://developers.google.com/protocol-buffers/docs/proto3#backwards_compatibility.
				msg = dynamicpb.NewMessage(desc.Message())
			default:
				mt, err := protoregistry.GlobalTypes.FindMessageByName(desc.Message().FullName())
				if err != nil {
					return 0, nil, err
				}
				msg = mt.New()
			}

			if err := Decode(msg.Interface(), bytes.NewReader(b)); err != nil {
				return 0, nil, err
			}

			return len(b), msg, nil
		default: // Packed repeated field.
			var (
				totalLen int
				list     = d.msg.NewField(desc).List()
			)

			_, maxLen, err := d.decodeVarint()
			if err != nil {
				return 0, nil, err
			}

			for totalLen < int(maxLen) {
				n, v, err := d.decodeValue(
					&nonRepeatedFieldDescriptor{desc},
					wireTypeByKind[desc.Kind()],
				)
				if err != nil {
					return 0, nil, err
				}

				totalLen += n

				list.Append(protoreflect.ValueOf(v))
			}

			return totalLen, list, nil
		}

	case wireTypeFixed32bit:
		n, err := d.decodeFixed32bit()
		if err != nil {
			return 4, nil, err
		}

		switch desc.Kind() {
		case protoreflect.Fixed32Kind:
			return 4, n, nil
		case protoreflect.Sfixed32Kind:
			return 4, int32(n), nil
		case protoreflect.FloatKind:
			return 4, math.Float32frombits(n), nil
		}
	}

	return 0, nil, errors.New("unreachable")
}

func (d *decoder) decodeTag() (uint64, error) {
	_, n, err := d.decodeVarint()
	return n, err
}

func (d *decoder) decodeVarint() (length int, n uint64, _ error) {
	for i := 0; ; i++ {
		b, err := d.in.ReadByte()
		if err != nil {
			return 0, 0, err
		}

		length++

		v, hasNext := dropMSB(b)
		n |= uint64(v) << (7 * i)

		if !hasNext {
			return length, n, nil
		}
	}
}

func (d *decoder) decodeFixed64bit() (uint64, error) {
	var n uint64 // byte is a synonym for uint8.
	if err := binary.Read(d.in, binary.LittleEndian, &n); err != nil {
		return 0, err
	}

	return n, nil
}

func (d *decoder) decodeFixed32bit() (uint32, error) {
	var n uint32 // byte is a synonym for uint8.
	if err := binary.Read(d.in, binary.LittleEndian, &n); err != nil {
		return 0, err
	}

	return n, nil
}

func (d *decoder) decodeBytes() ([]byte, error) {
	_, n, err := d.decodeVarint()
	if err != nil {
		return nil, err
	}

	b := make([]byte, n)
	if _, err := d.in.Read(b); err != nil {
		return nil, err
	}

	return b, nil
}

func dropMSB(b byte) (_ byte, hasNext bool) {
	hasNext = b>>7 == 1
	return b & 0x7f, hasNext
}

func decodeZigZag(n uint64) int64 {
	return int64(n>>1) ^ int64(n)<<63>>63
}

type nonRepeatedFieldDescriptor struct {
	protoreflect.FieldDescriptor
}

func (d *nonRepeatedFieldDescriptor) IsPacked() bool { return false }

var wireTypeByKind = map[protoreflect.Kind]int{
	protoreflect.Int32Kind:    wireTypeVarint,
	protoreflect.Int64Kind:    wireTypeVarint,
	protoreflect.Uint32Kind:   wireTypeVarint,
	protoreflect.Uint64Kind:   wireTypeVarint,
	protoreflect.Sint32Kind:   wireTypeVarint,
	protoreflect.Sint64Kind:   wireTypeVarint,
	protoreflect.BoolKind:     wireTypeVarint,
	protoreflect.EnumKind:     wireTypeVarint,
	protoreflect.Fixed64Kind:  wireTypeFixed64bit,
	protoreflect.Sfixed64Kind: wireTypeFixed64bit,
	protoreflect.DoubleKind:   wireTypeFixed64bit,
	protoreflect.BytesKind:    wireTypeBytesType,
	protoreflect.StringKind:   wireTypeBytesType,
	protoreflect.MessageKind:  wireTypeBytesType,
	protoreflect.Fixed32Kind:  wireTypeFixed32bit,
	protoreflect.Sfixed32Kind: wireTypeFixed32bit,
	protoreflect.FloatKind:    wireTypeFixed32bit,
}
