package main

// The agent: LeoLink.exe --agent, a scheduled task "at sign-in" of the user, without administrator rights.
// It runs in the user's session (also when it is locked) and
//   - switches on Leonzilla mode: starts the ticked programs (e.g. Dropbox, Leonzilla …), after the automatic
//     sign-in also locks the screen,
//   - executes what needs the session: lock, monitors, mute, Dropbox off / on, a message on the screen,
//   - in Leonzilla mode puts the PC to sleep (or shuts it down) when nothing has happened for a while:
//     no keyboard / mouse at the PC, no request from the phone to Leonzilla, no running Leonzilla job,
//   - ends Leonzilla mode as soon as somebody uses the keyboard or mouse at the PC (then nothing sleeps by itself).

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type agent struct {
	mu        sync.Mutex
	cfg       *Config
	cfgStamp  time.Time
	st        *AgentState
	link      *Link
	dropbox   string
	dropboxAt time.Time
	sleepIn   string
	lastPub   time.Time
	lastJSON  string
	pubMu     sync.Mutex
}

func runAgent() {
	logf("LeoLink %s – agent startuje (%s)", appVersion, sys.LoggedOnUser())
	a := &agent{st: loadAgentState()}
	a.cfg, a.cfgStamp = loadConfig(), configStamp()
	a.checkBoot()
	a.connect()
	last := time.Now().Round(0)
	for range time.Tick(5 * time.Second) {
		now := time.Now().Round(0)
		if gap := now.Sub(last); gap > 30*time.Second {
			logf("PC se probudilo (pauza %s) – znovu se připojuji", gap.Round(time.Second))
			a.link.close(false)
			a.connect()
		}
		last = now
		if st := configStamp(); !st.Equal(a.cfgStamp) {
			nc := loadConfig()
			a.mu.Lock()
			old := a.cfg
			a.cfg, a.cfgStamp = nc, st
			a.mu.Unlock()
			if nc.sameLink(old) {
				a.link.cfg = nc
				a.dropboxAt = time.Time{} // states in the new language
				a.publishSession(true)
			} else {
				a.link.close(false)
				a.connect()
			}
		}
		a.checkSleep()
		a.publishSession(false)
		writeHeartbeat(filepath.Join(userDir(), "agent_alive.json"), a.link)
	}
}

func (a *agent) connect() {
	c := a.cfg
	a.link = dial(c, "agent", map[string]handler{
		c.topic("cmd"):  a.onCmd,
		c.topic("wake"): a.onWake,
		c.topic("notify"): func(_ string, p []byte, retained bool) {
			if !retained && len(p) > 0 {
				logf("zpráva na obrazovku: %s", p)
				sys.Notify(string(p))
			}
		},
	}, func(*Link) { a.publishSession(true) })
}

// checkBoot: the service restarted the PC into Leonzilla mode (automatic sign-in) – start the programs and lock.
func (a *agent) checkBoot() {
	st := loadState()
	if st.RemoteBoot.IsZero() || st.RemoteBoot.Equal(a.st.HandledBoot) || time.Since(st.RemoteBoot) > 20*time.Minute {
		return
	}
	a.mu.Lock()
	a.st.HandledBoot = st.RemoteBoot
	a.mu.Unlock()
	a.startLeonzilla("po automatickém přihlášení", a.cfg.LockAfter)
}

func (a *agent) onWake(_ string, p []byte, retained bool) {
	if len(strings.TrimSpace(string(p))) == 0 || !wakeFresh(p, retained) {
		return // empty = already handled; a stale one is deleted by the service
	}
	_ = a.link.pub(a.cfg.topic("wake"), "", true)
	a.startLeonzilla("probuzení z Home Assistantu", false)
}

func (a *agent) onCmd(_ string, p []byte, _ bool) {
	cmd := strings.ToLower(strings.TrimSpace(string(p)))
	var err error
	switch cmd {
	case "lock":
		logf("příkaz: zamknout")
		err = sys.Lock()
	case "monitor_off":
		logf("příkaz: vypnout monitory")
		err = sys.Monitor(false)
	case "monitor_on":
		logf("příkaz: zapnout monitory")
		err = sys.Monitor(true)
		// waking the monitor moves the mouse a little – that is not somebody at the PC
		a.mu.Lock()
		if a.st.Remote {
			a.st.RemoteSince = time.Now().Add(2 * time.Second)
			_ = saveAgentState(a.st)
		}
		a.mu.Unlock()
	case "mute":
		err = sys.Mute()
	case "dropbox_off":
		logf("příkaz: Dropbox vypnout")
		err = sys.DropboxStop()
		a.dropboxAt = time.Time{}
	case "dropbox_on":
		logf("příkaz: Dropbox zapnout")
		err = sys.DropboxStart()
		a.dropboxAt = time.Time{}
	case "leonzilla":
		a.startLeonzilla("tlačítko v Home Assistantu", false)
	case "normal":
		a.endRemote("ukončeno z Home Assistantu")
	default:
		return // sleep / shutdown / restart – the service
	}
	if err != nil {
		logf("%s: %v", cmd, err)
	}
	time.AfterFunc(2*time.Second, func() { a.publishSession(true) })
}

