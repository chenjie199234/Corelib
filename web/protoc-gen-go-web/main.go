package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/chenjie199234/Corelib/internal/version"
	"github.com/chenjie199234/Corelib/pbex"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	showversion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showversion {
		fmt.Printf("protoc-gen-go-web %s\n", version.String())
		return
	}
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		//pre check
		needfile := make(map[string]bool)
		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}
			if *f.Proto.Syntax != "proto3" {
				panic("plugin only support proto3 syntax!")
			}
			for _, m := range f.Messages {
				if pbex.OneOfHasPBEX(m) {
					panic("oneof fields should not contain pbex")
				}
			}
			for _, s := range f.Services {
				if s.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
					continue
				}
				for _, m := range s.Methods {
					mop := m.Desc.Options().(*descriptorpb.MethodOptions)
					if mop.GetDeprecated() || !proto.HasExtension(mop, pbex.E_Method) {
						continue
					}
					if m.Desc.IsStreamingClient() || m.Desc.IsStreamingServer() {
						continue
					}
					emethod := proto.GetExtension(mop, pbex.E_Method).([]string)
					need := 0
					for _, em := range emethod {
						em = strings.ToUpper(em)
						if em == "GET" || em == "POST" || em == "PUT" || em == "PATCH" || em == "DELETE" {
							need++
						}
					}
					if need == 0 {
						continue
					}
					needfile[f.Desc.Path()] = true
					if need > 1 {
						panic(fmt.Sprintf("method: %s in service: %s,only one http method can be setted", m.Desc.Name(), s.Desc.Name()))
					}
					if pbex.OneOfHasPBEX(m.Input) {
						panic("oneof fields should not contain pbex")
					}
					if pbex.OneOfHasPBEX(m.Output) {
						panic("oneof fields should not contain pbex")
					}
					//Get and Delete method can only contain simple fields
					simple := false
					for _, em := range emethod {
						em = strings.ToUpper(em)
						if em == "GET" || em == "DELETE" {
							simple = true
							break
						}
					}
					if !simple {
						continue
					}
					for _, f := range m.Input.Fields {
						if f.Desc.Kind() != protoreflect.MessageKind {
							continue
						}
						panic(fmt.Sprintf("method: %s in service: %s with http method: get/delete,it's request message can't contain nested message and map", m.Desc.Name(), s.Desc.Name()))
					}
				}
			}
			//delete old file
			oldfile := f.GeneratedFilenamePrefix + "_web.pb.go"
			if e := os.RemoveAll(oldfile); e != nil {
				panic("remove old file " + oldfile + " error:" + e.Error())
			}
		}
		//gen file
		for _, f := range gen.Files {
			if status, ok := needfile[f.Desc.Path()]; !ok || !status {
				continue
			}
			generateFile(gen, f)
		}
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		return nil
	})
}
