package pbex

import (
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func HasOneOf(message *protogen.Message) bool {
	checked := make(map[string]*struct{})
	return hasoneof(message, checked)
}
func hasoneof(message *protogen.Message, checked map[string]*struct{}) bool {
	if _, ok := checked[message.GoIdent.String()]; ok {
		return false
	}
	checked[message.GoIdent.String()] = nil
	if len(message.Oneofs) > 0 {
		return true
	}
	for _, field := range message.Fields {
		if field.Desc.Kind() == protoreflect.MessageKind {
			if field.Desc.IsMap() {
				//map
				if field.Message.Fields[1].Desc.Kind() == protoreflect.MessageKind {
					//map's value is message
					if hasoneof(field.Message.Fields[1].Message, checked) {
						return true
					}
				}
			} else {
				//[]message or message
				if hasoneof(field.Message, checked) {
					return true
				}
			}
		}
	}
	return false
}
func NeedCheck(message *protogen.Message) bool {
	checked := make(map[string]*struct{})
	return needcheck(message, checked)
}
func needcheck(message *protogen.Message, checked map[string]*struct{}) bool {
	if _, ok := checked[message.GoIdent.String()]; ok {
		return false
	}
	checked[message.GoIdent.String()] = nil
	for _, field := range message.Fields {
		fop := field.Desc.Options().(*descriptorpb.FieldOptions)
		if field.Desc.IsList() || field.Desc.IsMap() {
			if proto.HasExtension(fop, E_MapRepeatedLenEq) ||
				proto.HasExtension(fop, E_MapRepeatedLenNotEq) ||
				proto.HasExtension(fop, E_MapRepeatedLenGt) ||
				proto.HasExtension(fop, E_MapRepeatedLenGte) ||
				proto.HasExtension(fop, E_MapRepeatedLenLt) ||
				proto.HasExtension(fop, E_MapRepeatedLenLte) {
				return true
			}
		}
		switch field.Desc.Kind() {
		case protoreflect.BoolKind:
			//bool
			if proto.HasExtension(fop, E_BoolEq) {
				return true
			}
		case protoreflect.Int32Kind:
			fallthrough
		case protoreflect.Sint32Kind:
			fallthrough
		case protoreflect.Sfixed32Kind:
			fallthrough
			//int32 or []int32
		case protoreflect.Int64Kind:
			fallthrough
		case protoreflect.Sint64Kind:
			fallthrough
		case protoreflect.Sfixed64Kind:
			//int64 or []int64
			if proto.HasExtension(fop, E_IntIn) ||
				proto.HasExtension(fop, E_IntNotIn) ||
				proto.HasExtension(fop, E_IntGt) ||
				proto.HasExtension(fop, E_IntGte) ||
				proto.HasExtension(fop, E_IntLt) ||
				proto.HasExtension(fop, E_IntLte) {
				return true
			}
		case protoreflect.Uint32Kind:
			fallthrough
		case protoreflect.Fixed32Kind:
			fallthrough
			//uint32 or []uint32
		case protoreflect.Uint64Kind:
			fallthrough
		case protoreflect.Fixed64Kind:
			//uint64 or []uint64
			if proto.HasExtension(fop, E_UintIn) ||
				proto.HasExtension(fop, E_UintNotIn) ||
				proto.HasExtension(fop, E_UintGt) ||
				proto.HasExtension(fop, E_UintGte) ||
				proto.HasExtension(fop, E_UintLt) ||
				proto.HasExtension(fop, E_UintLte) {
				return true
			}
		case protoreflect.FloatKind:
			//float32 or []float32
			fallthrough
		case protoreflect.DoubleKind:
			//float64 or []float64
			if proto.HasExtension(fop, E_FloatIn) ||
				proto.HasExtension(fop, E_FloatNotIn) ||
				proto.HasExtension(fop, E_FloatGt) ||
				proto.HasExtension(fop, E_FloatGte) ||
				proto.HasExtension(fop, E_FloatLt) ||
				proto.HasExtension(fop, E_FloatLte) {
				return true
			}
		case protoreflect.EnumKind:
			//enum or []enum
			return true
		case protoreflect.BytesKind:
			//[]bytes or [][]bytes
			fallthrough
		case protoreflect.StringKind:
			//string or []string
			if proto.HasExtension(fop, E_StringBytesIn) ||
				proto.HasExtension(fop, E_StringBytesNotIn) ||
				proto.HasExtension(fop, E_StringBytesRegMatch) ||
				proto.HasExtension(fop, E_StringBytesRegNotMatch) ||
				proto.HasExtension(fop, E_StringBytesLenEq) ||
				proto.HasExtension(fop, E_StringBytesLenNotEq) ||
				proto.HasExtension(fop, E_StringBytesLenGt) ||
				proto.HasExtension(fop, E_StringBytesLenGte) ||
				proto.HasExtension(fop, E_StringBytesLenLt) ||
				proto.HasExtension(fop, E_StringBytesLenLte) {
				return true
			}
		case protoreflect.MessageKind:
			if field.Desc.IsMap() {
				key := field.Message.Fields[0]
				value := field.Message.Fields[1]
				switch key.Desc.Kind() {
				case protoreflect.Int32Kind:
					fallthrough
				case protoreflect.Sint32Kind:
					fallthrough
				case protoreflect.Sfixed32Kind:
					fallthrough
				case protoreflect.Int64Kind:
					fallthrough
				case protoreflect.Sint64Kind:
					fallthrough
				case protoreflect.Sfixed64Kind:
					if proto.HasExtension(fop, E_MapKeyIntIn) ||
						proto.HasExtension(fop, E_MapKeyIntNotIn) ||
						proto.HasExtension(fop, E_MapKeyIntGt) ||
						proto.HasExtension(fop, E_MapKeyIntGte) ||
						proto.HasExtension(fop, E_MapKeyIntLt) ||
						proto.HasExtension(fop, E_MapKeyIntLte) {
						return true
					}
				case protoreflect.Uint32Kind:
					fallthrough
				case protoreflect.Fixed32Kind:
					fallthrough
				case protoreflect.Uint64Kind:
					fallthrough
				case protoreflect.Fixed64Kind:
					if proto.HasExtension(fop, E_MapKeyUintIn) ||
						proto.HasExtension(fop, E_MapKeyUintNotIn) ||
						proto.HasExtension(fop, E_MapKeyUintGt) ||
						proto.HasExtension(fop, E_MapKeyUintGte) ||
						proto.HasExtension(fop, E_MapKeyUintLt) ||
						proto.HasExtension(fop, E_MapKeyUintLte) {
						return true
					}
				case protoreflect.StringKind:
					if proto.HasExtension(fop, E_MapKeyStringIn) ||
						proto.HasExtension(fop, E_MapKeyStringNotIn) ||
						proto.HasExtension(fop, E_MapKeyStringRegMatch) ||
						proto.HasExtension(fop, E_MapKeyStringRegNotMatch) ||
						proto.HasExtension(fop, E_MapKeyStringLenEq) ||
						proto.HasExtension(fop, E_MapKeyStringLenNotEq) ||
						proto.HasExtension(fop, E_MapKeyStringLenGt) ||
						proto.HasExtension(fop, E_MapKeyStringLenGte) ||
						proto.HasExtension(fop, E_MapKeyStringLenLt) ||
						proto.HasExtension(fop, E_MapKeyStringLenLte) {
						return true
					}
				}
				switch value.Desc.Kind() {
				case protoreflect.EnumKind:
					return true
				case protoreflect.BoolKind:
					if proto.HasExtension(fop, E_MapValueBoolEq) {
						return true
					}
				case protoreflect.Int32Kind:
					fallthrough
				case protoreflect.Sint32Kind:
					fallthrough
				case protoreflect.Sfixed32Kind:
					fallthrough
				case protoreflect.Int64Kind:
					fallthrough
				case protoreflect.Sint64Kind:
					fallthrough
				case protoreflect.Sfixed64Kind:
					if proto.HasExtension(fop, E_MapValueIntIn) ||
						proto.HasExtension(fop, E_MapValueIntNotIn) ||
						proto.HasExtension(fop, E_MapValueIntGt) ||
						proto.HasExtension(fop, E_MapValueIntGte) ||
						proto.HasExtension(fop, E_MapValueIntLt) ||
						proto.HasExtension(fop, E_MapValueIntLte) {
						return true
					}
				case protoreflect.Uint32Kind:
					fallthrough
				case protoreflect.Fixed32Kind:
					fallthrough
				case protoreflect.Uint64Kind:
					fallthrough
				case protoreflect.Fixed64Kind:
					if proto.HasExtension(fop, E_MapValueUintIn) ||
						proto.HasExtension(fop, E_MapValueUintNotIn) ||
						proto.HasExtension(fop, E_MapValueUintGt) ||
						proto.HasExtension(fop, E_MapValueUintGte) ||
						proto.HasExtension(fop, E_MapValueUintLt) ||
						proto.HasExtension(fop, E_MapValueUintLte) {
						return true
					}
				case protoreflect.FloatKind:
					fallthrough
				case protoreflect.DoubleKind:
					if proto.HasExtension(fop, E_MapValueFloatIn) ||
						proto.HasExtension(fop, E_MapValueFloatNotIn) ||
						proto.HasExtension(fop, E_MapValueFloatGt) ||
						proto.HasExtension(fop, E_MapValueFloatGte) ||
						proto.HasExtension(fop, E_MapValueFloatLt) ||
						proto.HasExtension(fop, E_MapValueFloatLte) {
						return true
					}
				case protoreflect.BytesKind:
					fallthrough
				case protoreflect.StringKind:
					if proto.HasExtension(fop, E_MapValueStringBytesIn) ||
						proto.HasExtension(fop, E_MapValueStringBytesNotIn) ||
						proto.HasExtension(fop, E_MapValueStringBytesRegMatch) ||
						proto.HasExtension(fop, E_MapValueStringBytesRegNotMatch) ||
						proto.HasExtension(fop, E_MapValueStringBytesLenEq) ||
						proto.HasExtension(fop, E_MapValueStringBytesLenNotEq) ||
						proto.HasExtension(fop, E_MapValueStringBytesLenGt) ||
						proto.HasExtension(fop, E_MapValueStringBytesLenGte) ||
						proto.HasExtension(fop, E_MapValueStringBytesLenLt) ||
						proto.HasExtension(fop, E_MapValueStringBytesLenLte) {
						return true
					}
				case protoreflect.MessageKind:
					if proto.HasExtension(fop, E_MapValueMessageNotNil) {
						return true
					}
					if needcheck(value.Message, checked) {
						return true
					}
				}
			} else {
				//message or []message
				if proto.HasExtension(fop, E_MessageNotNil) {
					return true
				}
				if needcheck(field.Message, checked) {
					return true
				}
			}
		}
	}
	return false
}
