package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestMissingOrInvalidSessionBus(t *testing.T) {
	for _, address := range []string{"unix:path=" + filepath.Join(t.TempDir(), "nonexistent"), "tcp:host=127.0.0.1,port=1"} {
		t.Setenv("DBUS_SESSION_BUS_ADDRESS", address)
		vault, err := openNative(t.Context())
		if err == nil {
			vault.Close()
			t.Fatal("unexpected credential service connection")
		}
	}
}

func TestNativeVaultLocked(t *testing.T) {
	if os.Getenv("SA2_TEST_LOCK_LINUX_VAULT") != "1" {
		t.Skip("requires a disposable Linux credential store")
	}
	vault, err := openNative(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	native := vault.(*linuxVault)
	var locked []dbus.ObjectPath
	var prompt dbus.ObjectPath
	service := native.bus.Object(native.name, "/org/freedesktop/secrets")
	err = service.CallWithContext(t.Context(), secretService+".Lock", 0, []dbus.ObjectPath{native.collection}).Store(&locked, &prompt)
	if err != nil || prompt != "/" || len(locked) != 1 {
		t.Fatal("could not lock the test collection", err)
	}
	if next, err := openNative(t.Context()); err == nil {
		next.Close()
		t.Fatal("locked collection was accepted")
	}
}
