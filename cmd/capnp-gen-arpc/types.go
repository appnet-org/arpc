package main

type Schema struct {
	ID          string
	PackageName string
	Interfaces  map[string]*Interface
	Structs     map[string]*Struct
}

type Interface struct {
	Name    string
	Methods map[string]*Method
}

type Method struct {
	Name     string
	ReqType  string
	RespType string
}

type Struct struct {
	Name   string
	Fields map[string]*Field
}

type Field struct {
	Name string
	Type string
	Tag  int
}
