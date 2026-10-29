//go:build !windows

package main

// Stand-in for tests on Linux: what would happen on Windows is appended to $LEOLINK_FAKE/actions.log,
// the "signed-in user", idle time etc. are read from files in $LEOLINK_FAKE.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type fakeSys struct{}

func newSys() sysAPI { return fakeSys{} }

func fakeDir() string {
	d := os.Getenv("LEOLINK_FAKE")
	if d == "" {
		d = filepath.Join(os.TempDir(), "leolink-fake")
	}
	_ = os.MkdirAll(d, 0o755)
	return d
}

func fakeRead(name, def string) string {
	b, err := os.ReadFile(filepath.Join(fakeDir(), name))
	if err != nil {
		return def
	}
	return strings.TrimSpace(string(b))
}

func fakeWrite(name, v string) { _ = os.WriteFile(filepath.Join(fakeDir(), name), []byte(v), 0o644) }

func fakeAction(format string, a ...any) error {
	f, err := os.OpenFile(filepath.Join(fakeDir(), "actions.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", runMode, fmt.Sprintf(format, a...))
	return nil
}

func (fakeSys) Hostname() string     { return fakeRead("hostname", "TEST-PC") }
func (fakeSys) LoggedOnUser() string { return fakeRead("user", "") }
func (fakeSys) BootTime() time.Time  { return time.Date(2026, 10, 29, 7, 0, 0, 0, time.Local) }
func (fakeSys) Disks() []Disk        { return []Disk{{"C", 120.5, 476.3}, {"D", 800, 1863}} }

func (fakeSys) HasPassword() bool { return fakeRead("password", "") != "" }
func (fakeSys) StorePassword(p string) error {
	fakeWrite("password", p)
	return fakeAction("store-password %t", p != "")
}
func (fakeSys) VerifyPassword(u, d, p string) error {
	if p != fakeRead("realpassword", "heslo") {
		return errors.New(T("neznámé uživatelské jméno nebo chybné heslo"))
	}
	return nil
}
func (fakeSys) SetAutologon(u, d string) error {
	fakeWrite("autologon", u)
	return fakeAction("autologon-set %s\\%s", d, u)
}
func (fakeSys) ClearAutologon() error {
	_ = os.Remove(filepath.Join(fakeDir(), "autologon"))
	return fakeAction("autologon-clear")
}
func (fakeSys) AutologonActive() bool { return fakeRead("autologon", "") != "" }
func (fakeSys) Power(a string) error  { return fakeAction("power %s", a) }

func (fakeSys) CPU() float64 { return 12.5 }
func (fakeSys) RAM() float64 { return 41.0 }
func (fakeSys) SleepTimeout() int {
	n, _ := strconv.Atoi(fakeRead("sleeptimeout", "1800"))
	return n
}
func (fakeSys) SetSleepTimeout(sec int) error {
	fakeWrite("sleeptimeout", strconv.Itoa(sec))
	return fakeAction("sleep-timeout %d", sec)
}
func (fakeSys) ScreenTimeout() int {
	n, _ := strconv.Atoi(fakeRead("screentimeout", "600"))
	return n
}
func (fakeSys) SetScreenTimeout(sec int) error {
	fakeWrite("screentimeout", strconv.Itoa(sec))
	return fakeAction("screen-timeout %d", sec)
}
func (fakeSys) KeepAwake(on bool) { _ = fakeAction("keep-awake %t", on) }
func (fakeSys) Mute() error       { return fakeAction("mute") }
func (fakeSys) Notify(t string)   { _ = fakeAction("notify %s", t) }

func (fakeSys) Lock() error           { return fakeAction("lock") }
func (fakeSys) Monitor(on bool) error { return fakeAction("monitor %t", on) }
func (fakeSys) IdleSeconds() int {
	n, _ := strconv.Atoi(fakeRead("idle", "0"))
	return n
}
func (fakeSys) ProcRunning(exe string) bool {
	return strings.Contains(","+strings.ToLower(fakeRead("procs", ""))+",", ","+strings.ToLower(exe)+",")
}
func (fakeSys) StartApp(a App) error { return fakeAction("start %s min=%t", a.Path, a.Min) }
func (fakeSys) DropboxStop() error   { return fakeAction("dropbox-stop") }
func (fakeSys) DropboxStart() error  { return fakeAction("dropbox-start") }
func (fakeSys) DropboxState() string { return fakeRead("dropbox", "běží") }

// protect / unprotect: DPAPI on Windows; here a fixed-key AES-GCM, only so that tests round-trip.
func fakeKey() cipher.AEAD {
	k := sha256.Sum256([]byte("leolink-test"))
	b, _ := aes.NewCipher(k[:])
	g, _ := cipher.NewGCM(b)
	return g
}

func protect(p []byte) ([]byte, error) {
	g := fakeKey()
	n := make([]byte, g.NonceSize())
	return g.Seal(n, n, p, nil), nil
}

func unprotect(b []byte) ([]byte, error) {
	g := fakeKey()
	if len(b) < g.NonceSize() {
		return nil, errors.New("short")
	}
	return g.Open(nil, b[:g.NonceSize()], b[g.NonceSize():], nil)
}

func isElevated() bool { return true }

func singleInstance(string) bool { return true }

func osLangCzech() bool { return envLangCzech() }
