// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.30.0
// 	protoc        v3.21.12
// source: userdb.proto

package userdb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type ProtoDB struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Users map[string]*Password `protobuf:"bytes,1,rep,name=users,proto3" json:"users,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

func (x *ProtoDB) Reset() {
	*x = ProtoDB{}
	if protoimpl.UnsafeEnabled {
		mi := &file_userdb_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ProtoDB) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ProtoDB) ProtoMessage() {}

func (x *ProtoDB) ProtoReflect() protoreflect.Message {
	mi := &file_userdb_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ProtoDB.ProtoReflect.Descriptor instead.
func (*ProtoDB) Descriptor() ([]byte, []int) {
	return file_userdb_proto_rawDescGZIP(), []int{0}
}

func (x *ProtoDB) GetUsers() map[string]*Password {
	if x != nil {
		return x.Users
	}
	return nil
}

type Password struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Scheme:
	//
	//	*Password_Scrypt
	//	*Password_Plain
	//	*Password_Denied
	Scheme isPassword_Scheme `protobuf_oneof:"scheme"`
}

func (x *Password) Reset() {
	*x = Password{}
	if protoimpl.UnsafeEnabled {
		mi := &file_userdb_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Password) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Password) ProtoMessage() {}

func (x *Password) ProtoReflect() protoreflect.Message {
	mi := &file_userdb_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Password.ProtoReflect.Descriptor instead.
func (*Password) Descriptor() ([]byte, []int) {
	return file_userdb_proto_rawDescGZIP(), []int{1}
}

func (m *Password) GetScheme() isPassword_Scheme {
	if m != nil {
		return m.Scheme
	}
	return nil
}

func (x *Password) GetScrypt() *Scrypt {
	if x, ok := x.GetScheme().(*Password_Scrypt); ok {
		return x.Scrypt
	}
	return nil
}

func (x *Password) GetPlain() *Plain {
	if x, ok := x.GetScheme().(*Password_Plain); ok {
		return x.Plain
	}
	return nil
}

func (x *Password) GetDenied() *Denied {
	if x, ok := x.GetScheme().(*Password_Denied); ok {
		return x.Denied
	}
	return nil
}

type isPassword_Scheme interface {
	isPassword_Scheme()
}

type Password_Scrypt struct {
	Scrypt *Scrypt `protobuf:"bytes,2,opt,name=scrypt,proto3,oneof"`
}

type Password_Plain struct {
	Plain *Plain `protobuf:"bytes,3,opt,name=plain,proto3,oneof"`
}

type Password_Denied struct {
	Denied *Denied `protobuf:"bytes,4,opt,name=denied,proto3,oneof"`
}

func (*Password_Scrypt) isPassword_Scheme() {}

func (*Password_Plain) isPassword_Scheme() {}

func (*Password_Denied) isPassword_Scheme() {}

type Scrypt struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	LogN      uint64 `protobuf:"varint,1,opt,name=logN,proto3" json:"logN,omitempty"`
	R         int32  `protobuf:"varint,2,opt,name=r,proto3" json:"r,omitempty"`
	P         int32  `protobuf:"varint,3,opt,name=p,proto3" json:"p,omitempty"`
	KeyLen    int32  `protobuf:"varint,4,opt,name=keyLen,proto3" json:"keyLen,omitempty"`
	Salt      []byte `protobuf:"bytes,5,opt,name=salt,proto3" json:"salt,omitempty"`
	Encrypted []byte `protobuf:"bytes,6,opt,name=encrypted,proto3" json:"encrypted,omitempty"`
}

func (x *Scrypt) Reset() {
	*x = Scrypt{}
	if protoimpl.UnsafeEnabled {
		mi := &file_userdb_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Scrypt) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Scrypt) ProtoMessage() {}

func (x *Scrypt) ProtoReflect() protoreflect.Message {
	mi := &file_userdb_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Scrypt.ProtoReflect.Descriptor instead.
func (*Scrypt) Descriptor() ([]byte, []int) {
	return file_userdb_proto_rawDescGZIP(), []int{2}
}

func (x *Scrypt) GetLogN() uint64 {
	if x != nil {
		return x.LogN
	}
	return 0
}

func (x *Scrypt) GetR() int32 {
	if x != nil {
		return x.R
	}
	return 0
}

func (x *Scrypt) GetP() int32 {
	if x != nil {
		return x.P
	}
	return 0
}

func (x *Scrypt) GetKeyLen() int32 {
	if x != nil {
		return x.KeyLen
	}
	return 0
}

func (x *Scrypt) GetSalt() []byte {
	if x != nil {
		return x.Salt
	}
	return nil
}

func (x *Scrypt) GetEncrypted() []byte {
	if x != nil {
		return x.Encrypted
	}
	return nil
}

type Plain struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Password []byte `protobuf:"bytes,1,opt,name=password,proto3" json:"password,omitempty"`
}

func (x *Plain) Reset() {
	*x = Plain{}
	if protoimpl.UnsafeEnabled {
		mi := &file_userdb_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Plain) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Plain) ProtoMessage() {}

func (x *Plain) ProtoReflect() protoreflect.Message {
	mi := &file_userdb_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Plain.ProtoReflect.Descriptor instead.
func (*Plain) Descriptor() ([]byte, []int) {
	return file_userdb_proto_rawDescGZIP(), []int{3}
}

func (x *Plain) GetPassword() []byte {
	if x != nil {
		return x.Password
	}
	return nil
}

type Denied struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *Denied) Reset() {
	*x = Denied{}
	if protoimpl.UnsafeEnabled {
		mi := &file_userdb_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Denied) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Denied) ProtoMessage() {}

func (x *Denied) ProtoReflect() protoreflect.Message {
	mi := &file_userdb_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Denied.ProtoReflect.Descriptor instead.
func (*Denied) Descriptor() ([]byte, []int) {
	return file_userdb_proto_rawDescGZIP(), []int{4}
}

var File_userdb_proto protoreflect.FileDescriptor

var file_userdb_proto_rawDesc = []byte{
	0x0a, 0x0c, 0x75, 0x73, 0x65, 0x72, 0x64, 0x62, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x06,
	0x75, 0x73, 0x65, 0x72, 0x64, 0x62, 0x22, 0x87, 0x01, 0x0a, 0x07, 0x50, 0x72, 0x6f, 0x74, 0x6f,
	0x44, 0x42, 0x12, 0x30, 0x0a, 0x05, 0x75, 0x73, 0x65, 0x72, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x1a, 0x2e, 0x75, 0x73, 0x65, 0x72, 0x64, 0x62, 0x2e, 0x50, 0x72, 0x6f, 0x74, 0x6f,
	0x44, 0x42, 0x2e, 0x55, 0x73, 0x65, 0x72, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x05, 0x75,
	0x73, 0x65, 0x72, 0x73, 0x1a, 0x4a, 0x0a, 0x0a, 0x55, 0x73, 0x65, 0x72, 0x73, 0x45, 0x6e, 0x74,
	0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x03, 0x6b, 0x65, 0x79, 0x12, 0x26, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x10, 0x2e, 0x75, 0x73, 0x65, 0x72, 0x64, 0x62, 0x2e, 0x50, 0x61, 0x73,
	0x73, 0x77, 0x6f, 0x72, 0x64, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01,
	0x22, 0x8f, 0x01, 0x0a, 0x08, 0x50, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x12, 0x28, 0x0a,
	0x06, 0x73, 0x63, 0x72, 0x79, 0x70, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0e, 0x2e,
	0x75, 0x73, 0x65, 0x72, 0x64, 0x62, 0x2e, 0x53, 0x63, 0x72, 0x79, 0x70, 0x74, 0x48, 0x00, 0x52,
	0x06, 0x73, 0x63, 0x72, 0x79, 0x70, 0x74, 0x12, 0x25, 0x0a, 0x05, 0x70, 0x6c, 0x61, 0x69, 0x6e,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0d, 0x2e, 0x75, 0x73, 0x65, 0x72, 0x64, 0x62, 0x2e,
	0x50, 0x6c, 0x61, 0x69, 0x6e, 0x48, 0x00, 0x52, 0x05, 0x70, 0x6c, 0x61, 0x69, 0x6e, 0x12, 0x28,
	0x0a, 0x06, 0x64, 0x65, 0x6e, 0x69, 0x65, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0e,
	0x2e, 0x75, 0x73, 0x65, 0x72, 0x64, 0x62, 0x2e, 0x44, 0x65, 0x6e, 0x69, 0x65, 0x64, 0x48, 0x00,
	0x52, 0x06, 0x64, 0x65, 0x6e, 0x69, 0x65, 0x64, 0x42, 0x08, 0x0a, 0x06, 0x73, 0x63, 0x68, 0x65,
	0x6d, 0x65, 0x22, 0x82, 0x01, 0x0a, 0x06, 0x53, 0x63, 0x72, 0x79, 0x70, 0x74, 0x12, 0x12, 0x0a,
	0x04, 0x6c, 0x6f, 0x67, 0x4e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x04, 0x52, 0x04, 0x6c, 0x6f, 0x67,
	0x4e, 0x12, 0x0c, 0x0a, 0x01, 0x72, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x52, 0x01, 0x72, 0x12,
	0x0c, 0x0a, 0x01, 0x70, 0x18, 0x03, 0x20, 0x01, 0x28, 0x05, 0x52, 0x01, 0x70, 0x12, 0x16, 0x0a,
	0x06, 0x6b, 0x65, 0x79, 0x4c, 0x65, 0x6e, 0x18, 0x04, 0x20, 0x01, 0x28, 0x05, 0x52, 0x06, 0x6b,
	0x65, 0x79, 0x4c, 0x65, 0x6e, 0x12, 0x12, 0x0a, 0x04, 0x73, 0x61, 0x6c, 0x74, 0x18, 0x05, 0x20,
	0x01, 0x28, 0x0c, 0x52, 0x04, 0x73, 0x61, 0x6c, 0x74, 0x12, 0x1c, 0x0a, 0x09, 0x65, 0x6e, 0x63,
	0x72, 0x79, 0x70, 0x74, 0x65, 0x64, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x09, 0x65, 0x6e,
	0x63, 0x72, 0x79, 0x70, 0x74, 0x65, 0x64, 0x22, 0x23, 0x0a, 0x05, 0x50, 0x6c, 0x61, 0x69, 0x6e,
	0x12, 0x1a, 0x0a, 0x08, 0x70, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0c, 0x52, 0x08, 0x70, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x22, 0x08, 0x0a, 0x06,
	0x44, 0x65, 0x6e, 0x69, 0x65, 0x64, 0x42, 0x2c, 0x5a, 0x2a, 0x62, 0x6c, 0x69, 0x74, 0x69, 0x72,
	0x69, 0x2e, 0x63, 0x6f, 0x6d, 0x2e, 0x61, 0x72, 0x2f, 0x67, 0x6f, 0x2f, 0x63, 0x68, 0x61, 0x73,
	0x71, 0x75, 0x69, 0x64, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x75, 0x73,
	0x65, 0x72, 0x64, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_userdb_proto_rawDescOnce sync.Once
	file_userdb_proto_rawDescData = file_userdb_proto_rawDesc
)

func file_userdb_proto_rawDescGZIP() []byte {
	file_userdb_proto_rawDescOnce.Do(func() {
		file_userdb_proto_rawDescData = protoimpl.X.CompressGZIP(file_userdb_proto_rawDescData)
	})
	return file_userdb_proto_rawDescData
}

var file_userdb_proto_msgTypes = make([]protoimpl.MessageInfo, 6)
var file_userdb_proto_goTypes = []interface{}{
	(*ProtoDB)(nil),  // 0: userdb.ProtoDB
	(*Password)(nil), // 1: userdb.Password
	(*Scrypt)(nil),   // 2: userdb.Scrypt
	(*Plain)(nil),    // 3: userdb.Plain
	(*Denied)(nil),   // 4: userdb.Denied
	nil,              // 5: userdb.ProtoDB.UsersEntry
}
var file_userdb_proto_depIdxs = []int32{
	5, // 0: userdb.ProtoDB.users:type_name -> userdb.ProtoDB.UsersEntry
	2, // 1: userdb.Password.scrypt:type_name -> userdb.Scrypt
	3, // 2: userdb.Password.plain:type_name -> userdb.Plain
	4, // 3: userdb.Password.denied:type_name -> userdb.Denied
	1, // 4: userdb.ProtoDB.UsersEntry.value:type_name -> userdb.Password
	5, // [5:5] is the sub-list for method output_type
	5, // [5:5] is the sub-list for method input_type
	5, // [5:5] is the sub-list for extension type_name
	5, // [5:5] is the sub-list for extension extendee
	0, // [0:5] is the sub-list for field type_name
}

func init() { file_userdb_proto_init() }
func file_userdb_proto_init() {
	if File_userdb_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_userdb_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ProtoDB); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_userdb_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Password); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_userdb_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Scrypt); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_userdb_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Plain); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_userdb_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Denied); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	file_userdb_proto_msgTypes[1].OneofWrappers = []interface{}{
		(*Password_Scrypt)(nil),
		(*Password_Plain)(nil),
		(*Password_Denied)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_userdb_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   6,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_userdb_proto_goTypes,
		DependencyIndexes: file_userdb_proto_depIdxs,
		MessageInfos:      file_userdb_proto_msgTypes,
	}.Build()
	File_userdb_proto = out.File
	file_userdb_proto_rawDesc = nil
	file_userdb_proto_goTypes = nil
	file_userdb_proto_depIdxs = nil
}
