package session

/*
#cgo CFLAGS: -Wno-deprecated-declarations
#cgo LDFLAGS: -framework CoreFoundation -framework Security
#include <Security/Security.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>

static OSStatus open_keychain(SecKeychainRef *keychain) {
	OSStatus status = SecKeychainSetUserInteractionAllowed(false);
	if (status != errSecSuccess) {
		return status;
	}
	status = SecKeychainCopyDefault(keychain);
	if (status != errSecSuccess) {
		return status;
	}

	SecKeychainStatus flags = 0;
	status = SecKeychainGetStatus(*keychain, &flags);
	if (status == errSecSuccess && !(flags & kSecUnlockStateStatus)) {
		status = errSecInteractionNotAllowed;
	}
	if (status != errSecSuccess) {
		CFRelease(*keychain);
		*keychain = NULL;
	}
	return status;
}

static OSStatus read_item(SecKeychainRef keychain, const char *service, const char *account,
		UInt32 *length, void **data) {
	return SecKeychainFindGenericPassword(keychain, (UInt32)strlen(service), service,
		(UInt32)strlen(account), account, length, data, NULL);
}

static OSStatus write_item(SecKeychainRef keychain, const char *service, const char *account,
		UInt32 length, const void *data) {
	SecKeychainItemRef item = NULL;
	OSStatus status = SecKeychainFindGenericPassword(keychain, (UInt32)strlen(service), service,
		(UInt32)strlen(account), account, NULL, NULL, &item);
	if (status == errSecItemNotFound) {
		return SecKeychainAddGenericPassword(keychain, (UInt32)strlen(service), service,
			(UInt32)strlen(account), account, length, data, NULL);
	}
	if (status != errSecSuccess) {
		return status;
	}
	status = SecKeychainItemModifyAttributesAndData(item, NULL, length, data);
	CFRelease(item);
	return status;
}

static OSStatus delete_item(SecKeychainRef keychain, const char *service, const char *account) {
	SecKeychainItemRef item = NULL;
	OSStatus status = SecKeychainFindGenericPassword(keychain, (UInt32)strlen(service), service,
		(UInt32)strlen(account), account, NULL, NULL, &item);
	if (status != errSecSuccess) {
		return status;
	}
	status = SecKeychainItemDelete(item);
	CFRelease(item);
	return status;
}

static void close_keychain(SecKeychainRef keychain) {
	CFRelease(keychain);
}
*/
import "C"

import (
	"context"
	"unsafe"
)

type darwinVault struct {
	keychain C.SecKeychainRef
}

func openNative(context.Context) (Vault, error) {
	var keychain C.SecKeychainRef
	if C.open_keychain(&keychain) != C.errSecSuccess {
		return nil, ErrUnavailable
	}
	return darwinVault{keychain: keychain}, nil
}

func (v darwinVault) Read(_ context.Context, key string) ([]byte, error) {
	service, account, release := keychainNames(key)
	defer release()
	var length C.UInt32
	var data unsafe.Pointer
	status := C.read_item(v.keychain, service, account, &length, &data)
	if err := keychainError(status); err != nil {
		return nil, err
	}
	defer C.SecKeychainItemFreeContent(nil, data)
	if length == 0 || length > 1<<20 || data == nil {
		return nil, ErrUnavailable
	}
	defer clear(unsafe.Slice((*byte)(data), int(length)))
	return C.GoBytes(data, C.int(length)), nil
}

func (v darwinVault) Write(_ context.Context, key string, value []byte) error {
	if len(value) == 0 || len(value) > 1<<20 {
		return ErrUnavailable
	}
	service, account, release := keychainNames(key)
	defer release()
	return keychainError(C.write_item(v.keychain, service, account, C.UInt32(len(value)), unsafe.Pointer(unsafe.SliceData(value))))
}

func (v darwinVault) Delete(_ context.Context, key string) error {
	service, account, release := keychainNames(key)
	defer release()
	return keychainError(C.delete_item(v.keychain, service, account))
}

func (v darwinVault) Close() {
	C.close_keychain(v.keychain)
}

func keychainNames(key string) (*C.char, *C.char, func()) {
	service := C.CString(credentialService)
	account := C.CString(key)
	return service, account, func() {
		C.free(unsafe.Pointer(account))
		C.free(unsafe.Pointer(service))
	}
}

func keychainError(status C.OSStatus) error {
	switch status {
	case C.errSecSuccess:
		return nil
	case C.errSecItemNotFound:
		return ErrNotFound
	default:
		return ErrUnavailable
	}
}
