#!/bin/bash
# Build script for the example element plugin
# Usage: ./build.sh [output-name]

set -e

OUTPUT_NAME="${1:-element-example.so}"

echo "Building element plugin: $OUTPUT_NAME"
echo "Go version: $(go version)"

# Build the plugin
go build -buildmode=plugin -o "$OUTPUT_NAME" example_element.go

if [ $? -eq 0 ]; then
    echo "✓ Plugin built successfully: $OUTPUT_NAME"
    echo ""
    echo "To install, copy to /appnet/elements/:"
    echo "  sudo cp $OUTPUT_NAME /appnet/elements/"
    echo ""
    echo "The proxy will automatically load it within 1 second."
else
    echo "✗ Build failed"
    exit 1
fi

