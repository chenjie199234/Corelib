package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/chenjie199234/Corelib/internal/version"
	"github.com/chenjie199234/Corelib/pbex"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	showversion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showversion {
		fmt.Printf("protoc-gen-browser %s\n", version.String())
		return
	}
	var flags flag.FlagSet
	gentob := flags.Bool("gen_tob", false, "")
	protogen.Options{
		ParamFunc: flags.Set,
	}.Run(func(gen *protogen.Plugin) error {
		//pre check
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
					if mop.GetDeprecated() {
						continue
					}
					if m.Desc.IsStreamingClient() || m.Desc.IsStreamingServer() {
						panic("stream is not supported")
					}
					if pbex.OneOfHasPBEX(m.Input) {
						panic("oneof fields should not contain pbex")
					}
					if pbex.OneOfHasPBEX(m.Output) {
						panic("oneof fields should not contain pbex")
					}
					if !proto.HasExtension(mop, pbex.E_Method) {
						continue
					}
					httpmetohd := strings.ToUpper(proto.GetExtension(mop, pbex.E_Method).(string))
					if httpmetohd != http.MethodGet && httpmetohd != http.MethodPost && httpmetohd != http.MethodPut && httpmetohd != http.MethodDelete && httpmetohd != http.MethodPatch {
						panic(fmt.Sprintf("method: %s in service: %s with not supported httpmetohd: %s", m.Desc.Name(), s.Desc.Name(), httpmetohd))
					}
				}
			}
			//delete old file
			oldtocfile := f.GeneratedFilenamePrefix + "_browser_toc.ts"
			if e := os.RemoveAll(oldtocfile); e != nil {
				panic("remove old file " + oldtocfile + " error:" + e.Error())
			}
			oldtobfile := f.GeneratedFilenamePrefix + "_browser_tob.ts"
			if e := os.RemoveAll(oldtobfile); e != nil {
				panic("remove old file " + oldtobfile + " error:" + e.Error())
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
			generateFile(gen, f, true)
			if *gentob {
				generateFile(gen, f, false)
			}
		}
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		return nil
	})
}
