//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var mainHWND uintptr

func fatalBox(text string) {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(appTitle)
	procMessageBox.Call(mainHWND, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x10)
}

// relaunchElevated starts the settings window again with administrator rights (UAC prompt).
func relaunchElevated() bool {
	exe, _ := os.Executable()
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(exe))
	return windows.ShellExecute(0, verb, file, nil, dir, windows.SW_NORMAL) == nil
}

func openWindow(url string) {
	if !singleInstance("Local\\LeoLink-settings") {
		return
	}
	runtime.LockOSThread()
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		local = os.TempDir()
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus:     true,
		DataPath:      filepath.Join(local, "LeoLink", "WebView2"),
		WindowOptions: webview2.WindowOptions{Title: appTitle, Width: 1060, Height: 780, IconId: 1, Center: true},
	})
	if w == nil {
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
		select {}
	}
	defer w.Destroy()
	mainHWND = uintptr(w.Window())
	uiDispatch = w.Dispatch
	w.SetSize(820, 560, webview2.HintMin)
	w.Navigate(url)
	w.Run()
}

func openURL(u string) error {
	return hidden(exec.Command("rundll32", "url.dll,FileProtocolHandler", u)).Start()
}

func pickFile(title, start string) (string, error) {
	return showFileDialog(title, start, false, T("Programy (*.exe;*.lnk;*.bat;*.cmd)"), "*.exe;*.lnk;*.bat;*.cmd")
}

// ---------------------------------------------------------------------------

type Account struct {
	User   string `json:"user"`
	Domain string `json:"domain"`
	MSA    string `json:"msa"` // e-mail of the Microsoft account the Windows account is linked to
}

func accountInfo() Account {
	a := Account{User: os.Getenv("USERNAME"), Domain: os.Getenv("USERDOMAIN")}
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\IdentityCRL\UserExtendedProperties`, registry.ENUMERATE_SUB_KEYS); err == nil {
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			if strings.Contains(n, "@") {
				a.MSA = n
				break
			}
		}
	}
	return a
}

const passwordlessKey = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\PasswordLess\Device`

// passwordlessRequired: "For improved security, only allow Windows Hello sign-in for Microsoft accounts" –
// with it on, Windows does not accept a password and the automatic sign-in does not work.
func passwordlessRequired() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, passwordlessKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("DevicePasswordLessBuildVersion")
	return err == nil && v == 2
}

func setPasswordlessOff() error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, passwordlessKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetDWordValue("DevicePasswordLessBuildVersion", 0)
}

// detectApps: the usual programs for Leonzilla mode where they are found (running or installed).
func detectApps() []App {
	var out []App
	local := os.Getenv("LOCALAPPDATA")
	if p := filepath.Join(local, "AnthropicClaude", "claude.exe"); fileExists(p) {
		out = append(out, App{On: true, Name: "Claude", Path: p, Min: true})
	} else if ok, p := procImage("claude.exe"); ok && p != "" {
		out = append(out, App{On: true, Name: "Claude", Path: p, Min: true})
	}
	dbx := ""
	if ok, p := procImage("Dropbox.exe"); ok {
		dbx = p
	}
	for _, p := range []string{filepath.Join(os.Getenv("ProgramFiles(x86)"), `Dropbox\Client\Dropbox.exe`), filepath.Join(os.Getenv("ProgramFiles"), `Dropbox\Client\Dropbox.exe`)} {
		if dbx == "" && fileExists(p) {
			dbx = p
		}
	}
	if dbx != "" {
		out = append(out, App{On: true, Name: "Dropbox", Path: dbx, Args: "/systemstartup", Min: true})
	}
	lz := ""
	if ok, p := procImage("Leonzilla.exe"); ok {
		lz = p
	} else if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE); err == nil {
		v, _, _ := k.GetStringValue("Leonzilla")
		k.Close()
		if strings.HasPrefix(v, `"`) {
			if i := strings.Index(v[1:], `"`); i > 0 {
				lz = v[1 : i+1]
			}
		}
	}
	if lz != "" {
		out = append(out, App{On: true, Name: "Leonzilla", Path: lz, Args: "--minimized", Min: true})
	} else {
		out = append(out, App{On: false, Name: "Leonzilla", Path: "", Args: "--minimized", Min: true})
	}
	return out
}

// ---------------------------------------------------------------------------
// installation: LeoLink.exe → %ProgramFiles%\LeoLink (only administrators can change it – the service runs it as SYSTEM),
// two scheduled tasks, data folder %ProgramData%\LeoLink readable for users, writable only for SYSTEM / administrators.

const (
	taskService = `LeoLink\Service`
	taskAgent   = `LeoLink\Agent`
)

type Install struct {
	Dir       string `json:"dir"`
	Exe       string `json:"exe"`
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	Tasks     bool   `json:"tasks"`
}

func installInfo() Install {
	dir := filepath.Join(os.Getenv("ProgramFiles"), "LeoLink")
	in := Install{Dir: dir, Exe: filepath.Join(dir, "LeoLink.exe")}
	in.Installed = fileExists(in.Exe)
	if b, err := os.ReadFile(filepath.Join(dir, "version.txt")); err == nil {
		in.Version = strings.TrimSpace(string(b))
	}
	in.Tasks = hidden(exec.Command("schtasks", "/query", "/tn", taskService)).Run() == nil
	return in
}

func schtasks(args ...string) error {
	out, err := hidden(exec.Command("schtasks", args...)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks %s: %s", args[0], strings.TrimSpace(oemString(out)))
	}
	return nil
}

// oemString: console output of Windows tools is in the OEM code page (852 for Czech) – good enough for messages.
func oemString(b []byte) string {
	r := []rune{}
	for _, c := range b {
		r = append(r, cp852[c])
	}
	return string(r)
}

