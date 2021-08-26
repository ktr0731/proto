package pb

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/ktr0731/proto/pb/internal/testpb"
)

func TestDecode(t *testing.T) {
	b := marshal(t, &testpb.Message{
		Int32Field:    1,
		Int64Field:    2,
		Uint32Field:   3,
		Uint64Field:   4,
		Sint32Field:   -5,
		Sint64Field:   6,
		BoolField:     true,
		EnumField:     testpb.Message_BAZ,
		Fixed64Field:  7,
		Sfixed64Field: 8,
		DoubleField:   0.9,
		StringField:   "ｆｏｏ",
		BytesField:    []byte("bar"),
		EmbeddedMessageField: &testpb.EmbeddedMessage{
			Field: 10,
			CircularField: &testpb.Message{
				Int32Field: 100,
			},
		},
		Fixed32Field:                10,
		Sfixed32Field:               11,
		FloatField:                  0.12,
		RepeatedPackedInt32Field:    []int32{1, 2, 3},
		RepeatedInt32Field:          []int32{1, 2, 3},
		RepeatedPackedInt64Field:    []int64{1, 2, 3},
		RepeatedInt64Field:          []int64{1, 2, 3},
		RepeatedPackedUint32Field:   []uint32{1, 2, 3},
		RepeatedUint32Field:         []uint32{1, 2, 3},
		RepeatedPackedUint64Field:   []uint64{1, 2, 3},
		RepeatedUint64Field:         []uint64{1, 2, 3},
		RepeatedPackedSint32Field:   []int32{1, 2, 3},
		RepeatedSint32Field:         []int32{1, 2, 3},
		RepeatedPackedSint64Field:   []int64{1, 2, 3},
		RepeatedSint64Field:         []int64{1, 2, 3},
		RepeatedPackedBoolField:     []bool{true, false},
		RepeatedBoolField:           []bool{true, false},
		RepeatedPackedEnumField:     []testpb.Message_Enum{testpb.Message_BAR, testpb.Message_BAZ},
		RepeatedEnumField:           []testpb.Message_Enum{testpb.Message_BAR, testpb.Message_BAZ},
		RepeatedPackedFixed64Field:  []uint64{1, 2, 3},
		RepeatedFixed64Field:        []uint64{1, 2, 3},
		RepeatedPackedSfixed64Field: []int64{1, 2, 3},
		RepeatedSfixed64Field:       []int64{1, 2, 3},
		RepeatedPackedDoubleField:   []float64{1.1, 2.2, 3.3},
		RepeatedDoubleField:         []float64{1.1, 2.2, 3.3},
		RepeatedStringField:         []string{"foo", "bar", "baz"},
		RepeatedBytesField:          [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")},
		RepeatedPackedFixed32Field:  []uint32{1, 2, 3},
		RepeatedFixed32Field:        []uint32{1, 2, 3},
		RepeatedPackedSfixed32Field: []int32{1, 2, 3},
		RepeatedSfixed32Field:       []int32{1, 2, 3},
		RepeatedPackedFloatField:    []float32{1.1, 2.2, 3.3},
		RepeatedFloatField:          []float32{1.1, 2.2, 3.3},
		RepeatedEmbeddedMessageField: []*testpb.EmbeddedMessage{
			{Field: 100},
			{Field: 200},
			{Field: 300},
		},
		MapInt32:    map[int32]int32{1: 2},
		MapInt64:    map[int64]int64{1: 2},
		MapUint32:   map[uint32]uint32{1: 2},
		MapUint64:   map[uint64]uint64{1: 2},
		MapSint32:   map[int32]int32{1: 2},
		MapSint64:   map[int64]int64{1: 2},
		MapFixed32:  map[uint32]uint32{1: 2},
		MapFixed64:  map[uint64]uint64{1: 2},
		MapSfixed32: map[int32]int32{1: 2},
		MapSfixed64: map[int64]int64{1: 2},
		MapBool:     map[bool]bool{false: true},
		MapString:   map[string]string{"foo": "bar"},
		MapBytes:    map[string][]byte{"foo": []byte("bar")},
		MapFloat:    map[string]float32{"foo": 1.1},
		MapDouble:   map[string]float64{"foo": 2.2},
		MapEnum:     map[string]testpb.Message_Enum{"foo": testpb.Message_FOO},
		MapMessage:  map[string]*testpb.EmbeddedMessage{"foo": {Field: 10}},
		OneofField: &testpb.Message_OneofBoolField{
			OneofBoolField: true,
		},
	})

	var msg testpb.Message
	if err := Decode(&msg, bytes.NewReader(b)); err != nil {
		t.Fatalf("failed to Decode: %s", err)
	}

	var want testpb.Message
	if err := proto.Unmarshal(b, &want); err != nil {
		t.Fatalf("failed to Unmarshal: %s", err)
	}

	if diff := cmp.Diff(&want, &msg, protocmp.Transform()); diff != "" {
		t.Errorf("(-want, +got):\n%s", diff)
	}
}

func marshal(t *testing.T, m proto.Message) []byte {
	t.Helper()

	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("failed to marshal: %s", err)
	}

	return b
}