func (a *agent) startLeonzilla(why string, lock bool) {
	logf("režim Leonzilla zapnut (%s)", T(why))
	a.mu.Lock()
	apps := append([]App(nil), a.cfg.Apps...)
	a.mu.Unlock()
	for _, app := range apps {
		if !app.On || strings.TrimSpace(app.Path) == "" {
			continue
		}
		exe := filepath.Base(strings.ReplaceAll(app.Path, `\`, "/"))
		if strings.EqualFold(filepath.Ext(exe), ".exe") && sys.ProcRunning(exe) {
			logf("  %s už běží", app.Name)
			continue
		}
		if err := sys.StartApp(app); err != nil {
			logf("  %s se nepodařilo spustit: %v", app.Name, err)
		} else {
			logf("  spuštěno: %s", app.Name)
		}
	}
	a.mu.Lock()
	a.st.Remote, a.st.RemoteSince = true, time.Now()
	_ = saveAgentState(a.st)
	a.mu.Unlock()
	if lock {
		time.Sleep(3 * time.Second)
		if err := sys.Lock(); err != nil {
			logf("zamknutí: %v", err)
		} else {
			logf("obrazovka zamčena")
		}
	}
	a.publishSession(true)
}

func (a *agent) endRemote(why string) {
	a.mu.Lock()
	was := a.st.Remote
	a.st.Remote = false
	_ = saveAgentState(a.st)
	a.mu.Unlock()
	if was {
		logf("režim Leonzilla ukončen (%s)", T(why))
	}
	a.publishSession(true)
}

// ---------------------------------------------------------------------------
// Leonzilla tells what it is doing in %LOCALAPPDATA%\Leonzilla\activity.json (since 1.11.20):
// busy = a job is running, lastRemote = the last request from the phone.

type lzActivity struct {
	Busy       bool      `json:"busy"`
	LastRemote time.Time `json:"lastRemote"`
	Updated    time.Time `json:"updated"`
}

func leonzillaActivity() lzActivity {
	var act lzActivity
	local := os.Getenv("LOCALAPPDATA")
	if b, err := os.ReadFile(filepath.Join(local, "Leonzilla", "activity.json")); err == nil {
		_ = json.Unmarshal(b, &act)
	}
	if act.Busy && time.Since(act.Updated) > 15*time.Minute {
		act.Busy = false // Leonzilla ended without writing the file again
	}
	return act
}

// checkSleep: Leonzilla mode – nobody at the PC and nothing from the phone for SleepMin minutes → sleep / shut down.
func (a *agent) checkSleep() {
	a.mu.Lock()
	remote, since := a.st.Remote, a.st.RemoteSince
	cfg := a.cfg
	a.mu.Unlock()
	if !remote {
		a.setSleepIn("—")
		return
	}
	lastInput := time.Now().Add(-time.Duration(sys.IdleSeconds()) * time.Second)
	if lastInput.After(since.Add(10 * time.Second)) {
		a.endRemote("u PC někdo je – klávesnice nebo myš")
		return
	}
	if !cfg.AutoSleep {
		a.setSleepIn(T("vypnuto"))
		return
	}
	act := leonzillaActivity()
	if act.Busy {
		a.setSleepIn(T("čeká – Leonzilla pracuje"))
		return
	}
	last := since
	if act.LastRemote.After(last) {
		last = act.LastRemote
	}
	left := time.Duration(cfg.SleepMin)*time.Minute - time.Since(last)
	if left > 0 {
		a.setSleepIn(fmt.Sprintf(T("za %d min"), int(math.Ceil(left.Minutes()))))
		return
	}
	if cfg.SleepAct == "shutdown" {
		logf("režim Leonzilla: %d min se nic neděje – vypínám PC", cfg.SleepMin)
	} else {
		logf("režim Leonzilla: %d min se nic neděje – uspávám PC", cfg.SleepMin)
	}
	a.mu.Lock()
	a.st.Remote = false
	_ = saveAgentState(a.st)
	a.mu.Unlock()
	a.setSleepIn("—")
	a.publishSession(true)
	_ = a.link.pub(cfg.topic("status"), "offline", true)
	time.Sleep(500 * time.Millisecond)
	if err := sys.Power(cfg.SleepAct); err != nil {
		logf("nepovedlo se: %v", err)
		_ = a.link.pub(cfg.topic("status"), "online", true)
	}
}

func (a *agent) setSleepIn(s string) { a.mu.Lock(); a.sleepIn = s; a.mu.Unlock() }

func (a *agent) publishSession(force bool) {
	a.pubMu.Lock()
	defer a.pubMu.Unlock()
	if a.dropboxAt.IsZero() || time.Since(a.dropboxAt) > 2*time.Minute {
		a.dropbox, a.dropboxAt = sys.DropboxState(), time.Now() // reading the pause state takes a moment – not every 5 s
	}
	lz := T("neběží")
	if sys.ProcRunning("Leonzilla.exe") {
		lz = T("běží")
		if leonzillaActivity().Busy {
			lz = T("pracuje")
		}
	}
	a.mu.Lock()
	m := map[string]any{"remote": a.st.Remote, "leonzilla": lz, "dropbox": T(a.dropbox),
		"idle": sys.IdleSeconds() / 60, "sleepIn": a.sleepIn}
	if a.sleepIn == "" {
		m["sleepIn"] = "—"
	}
	a.mu.Unlock()
	b, _ := json.Marshal(m)
	if !force && string(b) == a.lastJSON && time.Since(a.lastPub) < time.Minute {
		return
	}
	if a.link.pub(a.cfg.topic("session"), b, true) == nil {
		a.lastJSON, a.lastPub = string(b), time.Now()
	}
}
