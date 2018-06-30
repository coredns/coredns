// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/devtools/build/v1/publish_build_event.proto

package build // import "google.golang.org/genproto/googleapis/devtools/build/v1"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import duration "github.com/golang/protobuf/ptypes/duration"
import empty "github.com/golang/protobuf/ptypes/empty"
import _ "google.golang.org/genproto/googleapis/api/annotations"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// The service level of the build request. Backends only uses this value when
// the BuildEnqueued event is published to determine what level of service
// this build should receive.
type PublishLifecycleEventRequest_ServiceLevel int32

const (
	// Non-interactive builds can tolerate longer event latencies. This is the
	// default ServiceLevel if callers do not specify one.
	PublishLifecycleEventRequest_NONINTERACTIVE PublishLifecycleEventRequest_ServiceLevel = 0
	// The events of an interactive build should be delivered with low latency.
	PublishLifecycleEventRequest_INTERACTIVE PublishLifecycleEventRequest_ServiceLevel = 1
)

var PublishLifecycleEventRequest_ServiceLevel_name = map[int32]string{
	0: "NONINTERACTIVE",
	1: "INTERACTIVE",
}
var PublishLifecycleEventRequest_ServiceLevel_value = map[string]int32{
	"NONINTERACTIVE": 0,
	"INTERACTIVE":    1,
}

func (x PublishLifecycleEventRequest_ServiceLevel) String() string {
	return proto.EnumName(PublishLifecycleEventRequest_ServiceLevel_name, int32(x))
}
func (PublishLifecycleEventRequest_ServiceLevel) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_publish_build_event_392a703d66bd0f43, []int{0, 0}
}

