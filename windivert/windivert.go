package windivert

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func init() {
	if err := InstallDriver(); err != nil {
		panic(err)
	}

	winDivert = windows.MustLoadDLL("WinDivert.dll")
	winDivertOpen = winDivert.MustFindProc("WinDivertOpen")

	var vers = map[string]struct{}{
		"2.0": struct{}{},
		"2.1": struct{}{},
		"2.2": struct{}{},
	}

	hd, err := Open("false", LayerNetwork, PriorityDefault, FlagDefault)
	if err != nil {
		panic(err)
	}
	defer hd.Close()

	major, err := hd.GetParam(VersionMajor)
	if err != nil {
		panic(err)
	}

	minor, err := hd.GetParam(VersionMinor)
	if err != nil {
		panic(err)
	}

	if err := hd.Shutdown(ShutdownBoth); err != nil {
		panic(err)
	}

	ver := strings.Join([]string{strconv.Itoa(int(major)), strconv.Itoa(int(minor))}, ".")
	if _, ok := vers[ver]; !ok {
		s := ""
		for k, _ := range vers {
			s += k
		}
		panic(fmt.Errorf("unsupported version %v of windivert, only support %v", ver, s))
	}
}

var (
	kernel32    = windows.MustLoadDLL("kernel32.dll")
	heapAlloc   = kernel32.MustFindProc("HeapAlloc")
	heapCreate  = kernel32.MustFindProc("HeapCreate")
	heapDestroy = kernel32.MustFindProc("HeapDestroy")
)

const (
	HEAP_CREATE_ENABLE_EXECUTE = 0x00040000
	HEAP_GENERATE_EXCEPTIONS   = 0x00000004
	HEAP_NO_SERIALIZE          = 0x00000001
)

func HeapAlloc(hHeap windows.Handle, dwFlags, dwBytes uint32) (unsafe.Pointer, error) {
	ret, _, errno := heapAlloc.Call(uintptr(hHeap), uintptr(dwFlags), uintptr(dwBytes))
	if ret == 0 {
		return nil, errno
	}

	return unsafe.Pointer(ret), nil
}

func HeapCreate(flOptions, dwInitialSize, dwMaximumSize uint32) (windows.Handle, error) {
	ret, _, errno := heapCreate.Call(uintptr(flOptions), uintptr(dwInitialSize), uintptr(dwMaximumSize))
	if ret == 0 {
		return windows.InvalidHandle, errno
	}

	return windows.Handle(ret), nil
}

func HeapDestroy(hHeap windows.Handle) error {
	ret, _, errno := heapDestroy.Call(uintptr(hHeap))
	if ret == 0 {
		return errno
	}

	return nil
}

var (
	winDivert     = (*windows.DLL)(nil)
	winDivertOpen = (*windows.Proc)(nil)
)

type filter struct {
	b1 uint32
	b2 uint32
	b3 [4]uint32
}

type version struct {
	magic uint64
	major uint32
	minor uint32
	bits  uint32
	_     [3]uint32
	_     [4]uint64
}

type FilterError int

func (e FilterError) Error() string {
	switch int(e) {
	case 0:
		return "WINDIVERT_ERROR_NONE"
	case 1:
		return "WINDIVERT_ERROR_NO_MEMORY"
	case 2:
		return "WINDIVERT_ERROR_TOO_DEEP"
	case 3:
		return "WINDIVERT_ERROR_TOO_LONG"
	case 4:
		return "WINDIVERT_ERROR_BAD_TOKEN"
	case 5:
		return "WINDIVERT_ERROR_BAD_TOKEN_FOR_LAYER"
	case 6:
		return "WINDIVERT_ERROR_UNEXPECTED_TOKEN"
	case 7:
		return "WINDIVERT_ERROR_INDEX_OOB"
	case 8:
		return "WINDIVERT_ERROR_OUTPUT_TOO_SHORT"
	case 9:
		return "WINDIVERT_ERROR_BAD_OBJECT"
	case 10:
		return "WINDIVERT_ERROR_ASSERTION_FAILED"
	default:
		return ""
	}
}

