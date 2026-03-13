package cgrpc

import (
	"errors"

	"google.golang.org/grpc/encoding"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var Name = "json"

func init() {
	encoding.RegisterCodec(&jsoncodec{})
}

type jsoncodec struct{}

func (c *jsoncodec) Marshal(v any) ([]byte, error) {
	vv, ok := v.(protoreflect.ProtoMessage)
	if !ok {
		return nil, errors.New("failed to marshal,not a proto.Message")
	}
	return protojson.MarshalOptions{UseProtoNames: true, UseEnumNumbers: true}.Marshal(vv)
}

func (c *jsoncodec) Unmarshal(data []byte, v any) error {
	vv, ok := v.(protoreflect.ProtoMessage)
	if !ok {
		return errors.New("failed to unmarshal,not a proto.Message")
	}
	return protojson.Unmarshal(data, vv)
}
func (c *jsoncodec) Name() string {
	return Name
}
