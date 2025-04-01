## Install the official Protobuf Go plugin:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

Make sure `$GOPATH/bin` is in your shell `PATH`.

## Install the aRPC codegen plugin

```bash
go install ./cmd/protoc-gen-aprc
```


## Generate aRPC stubs

```bash
protoc \
  --go_out=paths=source_relative:. \
  --aprc_out=paths=source_relative:. \
  <your-proto-file>
```
