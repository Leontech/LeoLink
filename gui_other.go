//go:build !windows

package main

// Stand-ins so the settings window can be tested in a browser on Linux (LEOLINK_HEADLESS=1).

import (
	"errors"
	"fmt"
	"os"
)

type Account struct {
	User   string `json:"user"`
	Domain string `json:"domain"`
	MSA    string `json:"msa"`
}

type Install struct {
	Dir       string `json:"dir"`
	Exe       string `json:"exe"`
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	Tasks     bool   `json:"tasks"`
}

func fatalBox(t string)                       { fmt.Fprintln(os.Stderr, t) }
func relaunchElevated() bool                  { return false }
func openWindow(url string)                   { fmt.Println(url); select {} }
func passwordlessRequired() bool              { return fakeRead("passwordless", "") == "1" }
func setPasswordlessOff() error               { fakeWrite("passwordless", "0"); return nil }
func pickFile(string, string) (string, error) { return "", errors.New("jen ve Windows") }

func openURL(u string) error { return fakeAction("open %s", u) }

func init() {
	// screenshots / tests: a made-up network adapter (the test machine has none in a private network)
	if os.Getenv("LEOLINK_FAKE_NIC") != "" {
		nics = func() []nic { return []nic{{"Ethernet", "3C:7C:3F:12:A4:5E", "192.168.1.25", "192.168.1.255"}} }
	}
}

func accountInfo() Account {
	return Account{User: "leon", Domain: "TEST-PC", MSA: fakeRead("msa", "")}
}

func detectApps() []App {
	return []App{
		{On: true, Name: "Claude", Path: `C:\Users\leon\AppData\Local\AnthropicClaude\claude.exe`, Min: true},
		{On: true, Name: "Dropbox", Path: `C:\Program Files (x86)\Dropbox\Client\Dropbox.exe`, Args: "/systemstartup", Min: true},
		{On: true, Name: "Leonzilla", Path: `C:\Tools\Leonzilla\Leonzilla.exe`, Args: "--minimized", Min: true},
	}
}

func installInfo() Install {
	v := fakeRead("installed", "")
	return Install{Dir: `C:\Program Files\LeoLink`, Exe: `C:\Program Files\LeoLink\LeoLink.exe`, Installed: v != "", Version: v, Tasks: v != ""}
}

func doInstall() error   { fakeWrite("installed", appVersion); return fakeAction("install") }
func doUninstall() error { _ = os.Remove(fakeDir() + "/installed"); return fakeAction("uninstall") }
