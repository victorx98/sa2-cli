package session

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func privatePlanFile(name string, create bool) (*os.File, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	sid := user.User.Sid
	desc, err := windows.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;;FA;;;" + sid.String() + ")")
	if err != nil {
		return nil, err
	}
	attrs := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: desc}
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	mode := uint32(windows.OPEN_EXISTING)
	permissions := uint32(windows.GENERIC_READ | windows.READ_CONTROL)
	if create {
		mode = windows.CREATE_NEW
		permissions |= windows.GENERIC_WRITE
	}
	h, err := windows.CreateFile(path, permissions, windows.FILE_SHARE_READ, &attrs, mode, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	// verifyLockHandle also checks owner, protected ACL, link count and reparse points.
	// Plan files have content so only a new file must be empty.
	if err = verifyPrivateHandle(h, sid, sid, false, create); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return os.NewFile(uintptr(h), name), nil
}
