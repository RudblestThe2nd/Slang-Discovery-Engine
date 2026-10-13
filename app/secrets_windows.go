//go:build windows

package main

import (
	"encoding/base64"
	"errors"
	"syscall"
	"unsafe"
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func blob(bytes []byte) dataBlob {
	if len(bytes) == 0 {
		return dataBlob{}
	}
	return dataBlob{uint32(len(bytes)), &bytes[0]}
}

var crypt32 = syscall.NewLazyDLL("crypt32.dll")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var cryptProtect = crypt32.NewProc("CryptProtectData")
var cryptUnprotect = crypt32.NewProc("CryptUnprotectData")
var localFree = kernel32.NewProc("LocalFree")

func dpapi(in []byte, encrypt bool) ([]byte, error) {
	source := blob(in)
	var target dataBlob
	var success uintptr
	var err error
	if encrypt {
		success, _, err = cryptProtect.Call(uintptr(unsafe.Pointer(&source)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&target)))
	}
	if !encrypt {
		success, _, err = cryptUnprotect.Call(uintptr(unsafe.Pointer(&source)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&target)))
	}
	if success == 0 {
		return nil, errors.New("Windows DPAPI islemi basarisiz: " + err.Error())
	}
	defer localFree.Call(uintptr(unsafe.Pointer(target.pbData)))
	out := make([]byte, int(target.cbData))
	copy(out, unsafe.Slice(target.pbData, int(target.cbData)))
	return out, nil
}
func seal(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	b, e := dpapi([]byte(s), true)
	if e != nil {
		return "", e
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
func reveal(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	b, e := base64.StdEncoding.DecodeString(s)
	if e != nil {
		return "", e
	}
	v, e := dpapi(b, false)
	return string(v), e
}
