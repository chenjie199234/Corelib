package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/chenjie199234/Corelib/internal/version"
	"github.com/chenjie199234/Corelib/pbex"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	ver := flag.Bool("v", false, "version info")
	flag.Parse()
	if *ver {
		fmt.Println(version.String())
		return
	}
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS)
		gen.SupportedEditionsMinimum = descriptorpb.Edition_EDITION_2024
		gen.SupportedEditionsMaximum = descriptorpb.Edition_EDITION_2024
		//pre check
		needfile := make(map[string]bool)
		for _, f := range gen.Files {
			needfile[f.Desc.Path()] = false
			if !f.Generate {
				continue
			}
			if f.Desc.Options().(*descriptorpb.FileOptions).GetDeprecated() {
				continue
			}
			if f.Proto.Edition == nil || *f.Proto.Edition < descriptorpb.Edition_EDITION_2024 {
				panic("plugin only support proto file's edition >= 2024")
			}
			for _, m := range f.Messages {
				if pbex.NeedValidate(m) {
					needfile[f.Desc.Path()] = true
				}
			}
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
		return nil
	})
}