const (
	WINDIVERT_MIN_POOL_SIZE = 12288
	WINDIVERT_MAX_POOL_SIZE = 131072
	WINDIVERT_FILTER_MAXLEN = 256
)

func CompileFilter(filter string, pool windows.Handle, layer Layer, object *filter) (uint, error) {
	return 0, nil
}

func AnalyzeFilter(layer Layer, object *filter, objLen uint) uint64 {
	return 0
}

func IoControlEx(h windows.Handle, code CtlCode, ioctl unsafe.Pointer, buf *byte, bufLen uint32, overlapped *windows.Overlapped) (iolen uint32, err error) {
	err = windows.DeviceIoControl(h, uint32(code), (*byte)(ioctl), uint32(unsafe.Sizeof(IoCtl{})), buf, bufLen, &iolen, overlapped)
	if err != windows.ERROR_IO_PENDING {
		return
	}

	err = windows.GetOverlappedResult(h, overlapped, &iolen, true)

	return
}

func IoControl(h windows.Handle, code CtlCode, ioctl unsafe.Pointer, buf *byte, bufLen uint32) (iolen uint32, err error) {
	event, _ := windows.CreateEvent(nil, 0, 0, nil)

	overlapped := windows.Overlapped{
		HEvent: event,
	}

	iolen, err = IoControlEx(h, code, ioctl, buf, bufLen, &overlapped)

	windows.CloseHandle(event)
	return
}

func FlagExclude(flags, flag1, flag2 uint64) bool {
	return flags&(flag1|flag2) != (flag1 | flag2)
}

func ValidateFlag(flags uint64) bool {
	FlagAll := FlagSniff | FlagDrop | FlagRecvOnly | FlagSendOnly | FlagNoInstall | FlagFragments

	return (flags & ^uint64(FlagAll) == 0) && FlagExclude(flags, FlagSniff, FlagDrop) && FlagExclude(flags, FlagRecvOnly, FlagSendOnly)
}

type Handle struct {
	sync.Mutex
	windows.Handle
	rOverlapped windows.Overlapped
	wOverlapped windows.Overlapped
}

func Open(filter string, layer Layer, priority int16, flags uint64) (*Handle, error) {
	if priority < PriorityLowest || priority > PriorityHighest {
		return nil, fmt.Errorf("Priority %v is not Correct, Max: %v, Min: %v", priority, PriorityHighest, PriorityLowest)
	}

	filterPtr, err := windows.BytePtrFromString(filter)
	if err != nil {
		return nil, err
	}

	runtime.LockOSThread()
	hd, _, err := winDivertOpen.Call(uintptr(unsafe.Pointer(filterPtr)), uintptr(layer), uintptr(priority), uintptr(flags))
	runtime.UnlockOSThread()

	if windows.Handle(hd) == windows.InvalidHandle {
		return nil, Error(err.(windows.Errno))
	}

	rEvent, _ := windows.CreateEvent(nil, 0, 0, nil)
	wEvent, _ := windows.CreateEvent(nil, 0, 0, nil)

	return &Handle{
		Mutex:  sync.Mutex{},
		Handle: windows.Handle(hd),
		rOverlapped: windows.Overlapped{
			HEvent: rEvent,
		},
		wOverlapped: windows.Overlapped{
			HEvent: wEvent,
		},
	}, nil
}

func (h *Handle) Recv(buffer []byte, address *Address) (uint, error) {
	addrLen := uint(unsafe.Sizeof(Address{}))
	recv := recv{
		Addr:       uint64(uintptr(unsafe.Pointer(address))),
		AddrLenPtr: uint64(uintptr(unsafe.Pointer(&addrLen))),
	}

	iolen, err := IoControlEx(h.Handle, IoCtlRecv, unsafe.Pointer(&recv), &buffer[0], uint32(len(buffer)), &h.rOverlapped)
	if err != nil {
		return uint(iolen), Error(err.(syscall.Errno))
	}

	return uint(iolen), nil
}

