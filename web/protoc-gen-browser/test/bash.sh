#!/bin/bash
 protoc -I ../../../pbex -I . --browser_out=paths=source_relative:. *.proto
 protoc -I ../../../pbex -I . --ts_proto_out=. --ts_proto_opt=outputServices=false,useNumericEnumForJson=true,env=browser,oneof=unions,unrecognizedEnum=false,unknownFields=false,useMapType=true,forceLong=long,snakeToCamel=false *.proto
