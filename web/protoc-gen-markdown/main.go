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
	ver := flag.Bool("v", false, "version info")
	flag.Parse()
	if *ver {
		fmt.Println(version.String())
		return
	}
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		//pre check
		needfile := make(map[string]bool)
		for _, f := range gen.Files {
			gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS)
			gen.SupportedEditionsMinimum = descriptorpb.Edition_EDITION_2024
			gen.SupportedEditionsMaximum = descriptorpb.Edition_EDITION_2024
			if !f.Generate {
				continue
			}
			if f.Desc.Options().(*descriptorpb.FileOptions).GetDeprecated() {
				continue
			}
			if f.Proto.Edition == nil || *f.Proto.Edition < descriptorpb.Edition_EDITION_2024 {
				panic("plugin only support proto file's edition >= 2024")
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
					emethod := proto.GetExtension(mop, pbex.E_Method).([]string)
					need := 0
					for _, em := range emethod {
						em = strings.ToUpper(em)
						if em == "GET" || em == "POST" || em == "PUT" || em == "PATCH" || em == "DELETE" {
							need++
							emethod[0] = em
						}
					}
					if need == 0 {
						continue
					}
					if need > 1 {
						panic(fmt.Sprintf("method: %s in service: %s,only one http method can be setted", m.Desc.Name(), s.Desc.Name()))
					}
					if m.Desc.IsStreamingClient() {
						panic(fmt.Sprintf("method: %s in service: %s,only server stream can be setted on web", m.Desc.Name(), s.Desc.Name()))
					}
					simple := emethod[0] == "GET" || emethod[0] == "DELETE"
					if simple {
						for _, f := range m.Input.Fields {
							if f.Desc.Kind() != protoreflect.MessageKind {
								continue
							}
							panic(fmt.Sprintf("method: %s in service: %s with http method: %s,it's request message can't contain nested message and map", m.Desc.Name(), s.Desc.Name(), emethod[0]))
						}
					}
					needfile[f.Desc.Path()] = true
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
			if status, ok := needfile[f.Desc.Path()]; !ok || !status {
				continue
			}
			generateFile(gen, f)
		}
		return nil
	})
}
