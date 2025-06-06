// Code generated by capnpc-go. DO NOT EDIT.

package echo_capnp

import (
	capnp "capnproto.org/go/capnp/v3"
	text "capnproto.org/go/capnp/v3/encoding/text"
	fc "capnproto.org/go/capnp/v3/flowcontrol"
	schemas "capnproto.org/go/capnp/v3/schemas"
	server "capnproto.org/go/capnp/v3/server"
	context "context"
	math "math"
)

type EchoRequest capnp.Struct

// EchoRequest_TypeID is the unique identifier for the type EchoRequest.
const EchoRequest_TypeID = 0xc1220377c3a1b7a0

func NewEchoRequest(s *capnp.Segment) (EchoRequest, error) {
	st, err := capnp.NewStruct(s, capnp.ObjectSize{DataSize: 8, PointerCount: 1})
	return EchoRequest(st), err
}

func NewRootEchoRequest(s *capnp.Segment) (EchoRequest, error) {
	st, err := capnp.NewRootStruct(s, capnp.ObjectSize{DataSize: 8, PointerCount: 1})
	return EchoRequest(st), err
}

func ReadRootEchoRequest(msg *capnp.Message) (EchoRequest, error) {
	root, err := msg.Root()
	return EchoRequest(root.Struct()), err
}

func (s EchoRequest) String() string {
	str, _ := text.Marshal(0xc1220377c3a1b7a0, capnp.Struct(s))
	return str
}

func (s EchoRequest) EncodeAsPtr(seg *capnp.Segment) capnp.Ptr {
	return capnp.Struct(s).EncodeAsPtr(seg)
}

func (EchoRequest) DecodeFromPtr(p capnp.Ptr) EchoRequest {
	return EchoRequest(capnp.Struct{}.DecodeFromPtr(p))
}

func (s EchoRequest) ToPtr() capnp.Ptr {
	return capnp.Struct(s).ToPtr()
}
func (s EchoRequest) IsValid() bool {
	return capnp.Struct(s).IsValid()
}

func (s EchoRequest) Message() *capnp.Message {
	return capnp.Struct(s).Message()
}

func (s EchoRequest) Segment() *capnp.Segment {
	return capnp.Struct(s).Segment()
}
func (s EchoRequest) Id() int32 {
	return int32(capnp.Struct(s).Uint32(0))
}

func (s EchoRequest) SetId(v int32) {
	capnp.Struct(s).SetUint32(0, uint32(v))
}

func (s EchoRequest) Score() float32 {
	return math.Float32frombits(capnp.Struct(s).Uint32(4))
}

func (s EchoRequest) SetScore(v float32) {
	capnp.Struct(s).SetUint32(4, math.Float32bits(v))
}

func (s EchoRequest) Content() (string, error) {
	p, err := capnp.Struct(s).Ptr(0)
	return p.Text(), err
}

func (s EchoRequest) HasContent() bool {
	return capnp.Struct(s).HasPtr(0)
}

func (s EchoRequest) ContentBytes() ([]byte, error) {
	p, err := capnp.Struct(s).Ptr(0)
	return p.TextBytes(), err
}

func (s EchoRequest) SetContent(v string) error {
	return capnp.Struct(s).SetText(0, v)
}

// EchoRequest_List is a list of EchoRequest.
type EchoRequest_List = capnp.StructList[EchoRequest]

// NewEchoRequest creates a new list of EchoRequest.
func NewEchoRequest_List(s *capnp.Segment, sz int32) (EchoRequest_List, error) {
	l, err := capnp.NewCompositeList(s, capnp.ObjectSize{DataSize: 8, PointerCount: 1}, sz)
	return capnp.StructList[EchoRequest](l), err
}

// EchoRequest_Future is a wrapper for a EchoRequest promised by a client call.
type EchoRequest_Future struct{ *capnp.Future }

func (f EchoRequest_Future) Struct() (EchoRequest, error) {
	p, err := f.Future.Ptr()
	return EchoRequest(p.Struct()), err
}

type EchoResponse capnp.Struct

// EchoResponse_TypeID is the unique identifier for the type EchoResponse.
const EchoResponse_TypeID = 0xb798c2c3642ab860