var cp852 = func() [256]rune {
	var t [256]rune
	for i := 0; i < 128; i++ {
		t[i] = rune(i)
	}
	hi := []rune("ÇüéâäůćçłëŐőîŹÄĆÉĹĺôöĽľŚśÖÜŤťŁ×čáíóúĄąŽžĘę¬źČş«»░▒▓│┤ÁÂĚŞ╣║╗╝Żż┐└┴┬├─┼Ăă╚╔╩╦╠═╬¤đĐĎËďŇÍÎě┘┌█▄ŢŮ▀ÓßÔŃńňŠšŔÚŕŰýÝţ´\u00ad˝˛ˇ˘§÷¸°¨˙űŘř■\u00a0")
	for i, c := range hi {
		t[128+i] = c
	}
	return t
}()

func killOthers() {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	me := uint32(os.Getpid())
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if pe.ProcessID == me || !strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), "LeoLink.exe") {
			continue
		}
		if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID); err == nil {
			buf := make([]uint16, 1024)
			n := uint32(len(buf))
			path := ""
			if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil {
				path = windows.UTF16ToString(buf[:n])
			}
			// only the installed copy (service / agent) – not another settings window
			if strings.EqualFold(filepath.Dir(path), installInfo().Dir) {
				_ = windows.TerminateProcess(h, 0)
			}
			windows.CloseHandle(h)
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	for i := 0; i < 20; i++ {
		if err = os.Rename(tmp, dst); err == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return err
}

func xmlEsc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

const taskSettings = `<Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings><StopOnIdleEnd>false</StopOnIdleEnd><RestartOnIdle>false</RestartOnIdle></IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>5</Priority>
    <RestartOnFailure><Interval>PT1M</Interval><Count>999</Count></RestartOnFailure>
  </Settings>`

func taskXML(desc, trigger, principal, exe, args string) string {
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Author>LeoLink</Author><Description>` + xmlEsc(desc) + `</Description></RegistrationInfo>
  <Triggers>` + trigger + `</Triggers>
  <Principals><Principal id="Author">` + principal + `</Principal></Principals>
  ` + taskSettings + `
  <Actions Context="Author"><Exec><Command>` + xmlEsc(exe) + `</Command><Arguments>` + args + `</Arguments></Exec></Actions>
</Task>`
}

func registerTask(name, xml string) error {
	f, err := os.CreateTemp("", "leolink-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	u := utf16.Encode([]rune(xml))
	b := []byte{0xFF, 0xFE}
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	_, _ = f.Write(b)
	f.Close()
	return schtasks("/create", "/tn", name, "/xml", f.Name(), "/f")
}

func doInstall() error {
	in := installInfo()
	self, _ := os.Executable()
	_ = schtasks("/end", "/tn", taskService)
	_ = schtasks("/end", "/tn", taskAgent)
	killOthers()
	if err := os.MkdirAll(in.Dir, 0o755); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(in.Exe)) {
		if err := copyFile(self, in.Exe); err != nil {
			return fmt.Errorf(T("kopírování do %s: %v"), in.Dir, err)
		}
	}
	_ = os.WriteFile(filepath.Join(in.Dir, "version.txt"), []byte(appVersion), 0o644)
	_ = os.MkdirAll(dataDir(), 0o755)
	// SYSTEM + Administrators: full, Users: read (the agent reads config.json, writes only to %LOCALAPPDATA%)
	_ = hidden(exec.Command("icacls", dataDir(), "/inheritance:r", "/grant:r", "*S-1-5-18:(OI)(CI)F", "*S-1-5-32-544:(OI)(CI)F", "*S-1-5-32-545:(OI)(CI)RX")).Run()

	svc := taskXML("LeoLink – služba: Home Assistant (MQTT), uspání / vypnutí, režim Leonzilla. Běží jako SYSTEM.",
		`<BootTrigger><Enabled>true</Enabled></BootTrigger>`,
		`<UserId>S-1-5-18</UserId><RunLevel>HighestAvailable</RunLevel>`, in.Exe, "--service")
	if err := registerTask(taskService, svc); err != nil {
		return err
	}
	user := os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")
	ag := taskXML("LeoLink – agent v relaci uživatele: spouští programy režimu Leonzilla, zamyká, monitory, Dropbox.",
		`<LogonTrigger><Enabled>true</Enabled><UserId>`+xmlEsc(user)+`</UserId></LogonTrigger>`,
		`<UserId>`+xmlEsc(user)+`</UserId><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel>`, in.Exe, "--agent")
	if err := registerTask(taskAgent, ag); err != nil {
		return err
	}
	if err := schtasks("/run", "/tn", taskService); err != nil {
		return err
	}
	return schtasks("/run", "/tn", taskAgent)
}

func doUninstall() error {
	in := installInfo()
	_ = schtasks("/end", "/tn", taskService)
	_ = schtasks("/end", "/tn", taskAgent)
	_ = schtasks("/delete", "/tn", taskService, "/f")
	_ = schtasks("/delete", "/tn", taskAgent, "/f")
	_ = schtasks("/delete", "/tn", `LeoLink`, "/f") // the folder in Task Scheduler
	killOthers()
	time.Sleep(500 * time.Millisecond)
	self, _ := os.Executable()
	for _, f := range []string{in.Exe, filepath.Join(in.Dir, "version.txt")} {
		if strings.EqualFold(filepath.Clean(self), filepath.Clean(f)) {
			p, _ := windows.UTF16PtrFromString(f)
			_ = windows.MoveFileEx(p, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
			continue
		}
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	_ = os.Remove(in.Dir)
	_ = os.Remove(filepath.Join(dataDir(), "service.json"))
	return nil
}

var _ = syscall.Errno(0)
