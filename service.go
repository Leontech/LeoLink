package main

// The service: LeoLink.exe --service, a scheduled task "at system startup" running as SYSTEM. It runs all the time,
// also when nobody is signed in, and survives sleep and the "fast startup" shut-down (session 0 is hibernated, not
// ended). It
//   - shows the PC in Home Assistant (online / offline, signed-in user, disks, CPU, RAM) and announces the entities,
//   - executes sleep / shut down / restart, the "keep awake" switch and the power-plan selects,
//   - handles "wake in Leonzilla mode" when nobody is signed in: prepares ONE automatic sign-in and restarts the PC,
//   - switches the automatic sign-in off again as soon as somebody is signed in (or after 10 minutes at the latest),
//     so a normal start keeps asking for the PIN.

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type service struct {
	mu       sync.Mutex
	stMu     sync.Mutex // st: main loop and MQTT handlers
	cfg      *Config
	cfgStamp time.Time
	st       *State
	link     *Link
	awake    bool
	wakeAt   time.Time // a wake request is waiting for the agent (somebody is signed in)
	lastUser string
	lastSys  time.Time
}

func runService() {
	s := &service{st: loadState()}
	s.cfg, s.cfgStamp = loadConfig(), configStamp() // (sets the language of the log)
	logf("LeoLink %s – služba startuje", appVersion)
	s.checkPending()
	s.connect()
	last := time.Now().Round(0)
	for range time.Tick(5 * time.Second) {
		now := time.Now().Round(0)
		if gap := now.Sub(last); gap > 30*time.Second {
			// the PC slept (or "fast startup" resumed it) – the TCP connection is dead, reconnect at once
			logf("PC se probudilo (pauza %s) – znovu se připojuji", gap.Round(time.Second))
			s.reconnect()
		}
		last = now
		if st := configStamp(); !st.Equal(s.cfgStamp) {
			nc := loadConfig()
			logf("nastavení se změnilo – načítám znovu")
			s.mu.Lock()
			old := s.cfg
			s.cfg, s.cfgStamp = nc, st
			s.mu.Unlock()
			if nc.sameLink(old) {
				s.link.cfg = nc
				go func() { // names (language, device name) may have changed
					s.link.publishDiscovery(sys.Disks(), sys.SleepTimeout(), sys.ScreenTimeout())
					s.publishSys()
				}()
			} else {
				s.reconnect()
			}
		}
		s.checkPending()
		s.checkWaitingWake()
		user := sys.LoggedOnUser()
		if user != s.lastUser || time.Since(s.lastSys) > time.Minute {
			s.lastUser = user
			s.publishSys()
		}
		writeHeartbeat(filepath.Join(dataDir(), "service.json"), s.link)
	}
}