func NewEchoResponse(s *capnp.Segment) (EchoResponse, error) {
	st, err := capnp.NewStruct(s, capnp.ObjectSize{DataSize: 8, PointerCount: 1})
	return EchoResponse(st), err
}

func NewRootEchoResponse(s *capnp.Segment) (EchoResponse, error) {
	st, err := capnp.NewRootStruct(s, capnp.ObjectSize{DataSize: 8, PointerCount: 1})
	return EchoResponse(st), err
}

func ReadRootEchoResponse(msg *capnp.Message) (EchoResponse, error) {
	root, err := msg.Root()
	return EchoResponse(root.Struct()), err
}

func (s EchoResponse) String() string {
	str, _ := text.Marshal(0xb798c2c3642ab860, capnp.Struct(s))
	return str
}

func (s EchoResponse) EncodeAsPtr(seg *capnp.Segment) capnp.Ptr {
	return capnp.Struct(s).EncodeAsPtr(seg)
}

func (EchoResponse) DecodeFromPtr(p capnp.Ptr) EchoResponse {
	return EchoResponse(capnp.Struct{}.DecodeFromPtr(p))
}

func (s EchoResponse) ToPtr() capnp.Ptr {
	return capnp.Struct(s).ToPtr()
}
func (s EchoResponse) IsValid() bool {
	return capnp.Struct(s).IsValid()
}

func (s EchoResponse) Message() *capnp.Message {
	return capnp.Struct(s).Message()
}

func (s EchoResponse) Segment() *capnp.Segment {
	return capnp.Struct(s).Segment()
}
func (s EchoResponse) Id() int32 {
	return int32(capnp.Struct(s).Uint32(0))
}

func (s EchoResponse) SetId(v int32) {
	capnp.Struct(s).SetUint32(0, uint32(v))
}

func (s EchoResponse) Score() float32 {
	return math.Float32frombits(capnp.Struct(s).Uint32(4))
}

func (s EchoResponse) SetScore(v float32) {
	capnp.Struct(s).SetUint32(4, math.Float32bits(v))
}

func (s EchoResponse) Content() (string, error) {
	p, err := capnp.Struct(s).Ptr(0)
	return p.Text(), err
}

func (s EchoResponse) HasContent() bool {
	return capnp.Struct(s).HasPtr(0)
}

func (s EchoResponse) ContentBytes() ([]byte, error) {
	p, err := capnp.Struct(s).Ptr(0)
	return p.TextBytes(), err
}

func (s EchoResponse) SetContent(v string) error {
	return capnp.Struct(s).SetText(0, v)
}

// EchoResponse_List is a list of EchoResponse.
type EchoResponse_List = capnp.StructList[EchoResponse]

// NewEchoResponse creates a new list of EchoResponse.
func NewEchoResponse_List(s *capnp.Segment, sz int32) (EchoResponse_List, error) {
	l, err := capnp.NewCompositeList(s, capnp.ObjectSize{DataSize: 8, PointerCount: 1}, sz)
	return capnp.StructList[EchoResponse](l), err
}

// EchoResponse_Future is a wrapper for a EchoResponse promised by a client call.
type EchoResponse_Future struct{ *capnp.Future }

func (f EchoResponse_Future) Struct() (EchoResponse, error) {
	p, err := f.Future.Ptr()
	return EchoResponse(p.Struct()), err
}

type EchoService capnp.Client

// EchoService_TypeID is the unique identifier for the type EchoService.
const EchoService_TypeID = 0xe8f0ccf9e3e40d00

func (c EchoService) Echo(ctx context.Context, params func(EchoService_echo_Params) error) (EchoService_echo_Results_Future, capnp.ReleaseFunc) {

	s := capnp.Send{
		Method: capnp.Method{
			InterfaceID:   0xe8f0ccf9e3e40d00,
			MethodID:      0,
			InterfaceName: "echo.capnp:EchoService",
			MethodName:    "echo",
		},
	}
	if params != nil {
		s.ArgsSize = capnp.ObjectSize{DataSize: 0, PointerCount: 1}
		s.PlaceArgs = func(s capnp.Struct) error { return params(EchoService_echo_Params(s)) }
	}

	ans, release := capnp.Client(c).SendCall(ctx, s)
	return EchoService_echo_Results_Future{Future: ans.Future()}, release

}

