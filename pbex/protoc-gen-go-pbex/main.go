package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chenjie199234/Corelib/internal/version"
	"github.com/chenjie199234/Corelib/pbex"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Fprintf(os.Stderr, "%v %v\n", filepath.Base(os.Args[0]), version.String())
		os.Exit(0)
	}
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		//pre check
		needfile := make(map[string]bool)
		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}
			if f.Desc.Options().(*descriptorpb.FileOptions).GetDeprecated() {
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
			needfile[f.Desc.Path()] = len(f.Messages) > 0
			//delete old file
			oldfile := f.GeneratedFilenamePrefix + "_pbex.pb.go"
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