// Publishes 'lifecycle events' that update the high-level state of a build:
// - BuildEnqueued: When a build is scheduled.
// - InvocationAttemptStarted: When work for a build starts; there can be
//     multiple invocations for a build (e.g. retries).
// - InvocationAttemptCompleted: When work for a build finishes.
// - BuildFinished: When a build is finished.
type PublishLifecycleEventRequest struct {
	// The interactivity of this build.
	ServiceLevel PublishLifecycleEventRequest_ServiceLevel `protobuf:"varint,1,opt,name=service_level,json=serviceLevel,proto3,enum=google.devtools.build.v1.PublishLifecycleEventRequest_ServiceLevel" json:"service_level,omitempty"`
	// The lifecycle build event. If this is a build tool event, the RPC will fail
	// with INVALID_REQUEST.
	BuildEvent *OrderedBuildEvent `protobuf:"bytes,2,opt,name=build_event,json=buildEvent,proto3" json:"build_event,omitempty"`
	// If the next event for this build or invocation (depending on the event
	// type) hasn't been published after this duration from when {build_event}
	// is written to BES, consider this stream expired. If this field is not set,
	// BES backend will use its own default value.
	StreamTimeout *duration.Duration `protobuf:"bytes,3,opt,name=stream_timeout,json=streamTimeout,proto3" json:"stream_timeout,omitempty"`
	// Additional information about a build request. These are define by the event
	// publishers, and the Build Event Service does not validate or interpret
	// them. They are used while notifying internal systems of new builds and
	// invocations if the OrderedBuildEvent.event type is
	// BuildEnqueued/InvocationAttemptStarted.
	NotificationKeywords []string `protobuf:"bytes,4,rep,name=notification_keywords,json=notificationKeywords,proto3" json:"notification_keywords,omitempty"`
	// This field identifies which project (if any) the build is associated with.
	ProjectId            string   `protobuf:"bytes,6,opt,name=project_id,json=projectId,proto3" json:"project_id,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *PublishLifecycleEventRequest) Reset()         { *m = PublishLifecycleEventRequest{} }
func (m *PublishLifecycleEventRequest) String() string { return proto.CompactTextString(m) }
func (*PublishLifecycleEventRequest) ProtoMessage()    {}
func (*PublishLifecycleEventRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_publish_build_event_392a703d66bd0f43, []int{0}
}
func (m *PublishLifecycleEventRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_PublishLifecycleEventRequest.Unmarshal(m, b)
}
func (m *PublishLifecycleEventRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_PublishLifecycleEventRequest.Marshal(b, m, deterministic)
}
func (dst *PublishLifecycleEventRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_PublishLifecycleEventRequest.Merge(dst, src)
}
func (m *PublishLifecycleEventRequest) XXX_Size() int {
	return xxx_messageInfo_PublishLifecycleEventRequest.Size(m)
}
func (m *PublishLifecycleEventRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_PublishLifecycleEventRequest.DiscardUnknown(m)
}

var xxx_messageInfo_PublishLifecycleEventRequest proto.InternalMessageInfo

func (m *PublishLifecycleEventRequest) GetServiceLevel() PublishLifecycleEventRequest_ServiceLevel {
	if m != nil {
		return m.ServiceLevel
	}
	return PublishLifecycleEventRequest_NONINTERACTIVE
}

func (m *PublishLifecycleEventRequest) GetBuildEvent() *OrderedBuildEvent {
	if m != nil {
		return m.BuildEvent
	}
	return nil
}

func (m *PublishLifecycleEventRequest) GetStreamTimeout() *duration.Duration {
	if m != nil {
		return m.StreamTimeout
	}
	return nil
}

func (m *PublishLifecycleEventRequest) GetNotificationKeywords() []string {
	if m != nil {
		return m.NotificationKeywords
	}
	return nil
}

func (m *PublishLifecycleEventRequest) GetProjectId() string {
	if m != nil {
		return m.ProjectId
	}
	return ""
}

// States which event has been committed. Any failure to commit will cause
// RPC errors, hence not recorded by this proto.
type PublishBuildToolEventStreamResponse struct {
	// The stream that contains this event.
	StreamId *StreamId `protobuf:"bytes,1,opt,name=stream_id,json=streamId,proto3" json:"stream_id,omitempty"`
	// The sequence number of this event that has been committed.
	SequenceNumber       int64    `protobuf:"varint,2,opt,name=sequence_number,json=sequenceNumber,proto3" json:"sequence_number,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *PublishBuildToolEventStreamResponse) Reset()         { *m = PublishBuildToolEventStreamResponse{} }
func (m *PublishBuildToolEventStreamResponse) String() string { return proto.CompactTextString(m) }
func (*PublishBuildToolEventStreamResponse) ProtoMessage()    {}
func (*PublishBuildToolEventStreamResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_publish_build_event_392a703d66bd0f43, []int{1}
}
func (m *PublishBuildToolEventStreamResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_PublishBuildToolEventStreamResponse.Unmarshal(m, b)
}
func (m *PublishBuildToolEventStreamResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_PublishBuildToolEventStreamResponse.Marshal(b, m, deterministic)
}
func (dst *PublishBuildToolEventStreamResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_PublishBuildToolEventStreamResponse.Merge(dst, src)
}
func (m *PublishBuildToolEventStreamResponse) XXX_Size() int {
	return xxx_messageInfo_PublishBuildToolEventStreamResponse.Size(m)
}
func (m *PublishBuildToolEventStreamResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_PublishBuildToolEventStreamResponse.DiscardUnknown(m)
}

var xxx_messageInfo_PublishBuildToolEventStreamResponse proto.InternalMessageInfo

func (m *PublishBuildToolEventStreamResponse) GetStreamId() *StreamId {
	if m != nil {
		return m.StreamId
	}
	return nil
}

func (m *PublishBuildToolEventStreamResponse) GetSequenceNumber() int64 {
	if m != nil {
		return m.SequenceNumber
	}
	return 0
}

// Build event with contextual information about the stream it belongs to and
// its position in that stream.
type OrderedBuildEvent struct {
	// Which build event stream this event belongs to.
	StreamId *StreamId `protobuf:"bytes,1,opt,name=stream_id,json=streamId,proto3" json:"stream_id,omitempty"`
	// The position of this event in the stream. The sequence numbers for a build
	// event stream should be a sequence of consecutive natural numbers starting
	// from one. (1, 2, 3, ...)
	SequenceNumber int64 `protobuf:"varint,2,opt,name=sequence_number,json=sequenceNumber,proto3" json:"sequence_number,omitempty"`
	// The actual event.
	Event                *BuildEvent `protobuf:"bytes,3,opt,name=event,proto3" json:"event,omitempty"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *OrderedBuildEvent) Reset()         { *m = OrderedBuildEvent{} }
func (m *OrderedBuildEvent) String() string { return proto.CompactTextString(m) }
func (*OrderedBuildEvent) ProtoMessage()    {}
func (*OrderedBuildEvent) Descriptor() ([]byte, []int) {
	return fileDescriptor_publish_build_event_392a703d66bd0f43, []int{2}
}
func (m *OrderedBuildEvent) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_OrderedBuildEvent.Unmarshal(m, b)
}
func (m *OrderedBuildEvent) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_OrderedBuildEvent.Marshal(b, m, deterministic)
}
func (dst *OrderedBuildEvent) XXX_Merge(src proto.Message) {
	xxx_messageInfo_OrderedBuildEvent.Merge(dst, src)
}
func (m *OrderedBuildEvent) XXX_Size() int {
	return xxx_messageInfo_OrderedBuildEvent.Size(m)
}
func (m *OrderedBuildEvent) XXX_DiscardUnknown() {
	xxx_messageInfo_OrderedBuildEvent.DiscardUnknown(m)
}

var xxx_messageInfo_OrderedBuildEvent proto.InternalMessageInfo

func (m *OrderedBuildEvent) GetStreamId() *StreamId {
	if m != nil {
		return m.StreamId
	}
	return nil
}

func (m *OrderedBuildEvent) GetSequenceNumber() int64 {
	if m != nil {
		return m.SequenceNumber
	}
	return 0
}

func (m *OrderedBuildEvent) GetEvent() *BuildEvent {
	if m != nil {
		return m.Event
	}
	return nil
}

type PublishBuildToolEventStreamRequest struct {
	// Which build event stream this event belongs to.
	StreamId *StreamId `protobuf:"bytes,1,opt,name=stream_id,json=streamId,proto3" json:"stream_id,omitempty"` // Deprecated: Do not use.
	// The position of this event in the stream. The sequence numbers for a build
	// event stream should be a sequence of consecutive natural numbers starting
	// from one. (1, 2, 3, ...)
	SequenceNumber int64 `protobuf:"varint,2,opt,name=sequence_number,json=sequenceNumber,proto3" json:"sequence_number,omitempty"` // Deprecated: Do not use.
	// The actual event.
	Event *BuildEvent `protobuf:"bytes,3,opt,name=event,proto3" json:"event,omitempty"` // Deprecated: Do not use.
	// The build event with position info.
	// New publishing clients should use this field rather than the 3 above.
	OrderedBuildEvent *OrderedBuildEvent `protobuf:"bytes,4,opt,name=ordered_build_event,json=orderedBuildEvent,proto3" json:"ordered_build_event,omitempty"`
	// The keywords to be attached to the notification which notifies the start
	// of a new build event stream. BES only reads this field when sequence_number
	// or ordered_build_event.sequence_number is 1 in this message. If this field
	// is empty, BES will not publish notification messages for this stream.
	NotificationKeywords []string `protobuf:"bytes,5,rep,name=notification_keywords,json=notificationKeywords,proto3" json:"notification_keywords,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *PublishBuildToolEventStreamRequest) Reset()         { *m = PublishBuildToolEventStreamRequest{} }
func (m *PublishBuildToolEventStreamRequest) String() string { return proto.CompactTextString(m) }
func (*PublishBuildToolEventStreamRequest) ProtoMessage()    {}
func (*PublishBuildToolEventStreamRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_publish_build_event_392a703d66bd0f43, []int{3}
}
func (m *PublishBuildToolEventStreamRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_PublishBuildToolEventStreamRequest.Unmarshal(m, b)
}
func (m *PublishBuildToolEventStreamRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_PublishBuildToolEventStreamRequest.Marshal(b, m, deterministic)
}
func (dst *PublishBuildToolEventStreamRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_PublishBuildToolEventStreamRequest.Merge(dst, src)
}
func (m *PublishBuildToolEventStreamRequest) XXX_Size() int {
	return xxx_messageInfo_PublishBuildToolEventStreamRequest.Size(m)
}
func (m *PublishBuildToolEventStreamRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_PublishBuildToolEventStreamRequest.DiscardUnknown(m)
}

var xxx_messageInfo_PublishBuildToolEventStreamRequest proto.InternalMessageInfo

// Deprecated: Do not use.
func (m *PublishBuildToolEventStreamRequest) GetStreamId() *StreamId {
	if m != nil {
		return m.StreamId
	}
	return nil
}

// Deprecated: Do not use.
func (m *PublishBuildToolEventStreamRequest) GetSequenceNumber() int64 {
	if m != nil {
		return m.SequenceNumber
	}
	return 0
}

// Deprecated: Do not use.
func (m *PublishBuildToolEventStreamRequest) GetEvent() *BuildEvent {
	if m != nil {
		return m.Event
	}
	return nil
}

func (m *PublishBuildToolEventStreamRequest) GetOrderedBuildEvent() *OrderedBuildEvent {
	if m != nil {
		return m.OrderedBuildEvent
	}
	return nil
}

func (m *PublishBuildToolEventStreamRequest) GetNotificationKeywords() []string {
	if m != nil {
		return m.NotificationKeywords
	}
	return nil
}

func init() {
	proto.RegisterType((*PublishLifecycleEventRequest)(nil), "google.devtools.build.v1.PublishLifecycleEventRequest")
	proto.RegisterType((*PublishBuildToolEventStreamResponse)(nil), "google.devtools.build.v1.PublishBuildToolEventStreamResponse")
	proto.RegisterType((*OrderedBuildEvent)(nil), "google.devtools.build.v1.OrderedBuildEvent")
	proto.RegisterType((*PublishBuildToolEventStreamRequest)(nil), "google.devtools.build.v1.PublishBuildToolEventStreamRequest")
	proto.RegisterEnum("google.devtools.build.v1.PublishLifecycleEventRequest_ServiceLevel", PublishLifecycleEventRequest_ServiceLevel_name, PublishLifecycleEventRequest_ServiceLevel_value)
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// PublishBuildEventClient is the client API for PublishBuildEvent service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type PublishBuildEventClient interface {
	// Publish a build event stating the new state of a build (typically from the
	// build queue). If the event is a BuildEnqueued event, also register the new
	// build request ID and its build type to BES.
	//
	// The backend will persist the event and deliver it to registered frontend
	// jobs immediately without batching.
	//
	// The commit status of the request is reported by the RPC's util_status()
	// function. The error code is the canoncial error code defined in
	// //util/task/codes.proto.
	PublishLifecycleEvent(ctx context.Context, in *PublishLifecycleEventRequest, opts ...grpc.CallOption) (*empty.Empty, error)
	// Publish build tool events belonging to the same stream to a backend job
	// using bidirectional streaming.
	PublishBuildToolEventStream(ctx context.Context, opts ...grpc.CallOption) (PublishBuildEvent_PublishBuildToolEventStreamClient, error)
}

type publishBuildEventClient struct {
	cc *grpc.ClientConn
}

func NewPublishBuildEventClient(cc *grpc.ClientConn) PublishBuildEventClient {
	return &publishBuildEventClient{cc}
}

func (c *publishBuildEventClient) PublishLifecycleEvent(ctx context.Context, in *PublishLifecycleEventRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	out := new(empty.Empty)
	err := c.cc.Invoke(ctx, "/google.devtools.build.v1.PublishBuildEvent/PublishLifecycleEvent", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *publishBuildEventClient) PublishBuildToolEventStream(ctx context.Context, opts ...grpc.CallOption) (PublishBuildEvent_PublishBuildToolEventStreamClient, error) {
	stream, err := c.cc.NewStream(ctx, &_PublishBuildEvent_serviceDesc.Streams[0], "/google.devtools.build.v1.PublishBuildEvent/PublishBuildToolEventStream", opts...)
	if err != nil {
		return nil, err
	}
	x := &publishBuildEventPublishBuildToolEventStreamClient{stream}
	return x, nil
}

type PublishBuildEvent_PublishBuildToolEventStreamClient interface {
	Send(*PublishBuildToolEventStreamRequest) error
	Recv() (*PublishBuildToolEventStreamResponse, error)
	grpc.ClientStream
}

type publishBuildEventPublishBuildToolEventStreamClient struct {
	grpc.ClientStream
}

func (x *publishBuildEventPublishBuildToolEventStreamClient) Send(m *PublishBuildToolEventStreamRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *publishBuildEventPublishBuildToolEventStreamClient) Recv() (*PublishBuildToolEventStreamResponse, error) {
	m := new(PublishBuildToolEventStreamResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// PublishBuildEventServer is the server API for PublishBuildEvent service.
type PublishBuildEventServer interface {
	// Publish a build event stating the new state of a build (typically from the
	// build queue). If the event is a BuildEnqueued event, also register the new
	// build request ID and its build type to BES.
	//
	// The backend will persist the event and deliver it to registered frontend
	// jobs immediately without batching.
	//
	// The commit status of the request is reported by the RPC's util_status()
	// function. The error code is the canoncial error code defined in
	// //util/task/codes.proto.
	PublishLifecycleEvent(context.Context, *PublishLifecycleEventRequest) (*empty.Empty, error)
	// Publish build tool events belonging to the same stream to a backend job
	// using bidirectional streaming.
	PublishBuildToolEventStream(PublishBuildEvent_PublishBuildToolEventStreamServer) error
}

func RegisterPublishBuildEventServer(s *grpc.Server, srv PublishBuildEventServer) {
	s.RegisterService(&_PublishBuildEvent_serviceDesc, srv)
}

func _PublishBuildEvent_PublishLifecycleEvent_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PublishLifecycleEventRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PublishBuildEventServer).PublishLifecycleEvent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.devtools.build.v1.PublishBuildEvent/PublishLifecycleEvent",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PublishBuildEventServer).PublishLifecycleEvent(ctx, req.(*PublishLifecycleEventRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _PublishBuildEvent_PublishBuildToolEventStream_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(PublishBuildEventServer).PublishBuildToolEventStream(&publishBuildEventPublishBuildToolEventStreamServer{stream})
}

type PublishBuildEvent_PublishBuildToolEventStreamServer interface {
	Send(*PublishBuildToolEventStreamResponse) error
	Recv() (*PublishBuildToolEventStreamRequest, error)
	grpc.ServerStream
}

type publishBuildEventPublishBuildToolEventStreamServer struct {
	grpc.ServerStream
}

func (x *publishBuildEventPublishBuildToolEventStreamServer) Send(m *PublishBuildToolEventStreamResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *publishBuildEventPublishBuildToolEventStreamServer) Recv() (*PublishBuildToolEventStreamRequest, error) {
	m := new(PublishBuildToolEventStreamRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

var _PublishBuildEvent_serviceDesc = grpc.ServiceDesc{
	ServiceName: "google.devtools.build.v1.PublishBuildEvent",
	HandlerType: (*PublishBuildEventServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "PublishLifecycleEvent",
			Handler:    _PublishBuildEvent_PublishLifecycleEvent_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "PublishBuildToolEventStream",
			Handler:       _PublishBuildEvent_PublishBuildToolEventStream_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "google/devtools/build/v1/publish_build_event.proto",
}

func init() {
	proto.RegisterFile("google/devtools/build/v1/publish_build_event.proto", fileDescriptor_publish_build_event_392a703d66bd0f43)
}

var fileDescriptor_publish_build_event_392a703d66bd0f43 = []byte{
	// 668 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xbc, 0x55, 0xcd, 0x6e, 0xd3, 0x4c,
	0x14, 0xfd, 0x26, 0x69, 0xab, 0x2f, 0x93, 0x36, 0xa5, 0x03, 0x05, 0x93, 0xb6, 0x28, 0x32, 0x08,
	0x22, 0x2a, 0xd9, 0x34, 0x95, 0x58, 0x14, 0xca, 0x4f, 0x4a, 0x16, 0x11, 0x55, 0x5a, 0xb9, 0x11,
	0x0b, 0x58, 0x58, 0x8e, 0x7d, 0x9b, 0x0e, 0x75, 0x3c, 0xc6, 0x33, 0x36, 0xea, 0x96, 0x17, 0xe8,
	0x82, 0x27, 0xe0, 0x31, 0x58, 0xf0, 0x14, 0xbc, 0x02, 0x48, 0x3c, 0x02, 0x4b, 0xe4, 0x19, 0x07,
	0x99, 0x06, 0x07, 0x91, 0x05, 0x3b, 0xcf, 0xdc, 0x7b, 0xcf, 0x9d, 0x73, 0xee, 0x8f, 0x71, 0x6b,
	0xc8, 0xd8, 0xd0, 0x07, 0xd3, 0x83, 0x44, 0x30, 0xe6, 0x73, 0x73, 0x10, 0x53, 0xdf, 0x33, 0x93,
	0x2d, 0x33, 0x8c, 0x07, 0x3e, 0xe5, 0x27, 0xb6, 0xbc, 0xb0, 0x21, 0x81, 0x40, 0x18, 0x61, 0xc4,
	0x04, 0x23, 0x9a, 0x8a, 0x31, 0xc6, 0x31, 0x86, 0x74, 0x31, 0x92, 0xad, 0xfa, 0x7a, 0x86, 0xe6,
	0x84, 0xd4, 0x74, 0x82, 0x80, 0x09, 0x47, 0x50, 0x16, 0x70, 0x15, 0x57, 0xdf, 0x2c, 0xcc, 0x95,
	0xcb, 0x31, 0x76, 0xbe, 0x91, 0x39, 0xcb, 0xd3, 0x20, 0x3e, 0x36, 0xbd, 0x38, 0x92, 0x68, 0x99,
	0x7d, 0xed, 0xa2, 0x1d, 0x46, 0xa1, 0x38, 0x53, 0x46, 0xfd, 0x43, 0x19, 0xaf, 0x1f, 0xaa, 0xf7,
	0xef, 0xd3, 0x63, 0x70, 0xcf, 0x5c, 0x1f, 0x3a, 0x29, 0xba, 0x05, 0x6f, 0x62, 0xe0, 0x82, 0x9c,
	0xe0, 0x25, 0x0e, 0x51, 0x42, 0x5d, 0xb0, 0x7d, 0x48, 0xc0, 0xd7, 0x50, 0x03, 0x35, 0x6b, 0xad,
	0x3d, 0xa3, 0x88, 0x9a, 0x31, 0x0d, 0xce, 0x38, 0x52, 0x58, 0xfb, 0x29, 0x94, 0xb5, 0xc8, 0x73,
	0x27, 0xb2, 0x8f, 0xab, 0x39, 0x76, 0x5a, 0xa9, 0x81, 0x9a, 0xd5, 0xd6, 0x66, 0x71, 0x9e, 0x83,
	0xc8, 0x83, 0x08, 0xbc, 0x76, 0x7a, 0x56, 0x39, 0xf0, 0xe0, 0xe7, 0x37, 0x79, 0x82, 0x6b, 0x5c,
	0x44, 0xe0, 0x8c, 0x6c, 0x41, 0x47, 0xc0, 0x62, 0xa1, 0x95, 0x25, 0xe0, 0xf5, 0x31, 0xe0, 0x58,
	0x0e, 0xe3, 0x59, 0x26, 0x97, 0xb5, 0xa4, 0x02, 0xfa, 0xca, 0x9f, 0x6c, 0xe3, 0xd5, 0x80, 0x09,
	0x7a, 0x4c, 0x5d, 0x69, 0xb6, 0x4f, 0xe1, 0xec, 0x2d, 0x8b, 0x3c, 0xae, 0xcd, 0x35, 0xca, 0xcd,
	0x8a, 0x75, 0x25, 0x6f, 0x7c, 0x9e, 0xd9, 0xc8, 0x06, 0xc6, 0x61, 0xc4, 0x5e, 0x83, 0x2b, 0x6c,
	0xea, 0x69, 0x0b, 0x0d, 0xd4, 0xac, 0x58, 0x95, 0xec, 0xa6, 0xeb, 0xe9, 0xdb, 0x78, 0x31, 0xaf,
	0x00, 0x21, 0xb8, 0xd6, 0x3b, 0xe8, 0x75, 0x7b, 0xfd, 0x8e, 0xf5, 0x74, 0xaf, 0xdf, 0x7d, 0xd1,
	0xb9, 0xf4, 0x1f, 0x59, 0xc6, 0xd5, 0xfc, 0x05, 0xd2, 0xcf, 0x11, 0xbe, 0x99, 0x89, 0x2a, 0xc9,
	0xf6, 0x19, 0xf3, 0x25, 0xc9, 0x23, 0xf9, 0x5e, 0x0b, 0x78, 0xc8, 0x02, 0x0e, 0xe4, 0x31, 0xae,
	0x64, 0x94, 0xa9, 0x27, 0xcb, 0x54, 0x6d, 0xe9, 0xc5, 0xf2, 0xa9, 0xe0, 0xae, 0x67, 0xfd, 0xcf,
	0xb3, 0x2f, 0x72, 0x07, 0x2f, 0xf3, 0xb4, 0x4e, 0x81, 0x0b, 0x76, 0x10, 0x8f, 0x06, 0x10, 0xc9,
	0x2a, 0x94, 0xad, 0xda, 0xf8, 0xba, 0x27, 0x6f, 0xf5, 0x8f, 0x08, 0xaf, 0x4c, 0xc8, 0xff, 0xef,
	0xf2, 0x93, 0x1d, 0x3c, 0xaf, 0x9a, 0x44, 0xd5, 0xf4, 0x56, 0x71, 0x96, 0x5c, 0x77, 0xa8, 0x10,
	0xfd, 0x5b, 0x09, 0xeb, 0x53, 0xd5, 0x54, 0x7d, 0xbf, 0x37, 0x13, 0x99, 0x76, 0x49, 0x43, 0x39,
	0x42, 0x9b, 0x05, 0x84, 0xa4, 0xdb, 0x45, 0x52, 0x8f, 0x66, 0x20, 0x25, 0x81, 0x54, 0x18, 0x79,
	0x85, 0x2f, 0x33, 0x55, 0x93, 0xfc, 0x26, 0xd2, 0xe6, 0xfe, 0x7e, 0x8e, 0x56, 0xd8, 0x44, 0x6d,
	0x0b, 0x87, 0x61, 0xbe, 0x78, 0x18, 0x5a, 0x5f, 0x4b, 0x78, 0x25, 0x2f, 0xb5, 0x82, 0x3a, 0x47,
	0x78, 0xf5, 0xb7, 0x3b, 0x82, 0xdc, 0x9f, 0x6d, 0xa9, 0xd4, 0xaf, 0x4e, 0xcc, 0x74, 0x27, 0x5d,
	0x71, 0xfa, 0xed, 0x77, 0x9f, 0xbf, 0xbc, 0x2f, 0x35, 0xf4, 0xb5, 0x74, 0x73, 0xfa, 0xbf, 0x84,
	0xf2, 0x9d, 0x6c, 0x6b, 0xef, 0xa0, 0xbb, 0xe4, 0x13, 0xc2, 0x6b, 0x53, 0x5a, 0x82, 0x3c, 0xfc,
	0xe3, 0xbb, 0xa6, 0x74, 0x52, 0x7d, 0x77, 0xc6, 0x68, 0x35, 0xd5, 0xfa, 0x86, 0x24, 0x71, 0x4d,
	0x27, 0x29, 0x09, 0xb8, 0xf8, 0xf6, 0x26, 0xba, 0x87, 0xda, 0x21, 0x5e, 0x77, 0xd9, 0xa8, 0x30,
	0x4d, 0x7b, 0xb1, 0xed, 0xb8, 0xa7, 0x10, 0x78, 0x87, 0xa9, 0x3c, 0x87, 0xe8, 0xe5, 0x6e, 0xe6,
	0x39, 0x64, 0xbe, 0x13, 0x0c, 0x0d, 0x16, 0x0d, 0xcd, 0x21, 0x04, 0x52, 0x3c, 0x53, 0x99, 0x9c,
	0x90, 0xf2, 0xc9, 0xbf, 0xcf, 0x03, 0xf9, 0xf1, 0x1d, 0xa1, 0xc1, 0x82, 0x74, 0xde, 0xfe, 0x11,
	0x00, 0x00, 0xff, 0xff, 0xf0, 0x9d, 0x50, 0x3a, 0x15, 0x07, 0x00, 0x00,
}
