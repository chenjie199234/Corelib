package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chenjie199234/Corelib/internal/version"
	"github.com/chenjie199234/Corelib/pbex"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Fprintf(os.Stderr, "%v %v\n", filepath.Base(os.Args[0]), version.String())
		os.Exit(0)
	}
	fmt.Println("protoc-gen-markdown run on version:", version.String())
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		//pre check
		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}
			if *f.Proto.Syntax != "proto3" {
				panic("this plugin only support proto3 syntax!")
			}
			for _, s := range f.Services {
				if s.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
					continue
				}
				for _, m := range s.Methods {
					mop := m.Desc.Options().(*descriptorpb.MethodOptions)
					if mop.GetDeprecated() {
						continue
					}
					if !proto.HasExtension(mop, pbex.E_Method) {
						continue
					}
					httpmetohd := strings.ToUpper(proto.GetExtension(mop, pbex.E_Method).(string))
					if httpmetohd != http.MethodGet && httpmetohd != http.MethodPost && httpmetohd != http.MethodPut && httpmetohd != http.MethodDelete && httpmetohd != http.MethodPatch {
						panic(fmt.Sprintf("method: %s in service: %s with not supported httpmetohd: %s", m.Desc.Name(), s.Desc.Name(), httpmetohd))
					}
					if pbex.HasOneOf(m.Input) || pbex.HasOneOf(m.Output) {
						panic("can't support oneof in proto!")
					}
				}
			}
			//delete old file
			oldfile := f.GeneratedFilenamePrefix + ".md"
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
			generateFile(gen, f)
		}
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		return nil
	})
}