func (c EchoService) WaitStreaming() error {
	return capnp.Client(c).WaitStreaming()
}

// String returns a string that identifies this capability for debugging
// purposes.  Its format should not be depended on: in particular, it
// should not be used to compare clients.  Use IsSame to compare clients
// for equality.
func (c EchoService) String() string {
	return "EchoService(" + capnp.Client(c).String() + ")"
}

// AddRef creates a new Client that refers to the same capability as c.
// If c is nil or has resolved to null, then AddRef returns nil.
func (c EchoService) AddRef() EchoService {
	return EchoService(capnp.Client(c).AddRef())
}

// Release releases a capability reference.  If this is the last
// reference to the capability, then the underlying resources associated
// with the capability will be released.
//
// Release will panic if c has already been released, but not if c is
// nil or resolved to null.
func (c EchoService) Release() {
	capnp.Client(c).Release()
}

// Resolve blocks until the capability is fully resolved or the Context
// expires.
func (c EchoService) Resolve(ctx context.Context) error {
	return capnp.Client(c).Resolve(ctx)
}

func (c EchoService) EncodeAsPtr(seg *capnp.Segment) capnp.Ptr {
	return capnp.Client(c).EncodeAsPtr(seg)
}

func (EchoService) DecodeFromPtr(p capnp.Ptr) EchoService {
	return EchoService(capnp.Client{}.DecodeFromPtr(p))
}

// IsValid reports whether c is a valid reference to a capability.
// A reference is invalid if it is nil, has resolved to null, or has
// been released.
func (c EchoService) IsValid() bool {
	return capnp.Client(c).IsValid()
}

// IsSame reports whether c and other refer to a capability created by the
// same call to NewClient.  This can return false negatives if c or other
// are not fully resolved: use Resolve if this is an issue.  If either
// c or other are released, then IsSame panics.
func (c EchoService) IsSame(other EchoService) bool {
	return capnp.Client(c).IsSame(capnp.Client(other))
}

// Update the flowcontrol.FlowLimiter used to manage flow control for
// this client. This affects all future calls, but not calls already
// waiting to send. Passing nil sets the value to flowcontrol.NopLimiter,
// which is also the default.
func (c EchoService) SetFlowLimiter(lim fc.FlowLimiter) {
	capnp.Client(c).SetFlowLimiter(lim)
}

// Get the current flowcontrol.FlowLimiter used to manage flow control
// for this client.
func (c EchoService) GetFlowLimiter() fc.FlowLimiter {
	return capnp.Client(c).GetFlowLimiter()
}

// A EchoService_Server is a EchoService with a local implementation.
type EchoService_Server interface {
	Echo(context.Context, EchoService_echo) error
}

// EchoService_NewServer creates a new Server from an implementation of EchoService_Server.
func EchoService_NewServer(s EchoService_Server) *server.Server {
	c, _ := s.(server.Shutdowner)
	return server.New(EchoService_Methods(nil, s), s, c)
}

// EchoService_ServerToClient creates a new Client from an implementation of EchoService_Server.
// The caller is responsible for calling Release on the returned Client.
func EchoService_ServerToClient(s EchoService_Server) EchoService {
	return EchoService(capnp.NewClient(EchoService_NewServer(s)))
}

// EchoService_Methods appends Methods to a slice that invoke the methods on s.
// This can be used to create a more complicated Server.
func EchoService_Methods(methods []server.Method, s EchoService_Server) []server.Method {
	if cap(methods) == 0 {
		methods = make([]server.Method, 0, 1)
	}

	methods = append(methods, server.Method{
		Method: capnp.Method{
			InterfaceID:   0xe8f0ccf9e3e40d00,
			MethodID:      0,
			InterfaceName: "echo.capnp:EchoService",
			MethodName:    "echo",
		},
		Impl: func(ctx context.Context, call *server.Call) error {
			return s.Echo(ctx, EchoService_echo{call})
		},
	})

	return methods
}

