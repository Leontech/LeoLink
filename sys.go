package main

// Everything LeoLink does to Windows goes through sysAPI. sys_windows.go is the real thing; sys_other.go is a
// stand-in that only records the actions, so the service / agent logic can be tested on Linux against a broker.

import "time"

type Disk struct {
	Name   string  `json:"name"`   // "C"
	FreeGB float64 `json:"freeGB"` // free space for the user
	SizeGB float64 `json:"sizeGB"`
}

type sysAPI interface {
	Hostname() string
	LoggedOnUser() string // "" = nobody is signed in (sign-in screen)
	BootTime() time.Time
	Disks() []Disk

	// automatic sign-in (service, SYSTEM / settings window as administrator)
	HasPassword() bool
	StorePassword(pass string) error // "" deletes it
	VerifyPassword(user, domain, pass string) error
	SetAutologon(user, domain string) error // one sign-in with the stored password
	ClearAutologon() error
	AutologonActive() bool

	Power(action string) error // "sleep" | "shutdown" | "restart"
	CPU() float64              // % since the previous call
	RAM() float64              // % used
	SleepTimeout() int         // seconds, 0 = never (active power plan, on AC power)
	SetSleepTimeout(sec int) error
	ScreenTimeout() int
	SetScreenTimeout(sec int) error
	KeepAwake(on bool) // the PC does not go to sleep on its own while on (the service keeps it, until restart)

	// in the user's session (agent)
	Lock() error
	Monitor(on bool) error
	IdleSeconds() int
	ProcRunning(exe string) bool
	StartApp(a App) error
	DropboxStop() error
	DropboxStart() error
	DropboxState() string // "neběží" | "běží" | "pozastaven"
	Mute() error          // toggles mute (media key)
	Notify(text string)   // message on the screen (does not wait)
}

var sys sysAPI = newSys()
