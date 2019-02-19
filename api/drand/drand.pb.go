// Code generated by protoc-gen-go. DO NOT EDIT.
// source: drand.proto

package drand

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type MessageType int32

const (
	MessageType_UNKNOWN MessageType = 0
	MessageType_INIT    MessageType = 1
	MessageType_COMMIT  MessageType = 2
)

var MessageType_name = map[int32]string{
	0: "UNKNOWN",
	1: "INIT",
	2: "COMMIT",
}

var MessageType_value = map[string]int32{
	"UNKNOWN": 0,
	"INIT":    1,
	"COMMIT":  2,
}

func (x MessageType) String() string {
	return proto.EnumName(MessageType_name, int32(x))
}

func (MessageType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_1d855c36cf2c0c50, []int{0}
}

type Message struct {
	Type                 MessageType `protobuf:"varint,1,opt,name=type,proto3,enum=drand.MessageType" json:"type,omitempty"`
	SenderId             uint32      `protobuf:"varint,3,opt,name=sender_id,json=senderId,proto3" json:"sender_id,omitempty"`
	BlockHash            []byte      `protobuf:"bytes,4,opt,name=block_hash,json=blockHash,proto3" json:"block_hash,omitempty"`
	Payload              []byte      `protobuf:"bytes,5,opt,name=payload,proto3" json:"payload,omitempty"`
	Signature            []byte      `protobuf:"bytes,6,opt,name=signature,proto3" json:"signature,omitempty"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *Message) Reset()         { *m = Message{} }
func (m *Message) String() string { return proto.CompactTextString(m) }
func (*Message) ProtoMessage()    {}
func (*Message) Descriptor() ([]byte, []int) {
	return fileDescriptor_1d855c36cf2c0c50, []int{0}
}

func (m *Message) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Message.Unmarshal(m, b)
}
func (m *Message) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Message.Marshal(b, m, deterministic)
}
func (m *Message) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Message.Merge(m, src)
}
func (m *Message) XXX_Size() int {
	return xxx_messageInfo_Message.Size(m)
}
func (m *Message) XXX_DiscardUnknown() {
	xxx_messageInfo_Message.DiscardUnknown(m)
}

var xxx_messageInfo_Message proto.InternalMessageInfo

func (m *Message) GetType() MessageType {
	if m != nil {
		return m.Type
	}
	return MessageType_UNKNOWN
}

func (m *Message) GetSenderId() uint32 {
	if m != nil {
		return m.SenderId
	}
	return 0
}

func (m *Message) GetBlockHash() []byte {
	if m != nil {
		return m.BlockHash
	}
	return nil
}

func (m *Message) GetPayload() []byte {
	if m != nil {
		return m.Payload
	}
	return nil
}

func (m *Message) GetSignature() []byte {
	if m != nil {
		return m.Signature
	}
	return nil
}

func init() {
	proto.RegisterEnum("drand.MessageType", MessageType_name, MessageType_value)
	proto.RegisterType((*Message)(nil), "drand.Message")
}

func init() { proto.RegisterFile("drand.proto", fileDescriptor_1d855c36cf2c0c50) }

var fileDescriptor_1d855c36cf2c0c50 = []byte{
	// 214 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x4c, 0x8f, 0xcf, 0x4a, 0x87, 0x40,
	0x10, 0x80, 0xdb, 0xf2, 0xe7, 0x9f, 0xb1, 0x42, 0xe6, 0xb4, 0x50, 0x81, 0x74, 0x08, 0xe9, 0x20,
	0x51, 0x8f, 0xd0, 0xa5, 0x25, 0x5c, 0x41, 0x8c, 0x8e, 0xb2, 0xb6, 0x8b, 0x4a, 0xe2, 0x2e, 0xbb,
	0x76, 0xf0, 0x81, 0x7a, 0xcf, 0x60, 0xad, 0xf8, 0x1d, 0xbf, 0xef, 0x9b, 0x81, 0x19, 0x48, 0xa5,
	0x15, 0x8b, 0x2c, 0x8d, 0xd5, 0xab, 0xc6, 0x83, 0x87, 0xdb, 0x6f, 0x02, 0x51, 0xa5, 0x9c, 0x13,
	0x83, 0xc2, 0x3b, 0x08, 0xd6, 0xcd, 0x28, 0x4a, 0x72, 0x52, 0x5c, 0x3e, 0x62, 0xb9, 0x8f, 0xff,
	0xd6, 0x76, 0x33, 0xaa, 0xf1, 0x1d, 0xaf, 0x20, 0x71, 0x6a, 0x91, 0xca, 0x76, 0x93, 0xa4, 0x67,
	0x39, 0x29, 0x2e, 0x9a, 0x78, 0x17, 0x4c, 0xe2, 0x0d, 0x40, 0x3f, 0xeb, 0x8f, 0xcf, 0x6e, 0x14,
	0x6e, 0xa4, 0x41, 0x4e, 0x8a, 0xf3, 0x26, 0xf1, 0xe6, 0x45, 0xb8, 0x11, 0x29, 0x44, 0x46, 0x6c,
	0xb3, 0x16, 0x92, 0x1e, 0x7c, 0xfb, 0x43, 0xbc, 0x86, 0xc4, 0x4d, 0xc3, 0x22, 0xd6, 0x2f, 0xab,
	0x68, 0xb8, 0xef, 0xfd, 0x8b, 0xfb, 0x07, 0x48, 0x8f, 0x0e, 0xc1, 0x14, 0xa2, 0x37, 0xfe, 0xca,
	0xeb, 0x77, 0x9e, 0x9d, 0x60, 0x0c, 0x01, 0xe3, 0xac, 0xcd, 0x08, 0x02, 0x84, 0xcf, 0x75, 0x55,
	0xb1, 0x36, 0x3b, 0xed, 0x43, 0xff, 0xe7, 0xd3, 0x4f, 0x00, 0x00, 0x00, 0xff, 0xff, 0x79, 0xfa,
	0xf8, 0x57, 0xf6, 0x00, 0x00, 0x00,
}