// EchoService_echo holds the state for a server call to EchoService.echo.
// See server.Call for documentation.
type EchoService_echo struct {
	*server.Call
}

// Args returns the call's arguments.
func (c EchoService_echo) Args() EchoService_echo_Params {
	return EchoService_echo_Params(c.Call.Args())
}

// AllocResults allocates the results struct.
func (c EchoService_echo) AllocResults() (EchoService_echo_Results, error) {
	r, err := c.Call.AllocResults(capnp.ObjectSize{DataSize: 0, PointerCount: 1})
	return EchoService_echo_Results(r), err
}

// EchoService_List is a list of EchoService.
type EchoService_List = capnp.CapList[EchoService]

// NewEchoService_List creates a new list of EchoService.
func NewEchoService_List(s *capnp.Segment, sz int32) (EchoService_List, error) {
	l, err := capnp.NewPointerList(s, sz)
	return capnp.CapList[EchoService](l), err
}

type EchoService_echo_Params capnp.Struct

// EchoService_echo_Params_TypeID is the unique identifier for the type EchoService_echo_Params.
const EchoService_echo_Params_TypeID = 0xd852f6756d123ece

func NewEchoService_echo_Params(s *capnp.Segment) (EchoService_echo_Params, error) {
	st, err := capnp.NewStruct(s, capnp.ObjectSize{DataSize: 0, PointerCount: 1})
	return EchoService_echo_Params(st), err
}

func NewRootEchoService_echo_Params(s *capnp.Segment) (EchoService_echo_Params, error) {
	st, err := capnp.NewRootStruct(s, capnp.ObjectSize{DataSize: 0, PointerCount: 1})
	return EchoService_echo_Params(st), err
}

func ReadRootEchoService_echo_Params(msg *capnp.Message) (EchoService_echo_Params, error) {
	root, err := msg.Root()
	return EchoService_echo_Params(root.Struct()), err
}

func (s EchoService_echo_Params) String() string {
	str, _ := text.Marshal(0xd852f6756d123ece, capnp.Struct(s))
	return str
}

func (s EchoService_echo_Params) EncodeAsPtr(seg *capnp.Segment) capnp.Ptr {
	return capnp.Struct(s).EncodeAsPtr(seg)
}

func (EchoService_echo_Params) DecodeFromPtr(p capnp.Ptr) EchoService_echo_Params {
	return EchoService_echo_Params(capnp.Struct{}.DecodeFromPtr(p))
}

func (s EchoService_echo_Params) ToPtr() capnp.Ptr {
	return capnp.Struct(s).ToPtr()
}
func (s EchoService_echo_Params) IsValid() bool {
	return capnp.Struct(s).IsValid()
}

func (s EchoService_echo_Params) Message() *capnp.Message {
	return capnp.Struct(s).Message()
}

func (s EchoService_echo_Params) Segment() *capnp.Segment {
	return capnp.Struct(s).Segment()
}
func (s EchoService_echo_Params) Req() (EchoRequest, error) {
	p, err := capnp.Struct(s).Ptr(0)
	return EchoRequest(p.Struct()), err
}

func (s EchoService_echo_Params) HasReq() bool {
	return capnp.Struct(s).HasPtr(0)
}

func (s EchoService_echo_Params) SetReq(v EchoRequest) error {
	return capnp.Struct(s).SetPtr(0, capnp.Struct(v).ToPtr())
}

// NewReq sets the req field to a newly
// allocated EchoRequest struct, preferring placement in s's segment.
func (s EchoService_echo_Params) NewReq() (EchoRequest, error) {
	ss, err := NewEchoRequest(capnp.Struct(s).Segment())
	if err != nil {
		return EchoRequest{}, err
	}
	err = capnp.Struct(s).SetPtr(0, capnp.Struct(ss).ToPtr())
	return ss, err
}

// EchoService_echo_Params_List is a list of EchoService_echo_Params.
type EchoService_echo_Params_List = capnp.StructList[EchoService_echo_Params]

// NewEchoService_echo_Params creates a new list of EchoService_echo_Params.
func NewEchoService_echo_Params_List(s *capnp.Segment, sz int32) (EchoService_echo_Params_List, error) {
	l, err := capnp.NewCompositeList(s, capnp.ObjectSize{DataSize: 0, PointerCount: 1}, sz)
	return capnp.StructList[EchoService_echo_Params](l), err
}