func configStamp() time.Time {
	st, err := os.Stat(filepath.Join(dataDir(), "config.json"))
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

func (s *service) connect() {
	c := s.cfg
	s.link = dial(c, "service", map[string]handler{
		c.topic("cmd"):        s.onCmd,
		c.topic("wake"):       s.onWake,
		c.topic("set/awake"):  s.onAwake,
		c.topic("set/sleep"):  s.onTimeout(false),
		c.topic("set/screen"): s.onTimeout(true),
	}, func(l *Link) {
		l.publishDiscovery(sys.Disks(), sys.SleepTimeout(), sys.ScreenTimeout())
		s.publishSys()
	})
}

func (s *service) reconnect() {
	s.link.close(false)
	s.connect()
}

func (s *service) publishSys() {
	disks := map[string]float64{}
	for _, d := range sys.Disks() {
		disks[d.Name] = math.Round(d.FreeGB*10) / 10
	}
	s.mu.Lock()
	awake := s.awake
	s.mu.Unlock()
	s.lastSys = time.Now()
	_ = s.link.pub(s.cfg.topic("sys"), map[string]any{
		"user": sys.LoggedOnUser(), "boot": sys.BootTime().UTC().Format(time.RFC3339), "disks": disks,
		"cpu": math.Round(sys.CPU()), "ram": math.Round(sys.RAM()), "awake": awake,
		"sleepAfter": fmtTimeout(sys.SleepTimeout()), "screenAfter": fmtTimeout(sys.ScreenTimeout()),
		"pendingLogon": s.pending(), "version": appVersion,
	}, true)
}

func (s *service) pending() bool {
	s.stMu.Lock()
	defer s.stMu.Unlock()
	return s.st.PendingLogon
}

func (s *service) onCmd(_ string, p []byte, _ bool) {
	cmd := strings.ToLower(strings.TrimSpace(string(p)))
	switch cmd {
	case "sleep", "shutdown", "restart":
		switch cmd {
		case "sleep":
			logf("příkaz z Home Assistantu: uspávám PC")
		case "shutdown":
			logf("příkaz z Home Assistantu: vypínám PC")
		default:
			logf("příkaz z Home Assistantu: restartuji PC")
		}
		s.powerAction(cmd)
	default:
		if sys.LoggedOnUser() == "" {
			logf("příkaz %q: nikdo není přihlášený – nemám kde ho provést", cmd)
		}
		// lock, monitor, Dropbox … – the agent in the user's session handles them
	}
}

// powerAction: tell Home Assistant first (the last will would come only after ~30 s), then act.
func (s *service) powerAction(cmd string) {
	_ = s.link.pub(s.cfg.topic("status"), "offline", true)
	time.Sleep(500 * time.Millisecond)
	if err := sys.Power(cmd); err != nil {
		logf("nepovedlo se (%s): %v", cmd, err)
		_ = s.link.pub(s.cfg.topic("status"), "online", true)
	}
}

func (s *service) onAwake(_ string, p []byte, _ bool) {
	on := strings.EqualFold(strings.TrimSpace(string(p)), "ON")
	s.mu.Lock()
	changed := on != s.awake
	s.awake = on
	s.mu.Unlock()
	if changed {
		sys.KeepAwake(on)
		if on {
			logf("Nespát: zapnuto – PC samo neusne, dokud to nevypneš (nebo do restartu)")
		} else {
			logf("Nespát: vypnuto")
		}
	}
	s.publishSys()
}

func (s *service) onTimeout(screen bool) handler {
	return func(_ string, p []byte, _ bool) {
		sec, ok := parseTimeout(string(p))
		if !ok {
			logf("neznámá hodnota %q", p)
			return
		}
		var err error
		if screen {
			err = sys.SetScreenTimeout(sec)
			logf("Vypnout obrazovku po: %s", fmtTimeout(sec))
		} else {
			err = sys.SetSleepTimeout(sec)
			logf("Uspat po: %s", fmtTimeout(sec))
		}
		if err != nil {
			logf("nastavení napájení se nepovedlo změnit: %v", err)
		}
		s.publishSys()
	}
}

// ---------------------------------------------------------------------------
// Leonzilla mode

func (s *service) clearWake() {
	_ = s.link.pub(s.cfg.topic("wake"), "", true)
}

func (s *service) onWake(_ string, p []byte, retained bool) {
	if len(strings.TrimSpace(string(p))) == 0 {
		s.mu.Lock()
		s.wakeAt = time.Time{}
		s.mu.Unlock()
		return
	}
	if !wakeFresh(p, retained) {
		t, _ := wakeTime(p)
		logf("žádost o režim Leonzilla je stará (%s) – mažu ji, nic nedělám", t.Local().Format("2.1. 15:04"))
		s.clearWake()
		return
	}
	if u := sys.LoggedOnUser(); u != "" {
		// somebody is signed in (also a locked or sleeping session) – the agent starts the programs
		s.mu.Lock()
		s.wakeAt = time.Now()
		s.mu.Unlock()
		logf("žádost o režim Leonzilla – %s je přihlášený, předávám agentovi", u)
		return
	}
	s.leonzillaBoot()
}

// checkWaitingWake: the agent did not take the request within 90 s (not running?) – drop it.
func (s *service) checkWaitingWake() {
	s.mu.Lock()
	at := s.wakeAt
	s.mu.Unlock()
	if at.IsZero() || time.Since(at) < 90*time.Second {
		return
	}
	if sys.LoggedOnUser() == "" {
		s.mu.Lock()
		s.wakeAt = time.Time{}
		s.mu.Unlock()
		s.leonzillaBoot()
		return
	}
	logf("agent žádost o režim Leonzilla nepřevzal (běží? – je nainstalovaný?) – mažu ji")
	s.mu.Lock()
	s.wakeAt = time.Time{}
	s.mu.Unlock()
	s.clearWake()
}

func (s *service) leonzillaBoot() {
	c := s.cfg
	if !c.Autologon || c.User == "" || !sys.HasPassword() {
		logf("režim Leonzilla: nikdo není přihlášený a automatické přihlášení není nastavené – PC zůstane na přihlašovací obrazovce")
		s.clearWake()
		return
	}
	s.clearWake()
	if err := sys.SetAutologon(c.User, c.Domain); err != nil {
		logf("režim Leonzilla: automatické přihlášení se nepovedlo připravit: %v", err)
		return
	}
	s.stMu.Lock()
	s.st.PendingLogon, s.st.PendingAt, s.st.RemoteBoot = true, time.Now(), time.Now()
	err := saveState(s.st)
	s.stMu.Unlock()
	if err != nil {
		logf("state.json: %v", err)
	}
	logf("režim Leonzilla: připraveno jedno automatické přihlášení (%s) – restartuji PC", c.User)
	go s.bootRestart()
}

// bootRestart: right after the PC resumes from "fast startup", Windows refuses to restart for a while
// (shutdown.exe exit status 1115 = "a system shutdown is in progress"). Try again every 5 s for 3 minutes –
// unless somebody signs in by hand in the meantime (then no restart; the agent starts the programs without locking).
func (s *service) bootRestart() {
	_ = s.link.pub(s.cfg.topic("status"), "offline", true)
	time.Sleep(500 * time.Millisecond)
	for i := 0; ; i++ {
		err := sys.Power("restart")
		if err == nil {
			return
		}
		if i == 0 {
			logf("restart zatím nejde (%v) – zkouším znovu každých 5 s", err)
		}
		time.Sleep(5 * time.Second)
		if u := sys.LoggedOnUser(); u != "" {
			logf("%s se mezitím přihlásil ručně – restart ruším", u) // the agent starts the programs without locking
			break
		}
		if i >= 36 {
			logf("restart se nepovedl ani po 3 minutách: %v", err)
			break
		}
	}
	_ = s.link.pub(s.cfg.topic("status"), "online", true)
}

// checkPending switches the automatic sign-in off once it has been used – or after 10 minutes, if it did not work.
func (s *service) checkPending() {
	s.stMu.Lock()
	defer s.stMu.Unlock()
	if !s.st.PendingLogon {
		return
	}
	switch {
	case sys.LoggedOnUser() != "":
		logf("automatické přihlášení proběhlo – vypínám ho (příští normální start zase s PINem)")
	case time.Since(s.st.PendingAt) > 10*time.Minute:
		logf("automatické přihlášení se do 10 minut nepovedlo – vypínám ho (zkontroluj účet a heslo v LeoLink)")
	default:
		return
	}
	if err := sys.ClearAutologon(); err != nil {
		logf("vypnutí automatického přihlášení: %v", err)
	}
	s.st.PendingLogon = false
	_ = saveState(s.st)
	go s.publishSys()
}

// ---------------------------------------------------------------------------

type heartbeat struct {
	PID       int       `json:"pid"`
	At        time.Time `json:"at"`
	Connected bool      `json:"connected"`
	Version   string    `json:"version"`
}

func writeHeartbeat(p string, l *Link) {
	b, _ := json.Marshal(heartbeat{os.Getpid(), time.Now(), l != nil && l.Up(), appVersion})
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = writeFileAtomic(p, b)
}

func readHeartbeat(p string) *heartbeat {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var h heartbeat
	if json.Unmarshal(b, &h) != nil || time.Since(h.At) > 2*time.Minute {
		return nil
	}
	return &h
}
