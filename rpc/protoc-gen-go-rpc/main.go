package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chenjie199234/Corelib/pbex"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

var version = "v0.0.1"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Fprintf(os.Stderr, "%v %v\n", filepath.Base(os.Args[0]), version)
		os.Exit(0)
	}
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		//pre check
		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}
			if *f.Proto.Syntax != "proto3" {
				panic("don't use proto2 syntax in proto,this plugin can't support!")
			}
			for _, s := range f.Services {
				if s.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
					continue
				}
				for _, m := range s.Methods {
					if m.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
						continue
					}
					if hasoneof(m.Input) || hasoneof(m.Output) {
						panic("can't support oneof in proto!")
					}
					if !notnilcheck(m.Input) {
						panic("message_not_nil and map_value_message_not_nil can only be set to true")
					}
				}
			}
			//delete old file
			oldfile := f.GeneratedFilenamePrefix + "_rpc.pb.go"
			if e := os.RemoveAll(oldfile); e != nil {
				panic("remove old file " + oldfile + " error:" + e.Error())
			}
		}
		//gen file
		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}
			if f.Desc.Options().(*descriptorpb.FileOptions).GetDeprecated() {
				continue
			}
			need := false
			for _, s := range f.Services {
				if s.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
					continue
				}
				for _, m := range s.Methods {
					if m.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
						continue
					}
					need = true
					break
				}
				if need {
					break
				}
			}
			if need {
				generateFile(gen, f)
			}
		}
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		return nil
	})
}
func hasoneof(message *protogen.Message) bool {
	if len(message.Oneofs) > 0 {
		return true
	}
	for _, field := range message.Fields {
		if field.Desc.Kind() == protoreflect.MessageKind {
			if field.Desc.IsMap() {
				//map
				if field.Message.Fields[1].Desc.Kind() == protoreflect.MessageKind {
					//map's value is message
					if hasoneof(field.Message.Fields[1].Message) {
						return true
					}
				}
			} else {
				//[]message or message
				if hasoneof(field.Message) {
					return true
				}
			}
		}
	}
	return false
}

//message_not_nil and map_value_message_not_nil extension can only be set to true
func notnilcheck(message *protogen.Message) bool {
	for _, field := range message.Fields {
		fop := field.Desc.Options().(*descriptorpb.FieldOptions)
		if field.Desc.Kind() == protoreflect.MessageKind {
			if field.Desc.IsMap() {
				if field.Message.Fields[1].Desc.Kind() == protoreflect.MessageKind {
					if proto.HasExtension(fop, pbex.E_MapValueMessageNotNil) {
						if !proto.GetExtension(fop, pbex.E_MapValueMessageNotNil).(bool) {
							return false
						}
					}
					if !notnilcheck(field.Message.Fields[1].Message) {
						return false
					}
				}
			} else {
				//[]message or message
				if proto.HasExtension(fop, pbex.E_MessageNotNil) {
					if !proto.GetExtension(fop, pbex.E_MessageNotNil).(bool) {
						return false
					}
				}
				if !notnilcheck(field.Message) {
					return false
				}
			}
		}
	}
	return true
}