// EchoService_echo_Params_Future is a wrapper for a EchoService_echo_Params promised by a client call.
type EchoService_echo_Params_Future struct{ *capnp.Future }

func (f EchoService_echo_Params_Future) Struct() (EchoService_echo_Params, error) {
	p, err := f.Future.Ptr()
	return EchoService_echo_Params(p.Struct()), err
}
func (p EchoService_echo_Params_Future) Req() EchoRequest_Future {
	return EchoRequest_Future{Future: p.Future.Field(0, nil)}
}

type EchoService_echo_Results capnp.Struct

// EchoService_echo_Results_TypeID is the unique identifier for the type EchoService_echo_Results.
const EchoService_echo_Results_TypeID = 0xf47bc8e9b85ec174

func NewEchoService_echo_Results(s *capnp.Segment) (EchoService_echo_Results, error) {
	st, err := capnp.NewStruct(s, capnp.ObjectSize{DataSize: 0, PointerCount: 1})
	return EchoService_echo_Results(st), err
}

func NewRootEchoService_echo_Results(s *capnp.Segment) (EchoService_echo_Results, error) {
	st, err := capnp.NewRootStruct(s, capnp.ObjectSize{DataSize: 0, PointerCount: 1})
	return EchoService_echo_Results(st), err
}

func ReadRootEchoService_echo_Results(msg *capnp.Message) (EchoService_echo_Results, error) {
	root, err := msg.Root()
	return EchoService_echo_Results(root.Struct()), err
}

func (s EchoService_echo_Results) String() string {
	str, _ := text.Marshal(0xf47bc8e9b85ec174, capnp.Struct(s))
	return str
}

func (s EchoService_echo_Results) EncodeAsPtr(seg *capnp.Segment) capnp.Ptr {
	return capnp.Struct(s).EncodeAsPtr(seg)
}

func (EchoService_echo_Results) DecodeFromPtr(p capnp.Ptr) EchoService_echo_Results {
	return EchoService_echo_Results(capnp.Struct{}.DecodeFromPtr(p))
}

func (s EchoService_echo_Results) ToPtr() capnp.Ptr {
	return capnp.Struct(s).ToPtr()
}
func (s EchoService_echo_Results) IsValid() bool {
	return capnp.Struct(s).IsValid()
}

func (s EchoService_echo_Results) Message() *capnp.Message {
	return capnp.Struct(s).Message()
}

func (s EchoService_echo_Results) Segment() *capnp.Segment {
	return capnp.Struct(s).Segment()
}
func (s EchoService_echo_Results) Resp() (EchoResponse, error) {
	p, err := capnp.Struct(s).Ptr(0)
	return EchoResponse(p.Struct()), err
}

func (s EchoService_echo_Results) HasResp() bool {
	return capnp.Struct(s).HasPtr(0)
}

func (s EchoService_echo_Results) SetResp(v EchoResponse) error {
	return capnp.Struct(s).SetPtr(0, capnp.Struct(v).ToPtr())
}

// NewResp sets the resp field to a newly
// allocated EchoResponse struct, preferring placement in s's segment.
func (s EchoService_echo_Results) NewResp() (EchoResponse, error) {
	ss, err := NewEchoResponse(capnp.Struct(s).Segment())
	if err != nil {
		return EchoResponse{}, err
	}
	err = capnp.Struct(s).SetPtr(0, capnp.Struct(ss).ToPtr())
	return ss, err
}

// EchoService_echo_Results_List is a list of EchoService_echo_Results.
type EchoService_echo_Results_List = capnp.StructList[EchoService_echo_Results]

// NewEchoService_echo_Results creates a new list of EchoService_echo_Results.
func NewEchoService_echo_Results_List(s *capnp.Segment, sz int32) (EchoService_echo_Results_List, error) {
	l, err := capnp.NewCompositeList(s, capnp.ObjectSize{DataSize: 0, PointerCount: 1}, sz)
	return capnp.StructList[EchoService_echo_Results](l), err
}

