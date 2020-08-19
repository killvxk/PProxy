package common

import (
	"encoding/binary"
	"net"
	"syscall"
	"unsafe"
)

func IpToInt32(dest string) uint32 {
	ip := net.ParseIP(dest)
	return binary.BigEndian.Uint32([]byte(ip)[net.IPv6len-net.IPv4len:])
}

func GetInterfaceIndex(dest string) (uint32, uint32, error) {
	handle, err := syscall.LoadLibrary("iphlpapi.dll")
	defer syscall.FreeLibrary(handle)
	if err != nil {
		panic("Load DLL failed.")
	}

	GetBestInterface, err := syscall.GetProcAddress(handle, "GetBestInterface")
	if err != nil {
		panic("No function named GetBestInterface")
	}
	var index uint32 = 0
	var dst = IpToInt32(dest)
	syscall.Syscall(GetBestInterface, 2, (uintptr)(unsafe.Pointer(&dst)), (uintptr)(unsafe.Pointer(&index)), 0)
	return index, 0, err
}