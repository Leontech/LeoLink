//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type winSys struct{}

func newSys() sysAPI { return &winSys{} }

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")
	advapi32 = windows.NewLazySystemDLL("advapi32.dll")
	wtsapi32 = windows.NewLazySystemDLL("wtsapi32.dll")
	powrprof = windows.NewLazySystemDLL("powrprof.dll")

	procGetTickCount64      = kernel32.NewProc("GetTickCount64")
	procGetTickCount        = kernel32.NewProc("GetTickCount")
	procGetSystemTimes      = kernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatus  = kernel32.NewProc("GlobalMemoryStatusEx")
	procSetThreadExecState  = kernel32.NewProc("SetThreadExecutionState")
	procGetLastInputInfo    = user32.NewProc("GetLastInputInfo")
	procLockWorkStation     = user32.NewProc("LockWorkStation")
	procPostMessage         = user32.NewProc("PostMessageW")
	procMouseEvent          = user32.NewProc("mouse_event")
	procKeybdEvent          = user32.NewProc("keybd_event")
	procMessageBox          = user32.NewProc("MessageBoxW")
	procWTSQuerySessionInfo = wtsapi32.NewProc("WTSQuerySessionInformationW")
	procSetSuspendState     = powrprof.NewProc("SetSuspendState")
	procPowerGetActive      = powrprof.NewProc("PowerGetActiveScheme")
	procPowerReadAC         = powrprof.NewProc("PowerReadACValueIndex")
	procPowerWriteAC        = powrprof.NewProc("PowerWriteACValueIndex")
	procPowerWriteDC        = powrprof.NewProc("PowerWriteDCValueIndex")
	procPowerSetActive      = powrprof.NewProc("PowerSetActiveScheme")
	procLsaOpenPolicy       = advapi32.NewProc("LsaOpenPolicy")
	procLsaStorePrivate     = advapi32.NewProc("LsaStorePrivateData")
	procLsaRetrievePrivate  = advapi32.NewProc("LsaRetrievePrivateData")
	procLsaClose            = advapi32.NewProc("LsaClose")
	procLsaFreeMemory       = advapi32.NewProc("LsaFreeMemory")
	procLsaNtStatusToWin    = advapi32.NewProc("LsaNtStatusToWinError")
	procLogonUser           = advapi32.NewProc("LogonUserW")
)

const createNoWindow = 0x08000000

func hidden(cmd *exec.Cmd) *exec.Cmd {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
	return cmd
}

func (*winSys) Hostname() string {
	if h := os.Getenv("COMPUTERNAME"); h != "" {
		return h
	}
	h, _ := os.Hostname()
	return h
}

// LoggedOnUser: the user of an interactive session (active, locked or switched away); "" at the sign-in screen.
func (*winSys) LoggedOnUser() string {
	var sessions *windows.WTS_SESSION_INFO
	var n uint32
	if err := windows.WTSEnumerateSessions(0, 0, 1, &sessions, &n); err != nil {
		return ""
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(sessions)))
	best := ""
	for _, s := range unsafe.Slice(sessions, n) {
		if s.SessionID == 0 || (s.State != windows.WTSActive && s.State != windows.WTSDisconnected) {
			continue
		}
		var buf *uint16
		var size uint32
		if r, _, _ := procWTSQuerySessionInfo.Call(0, uintptr(s.SessionID), 5 /* WTSUserName */, uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&size))); r == 0 || buf == nil {
			continue
		}
		name := windows.UTF16PtrToString(buf)
		windows.WTSFreeMemory(uintptr(unsafe.Pointer(buf)))
		if name == "" {
			continue
		}
		if s.State == windows.WTSActive {
			return name
		}
		best = name
	}
	return best
}

func (*winSys) BootTime() time.Time {
	ms, _, _ := procGetTickCount64.Call()
	return time.Now().Add(-time.Duration(ms) * time.Millisecond).Truncate(time.Minute)
}

