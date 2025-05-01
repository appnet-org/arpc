package main

import (
	"fmt"
	"os"
	"strings"
)

func writeCode(f *os.File, args ...string) {
	format := args[0]
	values := make([]any, len(args)-1)
	for i, v := range args[1:] {
		values[i] = v
	}

	result := fmt.Sprintf(format, values...) + "\n"
	f.WriteString(result)
}

func genMethod(f *os.File, iname string, mname string, method *Method) {
	writeCode(f, "func (c *arpc%sClient) %s(ctx context.Context, req *%s_) (*%s_, error) {", iname, mname, method.ReqType, method.RespType)
	writeCode(f, "    resp := new(%s_)", method.RespType)
	writeCode(f, "    if err := c.client.Call(\"%s\", \"%s\", req.Msg, &resp.Msg); err != nil {", iname, mname)
	writeCode(f, "        return nil, err")
	writeCode(f, "    }")
	writeCode(f, "    %s, err := ReadRoot%s(resp.Msg)", Uncapitalize(method.RespType), method.RespType)
	writeCode(f, "    if err != nil {")
	writeCode(f, "        return nil, err")
	writeCode(f, "    }")
	writeCode(f, "    resp.CapnpStruct = &%s", Uncapitalize(method.RespType))
	writeCode(f, "    return resp, nil")
	writeCode(f, "}")
	writeCode(f, "")
}

func genServiceClient(f *os.File, iname string, iface *Interface) {
	writeCode(f, "type %sClient interface {", iname)
	for mname, method := range iface.Methods {
		writeCode(f, "    %s(ctx context.Context, req *%s_) (*%s_, error)", mname, method.ReqType, method.RespType)
	}
	writeCode(f, "}")
	writeCode(f, "")

	writeCode(f, "type arpc%sClient struct {", iname)
	writeCode(f, "    client *rpc.Client")
	writeCode(f, "}")
	writeCode(f, "")

	writeCode(f, "func New%sClient(client *rpc.Client) %sClient {", iname, iname)
	writeCode(f, "    return &arpc%sClient{client: client}", iname)
	writeCode(f, "}")
	writeCode(f, "")

	for mname, method := range iface.Methods {
		genMethod(f, iname, mname, method)
	}
}

func genServiceServer(f *os.File, iname string, iface *Interface) {
	writeCode(f, "type %sServer interface {", iname)
	for mname, method := range iface.Methods {
		writeCode(f, "    %s(ctx context.Context, req *%s_) (*%s_, error)", mname, method.ReqType, method.RespType)
	}
	writeCode(f, "}")
	writeCode(f, "")

	writeCode(f, "func Register%sServer(s *rpc.Server, srv %sServer) {", iname, iname)
	writeCode(f, "    s.RegisterService(&rpc.ServiceDesc{")
	writeCode(f, "        ServiceName: \"%s\",", iname)
	writeCode(f, "        ServiceImpl: srv,")
	writeCode(f, "        Methods: map[string]*rpc.MethodDesc{")
	for mname := range iface.Methods {
		writeCode(f, "            \"%s\": {", mname)
		writeCode(f, "                MethodName: \"%s\",", mname)
		writeCode(f, "                Handler: _%s_%s_Handler,", iname, mname)
		writeCode(f, "            },")
	}
	writeCode(f, "        },")
	writeCode(f, "    }, srv)")
	writeCode(f, "}")
	writeCode(f, "")

	for mname, method := range iface.Methods {
		writeCode(f, "func _%s_%s_Handler(srv any, ctx context.Context, dec func(any) error) (any, error) {", iname, mname)
		writeCode(f, "    in := new(%s_)", method.ReqType)
		writeCode(f, "    if err := dec(&in.Msg); err != nil {")
		writeCode(f, "        return nil, err")
		writeCode(f, "    }")
		writeCode(f, "    %s, err := ReadRoot%s(in.Msg)", Uncapitalize(method.ReqType), method.ReqType)
		writeCode(f, "    if err != nil {")
		writeCode(f, "        return nil, err")
		writeCode(f, "    }")
		writeCode(f, "    in.CapnpStruct = &%s", Uncapitalize(method.ReqType))
		writeCode(f, "    resp, err := srv.(%sServer).%s(ctx, in)", iname, mname)
		writeCode(f, "    return resp.Msg, err")
		writeCode(f, "}")
		writeCode(f, "")
	}
}

func genWrapper(f *os.File, sname string, s *Struct) {
	writeCode(f, "type %s_ struct {", sname)
	writeCode(f, "    Msg		  *capnp.Message")
	writeCode(f, "    CapnpStruct *%s", sname)
	writeCode(f, "}")
	writeCode(f, "")

	signature := make([]string, 0)
	for fname, fd := range s.Fields {
		signature = append(signature, fname+" "+fd.Type)
		writeCode(f, "func (e *%s_) Get%s() (%s, error) {", sname, Capitalize(fname), fd.Type)
		writeCode(f, "    return e.CapnpStruct.%s()", Capitalize(fname))
		writeCode(f, "}")
		writeCode(f, "")
	}

	writeCode(f, "func Create%s(%s) (*%s_, error) {", sname, strings.Join(signature, ", "), sname)
	writeCode(f, "    msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))")
	writeCode(f, "    if err != nil {")
	writeCode(f, "        return nil, err")
	writeCode(f, "    }")
	writeCode(f, "    capnpStruct, err := NewRoot%s(seg)", sname)
	writeCode(f, "    if err != nil {")
	writeCode(f, "        return nil, err")
	writeCode(f, "    }")
	for fname := range s.Fields {
		writeCode(f, "    err = capnpStruct.Set%s(%s)", Capitalize(fname), fname)
		writeCode(f, "    if err != nil {")
		writeCode(f, "        return nil, err")
		writeCode(f, "    }")
	}
	writeCode(f, "    %s := &%s_{", Uncapitalize(sname), sname)
	writeCode(f, "        Msg:         msg,")
	writeCode(f, "        CapnpStruct: &capnpStruct,")
	writeCode(f, "    }")
	writeCode(f, "    return %s, nil", Uncapitalize(sname))
	writeCode(f, "}")
	writeCode(f, "")
}

func genCode(f *os.File, schema *Schema) {
	writeCode(f, "// Code generated by capnp-gen-arpc. DO NOT EDIT.")
	writeCode(f, "package %s", schema.PackageName)
	writeCode(f, "")
	writeCode(f, "import (")
	writeCode(f, "    \"context\"")
	writeCode(f, "    \"capnproto.org/go/capnp/v3\"")
	writeCode(f, "    \"github.com/appnet-org/arpc/pkg/rpc\"")
	writeCode(f, ")")
	writeCode(f, "")

	for sname, s := range schema.Structs {
		genWrapper(f, sname, s)
	}

	for iname, iface := range schema.Interfaces {
		genServiceClient(f, iname, iface)
		genServiceServer(f, iname, iface)
	}
}
