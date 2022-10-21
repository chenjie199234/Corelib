#!/bin/bash
cd $(dirname $0)
protoc --go_out=paths=source_relative:. cerror/cerror.proto
protoc --go_out=paths=source_relative:. crpc/msg.proto
protoc --go_out=paths=source_relative:. pbex/pbex.proto