func (*winSys) Disks() []Disk {
	var out []Disk
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, _ := windows.UTF16PtrFromString(root)
		if windows.GetDriveType(p) != windows.DRIVE_FIXED {
			continue
		}
		var free, total, totalFree uint64
		if windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree) != nil || total == 0 {
			continue
		}
		out = append(out, Disk{Name: root[:1], FreeGB: float64(free) / 1e9, SizeGB: float64(total) / 1e9})
	}
	return out
}

// ---------------------------------------------------------------------------
// CPU / RAM

var cpuMu sync.Mutex
var cpuIdle, cpuTotal uint64

func ft(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }

func (*winSys) CPU() float64 {
	var idle, kernel, user windows.Filetime
	if r, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user))); r == 0 {
		return 0
	}
	cpuMu.Lock()
	defer cpuMu.Unlock()
	i, t := ft(idle), ft(kernel)+ft(user) // kernel time includes idle time
	di, dt := i-cpuIdle, t-cpuTotal
	cpuIdle, cpuTotal = i, t
	if dt == 0 || di > dt {
		return 0
	}
	return 100 * float64(dt-di) / float64(dt)
}

func (*winSys) RAM() float64 {
	var m struct {
		Length, MemoryLoad                    uint32
		TotalPhys, AvailPhys, TotalPage       uint64
		AvailPage, TotalVirtual, AvailVirtual uint64
		AvailExtendedVirtual                  uint64
	}
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procGlobalMemoryStatus.Call(uintptr(unsafe.Pointer(&m))); r == 0 {
		return 0
	}
	return float64(m.MemoryLoad)
}

// ---------------------------------------------------------------------------
// power

func enableShutdownPrivilege() {
	var tok windows.Token
	if windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok) != nil {
		return
	}
	defer tok.Close()
	var luid windows.LUID
	name, _ := windows.UTF16PtrFromString("SeShutdownPrivilege")
	if windows.LookupPrivilegeValue(nil, name, &luid) != nil {
		return
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	_ = windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil)
}

func (*winSys) Power(action string) error {
	enableShutdownPrivilege()
	switch action {
	case "sleep":
		if r, _, err := procSetSuspendState.Call(0, 0, 0); r == 0 {
			return err
		}
		return nil
	case "shutdown", "restart":
		// /f: a running program with unsaved work must not stop it – nobody is at the PC to click
		flag := "/s"
		if action == "restart" {
			flag = "/r"
		}
		return hidden(exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "shutdown.exe"), flag, "/f", "/t", "0")).Run()
	}
	return fmt.Errorf(T("neznámá akce %q"), action)
}

var (
	guidSubSleep    = windows.GUID{Data1: 0x238C9FA8, Data2: 0x0AAD, Data3: 0x41ED, Data4: [8]byte{0x83, 0xF4, 0x97, 0xBE, 0x24, 0x2C, 0x8F, 0x20}}
	guidStandbyIdle = windows.GUID{Data1: 0x29F6C1DB, Data2: 0x86DA, Data3: 0x48C5, Data4: [8]byte{0x9F, 0xDB, 0xF2, 0xB6, 0x7B, 0x1F, 0x44, 0xDA}}
	guidSubVideo    = windows.GUID{Data1: 0x7516B95F, Data2: 0xF776, Data3: 0x4464, Data4: [8]byte{0x8C, 0x53, 0x06, 0x16, 0x7F, 0x40, 0xCC, 0x99}}
	guidVideoIdle   = windows.GUID{Data1: 0x3C0BC021, Data2: 0xC8A8, Data3: 0x4E07, Data4: [8]byte{0xA9, 0x73, 0x6B, 0x14, 0xCB, 0xCB, 0x2B, 0x7E}}
)

