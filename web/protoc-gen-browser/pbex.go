package main

import (
	"encoding/json"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/chenjie199234/Corelib/pbex"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func genPbex(field *protogen.Field, f *protogen.GeneratedFile) {
	oneof := field.Oneof != nil
	indent := "\t"
	if oneof {
		indent = "\t\t"
	}
	if pbex.FieldHasPBEX(field) ||
		field.Desc.Kind() == protoreflect.EnumKind ||
		(field.Desc.IsMap() && field.Message.Fields[1].Desc.Kind() == protoreflect.EnumKind) {
		fop := field.Desc.Options().(*descriptorpb.FieldOptions)
		f.P(indent, "/*if this is in a request,the following rules must be obeyed")
		if field.Desc.IsMap() || field.Desc.IsList() {
			elementpbex(field, fop, f, oneof)
		}
		switch field.Desc.Kind() {
		case protoreflect.BoolKind:
			boolpbex(field, fop, f, oneof)
		case protoreflect.EnumKind:
			enumpbex(field, fop, f, oneof)
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
			//int
			intpbex(field, fop, f, true, oneof)
		case protoreflect.Uint32Kind:
			fallthrough
		case protoreflect.Fixed32Kind:
			fallthrough
		case protoreflect.Uint64Kind:
			fallthrough
		case protoreflect.Fixed64Kind:
			//uint
			uintpbex(field, fop, f, true, oneof)
		case protoreflect.FloatKind:
			fallthrough
		case protoreflect.DoubleKind:
			//float
			floatpbex(field, fop, f, oneof)
		case protoreflect.BytesKind:
			fallthrough
		case protoreflect.StringKind:
			strpbex(field, fop, f, true, oneof)
		case protoreflect.MessageKind:
			if field.Desc.IsMap() {
				mappbex(field, fop, f)
			} else {
				msgpbex(field, fop, f, oneof)
			}
		}
		f.P(indent, "*/")
	}
}
func mappbex(field *protogen.Field, fop *descriptorpb.FieldOptions, g *protogen.GeneratedFile) {
	key := field.Message.Fields[0]
	val := field.Message.Fields[1]
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
		if proto.HasExtension(fop, pbex.E_MapKeyIntIn) ||
			proto.HasExtension(fop, pbex.E_MapKeyIntNotIn) ||
			proto.HasExtension(fop, pbex.E_MapKeyIntGt) ||
			proto.HasExtension(fop, pbex.E_MapKeyIntGte) ||
			proto.HasExtension(fop, pbex.E_MapKeyIntLt) ||
			proto.HasExtension(fop, pbex.E_MapKeyIntLte) {
			intpbex(field, fop, g, true, false)
		}
	case protoreflect.Uint32Kind:
		fallthrough
	case protoreflect.Fixed32Kind:
		fallthrough
	case protoreflect.Uint64Kind:
		fallthrough
	case protoreflect.Fixed64Kind:
		if proto.HasExtension(fop, pbex.E_MapKeyUintIn) ||
			proto.HasExtension(fop, pbex.E_MapKeyUintNotIn) ||
			proto.HasExtension(fop, pbex.E_MapKeyUintGt) ||
			proto.HasExtension(fop, pbex.E_MapKeyUintGte) ||
			proto.HasExtension(fop, pbex.E_MapKeyUintLt) ||
			proto.HasExtension(fop, pbex.E_MapKeyUintLte) {
			uintpbex(field, fop, g, true, false)
		}
	case protoreflect.StringKind:
		if proto.HasExtension(fop, pbex.E_MapKeyStringIn) ||
			proto.HasExtension(fop, pbex.E_MapKeyStringNotIn) ||
			proto.HasExtension(fop, pbex.E_MapKeyStringRegMatch) ||
			proto.HasExtension(fop, pbex.E_MapKeyStringRegNotMatch) ||
			proto.HasExtension(fop, pbex.E_MapKeyStringLenEq) ||
			proto.HasExtension(fop, pbex.E_MapKeyStringLenNotEq) ||
			proto.HasExtension(fop, pbex.E_MapKeyStringLenGt) ||
			proto.HasExtension(fop, pbex.E_MapKeyStringLenGte) ||
			proto.HasExtension(fop, pbex.E_MapKeyStringLenLt) ||
			proto.HasExtension(fop, pbex.E_MapKeyStringLenLte) {
			strpbex(field, fop, g, true, false)
		}
	}
	switch val.Desc.Kind() {
	case protoreflect.EnumKind:
		enumpbex(field, fop, g, false)
	case protoreflect.BoolKind:
		if proto.HasExtension(fop, pbex.E_MapValueBoolEq) {
			boolpbex(field, fop, g, false)
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
		if proto.HasExtension(fop, pbex.E_MapValueIntIn) ||
			proto.HasExtension(fop, pbex.E_MapValueIntNotIn) ||
			proto.HasExtension(fop, pbex.E_MapValueIntGt) ||
			proto.HasExtension(fop, pbex.E_MapValueIntGte) ||
			proto.HasExtension(fop, pbex.E_MapValueIntLt) ||
			proto.HasExtension(fop, pbex.E_MapValueIntLte) {
			intpbex(field, fop, g, false, false)
		}
	case protoreflect.Uint32Kind:
		fallthrough
	case protoreflect.Fixed32Kind:
		fallthrough
	case protoreflect.Uint64Kind:
		fallthrough
	case protoreflect.Fixed64Kind:
		if proto.HasExtension(fop, pbex.E_MapValueUintIn) ||
			proto.HasExtension(fop, pbex.E_MapValueUintNotIn) ||
			proto.HasExtension(fop, pbex.E_MapValueUintGt) ||
			proto.HasExtension(fop, pbex.E_MapValueUintGte) ||
			proto.HasExtension(fop, pbex.E_MapValueUintLt) ||
			proto.HasExtension(fop, pbex.E_MapValueUintLte) {
			uintpbex(field, fop, g, false, false)
		}
	case protoreflect.FloatKind:
		fallthrough
	case protoreflect.DoubleKind:
		if proto.HasExtension(fop, pbex.E_MapValueFloatIn) ||
			proto.HasExtension(fop, pbex.E_MapValueFloatNotIn) ||
			proto.HasExtension(fop, pbex.E_MapValueFloatGt) ||
			proto.HasExtension(fop, pbex.E_MapValueFloatGte) ||
			proto.HasExtension(fop, pbex.E_MapValueFloatLt) ||
			proto.HasExtension(fop, pbex.E_MapValueFloatLte) {
			floatpbex(field, fop, g, false)
		}
	case protoreflect.BytesKind:
		fallthrough
	case protoreflect.StringKind:
		if proto.HasExtension(fop, pbex.E_MapValueStringBytesIn) ||
			proto.HasExtension(fop, pbex.E_MapValueStringBytesNotIn) ||
			proto.HasExtension(fop, pbex.E_MapValueStringBytesRegMatch) ||
			proto.HasExtension(fop, pbex.E_MapValueStringBytesRegNotMatch) ||
			proto.HasExtension(fop, pbex.E_MapValueStringBytesLenEq) ||
			proto.HasExtension(fop, pbex.E_MapValueStringBytesLenNotEq) ||
			proto.HasExtension(fop, pbex.E_MapValueStringBytesLenGt) ||
			proto.HasExtension(fop, pbex.E_MapValueStringBytesLenGte) ||
			proto.HasExtension(fop, pbex.E_MapValueStringBytesLenLt) ||
			proto.HasExtension(fop, pbex.E_MapValueStringBytesLenLte) {
			strpbex(field, fop, g, false, false)
		}
	case protoreflect.MessageKind:
		if proto.HasExtension(fop, pbex.E_MapValueMessageNotNil) || pbex.NeedValidate(val.Message) {
			msgpbex(field, fop, g, false)
		}
	}
}
func elementpbex(field *protogen.Field, fop *descriptorpb.FieldOptions, g *protogen.GeneratedFile, oneof bool) {
	indent := "\t\t"
	if oneof {
		indent = "\t\t\t"
	}
	var eq, noteq, gt, gte, lt, lte *uint64
	if proto.HasExtension(fop, pbex.E_MapRepeatedLenEq) {
		leneq := proto.GetExtension(fop, pbex.E_MapRepeatedLenEq).(uint64)
		if leneq > math.MaxInt64 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		eq = &leneq
	}
	if proto.HasExtension(fop, pbex.E_MapRepeatedLenNotEq) {
		lennoteq := proto.GetExtension(fop, pbex.E_MapRepeatedLenNotEq).(uint64)
		if lennoteq > math.MaxInt64 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if eq != nil && *eq == lennoteq {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if eq == nil {
			noteq = &lennoteq
		}
	}
	if proto.HasExtension(fop, pbex.E_MapRepeatedLenGt) {
		lengt := proto.GetExtension(fop, pbex.E_MapRepeatedLenGt).(uint64)
		if lengt >= math.MaxInt64 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if eq != nil && *eq <= lengt {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if noteq != nil && *noteq <= lengt {
			noteq = nil
		}
		if lengt+1 == math.MaxInt64 {
			if noteq != nil && *noteq == math.MaxInt64 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if eq == nil {
				tmpeq := uint64(math.MaxInt64)
				eq = &tmpeq
				noteq = nil
			}
		}
		if eq == nil && lengt+2 == math.MaxInt64 {
			if noteq != nil {
				switch *noteq {
				case math.MaxInt64:
					tmpeq := uint64(math.MaxInt64 - 1)
					eq = &tmpeq
					noteq = nil
				case math.MaxInt64 - 1:
					tmpeq := uint64(math.MaxInt64)
					eq = &tmpeq
					noteq = nil
				}
			}
		}
		if eq == nil {
			gt = &lengt
		}
	}
	if proto.HasExtension(fop, pbex.E_MapRepeatedLenGte) {
		lengte := proto.GetExtension(fop, pbex.E_MapRepeatedLenGte).(uint64)
		if lengte == 0 {
			panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if lengte > math.MaxInt64 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if eq != nil && *eq < lengte {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if noteq != nil && *noteq < lengte {
			noteq = nil
		}
		if lengte == math.MaxInt64 {
			if noteq != nil && *noteq == math.MaxInt64 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if eq == nil {
				eq = &lengte
				noteq = nil
			}
		}
		if eq == nil && lengte+1 == math.MaxInt64 {
			if noteq != nil {
				switch *noteq {
				case math.MaxInt64:
					tmpeq := uint64(math.MaxInt64 - 1)
					eq = &tmpeq
					noteq = nil
				case math.MaxInt64 - 1:
					tmpeq := uint64(math.MaxInt64)
					eq = &tmpeq
					noteq = nil
				}
			}
		}
		if eq == nil {
			gte = &lengte
		}
	}
	if proto.HasExtension(fop, pbex.E_MapRepeatedLenLt) {
		lenlt := proto.GetExtension(fop, pbex.E_MapRepeatedLenLt).(uint64)
		if lenlt == 0 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if lenlt > math.MaxInt64 {
			panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if eq != nil && *eq >= lenlt {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if noteq != nil && *noteq >= lenlt {
			noteq = nil
		}
		if lenlt == 1 {
			if noteq != nil && *noteq == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if eq == nil {
				tmpeq := uint64(0)
				eq = &tmpeq
				noteq = nil
			}
		}
		if eq == nil && lenlt == 2 {
			if noteq != nil {
				switch *noteq {
				case 0:
					tmpeq := uint64(1)
					eq = &tmpeq
					noteq = nil
				case 1:
					tmpeq := uint64(0)
					eq = &tmpeq
					noteq = nil
				}
			}
		}
		if eq == nil {
			lt = &lenlt
		}
	}
	if proto.HasExtension(fop, pbex.E_MapRepeatedLenLte) {
		lenlte := proto.GetExtension(fop, pbex.E_MapRepeatedLenLte).(uint64)
		if lenlte >= math.MaxInt64 {
			panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if eq != nil && *eq > lenlte {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if noteq != nil && *noteq > lenlte {
			noteq = nil
		}
		if lenlte == 0 {
			if noteq != nil && *noteq == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if eq == nil {
				eq = &lenlte
				noteq = nil
			}
		}
		if eq == nil && lenlte == 1 {
			if noteq != nil {
				switch *noteq {
				case 0:
					tmpeq := uint64(1)
					eq = &tmpeq
					noteq = nil
				case 1:
					tmpeq := uint64(0)
					eq = &tmpeq
					noteq = nil
				}
			}
		}
		if eq == nil {
			lte = &lenlte
		}
	}
	if gte != nil && gt != nil {
		if *gte > *gt {
			*gt = *gte - 1
		}
		gte = nil
	} else if gte != nil {
		gt = gte
		(*gt)--
		gte = nil
	}
	if lte != nil && lt != nil {
		if *lte < *lt {
			*lt = *lte + 1
		}
		lte = nil
	} else if lte != nil {
		lt = lte
		(*lt)++
		lte = nil
	}
	if lt != nil && gt != nil && ((*gt) >= (*lt) || (*gt) >= (*lt)-1) {
		panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
	}
	if eq == nil && gt != nil && lt != nil && (*gt) == (*lt)-2 {
		if noteq != nil && *noteq == (*gt)+1 {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		eq = gt
		(*eq)++
		gt = nil
		lt = nil
		noteq = nil
	}
	if eq == nil && gt != nil && lt != nil && (*gt) == (*lt)-3 && noteq != nil {
		switch *noteq {
		case (*gt) + 1:
			eq = gt
			(*eq) += 2
			gt = nil
			lt = nil
			noteq = nil
		case (*gt) + 2:
			eq = gt
			(*eq)++
			gt = nil
			lt = nil
			noteq = nil
		}
	}
	if eq != nil {
		g.P(indent, "length/size must === ", *eq)
	} else if noteq != nil || gt != nil || lt != nil {
		all := make([]string, 0, 3)
		if gt != nil {
			if noteq != nil && (*gt)+1 == *noteq {
				all = append(all, ">"+strconv.FormatUint(*noteq, 10))
				noteq = nil
			} else if *gt == math.MaxInt64-1 {
				all = append(all, "==="+strconv.FormatUint(math.MaxInt64, 10))
			} else {
				all = append(all, ">"+strconv.FormatUint(*gt, 10))
			}
		}
		if lt != nil {
			if noteq != nil && (*lt)-1 == *noteq {
				all = append(all, "<"+strconv.FormatUint(*noteq, 10))
				noteq = nil
			} else if *lt == 1 {
				all = append(all, "===0")
			} else {
				all = append(all, "<"+strconv.FormatUint(*lt, 10))
			}
		}
		if noteq != nil {
			all = append(all, "!=="+strconv.FormatUint(*noteq, 10))
		}
		g.P(indent, "length/size must ", strings.Join(all, " || "))
	}
}
func boolpbex(field *protogen.Field, fop *descriptorpb.FieldOptions, g *protogen.GeneratedFile, oneof bool) {
	indent := "\t\t"
	if oneof {
		indent = "\t\t\t"
	}
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueBoolEq) {
			valeq := proto.GetExtension(fop, pbex.E_MapValueBoolEq).(bool)
			g.P(indent, "map's value must be ", valeq)
		}
	} else if proto.HasExtension(fop, pbex.E_BoolEq) {
		booleq := proto.GetExtension(fop, pbex.E_BoolEq).(bool)
		if field.Desc.IsList() {
			g.P(indent, "element value must be ", booleq)
		} else {
			g.P(indent, "value must be ", booleq)
		}
	}
}
func enumpbex(field *protogen.Field, fop *descriptorpb.FieldOptions, g *protogen.GeneratedFile, oneof bool) {
	indent := "\t\t"
	if oneof {
		indent = "\t\t\t"
	}
	target := ""
	if field.Desc.IsMap() {
		target = "map's value"
	} else if field.Desc.IsList() {
		target = "element value"
	} else {
		target = "value"
	}
	values := make([]int64, 0)
	if field.Desc.IsMap() {
		for _, v := range field.Message.Fields[1].Enum.Values {
			values = append(values, int64(v.Desc.Number()))
		}
	} else {
		for _, v := range field.Enum.Values {
			values = append(values, int64(v.Desc.Number()))
		}
	}
	var in, notin []int64
	var gt, gte, lt, lte *int64
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueEnumIn) {
			in = proto.GetExtension(fop, pbex.E_MapValueEnumIn).([]int64)
		}
	} else if proto.HasExtension(fop, pbex.E_EnumIn) {
		in = proto.GetExtension(fop, pbex.E_EnumIn).([]int64)
	}
	if len(in) > 0 {
		in = slices.DeleteFunc(in, func(e int64) bool {
			return !slices.Contains(values, e)
		})
		values = in
		if len(values) == 0 {
			panic("pbex options conflict with enum in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueEnumNotIn) {
			notin = proto.GetExtension(fop, pbex.E_MapValueEnumNotIn).([]int64)
		}
	} else if proto.HasExtension(fop, pbex.E_EnumNotIn) {
		notin = proto.GetExtension(fop, pbex.E_EnumNotIn).([]int64)
	}
	if len(notin) > 0 {
		values = slices.DeleteFunc(values, func(e int64) bool {
			return slices.Contains(notin, e)
		})
		if len(values) == 0 {
			panic("pbex options conflict with enum in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueEnumGt) {
			tmpgt := proto.GetExtension(fop, pbex.E_MapValueEnumGt).(int64)
			gt = &tmpgt
		}
	} else if proto.HasExtension(fop, pbex.E_EnumGt) {
		tmpgt := proto.GetExtension(fop, pbex.E_EnumGt).(int64)
		gt = &tmpgt
	}
	if gt != nil {
		values = slices.DeleteFunc(values, func(e int64) bool {
			return e <= *gt
		})
		if len(values) == 0 {
			panic("pbex options conflict with enum in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueEnumGte) {
			tmpgte := proto.GetExtension(fop, pbex.E_MapValueEnumGte).(int64)
			gte = &tmpgte
		}
	} else if proto.HasExtension(fop, pbex.E_EnumGte) {
		tmpgte := proto.GetExtension(fop, pbex.E_EnumGte).(int64)
		gte = &tmpgte
	}
	if gte != nil {
		values = slices.DeleteFunc(values, func(e int64) bool {
			return e < *gte
		})
		if len(values) == 0 {
			panic("pbex options conflict with enum in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueEnumLt) {
			tmplt := proto.GetExtension(fop, pbex.E_MapValueEnumLt).(int64)
			lt = &tmplt
		}
	} else if proto.HasExtension(fop, pbex.E_EnumLt) {
		tmplt := proto.GetExtension(fop, pbex.E_EnumLt).(int64)
		lt = &tmplt
	}
	if lt != nil {
		values = slices.DeleteFunc(values, func(e int64) bool {
			return e >= *lt
		})
		if len(values) == 0 {
			panic("pbex options conflict with enum in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueEnumLte) {
			tmplte := proto.GetExtension(fop, pbex.E_MapValueEnumLte).(int64)
			lte = &tmplte
		}
	} else if proto.HasExtension(fop, pbex.E_EnumLte) {
		tmplte := proto.GetExtension(fop, pbex.E_EnumLte).(int64)
		lte = &tmplte
	}
	if lte != nil {
		values = slices.DeleteFunc(values, func(e int64) bool {
			return e > *lte
		})
		if len(values) == 0 {
			panic("pbex options conflict with enum in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	d, _ := json.Marshal(values)
	g.P(indent, target, " must in ", string(d))
}

// mapkv: only useful when the filed's type is map
// true-map key,false-map value
func intpbex(field *protogen.Field, fop *descriptorpb.FieldOptions, g *protogen.GeneratedFile, mapkv, oneof bool) {
	var bit uint8
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if field.Message.Fields[0].Desc.Kind() == protoreflect.Int32Kind ||
				field.Message.Fields[0].Desc.Kind() == protoreflect.Sint32Kind ||
				field.Message.Fields[0].Desc.Kind() == protoreflect.Sfixed32Kind {
				bit = 32
			} else {
				bit = 64
			}
		} else {
			//value
			if field.Message.Fields[1].Desc.Kind() == protoreflect.Int32Kind ||
				field.Message.Fields[1].Desc.Kind() == protoreflect.Sint32Kind ||
				field.Message.Fields[1].Desc.Kind() == protoreflect.Sfixed32Kind {
				bit = 32
			} else {
				bit = 64
			}
		}
	} else if field.Desc.Kind() == protoreflect.Int32Kind ||
		field.Desc.Kind() == protoreflect.Sint32Kind ||
		field.Desc.Kind() == protoreflect.Sfixed32Kind {
		bit = 32
	} else {
		bit = 64
	}
	var in, notin []int64
	var gt, gte, lt, lte *int64
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyIntIn) {
				in = proto.GetExtension(fop, pbex.E_MapKeyIntIn).([]int64)
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueIntIn) {
				in = proto.GetExtension(fop, pbex.E_MapValueIntIn).([]int64)
			}
		}
	} else if proto.HasExtension(fop, pbex.E_IntIn) {
		in = proto.GetExtension(fop, pbex.E_IntIn).([]int64)
	}
	if len(in) > 0 && bit == 32 {
		for _, v := range in {
			if v <= math.MaxInt32 && v >= math.MinInt32 {
				continue
			}
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyIntNotIn) {
				notin = proto.GetExtension(fop, pbex.E_MapKeyIntNotIn).([]int64)
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueIntNotIn) {
				notin = proto.GetExtension(fop, pbex.E_MapValueIntNotIn).([]int64)
			}
		}
	} else if proto.HasExtension(fop, pbex.E_IntNotIn) {
		notin = proto.GetExtension(fop, pbex.E_IntNotIn).([]int64)
	}
	if len(notin) > 1 {
		dup := make(map[int64]*struct{})
		for _, v := range notin {
			dup[v] = nil
		}
		notin = notin[:0]
		for k := range dup {
			notin = append(notin, k)
		}
	}
	if len(notin) > 0 && bit == 32 {
		for _, v := range notin {
			if v <= math.MaxInt32 && v >= math.MinInt32 {
				continue
			}
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	if len(notin) > 0 && len(in) > 0 {
		in = slices.DeleteFunc(in, func(e int64) bool {
			return slices.Contains(notin, e)
		})
		if len(in) == 0 {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		notin = nil
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyIntGt) {
				tmpgt := proto.GetExtension(fop, pbex.E_MapKeyIntGt).(int64)
				gt = &tmpgt
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueIntGt) {
				tmpgt := proto.GetExtension(fop, pbex.E_MapValueIntGt).(int64)
				gt = &tmpgt
			}
		}
	} else if proto.HasExtension(fop, pbex.E_IntGt) {
		tmpgt := proto.GetExtension(fop, pbex.E_IntGt).(int64)
		gt = &tmpgt
	}
	if gt != nil {
		if bit == 32 {
			if *gt >= math.MaxInt32 {
				panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if *gt < math.MinInt32 {
				panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else if *gt == math.MaxInt64 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e int64) bool {
				return e <= *gt
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		notin = slices.DeleteFunc(notin, func(e int64) bool {
			return e <= *gt
		})
		if len(in) > 0 {
			gt = nil
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyIntGte) {
				tmpgte := proto.GetExtension(fop, pbex.E_MapKeyIntGte).(int64)
				gte = &tmpgte
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueIntGte) {
				tmpgte := proto.GetExtension(fop, pbex.E_MapValueIntGte).(int64)
				gte = &tmpgte
			}
		}
	} else if proto.HasExtension(fop, pbex.E_IntGte) {
		tmpgte := proto.GetExtension(fop, pbex.E_IntGte).(int64)
		gte = &tmpgte
	}
	if gte != nil {
		if bit == 32 {
			if *gte > math.MaxInt32 {
				panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if *gte <= math.MinInt32 {
				panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else if *gte == math.MinInt64 {
			panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e int64) bool {
				return e < *gte
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		notin = slices.DeleteFunc(notin, func(e int64) bool {
			return e < *gte
		})
		if len(in) > 0 {
			gte = nil
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyIntLt) {
				tmplt := proto.GetExtension(fop, pbex.E_MapKeyIntLt).(int64)
				lt = &tmplt
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueIntLt) {
				tmplt := proto.GetExtension(fop, pbex.E_MapValueIntLt).(int64)
				lt = &tmplt
			}
		}
	} else if proto.HasExtension(fop, pbex.E_IntLt) {
		tmplt := proto.GetExtension(fop, pbex.E_IntLt).(int64)
		lt = &tmplt
	}
	if lt != nil {
		if bit == 32 {
			if *lt <= math.MinInt32 {
				panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if *lt > math.MaxInt32 {
				panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else if *lt == math.MinInt64 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e int64) bool {
				return e >= *lt
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		notin = slices.DeleteFunc(notin, func(e int64) bool {
			return e >= *lt
		})
		if len(in) > 0 {
			lt = nil
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyIntLte) {
				tmplte := proto.GetExtension(fop, pbex.E_MapKeyIntLte).(int64)
				lte = &tmplte
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueIntLte) {
				tmplte := proto.GetExtension(fop, pbex.E_MapValueIntLte).(int64)
				lte = &tmplte
			}
		}
	} else if proto.HasExtension(fop, pbex.E_IntLte) {
		tmplte := proto.GetExtension(fop, pbex.E_IntLte).(int64)
		lte = &tmplte
	}
	if lte != nil {
		if bit == 32 {
			if *lte < math.MinInt32 {
				panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if *lte >= math.MaxInt32 {
				panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else if *lte == math.MaxInt64 {
			panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e int64) bool {
				return e > *lte
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		notin = slices.DeleteFunc(notin, func(e int64) bool {
			return e > *lte
		})
		if len(in) > 0 {
			lte = nil
		}
	}
	if gte != nil && gt != nil {
		if *gte > *gt {
			*gt = *gte - 1
		}
		gte = nil
	} else if gte != nil {
		gt = gte
		(*gt)--
		gte = nil
	}
	if lte != nil && lt != nil {
		if *lte < *lt {
			*lt = *lte + 1
		}
		lte = nil
	} else if lte != nil {
		lt = lte
		(*lt)++
		lte = nil
	}
	// > 3 && != 4 is same as > 4
	if len(notin) > 0 && gt != nil {
		slices.Sort(notin)
		for _, v := range notin {
			if v == (*gt)+1 {
				(*gt)++
			}
		}
	}
	// < 3 && != 2 is same as < 2
	if len(notin) > 0 && lt != nil {
		slices.Sort(notin)
		slices.Reverse(notin)
		for _, v := range notin {
			if v == (*lt)-1 {
				(*lt)--
			}
		}
	}
	if gt != nil {
		notin = slices.DeleteFunc(notin, func(e int64) bool {
			return e <= *gt
		})
	}
	if lt != nil {
		notin = slices.DeleteFunc(notin, func(e int64) bool {
			return e >= *lt
		})
	}
	if len(in) == 0 && (gt != nil || lt != nil) {
		start := int64(0)
		end := int64(0)
		if gt != nil && lt != nil {
			start = (*gt) + 1
			end = (*lt) - 1
		} else if gt != nil {
			start = (*gt) + 1
			if bit == 32 {
				end = math.MaxInt32
			} else {
				end = math.MaxInt64
			}
		} else if lt != nil {
			if bit == 32 {
				start = math.MinInt32
			} else {
				start = math.MinInt64
			}
			end = (*lt) - 1
		}
		// if > != ...len(notin)... != <
		// len(notin)+2 is the largest calculate times
		// if the available value num <= len(notin)+2,we can reduce the calculate times by switch it to in mode
		tmp := make([]int64, 0, len(notin)+3)
		for i := start; i <= end; i++ {
			if !slices.Contains(notin, i) {
				tmp = append(tmp, i)
				if len(tmp) >= len(notin)+3 {
					break
				}
			}
		}
		if len(tmp) == 0 {
			//no element in the range is available
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		} else if len(tmp) <= len(notin)+2 {
			//switch to the in mode
			in = tmp
		} else {
			//len(notin)+3
			//do nothing
		}
	}
	indent := "\t\t"
	if oneof {
		indent = "\t\t\t"
	}
	target := ""
	if field.Desc.IsMap() {
		if mapkv {
			target = "map's key"
		} else {
			target = "map's value"
		}
	} else if field.Desc.IsList() {
		target = "element value"
	} else {
		target = "value"
	}
	if len(in) != 0 {
		dup := make(map[int64]*struct{})
		all := make([]string, 0, 10)
		for _, v := range in {
			if _, ok := dup[v]; ok {
				continue
			}
			dup[v] = nil
			all = append(all, strconv.FormatInt(v, 10))
		}
		g.P(indent, target, " must in [", strings.Join(all, ","), "]")
	} else if lt != nil || gt != nil || notin != nil {
		all := make([]string, 0, 10)
		if lt != nil {
			all = append(all, "<"+strconv.FormatInt(*lt, 10))
		}
		if gt != nil {
			all = append(all, ">"+strconv.FormatInt(*gt, 10))
		}
		for _, v := range notin {
			all = append(all, "!=="+strconv.FormatInt(v, 10))
		}
		g.P(indent, target, " must ", strings.Join(all, " && "))
	}
}

// mapkv: only useful when the filed's type is map
// true-map key,false-map value
func uintpbex(field *protogen.Field, fop *descriptorpb.FieldOptions, g *protogen.GeneratedFile, mapkv, oneof bool) {
	var bit uint8
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if field.Message.Fields[0].Desc.Kind() == protoreflect.Uint32Kind ||
				field.Message.Fields[0].Desc.Kind() == protoreflect.Fixed32Kind {
				bit = 32
			} else {
				bit = 64
			}
		} else {
			//value
			if field.Message.Fields[1].Desc.Kind() == protoreflect.Uint32Kind ||
				field.Message.Fields[1].Desc.Kind() == protoreflect.Fixed32Kind {
				bit = 32
			} else {
				bit = 64
			}
		}
	} else if field.Desc.Kind() == protoreflect.Uint32Kind ||
		field.Desc.Kind() == protoreflect.Fixed32Kind {
		bit = 32
	} else {
		bit = 64
	}
	var in, notin []uint64
	var gt, gte, lt, lte *uint64
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyUintIn) {
				in = proto.GetExtension(fop, pbex.E_MapKeyUintIn).([]uint64)
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueUintIn) {
				in = proto.GetExtension(fop, pbex.E_MapValueUintIn).([]uint64)
			}
		}
	} else if proto.HasExtension(fop, pbex.E_UintIn) {
		in = proto.GetExtension(fop, pbex.E_UintIn).([]uint64)
	}
	if len(in) > 0 && bit == 32 {
		for _, v := range in {
			if v <= math.MaxUint32 {
				continue
			}
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyUintNotIn) {
				notin = proto.GetExtension(fop, pbex.E_MapKeyUintNotIn).([]uint64)
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueUintNotIn) {
				notin = proto.GetExtension(fop, pbex.E_MapValueUintNotIn).([]uint64)
			}
		}
	} else if proto.HasExtension(fop, pbex.E_UintNotIn) {
		notin = proto.GetExtension(fop, pbex.E_UintNotIn).([]uint64)
	}
	if len(notin) > 1 {
		dup := make(map[uint64]*struct{})
		for _, v := range notin {
			dup[v] = nil
		}
		notin = notin[:0]
		for k := range dup {
			notin = append(notin, k)
		}
	}
	if len(notin) > 0 && bit == 32 {
		for _, v := range notin {
			if v <= math.MaxUint32 {
				continue
			}
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	if len(notin) > 0 && len(in) > 0 {
		in = slices.DeleteFunc(in, func(e uint64) bool {
			return slices.Contains(notin, e)
		})
		if len(in) == 0 {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		notin = nil
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyUintGt) {
				tmpgt := proto.GetExtension(fop, pbex.E_MapKeyUintGt).(uint64)
				gt = &tmpgt
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueUintGt) {
				tmpgt := proto.GetExtension(fop, pbex.E_MapValueUintGt).(uint64)
				gt = &tmpgt
			}
		}
	} else if proto.HasExtension(fop, pbex.E_UintGt) {
		tmpgt := proto.GetExtension(fop, pbex.E_UintGt).(uint64)
		gt = &tmpgt
	}
	if gt != nil {
		if bit == 32 {
			if *gt >= math.MaxUint32 {
				panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else if *gt == math.MaxUint64 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e uint64) bool {
				return e <= *gt
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		notin = slices.DeleteFunc(notin, func(e uint64) bool {
			return e <= *gt
		})
		if len(in) > 0 {
			gt = nil
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyUintGte) {
				tmpgte := proto.GetExtension(fop, pbex.E_MapKeyUintGte).(uint64)
				gte = &tmpgte
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueUintGte) {
				tmpgte := proto.GetExtension(fop, pbex.E_MapValueUintGte).(uint64)
				gte = &tmpgte
			}
		}
	} else if proto.HasExtension(fop, pbex.E_UintGte) {
		tmpgte := proto.GetExtension(fop, pbex.E_UintGte).(uint64)
		gte = &tmpgte
	}
	if gte != nil {
		if bit == 32 {
			if *gte > math.MaxUint32 {
				panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		if *gte == 0 {
			panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e uint64) bool {
				return e < *gte
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		notin = slices.DeleteFunc(notin, func(e uint64) bool {
			return e < *gte
		})
		if len(in) > 0 {
			gte = nil
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyUintLt) {
				tmplt := proto.GetExtension(fop, pbex.E_MapKeyUintLt).(uint64)
				lt = &tmplt
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueUintLt) {
				tmplt := proto.GetExtension(fop, pbex.E_MapValueUintLt).(uint64)
				lt = &tmplt
			}
		}
	} else if proto.HasExtension(fop, pbex.E_UintLt) {
		tmplt := proto.GetExtension(fop, pbex.E_UintLt).(uint64)
		lt = &tmplt
	}
	if lt != nil {
		if bit == 32 {
			if *lt > math.MaxUint32 {
				panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		if *lt == 0 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e uint64) bool {
				return e >= *lt
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		notin = slices.DeleteFunc(notin, func(e uint64) bool {
			return e >= *lt
		})
		if len(in) > 0 {
			lt = nil
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyUintLte) {
				tmplte := proto.GetExtension(fop, pbex.E_MapKeyUintLte).(uint64)
				lte = &tmplte
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueUintLte) {
				tmplte := proto.GetExtension(fop, pbex.E_MapValueUintLte).(uint64)
				lte = &tmplte
			}
		}
	} else if proto.HasExtension(fop, pbex.E_UintLte) {
		tmplte := proto.GetExtension(fop, pbex.E_UintLte).(uint64)
		lte = &tmplte
	}
	if lte != nil {
		if bit == 32 {
			if *lte >= math.MaxUint32 {
				panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else if *lte == math.MaxUint64 {
			panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e uint64) bool {
				return e > *lte
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		notin = slices.DeleteFunc(notin, func(e uint64) bool {
			return e > *lte
		})
		if len(in) > 0 {
			lte = nil
		}
	}
	if gte != nil && gt != nil {
		if *gte > *gt {
			*gt = *gte - 1
		}
		gte = nil
	} else if gte != nil {
		gt = gte
		(*gt)--
		gte = nil
	}
	if lte != nil && lt != nil {
		if *lte < *lt {
			*lt = *lte + 1
		}
		lte = nil
	} else if lte != nil {
		lt = lte
		(*lt)++
		lte = nil
	}
	// > 3 && != 4 is same as > 4
	if len(notin) > 0 && gt != nil {
		slices.Sort(notin)
		for _, v := range notin {
			if v == (*gt)+1 {
				(*gt)++
			}
		}
	}
	// < 3 && != 2 is same as < 2
	if len(notin) > 0 && lt != nil {
		slices.Sort(notin)
		slices.Reverse(notin)
		for _, v := range notin {
			if v == (*lt)-1 {
				(*lt)--
			}
		}
	}
	if gt != nil {
		notin = slices.DeleteFunc(notin, func(e uint64) bool {
			return e <= *gt
		})
	}
	if lt != nil {
		notin = slices.DeleteFunc(notin, func(e uint64) bool {
			return e >= *lt
		})
	}
	if len(in) == 0 && (gt != nil || lt != nil) {
		start := uint64(0)
		end := uint64(0)
		if gt != nil && lt != nil {
			start = (*gt) + 1
			end = (*lt) - 1
		} else if gt != nil {
			start = (*gt) + 1
			if bit == 32 {
				end = math.MaxUint32
			} else {
				end = math.MaxUint64
			}
		} else if lt != nil {
			end = (*lt) - 1
		}
		// if > != ...len(notin)... != <
		// len(notin)+2 is the largest calculate times
		// if the available value num <= len(notin)+2,we can reduce the calculate times by switch it to in mode
		tmp := make([]uint64, 0, len(notin)+3)
		for i := start; i <= end; i++ {
			if !slices.Contains(notin, i) {
				tmp = append(tmp, i)
				if len(tmp) >= len(notin)+3 {
					break
				}
			}
		}
		if len(tmp) == 0 {
			//no element in the range is available
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		} else if len(tmp) <= len(notin)+2 {
			//switch to the in mode
			in = tmp
		} else {
			//len(notin)+3
			//do nothing
		}
	}
	indent := "\t\t"
	if oneof {
		indent = "\t\t\t"
	}
	target := ""
	if field.Desc.IsMap() {
		if mapkv {
			target = "map's key"
		} else {
			target = "map's value"
		}
	} else if field.Desc.IsList() {
		target = "element value"
	} else {
		target = "value"
	}
	if len(in) > 0 {
		dup := make(map[uint64]*struct{})
		all := make([]string, 0, 10)
		for _, v := range in {
			if _, ok := dup[v]; ok {
				continue
			}
			dup[v] = nil
			all = append(all, strconv.FormatUint(v, 10))
		}
		g.P(indent, target, " must in [", strings.Join(all, ","), "]")
	} else if lt != nil || gt != nil || notin != nil {
		all := make([]string, 0, 10)
		if lt != nil {
			all = append(all, "<"+strconv.FormatUint(*lt, 10))
		}
		if gt != nil {
			all = append(all, ">"+strconv.FormatUint(*gt, 10))
		}
		for _, v := range notin {
			all = append(all, "!=="+strconv.FormatUint(v, 10))
		}
		g.P(indent, target, " must ", strings.Join(all, " && "))
	}
}

func floatpbex(field *protogen.Field, fop *descriptorpb.FieldOptions, g *protogen.GeneratedFile, oneof bool) {
	var bit uint8
	if field.Desc.IsMap() {
		if field.Message.Fields[1].Desc.Kind() == protoreflect.FloatKind {
			bit = 32
		} else {
			bit = 64
		}
	} else if field.Desc.Kind() == protoreflect.FloatKind {
		bit = 32
	} else {
		bit = 64
	}
	var in, notin []float64
	var gt, gte, lt, lte *float64
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueFloatIn) {
			in = proto.GetExtension(fop, pbex.E_MapValueFloatIn).([]float64)
		}
	} else if proto.HasExtension(fop, pbex.E_FloatIn) {
		in = proto.GetExtension(fop, pbex.E_FloatIn).([]float64)
	}
	if len(in) > 0 && bit == 32 {
		for _, v := range in {
			if v >= (-math.MaxFloat32) && v <= math.MaxFloat32 {
				continue
			}
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueFloatNotIn) {
			notin = proto.GetExtension(fop, pbex.E_MapValueFloatNotIn).([]float64)
		}
	} else if proto.HasExtension(fop, pbex.E_FloatNotIn) {
		notin = proto.GetExtension(fop, pbex.E_FloatNotIn).([]float64)
	}
	if len(notin) > 1 {
		dup := make(map[float64]*struct{})
		for _, v := range notin {
			dup[v] = nil
		}
		notin = notin[:0]
		for k := range dup {
			notin = append(notin, k)
		}
	}
	if len(notin) > 0 && bit == 32 {
		for _, v := range notin {
			if v >= (-math.MaxFloat32) && v <= math.MaxFloat32 {
				continue
			}
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	if len(notin) > 0 && len(in) > 0 {
		in = slices.DeleteFunc(in, func(e float64) bool {
			return slices.Contains(notin, e)
		})
		if len(in) == 0 {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		notin = nil
	}
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueFloatGt) {
			tmpgt := proto.GetExtension(fop, pbex.E_MapValueFloatGt).(float64)
			gt = &tmpgt
		}
	} else if proto.HasExtension(fop, pbex.E_FloatGt) {
		tmpgt := proto.GetExtension(fop, pbex.E_FloatGt).(float64)
		gt = &tmpgt
	}
	if gt != nil {
		if bit == 32 {
			if *gt < (-math.MaxFloat32) {
				panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if *gt >= math.MaxFloat32 {
				panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else if *gt == math.MaxFloat64 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e float64) bool {
				return e <= *gt
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		notin = slices.DeleteFunc(notin, func(e float64) bool {
			return e <= *gt
		})
		if len(in) > 0 {
			gt = nil
		}

	}
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueFloatGte) {
			tmpgte := proto.GetExtension(fop, pbex.E_MapValueFloatGte).(float64)
			gte = &tmpgte
		}
	} else if proto.HasExtension(fop, pbex.E_FloatGte) {
		tmpgte := proto.GetExtension(fop, pbex.E_FloatGte).(float64)
		gte = &tmpgte
	}
	if gte != nil {
		if bit == 32 {
			if *gte <= -math.MaxFloat32 {
				panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if *gte > math.MaxFloat32 {
				panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else if *gte == -math.MaxFloat64 {
			panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e float64) bool {
				return e < *gte
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		notin = slices.DeleteFunc(notin, func(e float64) bool {
			return e < *gte
		})
		if len(in) > 0 {
			gte = nil
		}
	}
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueFloatLt) {
			tmplt := proto.GetExtension(fop, pbex.E_MapValueFloatLt).(float64)
			lt = &tmplt
		}
	} else if proto.HasExtension(fop, pbex.E_FloatLt) {
		tmplt := proto.GetExtension(fop, pbex.E_FloatLt).(float64)
		lt = &tmplt
	}
	if lt != nil {
		if bit == 32 {
			if *lt > math.MaxFloat32 {
				panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if *lt <= (-math.MaxFloat32) {
				panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else if *lt == (-math.MaxFloat64) {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e float64) bool {
				return e >= *lt
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		notin = slices.DeleteFunc(notin, func(e float64) bool {
			return e >= *lt
		})
		if len(in) > 0 {
			lt = nil
		}
	}
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueFloatLte) {
			tmplte := proto.GetExtension(fop, pbex.E_MapValueFloatLte).(float64)
			lte = &tmplte
		}
	} else if proto.HasExtension(fop, pbex.E_FloatLte) {
		tmplte := proto.GetExtension(fop, pbex.E_FloatLte).(float64)
		lte = &tmplte
	}
	if lte != nil {
		if bit == 32 {
			if *lte >= math.MaxFloat32 {
				panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if *lte < (-math.MaxFloat32) {
				panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else if *lte == math.MaxFloat64 {
			panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e float64) bool {
				return e > *lte
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		}
		notin = slices.DeleteFunc(notin, func(e float64) bool {
			return e > *lte
		})
		if len(in) > 0 {
			lte = nil
		}
	}
	if gt != nil {
		if lt != nil && *gt >= *lt {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if lte != nil && *gt >= *lte {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	if gte != nil {
		if lt != nil && *gte >= *lt {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if lte != nil && *gte > *lte {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
	}
	if len(in) == 0 && gte != nil && lte != nil && *gte == *lte {
		if slices.Contains(notin, *gte) {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		in = append(in, *gte)
		gt = nil
		gte = nil
		lte = nil
		lt = nil
		notin = nil
	}
	indent := "\t\t"
	if oneof {
		indent = "\t\t\t"
	}
	target := ""
	if field.Desc.IsMap() {
		target = "map's value"
	} else if field.Desc.IsList() {
		target = "element value"
	} else {
		target = "value"
	}
	if len(in) > 0 {
		dup := make(map[float64]*struct{})
		all := make([]string, 0, 10)
		for _, v := range in {
			if _, ok := dup[v]; ok {
				continue
			}
			dup[v] = nil
			all = append(all, strconv.FormatFloat(v, 'f', -1, 64))
		}
		g.P(indent, target, " must in [", strings.Join(all, ","), "]")
	} else if notin != nil || gt != nil || gte != nil || lt != nil || lte != nil {
		all := make([]string, 0, 10)
		if gt != nil && gte != nil {
			if *gte <= *gt {
				//use gt
				gte = nil
			} else {
				//use gte
				gt = nil
			}
		}
		if gt != nil {
			all = append(all, ">"+strconv.FormatFloat(*gt, 'f', -1, 64))
		}
		if gte != nil {
			oldlen := len(notin)
			notin = slices.DeleteFunc(notin, func(e float64) bool {
				return e == *gte
			})
			if len(notin) == oldlen {
				all = append(all, ">="+strconv.FormatFloat(*gte, 'f', -1, 64))
			} else {
				all = append(all, ">"+strconv.FormatFloat(*gte, 'f', -1, 64))
			}
		}
		if lt != nil && lte != nil {
			if *lte >= *lt {
				//use lt
				lte = nil
			} else {
				//use lte
				lt = nil
			}
		}
		if lt != nil {
			all = append(all, "<"+strconv.FormatFloat(*lt, 'f', -1, 64))
		}
		if lte != nil {
			oldlen := len(notin)
			notin = slices.DeleteFunc(notin, func(e float64) bool {
				return e == *lte
			})
			if len(notin) == oldlen {
				all = append(all, "<="+strconv.FormatFloat(*lte, 'f', -1, 64))
			} else {
				all = append(all, "<"+strconv.FormatFloat(*lte, 'f', -1, 64))
			}
		}
		for _, v := range notin {
			all = append(all, "!=="+strconv.FormatFloat(v, 'f', -1, 64))
		}
		g.P(indent, target, " must ", strings.Join(all, " && "))
	}
}

// mapkv: only useful when the filed's type is map
// true-map key,false-map value
func strpbex(field *protogen.Field, fop *descriptorpb.FieldOptions, g *protogen.GeneratedFile, mapkv, oneof bool) {
	var in, notin []string
	var eq, noteq, gt, gte, lt, lte *uint64
	var match, notmatch []string
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyStringLenEq) {
				leneq := proto.GetExtension(fop, pbex.E_MapKeyStringLenEq).(uint64)
				eq = &leneq
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueStringBytesLenEq) {
				leneq := proto.GetExtension(fop, pbex.E_MapValueStringBytesLenEq).(uint64)
				eq = &leneq
			}
		}
	} else if proto.HasExtension(fop, pbex.E_StringBytesLenEq) {
		leneq := proto.GetExtension(fop, pbex.E_StringBytesLenEq).(uint64)
		eq = &leneq
	}
	if eq != nil && *eq > math.MaxInt64 {
		panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyStringLenNotEq) {
				lennoteq := proto.GetExtension(fop, pbex.E_MapKeyStringLenNotEq).(uint64)
				noteq = &lennoteq
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueStringBytesLenNotEq) {
				lennoteq := proto.GetExtension(fop, pbex.E_MapValueStringBytesLenNotEq).(uint64)
				noteq = &lennoteq
			}
		}
	} else if proto.HasExtension(fop, pbex.E_StringBytesLenNotEq) {
		lennoteq := proto.GetExtension(fop, pbex.E_StringBytesLenNotEq).(uint64)
		noteq = &lennoteq
	}
	if noteq != nil {
		if *noteq > math.MaxInt64 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if eq != nil && *eq == *noteq {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if eq != nil {
			noteq = nil
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyStringLenGt) {
				lengt := proto.GetExtension(fop, pbex.E_MapKeyStringLenGt).(uint64)
				gt = &lengt
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueStringBytesLenGt) {
				lengt := proto.GetExtension(fop, pbex.E_MapValueStringBytesLenGt).(uint64)
				gt = &lengt
			}
		}
	} else if proto.HasExtension(fop, pbex.E_StringBytesLenGt) {
		lengt := proto.GetExtension(fop, pbex.E_StringBytesLenGt).(uint64)
		gt = &lengt
	}
	if gt != nil {
		if *gt >= math.MaxInt64 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if eq != nil && *eq <= *gt {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if noteq != nil && *noteq <= *gt {
			noteq = nil
		}
		if *gt+1 == math.MaxInt64 {
			if noteq != nil && *noteq == math.MaxInt64 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if eq == nil {
				tmpeq := uint64(math.MaxInt64)
				eq = &tmpeq
				noteq = nil
			}
		}
		if eq == nil && *gt+2 == math.MaxInt64 {
			if noteq != nil {
				switch *noteq {
				case math.MaxInt64:
					tmpeq := uint64(math.MaxInt64 - 1)
					eq = &tmpeq
					noteq = nil
				case math.MaxInt64 - 1:
					tmpeq := uint64(math.MaxInt64)
					eq = &tmpeq
					noteq = nil
				}
			}
		}
		if eq != nil {
			gt = nil
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyStringLenGte) {
				lengte := proto.GetExtension(fop, pbex.E_MapKeyStringLenGte).(uint64)
				gte = &lengte
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueStringBytesLenGte) {
				lengte := proto.GetExtension(fop, pbex.E_MapValueStringBytesLenGte).(uint64)
				gte = &lengte
			}
		}
	} else if proto.HasExtension(fop, pbex.E_StringBytesLenGte) {
		lengte := proto.GetExtension(fop, pbex.E_StringBytesLenGte).(uint64)
		gte = &lengte
	}
	if gte != nil {
		if *gte == 0 {
			panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if *gte > math.MaxInt64 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if eq != nil && *eq < *gte {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if noteq != nil && *noteq < *gte {
			noteq = nil
		}
		if *gte == math.MaxInt64 {
			if noteq != nil && *noteq == math.MaxInt64 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if eq == nil {
				eq = gte
				noteq = nil
			}
		}
		if eq == nil && *gte+1 == math.MaxInt64 {
			if noteq != nil {
				switch *noteq {
				case math.MaxInt64:
					tmpeq := uint64(math.MaxInt64 - 1)
					eq = &tmpeq
					noteq = nil
				case math.MaxInt64 - 1:
					tmpeq := uint64(math.MaxInt64)
					eq = &tmpeq
					noteq = nil
				}
			}
		}
		if eq != nil {
			gte = nil
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyStringLenLt) {
				lenlt := proto.GetExtension(fop, pbex.E_MapKeyStringLenLt).(uint64)
				lt = &lenlt
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueStringBytesLenLt) {
				lenlt := proto.GetExtension(fop, pbex.E_MapValueStringBytesLenLt).(uint64)
				lt = &lenlt
			}
		}
	} else if proto.HasExtension(fop, pbex.E_StringBytesLenLt) {
		lenlt := proto.GetExtension(fop, pbex.E_StringBytesLenLt).(uint64)
		lt = &lenlt
	}
	if lt != nil {
		if *lt == 0 {
			panic("pbex options value overflow in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if *lt > math.MaxInt64 {
			panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if eq != nil && *eq >= *lt {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if noteq != nil && *noteq >= *lt {
			noteq = nil
		}
		if *lt == 1 {
			if noteq != nil && *noteq == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if eq == nil {
				tmpeq := uint64(0)
				eq = &tmpeq
				noteq = nil
			}
		}
		if eq == nil && *lt == 2 {
			if noteq != nil {
				switch *noteq {
				case 0:
					tmpeq := uint64(1)
					eq = &tmpeq
					noteq = nil
				case 1:
					tmpeq := uint64(0)
					eq = &tmpeq
					noteq = nil
				}
			}
		}
		if eq != nil {
			lt = nil
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyStringLenLte) {
				lenlte := proto.GetExtension(fop, pbex.E_MapKeyStringLenLte).(uint64)
				lte = &lenlte
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueStringBytesLenLte) {
				lenlte := proto.GetExtension(fop, pbex.E_MapValueStringBytesLenLte).(uint64)
				lte = &lenlte
			}
		}
	} else if proto.HasExtension(fop, pbex.E_StringBytesLenLte) {
		lenlte := proto.GetExtension(fop, pbex.E_StringBytesLenLte).(uint64)
		lte = &lenlte
	}
	if lte != nil {
		if *lte >= math.MaxInt64 {
			panic("pbex options value useless in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if eq != nil && *eq > *lte {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		if noteq != nil && *noteq > *lte {
			noteq = nil
		}
		if *lte == 0 {
			if noteq != nil && *noteq == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
			if eq == nil {
				eq = lte
				noteq = nil
			}
		}
		if eq == nil && *lte == 1 {
			if noteq != nil {
				switch *noteq {
				case 0:
					tmpeq := uint64(1)
					eq = &tmpeq
					noteq = nil
				case 1:
					tmpeq := uint64(0)
					eq = &tmpeq
					noteq = nil
				}
			}
		}
		if eq != nil {
			lte = nil
		}
	}
	if gte != nil && gt != nil {
		if *gte > *gt {
			*gt = *gte - 1
		}
		gte = nil
	} else if gte != nil {
		gt = gte
		(*gt)--
		gte = nil
	}
	if lte != nil && lt != nil {
		if *lte < *lt {
			*lt = *lte + 1
		}
		lte = nil
	} else if lte != nil {
		lt = lte
		(*lt)++
		lte = nil
	}
	if lt != nil && gt != nil && ((*gt) >= (*lt) || (*gt) >= (*lt)-1) {
		panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
	}
	if eq == nil && gt != nil && lt != nil && (*gt) == (*lt)-2 {
		if noteq != nil && *noteq == (*gt)+1 {
			panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
		}
		eq = gt
		(*eq)++
		gt = nil
		lt = nil
		noteq = nil
	}
	if eq == nil && gt != nil && lt != nil && (*gt) == (*lt)-3 && noteq != nil {
		switch *noteq {
		case (*gt) + 1:
			eq = gt
			(*eq) += 2
			gt = nil
			lt = nil
			noteq = nil
		case (*gt) + 2:
			eq = gt
			(*eq)++
			gt = nil
			lt = nil
			noteq = nil
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyStringIn) {
				in = proto.GetExtension(fop, pbex.E_MapKeyStringIn).([]string)
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueStringBytesIn) {
				in = proto.GetExtension(fop, pbex.E_MapValueStringBytesIn).([]string)
			}
		}
	} else if proto.HasExtension(fop, pbex.E_StringBytesIn) {
		in = proto.GetExtension(fop, pbex.E_StringBytesIn).([]string)
	}
	if len(in) > 0 {
		if eq != nil {
			in = slices.DeleteFunc(in, func(e string) bool {
				return uint64(len(e)) != *eq
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else {
			if noteq != nil {
				in = slices.DeleteFunc(in, func(e string) bool {
					return uint64(len(e)) == *noteq
				})
				if len(in) == 0 {
					panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
				}
			}
			if gt != nil {
				in = slices.DeleteFunc(in, func(e string) bool {
					return uint64(len(e)) <= *gt
				})
				if len(in) == 0 {
					panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
				}
			}
			if lt != nil {
				in = slices.DeleteFunc(in, func(e string) bool {
					return uint64(len(e)) >= *lt
				})
				if len(in) == 0 {
					panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
				}
			}
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			//key
			if proto.HasExtension(fop, pbex.E_MapKeyStringNotIn) {
				notin = proto.GetExtension(fop, pbex.E_MapKeyStringNotIn).([]string)
			}
		} else {
			//value
			if proto.HasExtension(fop, pbex.E_MapValueStringBytesNotIn) {
				notin = proto.GetExtension(fop, pbex.E_MapValueStringBytesNotIn).([]string)
			}
		}
	} else if proto.HasExtension(fop, pbex.E_StringBytesNotIn) {
		notin = proto.GetExtension(fop, pbex.E_StringBytesNotIn).([]string)
	}
	if len(notin) > 0 {
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e string) bool {
				return slices.Contains(notin, e)
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else if eq != nil {
			notin = slices.DeleteFunc(notin, func(e string) bool {
				return uint64(len(e)) != *eq
			})
		} else {
			if noteq != nil {
				notin = slices.DeleteFunc(notin, func(e string) bool {
					return uint64(len(e)) == *noteq
				})
			}
			if gt != nil {
				notin = slices.DeleteFunc(notin, func(e string) bool {
					return uint64(len(e)) <= *gt
				})
			}
			if lt != nil {
				notin = slices.DeleteFunc(notin, func(e string) bool {
					return uint64(len(e)) >= *lt
				})
			}
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			// key
			if proto.HasExtension(fop, pbex.E_MapKeyStringRegMatch) {
				match = proto.GetExtension(fop, pbex.E_MapKeyStringRegMatch).([]string)
			}
		} else {
			// value
			if proto.HasExtension(fop, pbex.E_MapValueStringBytesRegMatch) {
				match = proto.GetExtension(fop, pbex.E_MapValueStringBytesRegMatch).([]string)
			}
		}
	} else if proto.HasExtension(fop, pbex.E_StringBytesRegMatch) {
		match = proto.GetExtension(fop, pbex.E_StringBytesRegMatch).([]string)
	}
	if len(match) > 0 {
		dup := make(map[string]*struct{})
		match = slices.DeleteFunc(match, func(e string) bool {
			if _, ok := dup[e]; ok {
				return true
			}
			dup[e] = nil
			return false
		})
	}
	if len(match) > 0 {
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e string) bool {
				for _, reg := range match {
					//the regexp content already compiled in the geninit function,so the error will never exist
					matched, _ := regexp.MatchString(reg, e)
					if !matched {
						return true
					}
				}
				return false
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else if len(notin) > 0 {
			notin = slices.DeleteFunc(notin, func(e string) bool {
				for _, reg := range match {
					//the regexp content already compiled in the geninit function,so the error will never exist
					matched, _ := regexp.MatchString(reg, e)
					if !matched {
						return true
					}
				}
				return false
			})
		}
	}
	if field.Desc.IsMap() {
		if mapkv {
			// key
			if proto.HasExtension(fop, pbex.E_MapKeyStringRegNotMatch) {
				notmatch = proto.GetExtension(fop, pbex.E_MapKeyStringRegNotMatch).([]string)
			}
		} else {
			// value
			if proto.HasExtension(fop, pbex.E_MapValueStringBytesRegNotMatch) {
				notmatch = proto.GetExtension(fop, pbex.E_MapValueStringBytesRegNotMatch).([]string)
			}
		}
	} else if proto.HasExtension(fop, pbex.E_StringBytesRegNotMatch) {
		notmatch = proto.GetExtension(fop, pbex.E_StringBytesRegNotMatch).([]string)
	}
	if len(notmatch) > 0 {
		dup := make(map[string]*struct{})
		notmatch = slices.DeleteFunc(notmatch, func(e string) bool {
			if _, ok := dup[e]; ok {
				return true
			}
			dup[e] = nil
			return false
		})
	}
	if len(notmatch) > 0 {
		if len(in) > 0 {
			in = slices.DeleteFunc(in, func(e string) bool {
				for _, reg := range notmatch {
					//the regexp content already compiled in the geninit function,so the error will never exist
					matched, _ := regexp.MatchString(reg, e)
					if matched {
						return true
					}
				}
				return false
			})
			if len(in) == 0 {
				panic("pbex options conflict in field:" + string(field.Desc.Name()) + " in message:" + string(field.Parent.Desc.Name()))
			}
		} else if len(notin) > 0 {
			notin = slices.DeleteFunc(notin, func(e string) bool {
				for _, reg := range notmatch {
					//the regexp content already compiled in the geninit function,so the error will never exist
					matched, _ := regexp.MatchString(reg, e)
					if matched {
						return true
					}
				}
				return false
			})
		}
	}
	isbytes := ""
	if field.Desc.IsMap() && !mapkv && field.Message.Fields[1].Desc.Kind() == protoreflect.BytesKind {
		isbytes = "'s utf8 encode value"
	}
	if !field.Desc.IsMap() && field.Desc.Kind() == protoreflect.BytesKind {
		isbytes = "'s utf8 encode value"
	}
	indent := "\t\t"
	if oneof {
		indent = "\t\t\t"
	}
	target := ""
	if field.Desc.IsMap() {
		if mapkv {
			target = "map's key"
		} else {
			target = "map's value"
		}
	} else if field.Desc.IsList() {
		target = "element value"
	} else {
		target = "value"
	}
	if len(in) > 0 {
		d, _ := json.Marshal(in)
		g.P(indent, target, isbytes, " must in ", string(d))
	} else if eq != nil || noteq != nil || gt != nil || lt != nil || len(notin) > 0 || len(match) > 0 || len(notmatch) > 0 {
		all := make([]string, 0, 10)
		if eq != nil {
			all = append(all, "==="+strconv.FormatUint(*eq, 10))
		} else {
			if gt != nil {
				if noteq != nil && *noteq == (*gt)+1 {
					all = append(all, ">"+strconv.FormatUint(*noteq, 10))
					noteq = nil
				} else if *gt == math.MaxInt64-1 {
					all = append(all, "==="+strconv.FormatUint(math.MaxInt64, 10))
				} else {
					all = append(all, ">"+strconv.FormatUint(*gt, 10))
				}
			}
			if lt != nil {
				if noteq != nil && *noteq == (*lt)-1 {
					all = append(all, "<"+strconv.FormatUint(*noteq, 10))
					noteq = nil
				} else if *lt == 1 {
					all = append(all, "===0")
				} else {
					all = append(all, "<"+strconv.FormatUint(*lt, 10))
				}
			}
			if noteq != nil {
				all = append(all, "!=="+strconv.FormatUint(*noteq, 10))
			}
		}
		if len(all) > 0 {
			g.P(indent, target, isbytes, "'s length must ", strings.Join(all, " && "))
		}
		if len(notin) > 0 {
			d, _ := json.Marshal(notin)
			g.P(indent, target, isbytes, " must not in ", string(d))
		}
		if len(match) > 0 {
			d, _ := json.Marshal(match)
			g.P(indent, target, isbytes, " must match all regexp in ", string(d))
		}
		if len(notmatch) > 0 {
			d, _ := json.Marshal(notmatch)
			g.P(indent, target, isbytes, " must not match all regexp in ", string(d))
		}
	}
}

func msgpbex(field *protogen.Field, fop *descriptorpb.FieldOptions, g *protogen.GeneratedFile, oneof bool) {
	indent := "\t\t"
	if oneof {
		indent = "\t\t\t"
	}
	target := ""
	if field.Desc.IsMap() {
		target = "map's value"
	} else if field.Desc.IsList() {
		target = "element value"
	} else {
		target = "value"
	}
	var notnil bool
	if field.Desc.IsMap() {
		if proto.HasExtension(fop, pbex.E_MapValueMessageNotNil) {
			notnil = proto.GetExtension(fop, pbex.E_MapValueMessageNotNil).(bool)
		}
	} else if proto.HasExtension(fop, pbex.E_MessageNotNil) {
		notnil = proto.GetExtension(fop, pbex.E_MessageNotNil).(bool)
	}
	if notnil {
		g.P(indent, target, " must not be undefined/null")
	}
}
