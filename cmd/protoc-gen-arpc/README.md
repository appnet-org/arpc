## Prerequisites

Install the official Protobuf Go plugin:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

Make sure `$GOPATH/bin` is in your shell `PATH`.

## Install the aRPC Codegen Plugin

From the root of your repository, run:

```bash
go install ./cmd/protoc-gen-arpc
```

## Generate aRPC Stubs

Use the following command to generate Go and aRPC stubs from your `.proto` files:

```bash
protoc \
  --proto_path=. \
  --go_out=paths=source_relative:. \
  --arpc_out=paths=source_relative:. \
  <your-proto-file>
```

Replace `<your-proto-file>` with the path to your `.proto` file.