func activeScheme() (*windows.GUID, func(), error) {
	var g *windows.GUID
	if r, _, _ := procPowerGetActive.Call(0, uintptr(unsafe.Pointer(&g))); r != 0 || g == nil {
		return nil, nil, fmt.Errorf(T("plán napájení nejde zjistit (%d)"), r)
	}
	return g, func() { windows.LocalFree(windows.Handle(unsafe.Pointer(g))) }, nil
}

func readPower(sub, set *windows.GUID) int {
	g, free, err := activeScheme()
	if err != nil {
		return -1
	}
	defer free()
	var v uint32
	if r, _, _ := procPowerReadAC.Call(0, uintptr(unsafe.Pointer(g)), uintptr(unsafe.Pointer(sub)), uintptr(unsafe.Pointer(set)), uintptr(unsafe.Pointer(&v))); r != 0 {
		return -1
	}
	return int(v)
}

func writePower(sub, set *windows.GUID, sec int) error {
	g, free, err := activeScheme()
	if err != nil {
		return err
	}
	defer free()
	for _, p := range []*windows.LazyProc{procPowerWriteAC, procPowerWriteDC} {
		if r, _, _ := p.Call(0, uintptr(unsafe.Pointer(g)), uintptr(unsafe.Pointer(sub)), uintptr(unsafe.Pointer(set)), uintptr(sec)); r != 0 {
			return syscall.Errno(r)
		}
	}
	if r, _, _ := procPowerSetActive.Call(0, uintptr(unsafe.Pointer(g))); r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func (*winSys) SleepTimeout() int            { return readPower(&guidSubSleep, &guidStandbyIdle) }
func (*winSys) SetSleepTimeout(s int) error  { return writePower(&guidSubSleep, &guidStandbyIdle, s) }
func (*winSys) ScreenTimeout() int           { return readPower(&guidSubVideo, &guidVideoIdle) }
func (*winSys) SetScreenTimeout(s int) error { return writePower(&guidSubVideo, &guidVideoIdle, s) }

// KeepAwake: SetThreadExecutionState holds only while its thread lives – one locked OS thread keeps it.
var awakeCh chan bool

func (*winSys) KeepAwake(on bool) {
	if awakeCh == nil {
		awakeCh = make(chan bool)
		go func() {
			runtime.LockOSThread()
			for v := range awakeCh {
				const esContinuous, esSystemRequired = 0x80000000, 0x00000001
				if v {
					procSetThreadExecState.Call(esContinuous | esSystemRequired)
				} else {
					procSetThreadExecState.Call(esContinuous)
				}
			}
		}()
	}
	awakeCh <- on
}

// ---------------------------------------------------------------------------
// automatic sign-in: Winlogon AutoAdminLogon with the password as the LSA secret "DefaultPassword"
// (never in plain text in the registry) and AutoLogonCount=1 – Windows itself switches it off after one sign-in.
// LeoLink keeps its own copy of the password as the LSA secret "L$LeoLink" (only SYSTEM / administrators can read it).

const (
	winlogonKey   = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon`
	secretOwn     = "L$LeoLink"
	secretDefault = "DefaultPassword"
)

type lsaString struct {
	Length, MaximumLength uint16
	Buffer                *uint16
}

func newLsaString(s string) (lsaString, []uint16) {
	u := utf16.Encode([]rune(s))
	if len(u) == 0 {
		return lsaString{}, nil
	}
	return lsaString{uint16(len(u) * 2), uint16(len(u) * 2), &u[0]}, u
}

func lsaErr(status uintptr) error {
	w, _, _ := procLsaNtStatusToWin.Call(status)
	return syscall.Errno(w)
}

func lsaOpen() (uintptr, error) {
	var oa struct {
		Length                          uint32
		RootDirectory                   uintptr
		ObjectName                      uintptr
		Attributes                      uint32
		SecurityDescriptor, SecurityQoS uintptr
	}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h uintptr
	if r, _, _ := procLsaOpenPolicy.Call(0, uintptr(unsafe.Pointer(&oa)), 0x00F0FFF /* POLICY_ALL_ACCESS */, uintptr(unsafe.Pointer(&h))); r != 0 {
		return 0, lsaErr(r)
	}
	return h, nil
}

func lsaStore(key string, value *string) error {
	h, err := lsaOpen()
	if err != nil {
		return err
	}
	defer procLsaClose.Call(h)
	k, kb := newLsaString(key)
	var data uintptr
	var vb []uint16
	var v lsaString
	if value != nil {
		v, vb = newLsaString(*value)
		data = uintptr(unsafe.Pointer(&v))
	}
	r, _, _ := procLsaStorePrivate.Call(h, uintptr(unsafe.Pointer(&k)), data)
	runtime.KeepAlive(kb)
	runtime.KeepAlive(vb)
	if r != 0 && !(value == nil && uint32(r) == 0xC0000034) { // deleting a secret that does not exist is fine
		return lsaErr(r)
	}
	return nil
}

func lsaRetrieve(key string) (string, error) {
	h, err := lsaOpen()
	if err != nil {
		return "", err
	}
	defer procLsaClose.Call(h)
	k, kb := newLsaString(key)
	var p *lsaString
	r, _, _ := procLsaRetrievePrivate.Call(h, uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&p)))
	runtime.KeepAlive(kb)
	if r != 0 {
		return "", lsaErr(r)
	}
	if p == nil {
		return "", nil
	}
	defer procLsaFreeMemory.Call(uintptr(unsafe.Pointer(p)))
	if p.Buffer == nil || p.Length == 0 {
		return "", nil
	}
	return string(utf16.Decode(unsafe.Slice(p.Buffer, p.Length/2))), nil
}

func (*winSys) HasPassword() bool {
	p, err := lsaRetrieve(secretOwn)
	return err == nil && p != ""
}

func (*winSys) StorePassword(p string) error {
	if p == "" {
		return lsaStore(secretOwn, nil)
	}
	return lsaStore(secretOwn, &p)
}

func (*winSys) VerifyPassword(user, domain, pass string) error {
	if domain == "" {
		domain = "."
	}
	u, _ := windows.UTF16PtrFromString(user)
	d, _ := windows.UTF16PtrFromString(domain)
	p, _ := windows.UTF16PtrFromString(pass)
	var tok windows.Handle
	r, _, err := procLogonUser.Call(uintptr(unsafe.Pointer(u)), uintptr(unsafe.Pointer(d)), uintptr(unsafe.Pointer(p)),
		2 /* LOGON32_LOGON_INTERACTIVE */, 0, uintptr(unsafe.Pointer(&tok)))
	if r == 0 {
		return err
	}
	windows.CloseHandle(tok)
	return nil
}

func (s *winSys) SetAutologon(user, domain string) error {
	pw, err := lsaRetrieve(secretOwn)
	if err != nil || pw == "" {
		return errors.New(T("heslo pro automatické přihlášení není uložené"))
	}
	if err := lsaStore(secretDefault, &pw); err != nil {
		return fmt.Errorf("LSA DefaultPassword: %w", err)
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, winlogonKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	_ = k.DeleteValue("DefaultPassword") // a plain-text password in the registry would take precedence
	if domain == "" {
		domain = s.Hostname()
	}
	for _, e := range []error{
		k.SetStringValue("DefaultUserName", user),
		k.SetStringValue("DefaultDomainName", domain),
		k.SetDWordValue("AutoLogonCount", 1),
		k.SetStringValue("AutoAdminLogon", "1"),
	} {
		if e != nil {
			return e
		}
	}
	return nil
}

func (*winSys) ClearAutologon() error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, winlogonKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	err = k.SetStringValue("AutoAdminLogon", "0")
	_ = k.DeleteValue("AutoLogonCount")
	_ = k.DeleteValue("DefaultPassword")
	if e := lsaStore(secretDefault, nil); e != nil && err == nil {
		err = e
	}
	return err
}

func (*winSys) AutologonActive() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, winlogonKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, _ := k.GetStringValue("AutoAdminLogon")
	return v == "1"
}

// ---------------------------------------------------------------------------
// session (agent)

func (*winSys) Lock() error {
	if r, _, err := procLockWorkStation.Call(); r == 0 {
		return err
	}
	return nil
}

func (*winSys) Monitor(on bool) error {
	const hwndBroadcast, wmSysCommand, scMonitorPower = 0xFFFF, 0x0112, 0xF170
	if !on {
		procPostMessage.Call(hwndBroadcast, wmSysCommand, scMonitorPower, 2)
		return nil
	}
	procPostMessage.Call(hwndBroadcast, wmSysCommand, scMonitorPower, ^uintptr(0)) // -1 = on
	procMouseEvent.Call(1 /* MOUSEEVENTF_MOVE */, 1, 0, 0, 0)
	procMouseEvent.Call(1, ^uintptr(0), 0, 0, 0)
	return nil
}

func (*winSys) Mute() error {
	const vkVolumeMute, keyUp = 0xAD, 0x2
	procKeybdEvent.Call(vkVolumeMute, 0, 0, 0)
	procKeybdEvent.Call(vkVolumeMute, 0, keyUp, 0)
	return nil
}

func (*winSys) Notify(text string) {
	go func() {
		t, _ := windows.UTF16PtrFromString(text)
		c, _ := windows.UTF16PtrFromString("Home Assistant")
		// MB_OK | MB_ICONINFORMATION | MB_SETFOREGROUND | MB_TOPMOST
		procMessageBox.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x40|0x10000|0x40000)
	}()
}

func (*winSys) IdleSeconds() int {
	var li struct{ Size, Time uint32 }
	li.Size = 8
	if r, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&li))); r == 0 {
		return 0
	}
	now, _, _ := procGetTickCount.Call()
	return int((uint32(now) - li.Time) / 1000)
}

func procImage(name string) (bool, string) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, ""
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if !strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), name) {
			continue
		}
		path := ""
		if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID); err == nil {
			buf := make([]uint16, 1024)
			n := uint32(len(buf))
			if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil {
				path = windows.UTF16ToString(buf[:n])
			}
			windows.CloseHandle(h)
		}
		return true, path
	}
	return false, ""
}

func (*winSys) ProcRunning(exe string) bool { ok, _ := procImage(exe); return ok }

// StartApp goes through "start" of cmd.exe: works for .exe, .lnk, documents and shell: links; /min = minimized.
func (*winSys) StartApp(a App) error {
	p := strings.TrimSpace(a.Path)
	if id := packagedAppID(p); id != "" {
		p = `shell:AppsFolder\` + id
	}
	if !strings.Contains(p, ":") || strings.HasPrefix(strings.ToLower(p), "shell:") {
		// shell:AppsFolder\…, nothing to check
	} else if _, err := os.Stat(p); err != nil {
		return fmt.Errorf(T("nenalezeno: %s"), p)
	}
	line := `cmd.exe /d /c start ""`
	if a.Min {
		line += " /min"
	}
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		line += ` /d "` + filepath.Dir(p) + `"`
	}
	line += ` "` + p + `"`
	if args := strings.TrimSpace(a.Args); args != "" {
		line += " " + args
	}
	comspec := os.Getenv("ComSpec")
	if comspec == "" {
		comspec = "cmd.exe"
	}
	cmd := exec.Command(comspec)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
	return hidden(cmd).Run()
}

func (*winSys) DropboxStop() error {
	if ok, _ := procImage("Dropbox.exe"); !ok {
		return nil
	}
	return hidden(exec.Command("taskkill", "/F", "/T", "/IM", "Dropbox.exe")).Run()
}

func (*winSys) DropboxStart() error {
	if ok, _ := procImage("Dropbox.exe"); ok {
		return nil
	}
	c := dropboxStartCmd("")
	if c == "" {
		for _, p := range []string{
			filepath.Join(os.Getenv("ProgramFiles(x86)"), `Dropbox\Client\Dropbox.exe`),
			filepath.Join(os.Getenv("ProgramFiles"), `Dropbox\Client\Dropbox.exe`),
			filepath.Join(os.Getenv("LOCALAPPDATA"), `Dropbox\Client\Dropbox.exe`),
		} {
			if _, err := os.Stat(p); err == nil {
				c = `"` + p + `" /systemstartup`
				break
			}
		}
	}
	if c == "" {
		return errors.New(T("Dropbox nenalezen"))
	}
	return runDetached(c)
}

func (*winSys) DropboxState() string {
	if ok, _ := procImage("Dropbox.exe"); !ok {
		return "neběží"
	}
	if t := dropboxTrayText(); t != "" && dropboxPausedRe.MatchString(t) {
		return "pozastaven"
	}
	return "běží"
}

// ---------------------------------------------------------------------------

func protect(p []byte) ([]byte, error) {
	if len(p) == 0 {
		return nil, nil
	}
	in := windows.DataBlob{Size: uint32(len(p)), Data: &p[0]}
	var out windows.DataBlob
	// CRYPTPROTECT_UI_FORBIDDEN | CRYPTPROTECT_LOCAL_MACHINE: the service (SYSTEM) and the agent (user) must both read it
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, 0x1|0x4, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func unprotect(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, nil
	}
	in := windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, 0x1, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

// osLangCzech: Windows display language is Czech or Slovak.
func osLangCzech() bool {
	r, _, _ := kernel32.NewProc("GetUserDefaultUILanguage").Call()
	p := r & 0x3ff
	return p == 0x05 || p == 0x1b
}

func isElevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

func singleInstance(name string) bool {
	n, _ := windows.UTF16PtrFromString(name)
	_, err := windows.CreateMutex(nil, false, n)
	return !errors.Is(err, windows.ERROR_ALREADY_EXISTS)
}

// Started directly by its path it runs WITHOUT the package identity, which is not supported. Such apps must be started
// through shell:AppsFolder\<family>!<app id>.
// The path also changes with every update (Claude_4.29.0.0_x64__…), the family name does not.
func packagedAppID(p string) string {
	lp := strings.ToLower(p)
	i := strings.Index(lp, `\windowsapps\`)
	if i < 0 {
		return ""
	}
	rest := p[i+len(`\windowsapps\`):]
	j := strings.IndexByte(rest, '\\')
	if j < 0 {
		return ""
	}
	full, rel := rest[:j], rest[j+1:] // Claude_4.29.0.0_x64__pz8sxrjxfjjc, app\claude.exe
	parts := strings.Split(full, "_")
	if len(parts) < 2 {
		return ""
	}
	family := parts[0] + "_" + parts[len(parts)-1]
	b, err := os.ReadFile(filepath.Join(p[:i+len(`\windowsapps\`)]+full, "AppxManifest.xml"))
	if err != nil {
		return ""
	}
	return family + "!" + manifestAppID(string(b), rel)
}

var reAppxApp = regexp.MustCompile(`(?s)<Application\s[^>]*>`)
var reAppxAttr = regexp.MustCompile(`(\w+)="([^"]*)"`)

// manifestAppID: Id of the <Application> whose Executable is rel (first one otherwise).
func manifestAppID(manifest, rel string) string {
	first := ""
	for _, tag := range reAppxApp.FindAllString(manifest, -1) {
		attr := map[string]string{}
		for _, m := range reAppxAttr.FindAllStringSubmatch(tag, -1) {
			attr[m[1]] = m[2]
		}
		if first == "" {
			first = attr["Id"]
		}
		if strings.EqualFold(strings.ReplaceAll(attr["Executable"], "/", `\`), rel) {
			return attr["Id"]
		}
	}
	if first == "" {
		first = "App"
	}
	return first
}
