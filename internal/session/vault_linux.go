package session

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

const (
	secretService    = "org.freedesktop.Secret.Service"
	secretCollection = "org.freedesktop.Secret.Collection"
	secretItem       = "org.freedesktop.Secret.Item"
	secretBusName    = "org.freedesktop.secrets"
	keyringTimeout   = 5 * time.Second
)

type linuxVault struct {
	bus        *dbus.Conn
	name       string
	collection dbus.ObjectPath
	session    dbus.ObjectPath
}

type dbusSecret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

func openNative(ctx context.Context) (Vault, error) {
	ctx, cancel := context.WithTimeout(ctx, keyringTimeout)
	defer cancel()
	bus, err := openSessionBus(ctx)
	if err != nil {
		return nil, err
	}
	vault := &linuxVault{bus: bus}
	if err := vault.initialize(ctx); err != nil {
		vault.Close()
		return nil, err
	}
	return vault, nil
}

func openSessionBus(ctx context.Context) (*dbus.Conn, error) {
	path := filepath.Join("/run/user", strconv.Itoa(os.Getuid()), "bus")
	if address := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); address != "" {
		if strings.Contains(address, ";") || !strings.HasPrefix(address, "unix:path=") {
			return nil, ErrUnavailable
		}
		value, options, _ := strings.Cut(strings.TrimPrefix(address, "unix:path="), ",")
		if options != "" && (!strings.HasPrefix(options, "guid=") || strings.Contains(options, ",")) {
			return nil, ErrUnavailable
		}
		decoded, err := dbus.UnescapeBusAddressValue(value)
		if err != nil || !filepath.IsAbs(decoded) || strings.ContainsRune(decoded, 0) {
			return nil, ErrUnavailable
		}
		path = decoded
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	socket := conn.(*net.UnixConn)
	raw, err := socket.SyscallConn()
	if err != nil {
		socket.Close()
		return nil, err
	}
	var peer *unix.Ucred
	var peerErr error
	if err := raw.Control(func(fd uintptr) {
		peer, peerErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil || peerErr != nil || peer == nil || int(peer.Uid) != os.Getuid() {
		socket.Close()
		return nil, ErrUnavailable
	}
	if end, ok := ctx.Deadline(); ok {
		if err := socket.SetDeadline(end); err != nil {
			socket.Close()
			return nil, err
		}
	}
	bus, err := dbus.ConnectUnix(socket, dbus.WithAuth(dbus.AuthExternal(strconv.Itoa(os.Getuid()))))
	if err != nil {
		socket.Close()
		return nil, err
	}
	if err := socket.SetDeadline(time.Time{}); err != nil {
		bus.Close()
		return nil, err
	}
	return bus, nil
}

func (v *linuxVault) initialize(ctx context.Context) error {
	service := v.bus.Object(secretBusName, "/org/freedesktop/secrets")
	if err := service.CallWithContext(ctx, secretService+".ReadAlias", 0, "default").Store(&v.collection); err != nil {
		return err
	}
	var sessionCollection dbus.ObjectPath
	if err := service.CallWithContext(ctx, secretService+".ReadAlias", 0, "session").Store(&sessionCollection); err != nil {
		return err
	}
	if v.collection == "/" || v.collection == sessionCollection {
		return ErrUnavailable
	}
	if err := v.bus.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, secretBusName).Store(&v.name); err != nil {
		return err
	}
	var uid uint32
	if err := v.bus.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetConnectionUnixUser", 0, v.name).Store(&uid); err != nil ||
		int(uid) != os.Getuid() {
		return ErrUnavailable
	}
	var locked dbus.Variant
	if err := v.bus.Object(v.name, v.collection).CallWithContext(ctx,
		"org.freedesktop.DBus.Properties.Get", 0, secretCollection, "Locked").Store(&locked); err != nil {
		return err
	}
	if value, ok := locked.Value().(bool); !ok || value {
		return ErrUnavailable
	}
	var output dbus.Variant
	// The session bus is restricted to a local peer owned by this OS account.
	return v.bus.Object(v.name, "/org/freedesktop/secrets").CallWithContext(ctx,
		secretService+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&output, &v.session)
}

func (v *linuxVault) Read(ctx context.Context, key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, keyringTimeout)
	defer cancel()
	item, err := v.find(ctx, key)
	if err != nil {
		return nil, err
	}
	var secret dbusSecret
	if err := item.CallWithContext(ctx, secretItem+".GetSecret", 0, v.session).Store(&secret); err != nil {
		return nil, err
	}
	if secret.Session != v.session || len(secret.Parameters) != 0 {
		clear(secret.Value)
		return nil, ErrUnavailable
	}
	return secret.Value, nil
}

func (v *linuxVault) Write(ctx context.Context, key string, data []byte) error {
	ctx, cancel := context.WithTimeout(ctx, keyringTimeout)
	defer cancel()
	item, err := v.find(ctx, key)
	secret := dbusSecret{Session: v.session, Value: data, ContentType: "application/octet-stream"}
	if err == nil {
		return item.CallWithContext(ctx, secretItem+".SetSecret", 0, secret).Err
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	properties := map[string]dbus.Variant{
		secretItem + ".Label":      dbus.MakeVariant("SuperAcademy CLI session"),
		secretItem + ".Attributes": dbus.MakeVariant(attributes(key)),
	}
	var path, prompt dbus.ObjectPath
	err = v.bus.Object(v.name, v.collection).CallWithContext(ctx,
		secretCollection+".CreateItem", 0, properties, secret, true).Store(&path, &prompt)
	if err != nil {
		return err
	}
	if path == "/" || prompt != "/" {
		return ErrUnavailable
	}
	return nil
}

func (v *linuxVault) Delete(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, keyringTimeout)
	defer cancel()
	item, err := v.find(ctx, key)
	if err != nil {
		return err
	}
	var prompt dbus.ObjectPath
	if err := item.CallWithContext(ctx, secretItem+".Delete", 0).Store(&prompt); err != nil {
		return err
	}
	if prompt != "/" {
		return ErrUnavailable
	}
	return nil
}

func (v *linuxVault) Close() {
	_ = v.bus.Close()
}

func attributes(key string) map[string]string {
	return map[string]string{"service": credentialService, "origin": key}
}

func (v *linuxVault) find(ctx context.Context, key string) (dbus.BusObject, error) {
	var paths []dbus.ObjectPath
	err := v.bus.Object(v.name, v.collection).CallWithContext(ctx,
		secretCollection+".SearchItems", 0, attributes(key)).Store(&paths)
	if err != nil {
		return nil, err
	}
	switch len(paths) {
	case 0:
		return nil, ErrNotFound
	case 1:
		return v.bus.Object(v.name, paths[0]), nil
	default:
		return nil, ErrUnavailable
	}
}
