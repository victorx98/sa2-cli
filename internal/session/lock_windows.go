package session

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func stateDirectory() (string, error) {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil || !filepath.IsAbs(dir) {
		return "", ErrUnsafeLock
	}
	return filepath.Join(dir, "sa2-cli-go"), nil
}

func openLock(dir, name string) (*os.File, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	sid := user.User.Sid
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, err
	}
	security, err := windows.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;;FA;;;SY)(A;;FA;;;" + sid.String() + ")")
	if err != nil {
		return nil, err
	}
	attrs := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: security,
	}
	dirPath, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return nil, err
	}
	if err := windows.CreateDirectory(dirPath, &attrs); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, err
	}
	const share = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE
	dirHandle, err := windows.CreateFile(dirPath, windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL,
		share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(dirHandle)
	if err := verifyLockHandle(dirHandle, sid, system, true); err != nil {
		return nil, err
	}

	path, err := windows.UTF16PtrFromString(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL,
		share, &attrs, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	if err := verifyLockHandle(handle, sid, system, false); err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	return os.NewFile(uintptr(handle), name), nil
}

func verifyLockHandle(handle windows.Handle, sid, system *windows.SID, directory bool) error {
	return verifyPrivateHandle(handle, sid, system, directory, true)
}
func verifyPrivateHandle(handle windows.Handle, sid, system *windows.SID, directory bool, empty bool) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		directory != (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) ||
		!directory && (empty && (info.FileSizeHigh != 0 || info.FileSizeLow != 0) || info.NumberOfLinks != 1) {
		return ErrUnsafeLock
	}
	sd, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		return ErrUnsafeLock
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !owner.Equals(sid) {
		return ErrUnsafeLock
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil || dacl.AceCount == 0 {
		return ErrUnsafeLock
	}
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return ErrUnsafeLock
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !aceSID.Equals(sid) && !aceSID.Equals(system) {
			return ErrUnsafeLock
		}
	}
	return nil
}

func tryLock(file *os.File) (bool, error) {
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, new(windows.Overlapped))
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}
