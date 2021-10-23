package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chenjie199234/Corelib/pbex"
	"google.golang.org/protobuf/compiler/protogen"
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
					if pbex.HasOneOf(m.Input) || pbex.HasOneOf(m.Output) {
						panic("can't support oneof in proto!")
					}
				}
			}
			//delete old file
			oldfile := f.GeneratedFilenamePrefix + "_crpc.pb.go"
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
