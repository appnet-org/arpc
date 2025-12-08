#!/bin/bash

# compile the protoc-gen-symphony and protoc-gen-argc plugin
cd ..
go install ./protoc-gen-symphony
go install ./protoc-gen-arpc
cd test

# generate code from the test.proto file
protoc  --symphony_out=paths=source_relative:. \
        --go_out=paths=source_relative:. \
        test.proto

# Run go test
go test -v .

