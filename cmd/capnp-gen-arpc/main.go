package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	filename := flag.String("file", "../../examples/echo_capnp/capnp/echo.capnp", "Cap'n Proto file to parse")
	schema, err := parse(*filename)
	if err != nil {
		fmt.Println("Error parsing Cap'n Proto file:", err)
		os.Exit(1)
	}

	absPath, _ := filepath.Abs(*filename)
	stubFile := absPath[:len(absPath)-len(".capnp")] + "_arpc.capnp.go"
	file, err := os.Create(stubFile)
	if err != nil {
		fmt.Println("Error creating file:", err)
		os.Exit(1)
	}
	defer file.Close()

	genCode(file, schema)
}
