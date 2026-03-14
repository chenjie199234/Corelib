package main

import (
	"flag"
	"fmt"
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
	var flags flag.FlagSet
	var outdir = flags.String("outdir", "./", "")
	protogen.Options{ParamFunc: flags.Set}.Run(func(plugin *protogen.Plugin) error {
		//pre check
		services := make(map[*protogen.Service]map[*protogen.Method]*struct{})
		enums := make(map[*protogen.Enum]*struct{})
		msgs := make(map[*protogen.Message][]bool) //first element:toFORM,second element:toJSON,third element:fromJSON
		for _, f := range plugin.Files {
			plugin.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS)
			plugin.SupportedEditionsMinimum = descriptorpb.Edition_EDITION_2024
			plugin.SupportedEditionsMaximum = descriptorpb.Edition_EDITION_2024
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
						panic(fmt.Sprintf("method: %s in service: %s only server stream can be setted on web", m.Desc.Name(), s.Desc.Name()))
					}
					//Get and Delete method can only contain simple fields
					simple := emethod[0] == "GET" || emethod[0] == "DELETE"
					if simple {
						for _, f := range m.Input.Fields {
							if f.Desc.Kind() != protoreflect.MessageKind {
								continue
							}
							panic(fmt.Sprintf("method: %s in service: %s with http method: %s,it's request message can't contain nested message and map", m.Desc.Name(), s.Desc.Name(), emethod[0]))
						}
					}
					if _, ok := services[s]; !ok {
						services[s] = make(map[*protogen.Method]*struct{})
					}
					services[s][m] = nil
					prepare_input(simple, m.Input, msgs, enums)
					prepare_output(m.Output, msgs, enums)
				}
			}
		}
		for enum := range enums {
			genEnum(plugin, enum, *outdir)
		}
		for message, status := range msgs {
			genMessage(plugin, message, status, *outdir)
		}
		jsonreplacer := false
		normal := false
		sse := false
		for service, methods := range services {
			for method := range methods {
				if !sse {
					sse = method.Desc.IsStreamingServer()
				}
				if !normal {
					normal = !method.Desc.IsStreamingServer()
				}
				mop := method.Desc.Options().(*descriptorpb.MethodOptions)
				emethod := proto.GetExtension(mop, pbex.E_Method).([]string)
				for _, em := range emethod {
					em = strings.ToUpper(em)
					if em == "POST" || em == "PUT" || em == "PATCH" {
						jsonreplacer = true
						break
					}
				}
				genServiceMethod(plugin, service, method, *outdir)
			}
		}
		if len(services) > 0 {
			genUtil(plugin, *outdir, jsonreplacer, normal, sse)
		}
		return nil
	})
}
func prepare_input(simple bool, m *protogen.Message, msgs map[*protogen.Message][]bool, enums map[*protogen.Enum]*struct{}) {
	status, ok := msgs[m]
	if !ok {
		status = []bool{false, false, false}
		msgs[m] = status
	}
	if simple {
		status[0] = true //toFORM
	} else {
		status[1] = true //toJSON
	}
	if ok {
		return
	}
	for _, f := range m.Fields {
		switch f.Desc.Kind() {
		case protoreflect.EnumKind:
			enums[f.Enum] = nil
		case protoreflect.MessageKind:
			if f.Desc.IsMap() {
				switch f.Message.Fields[1].Desc.Kind() {
				case protoreflect.MessageKind:
					prepare_input(simple, f.Message.Fields[1].Message, msgs, enums)
				case protoreflect.EnumKind:
					enums[f.Message.Fields[1].Enum] = nil
				}
			} else {
				prepare_input(simple, f.Message, msgs, enums)
			}
		}
	}
}
func prepare_output(m *protogen.Message, msgs map[*protogen.Message][]bool, enums map[*protogen.Enum]*struct{}) {
	status, ok := msgs[m]
	if !ok {
		status = []bool{false, false, true}
		msgs[m] = status
	} else {
		status[2] = true
	}
	if ok {
		return
	}
	for _, f := range m.Fields {
		switch f.Desc.Kind() {
		case protoreflect.EnumKind:
			enums[f.Enum] = nil
		case protoreflect.MessageKind:
			if f.Desc.IsMap() {
				switch f.Message.Fields[1].Desc.Kind() {
				case protoreflect.MessageKind:
					prepare_output(f.Message.Fields[1].Message, msgs, enums)
				case protoreflect.EnumKind:
					enums[f.Message.Fields[1].Enum] = nil
				}
			} else {
				prepare_output(f.Message, msgs, enums)
			}
		}
	}
}