func (h *Handle) RecvEx(buffer []byte, address []Address, overlapped *windows.Overlapped) (uint, uint, error) {
	addrLen := uint(len(address)) * uint(unsafe.Sizeof(Address{}))
	recv := recv{
		Addr:       uint64(uintptr(unsafe.Pointer(&address[0]))),
		AddrLenPtr: uint64(uintptr(unsafe.Pointer(&addrLen))),
	}

	iolen, err := IoControlEx(h.Handle, IoCtlRecv, unsafe.Pointer(&recv), &buffer[0], uint32(len(buffer)), &h.rOverlapped)
	if err != nil {
		return uint(iolen), addrLen / uint(unsafe.Sizeof(Address{})), Error(err.(syscall.Errno))
	}

	return uint(iolen), addrLen / uint(unsafe.Sizeof(Address{})), nil
}

func (h *Handle) Send(buffer []byte, address *Address) (uint, error) {
	send := send{
		Addr:    uint64(uintptr(unsafe.Pointer(address))),
		AddrLen: uint64(unsafe.Sizeof(Address{})),
	}

	iolen, err := IoControlEx(h.Handle, IoCtlSend, unsafe.Pointer(&send), &buffer[0], uint32(len(buffer)), &h.wOverlapped)
	if err != nil {
		return uint(iolen), Error(err.(syscall.Errno))
	}

	return uint(iolen), nil
}

func (h *Handle) SendEx(buffer []byte, address []Address, overlapped *windows.Overlapped) (uint, error) {
	send := send{
		Addr:    uint64(uintptr(unsafe.Pointer(&address[0]))),
		AddrLen: uint64(unsafe.Sizeof(Address{})) * uint64(len(address)),
	}

	iolen, err := IoControlEx(h.Handle, IoCtlSend, unsafe.Pointer(&send), &buffer[0], uint32(len(buffer)), &h.wOverlapped)
	if err != nil {
		return uint(iolen), Error(err.(syscall.Errno))
	}

	return uint(iolen), nil
}

func (h *Handle) Shutdown(how Shutdown) error {
	shutdown := shutdown{
		How: uint32(how),
	}

	_, err := IoControl(h.Handle, IoCtlShutdown, unsafe.Pointer(&shutdown), nil, 0)
	if err != nil {
		return Error(err.(syscall.Errno))
	}

	return nil
}

func (h *Handle) Close() error {
	windows.CloseHandle(h.rOverlapped.HEvent)
	windows.CloseHandle(h.wOverlapped.HEvent)

	err := windows.CloseHandle(h.Handle)
	if err != nil {
		return Error(err.(syscall.Errno))
	}

	return nil
}

func (h *Handle) GetParam(p Param) (uint64, error) {
	getParam := getParam{
		Param: uint32(p),
		Value: 0,
	}

	_, err := IoControl(h.Handle, IoCtlGetParam, unsafe.Pointer(&getParam), (*byte)(unsafe.Pointer(&getParam.Value)), uint32(unsafe.Sizeof(getParam.Value)))
	if err != nil {
		return getParam.Value, Error(err.(syscall.Errno))
	}

	return getParam.Value, nil
}

func (h *Handle) SetParam(p Param, v uint64) error {
	switch p {
	case QueueLength:
		if v < QueueLengthMin || v > QueueLengthMax {
			return fmt.Errorf("Queue length %v is not correct, Max: %v, Min: %v", v, QueueLengthMax, QueueLengthMin)
		}
	case QueueTime:
		if v < QueueTimeMin || v > QueueTimeMax {
			return fmt.Errorf("Queue time %v is not correct, Max: %v, Min: %v", v, QueueTimeMax, QueueTimeMin)
		}
	case QueueSize:
		if v < QueueSizeMin || v > QueueSizeMax {
			return fmt.Errorf("Queue size %v is not correct, Max: %v, Min: %v", v, QueueSizeMax, QueueSizeMin)
		}
	default:
		return errors.New("VersionMajor and VersionMinor only can be used in function GetParam")
	}

	setParam := setParam{
		Value: v,
		Param: uint32(p),
	}

	_, err := IoControl(h.Handle, IoCtlSetParam, unsafe.Pointer(&setParam), nil, 0)
	if err != nil {
		return Error(err.(syscall.Errno))
	}

	return nil
}
