package platforms

import (
	"encoding/binary"
	"errors"
)

type protoField struct {
	Number uint64
	Bytes  []byte
}
type protoFields map[uint64][]protoField

func readProto(data []byte) (protoFields, error) {
	out := protoFields{}
	for len(data) > 0 {
		key, n := binary.Uvarint(data)
		if n <= 0 || key>>3 == 0 {
			return nil, errors.New("invalid protobuf field")
		}
		data = data[n:]
		var field protoField
		switch key & 7 {
		case 0:
			field.Number, n = binary.Uvarint(data)
			if n <= 0 {
				return nil, errors.New("invalid protobuf number")
			}
			data = data[n:]
		case 1, 5:
			n = 8
			if key&7 == 5 {
				n = 4
			}
			if len(data) < n {
				return nil, errors.New("truncated protobuf field")
			}
			data = data[n:]
		case 2:
			length, k := binary.Uvarint(data)
			if k <= 0 || length > uint64(len(data)-k) {
				return nil, errors.New("truncated protobuf bytes")
			}
			data = data[k:]
			field.Bytes = data[:int(length)]
			data = data[int(length):]
		default:
			return nil, errors.New("unsupported protobuf field")
		}
		out[key>>3] = append(out[key>>3], field)
	}
	return out, nil
}
func (p protoFields) first(tag uint64) protoField {
	if len(p[tag]) > 0 {
		return p[tag][0]
	}
	return protoField{}
}
func protoNumber(dst []byte, tag, value uint64) []byte {
	dst = binary.AppendUvarint(dst, tag<<3)
	return binary.AppendUvarint(dst, value)
}
func protoBytes(dst []byte, tag uint64, data []byte) []byte {
	dst = binary.AppendUvarint(dst, tag<<3|2)
	dst = binary.AppendUvarint(dst, uint64(len(data)))
	return append(dst, data...)
}
