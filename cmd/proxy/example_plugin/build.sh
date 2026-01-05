#!/bin/bash
# Build script for the example element plugin
# Usage: ./build.sh [output-name]
# If no name provided, generates a timestamped filename: element-example-YYYYMMDD-HHMMSS.so

set -e

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROXY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Generate timestamp-based filename if not provided
if [ -z "$1" ]; then
    TIMESTAMP=$(date +"%Y%m%d-%H%M%S")
    OUTPUT_NAME="element-example-${TIMESTAMP}.so"
else
    OUTPUT_NAME="$1"
fi

echo "Building element plugin: $OUTPUT_NAME"
echo "Go version: $(go version)"

# Build the plugin from the proxy module directory to ensure same module context
# This is critical - plugins MUST be built with the same Go version and module
# context as the main application to avoid "different version of package" errors
cd "$PROXY_DIR"
go build -buildmode=plugin -o "$SCRIPT_DIR/$OUTPUT_NAME" -trimpath "$SCRIPT_DIR/example_element.go"

if [ $? -eq 0 ]; then
    echo "✓ Plugin built successfully: $OUTPUT_NAME"
    
    # Ensure target directory exists and move the plugin
    mkdir -p /tmp/arpc/elements/
    mv "$SCRIPT_DIR/$OUTPUT_NAME" /tmp/arpc/elements/
    
    echo "✓ Plugin installed to /tmp/arpc/elements/$OUTPUT_NAME"
    echo ""
    echo "The proxy will automatically load it within 1 second."
    echo "Note: The elementloader selects the highest alphabetically sorted file,"
    echo "      so timestamped builds will automatically be selected when newer."
else
    echo "✗ Build failed"
    exit 1
fi

