//go:build windows

package main

// Dropbox helpers taken over from Leonzilla (dropbox_windows.go).

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows/registry"
)

// texts Dropbox shows in the tray tooltip while syncing is paused, in the languages of the Dropbox app
var dropboxPausedRe = regexp.MustCompile(`(?i)pozastav|paus|pauz|angehalten|wstrzyma|призупин|keskeyt|duraklat|dijeda|一時停止|暂停|暫停|일시\s?중지|sospes|หยุดชั่วคราว`)

// dropboxStartCmd: the autostart command from the registry (starts quietly in the tray), or the exe.
func dropboxStartCmd(exe string) string {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(root, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue("Dropbox")
		k.Close()
		if err == nil && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	if exe != "" {
		return `"` + exe + `" /systemstartup`
	}
	return ""
}

func runDetached(cmdline string) error {
	exe := cmdline
	if strings.HasPrefix(exe, `"`) {
		if i := strings.Index(exe[1:], `"`); i >= 0 {
			exe = exe[1 : i+1]
		}
	} else if i := strings.Index(exe, " "); i > 0 {
		exe = exe[:i]
	}
	if _, err := os.Stat(exe); err != nil {
		return errors.New(T("nenalezen ") + exe)
	}
	c := exec.Command(exe)
	c.SysProcAttr = &syscall.SysProcAttr{CmdLine: cmdline, CreationFlags: 0x00000008 | 0x00000200} // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}

const dropboxTrayPS = `$ErrorActionPreference='SilentlyContinue'
[Console]::OutputEncoding=[Text.Encoding]::UTF8
Add-Type -AssemblyName UIAutomationClient,UIAutomationTypes
$A=[System.Windows.Automation.AutomationElement]; $S=[System.Windows.Automation.TreeScope]; $T=[System.Windows.Automation.Condition]::TrueCondition
$out=@()
foreach($cls in 'Shell_TrayWnd','NotifyIconOverflowWindow','TopLevelWindowForOverflowXamlIsland'){
  $c=New-Object System.Windows.Automation.PropertyCondition($A::ClassNameProperty,$cls)
  foreach($w in $A::RootElement.FindAll($S::Children,$c)){
    foreach($e in $w.FindAll($S::Descendants,$T)){ $n=$e.Current.Name; if($n -match 'Dropbox'){ $out+=($n -replace '[\r\n]+',' | ') } }
  }
}
$out -join [char]10`

func dropboxTrayText() string {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", dropboxTrayPS)
	hidden(cmd)
	b, err := cmd.Output()
	if err != nil && len(b) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(string(b), "\ufeff"))
}
