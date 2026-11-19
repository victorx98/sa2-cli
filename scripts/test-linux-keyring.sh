#!/usr/bin/env bash
set -euo pipefail
umask 077

mkdir -p .cache
sa2_test_root=$(mktemp -d "$PWD/.cache/keyring.XXXXXX")
sa2_bus_pid=
cleanup() {
  if [[ -n "$sa2_bus_pid" ]]; then
    kill "$sa2_bus_pid" 2>/dev/null || true
  fi
  rm -rf -- "$sa2_test_root"
}
trap cleanup EXIT

export XDG_RUNTIME_DIR="$sa2_test_root/runtime"
export XDG_CONFIG_HOME="$sa2_test_root/config"
export XDG_DATA_HOME="$sa2_test_root/data"
export TMPDIR="$sa2_test_root/temp"
unset DISPLAY WAYLAND_DISPLAY GNOME_KEYRING_CONTROL GNOME_KEYRING_PID
mkdir -p "$XDG_RUNTIME_DIR" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$TMPDIR"

sa2_bus_pid=$(dbus-daemon --session --address="unix:path=$sa2_test_root/bus" --fork --print-pid)
export DBUS_SESSION_BUS_ADDRESS="unix:path=$sa2_test_root/bus"
printf '%s' 'disposable-sa2-test-collection' | "${SA2_TEST_KEYRING_DAEMON:-gnome-keyring-daemon}" --unlock --components=secrets >/dev/null

SA2_TEST_NATIVE_VAULT=1 go test ./internal/session -count=1 -run '^TestNativeVault$'
SA2_TEST_LOCK_LINUX_VAULT=1 go test ./internal/session -count=1 -run '^TestNativeVaultLocked$'
