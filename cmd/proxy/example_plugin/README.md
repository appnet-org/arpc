# Example Element Plugin

This directory contains an example element plugin that demonstrates how to create a dynamically loadable element for the proxy.

## Building the Plugin

### Quick Build (Recommended)

Use the provided build script, which automatically generates a timestamped filename:

```bash
cd /users/xzhu/arpc/cmd/proxy/example_plugin
chmod +x build.sh
./build.sh
```

This will create a file like `element-example-20240115-143022.so` (timestamp format: YYYYMMDD-HHMMSS).

The timestamp ensures that newer builds are automatically selected by the elementloader, which picks the **highest alphabetically sorted** file matching the `element-` prefix.

### Custom Build Name

You can also specify a custom name:

```bash
./build.sh element-example-v2.so
```

### Manual Build

To build this plugin as a `.so` file manually:

```bash
cd /users/xzhu/arpc/cmd/proxy/example_plugin
go build -buildmode=plugin -o element-example.so example_element.go
```

**Important Notes:**
1. The plugin must be built with the same Go version as the main proxy binary
2. The plugin must use the same module dependencies (same `go.mod`)
3. The compiled `.so` file must be placed in `/tmp/arpc/elements/` directory
4. The filename must start with `element-` prefix (e.g., `element-example.so`, `element-example-v2.so`)
5. **File Selection**: The elementloader selects the **highest alphabetically sorted** file. Using timestamps (like `element-example-20240115-143022.so`) ensures newer builds are automatically selected.
6. **Type Sharing**: The plugin defines `RPCElement` interface locally. For production, consider moving the interface to a shared package (e.g., `github.com/appnet-org/proxy/element`) so both the proxy and plugins can import it directly.

## Plugin Requirements

1. **Package**: Must be `package main`
2. **Exported Symbol**: Must export a variable named `ElementInit` that implements the `elementInit` interface:
   ```go
   type elementInit interface {
       Element() RPCElement
       Kill() // Optional cleanup
   }
   ```
3. **RPCElement Interface**: The element must implement:
   ```go
   type RPCElement interface {
       ProcessRequest(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error)
       ProcessResponse(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error)
       Name() string
   }
   ```

## Loading the Plugin

1. Copy the compiled `.so` file to `/tmp/arpc/elements/`:
   ```bash
   sudo mkdir -p /tmp/arpc/elements
   sudo cp element-example-*.so /tmp/arpc/elements/
   ```

2. The proxy will automatically detect and load the plugin within 1 second

3. **File Selection**: The proxy loads the **highest alphabetically sorted** file matching the `element-` prefix:
   - `element-example.so` < `element-example-v2.so` < `element-example-v3.so`
   - `element-example-20240115-143022.so` < `element-example-20240115-150000.so` (newer timestamp = higher)
   - To update, place a new file with a higher alphabetical name (timestamped builds do this automatically)

## Example: Creating a Custom Element

Here's a template for creating your own element:

```go
package main

import (
    "context"
    "github.com/appnet-org/proxy/util"
)

type MyCustomElement struct {
    // Your fields here
}

func (e *MyCustomElement) ProcessRequest(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
    // Your request processing logic
    return packet, util.PacketVerdictPass, ctx, nil
}

func (e *MyCustomElement) ProcessResponse(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
    // Your response processing logic
    return packet, util.PacketVerdictPass, ctx, nil
}

func (e *MyCustomElement) Name() string {
    return "MyCustomElement"
}

type MyCustomElementInit struct {
    element *MyCustomElement
}

func (e *MyCustomElementInit) Element() RPCElement {
    return e.element
}

func (e *MyCustomElementInit) Kill() {
    // Cleanup if needed
}

var ElementInit = &MyCustomElementInit{
    element: &MyCustomElement{},
}
```

## Troubleshooting

- **Plugin not loading**: Check that the file is in `/appnet/elements/` and starts with `element-`
- **Type errors**: Ensure the plugin uses the same module version and Go version as the proxy
- **Symbol not found**: Make sure you export a variable named exactly `ElementInit`
- **Version mismatch**: Rebuild both the proxy and plugin with the same Go version

