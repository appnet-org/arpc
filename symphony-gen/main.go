package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var inputPath string
	flag.StringVar(&inputPath, "input", "", "Path to the .syn file to parse")
	flag.Parse()

	if inputPath == "" {
		flag.Usage()
		log.Fatal("error: --input flag is required")
	}

	absPath, err := filepath.Abs(inputPath)
	if err != nil {
		log.Fatalf("error getting absolute path: %v", err)
	}
	if !strings.HasSuffix(absPath, ".syn") {
		log.Fatalf("error: input file must have .syn extension")
	}

	base := strings.TrimSuffix(absPath, ".syn")

	stubFile := base + "_arpc.syn.go"
	if err := touch(stubFile); err != nil {
		log.Fatalf("error creating stub file: %v", err)
	}

	serializationFile := base + ".syn.go"
	if err := touch(serializationFile); err != nil {
		log.Fatalf("error creating serialization file: %v", err)
	}
}

func touch(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}
