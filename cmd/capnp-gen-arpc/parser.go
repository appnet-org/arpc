package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func primitiveTypeLookUp(typeName string) string {
	if typeName == "Text" {
		return "string"
	}
	return typeName
}

func parse(capnpFilename string) (*Schema, error) {
	if !strings.HasSuffix(capnpFilename, ".capnp") {
		return nil, fmt.Errorf("file %s is not a .capnp file", capnpFilename)
	}
	file, err := os.Open(capnpFilename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	schema := Schema{
		Interfaces: make(map[string]*Interface),
		Structs:    make(map[string]*Struct),
	}
	var currentInterface *Interface
	var currentStruct *Struct

	methodRe := regexp.MustCompile(`(\w+)\s+@(\d+)\s+\((\w+)\s*:\s*(\w+)\)\s*->\s*\((\w+)\s*:\s*(\w+)\)`)
	fieldRe := regexp.MustCompile(`(\w+)\s+@(\d+)\s*:\s*(\w+);?`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "$Go.import") {
			continue
		}
		if strings.HasPrefix(line, "using") {
			continue
		}
		if line == "}" {
			// TODO: support scopes
			continue
		}

		if strings.HasPrefix(line, "@0x") {
			// Capnp ID
			schema.ID = strings.TrimSuffix(line, ";")
		} else if strings.HasPrefix(line, "$Go.package") {
			// Package name
			schema.PackageName = strings.TrimSuffix(strings.TrimPrefix(line, "$Go.package(\""), "\");")
		} else if strings.HasPrefix(line, "interface ") {
			// Interface definition
			name := strings.TrimSuffix(strings.Fields(line)[1], "{")
			currentInterface = &Interface{
				Name:    name,
				Methods: make(map[string]*Method),
			}
			schema.Interfaces[name] = currentInterface
		} else if strings.HasPrefix(line, "struct ") {
			// Struct definition
			name := strings.TrimSuffix(strings.Fields(line)[1], "{")
			currentStruct = &Struct{
				Name:   name,
				Fields: make(map[string]*Field),
			}
			schema.Structs[name] = currentStruct
		} else if matches := methodRe.FindStringSubmatch(line); len(matches) == 7 {
			// Method definition
			// Format: methodName @Tag (req : reqType) -> (resp: respType))
			methodName := strings.ToUpper(matches[1][:1]) + matches[1][1:]
			reqType := matches[4]
			respType := matches[6]
			method := Method{
				Name:     methodName,
				ReqType:  reqType,
				RespType: respType,
			}
			if currentInterface != nil {
				currentInterface.Methods[methodName] = &method
			} else {
				return nil, fmt.Errorf("method defined outside of an interface: %s", line)
			}
		} else if matches := fieldRe.FindStringSubmatch(line); len(matches) == 4 {
			// Field definition
			// Format: fieldName @Tag : fieldType;
			fieldName := matches[1]
			tag, _ := strconv.Atoi(matches[2])
			typeName := primitiveTypeLookUp(matches[3])
			field := Field{
				Name: fieldName,
				Tag:  tag,
				Type: typeName,
			}
			if currentStruct != nil {
				currentStruct.Fields[fieldName] = &field
			} else {
				return nil, fmt.Errorf("field defined outside of a struct: %s", line)
			}
		} else {
			// Unexpected line
			return nil, fmt.Errorf("unexpected line: %s", line)
		}
	}

	return &schema, nil
}