// EchoService_echo_Results_Future is a wrapper for a EchoService_echo_Results promised by a client call.
type EchoService_echo_Results_Future struct{ *capnp.Future }

func (f EchoService_echo_Results_Future) Struct() (EchoService_echo_Results, error) {
	p, err := f.Future.Ptr()
	return EchoService_echo_Results(p.Struct()), err
}
func (p EchoService_echo_Results_Future) Resp() EchoResponse_Future {
	return EchoResponse_Future{Future: p.Future.Field(0, nil)}
}

const schema_bf5147bb3b06fa3d = "x\xda\xc4\x92?k\x14Q\x14\xc5\xcf\xb9o6#\xc9" +
	".\xd9\xe7\xac\x88i\x02K\xaa\x05\x83\x7f:Ew\x11" +
	"C\x14\x14\xe6m\xac\xc5e\xf2 \x0133\x99?\x8a" +
	"\x88\xf85,\xf5\x03\x88\x95$\xc4B\x924\x16b'" +
	"ha\xa5\x85\xe9\x14D\xd0\xc2\x91\xc9\x9a\x1d\x85\x05\xcb" +
	"t\x97\xcby?~\x97\xf3N\xb9\xec9\xa7\x1b\xb3\x0a" +
	"b\xe6j\x13\xc5\xad\xcd\xce\xf2\xee\xce\xa3\x0d\x98)\xb2" +
	"\xb8\xf0s\xe2\xfc\x8bE\xf3\x125\xba\x80\x97s\xc7{" +
	"\xb0?\xdd\xe33\xb0x\xbc\xf1d\xf7\xaejo\x8f\x0d" +
	"k\xd9\xf2NH9\x1d\x932\xfc\xe6\xe2\xd1\xb5\xfc{" +
	"\xff\x1d\xf4qb\x989\xfbTf\x08z\xcf\xa5\x0b\xfe" +
	"j|\xfa\xf8\xe3\xf5\x97\xcfzJU(\xd0{+[" +
	"\xde\x87}\xce{Y\xf4\xa8\\\xa0\xc8\xb6on\xee\xbd" +
	"\xba\xff\xedo\xd6\x9e\xb4K\xd6W\xe9\xe2da\x83\x95" +
	"h>\x18\xc4\x12\xc6\xe7\x16\x82\x95\xa8o\xd38\x0aS" +
	"\x0b\x9f4u\xe5\x00\x0e\x01\xbd0\x03\x98\x9e\xa2\xb9&" +
	"\xd4d\x8b\xe5\xf2\xea\x19\xc0\\V4\xbe\x90\xd2\xa2\x00" +
	"\xfa\xfa%\xc0\\Q47\x84ju\x99\x0e\x84\x0e8" +
	"\x9b\x06Qb9\x09\xe1$\xf80\x88\xc2\xcc\x86\x19\xeb" +
	"\x10\xd6\xc1\x91\x05\x0f,\xbav=\xb7iv(\x12\xea" +
	"\x8f\xc4\x92M\xee\xac\x06v\xbe\xdc\xcf\xf9\x83d\xa0\xd6" +
	"R\xe3\x8cl\x1am\xc0\x1cQ4-\xa1\x9b\xd8u6" +
	"\xab\x9aA6\xc7\x9c\xb5\xd4\x1d\"\xcb\xb3\x1cU\x03F" +
	"]\xf3\xa0(\xad;\x10]s\xa7\xcb\xb7=\xfa\xfc\x8f" +
	"W\xdf\xa6\xb9{;\xfbG\xacS\x89M'6\x8d\xd9" +
	"\xac~\xeb\xd0\xecw\x00\x00\x00\xff\xff\xc6\xce\xac\xe3"

func RegisterSchema(reg *schemas.Registry) {
	reg.Register(&schemas.Schema{
		String: schema_bf5147bb3b06fa3d,
		Nodes: []uint64{
			0xb798c2c3642ab860,
			0xc1220377c3a1b7a0,
			0xd852f6756d123ece,
			0xe8f0ccf9e3e40d00,
			0xf47bc8e9b85ec174,
		},
		Compressed: true,
	})
}
