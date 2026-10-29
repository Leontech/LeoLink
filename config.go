package main

// Configuration and state of LeoLink. They live in %ProgramData%\LeoLink (one per PC, not in Dropbox): the service
// task runs as SYSTEM, also when nobody is signed in; the agent runs in the user's session – both read config.json,
// only the settings window (as administrator) writes it.
// The MQTT password is stored encrypted for this computer (DPAPI, machine scope); the Windows password
// for the one-time automatic sign-in is not here at all – it is an LSA secret (see sys_windows.go).

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type App struct {
	On   bool   `json:"on"`
	Name string `json:"name"`
	Path string `json:"path"`
	Args string `json:"args,omitempty"`
	Min  bool   `json:"min"` // start minimized
}

type Config struct {
	// MQTT broker (Mosquitto in Home Assistant) – LeoLink connects to it, nothing on the PC listens.
	Broker  string `json:"broker"` // host or IP, e.g. 192.168.1.10
	Port    int    `json:"port"`
	TLS     bool   `json:"tls,omitempty"`
	MqUser  string `json:"mqUser"`
	PassEnc string `json:"mqPass,omitempty"` // encrypted for this computer (protect())
	Node    string `json:"node"`             // topic name of this PC: leolink/<node>/… (default: computer name)
	Name    string `json:"name"`             // device name in Home Assistant (default: computer name)
	Lang    string `json:"lang,omitempty"`   // "cs" | "en" | "" = by Windows (window, log, entity names in Home Assistant)

	Autologon bool   `json:"autologon"` // Leonzilla mode from a switched-off PC: one automatic sign-in
	User      string `json:"user"`
	Domain    string `json:"domain"`
	LockAfter bool   `json:"lockAfter"` // lock the screen right after the automatic sign-in
	Apps      []App  `json:"apps"`

	AutoSleep bool   `json:"autoSleep"` // after a remote session: sleep / shut down when nothing happens
	SleepMin  int    `json:"sleepMin"`
	SleepAct  string `json:"sleepAct"` // "sleep" | "shutdown"
}

// State is written by the service (SYSTEM) – what is pending around the automatic sign-in.
type State struct {
	PendingLogon bool      `json:"pendingLogon"` // automatic sign-in prepared, not done yet
	PendingAt    time.Time `json:"pendingAt"`
	RemoteBoot   time.Time `json:"remoteBoot"` // restart into Leonzilla mode prepared at this time
}

// AgentState is written by the agent (in the user's session, %LOCALAPPDATA%\LeoLink\agent.json).
type AgentState struct {
	Remote      bool      `json:"remote"` // Leonzilla mode is on (woken / switched on from Home Assistant)
	RemoteSince time.Time `json:"remoteSince"`
	HandledBoot time.Time `json:"handledBoot"` // State.RemoteBoot already handled
}

var cfgMu sync.Mutex

func dataDir() string {
	if d := os.Getenv("LEOLINK_DIR"); d != "" {
		return d
	}
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = os.TempDir()
	}
	return filepath.Join(pd, "LeoLink")
}

func defaultConfig() *Config {
	return &Config{Port: 1883, AutoSleep: true, SleepMin: 30, SleepAct: "sleep", LockAfter: true}
}

func loadConfig() *Config {
	c := defaultConfig()
	if b, err := os.ReadFile(filepath.Join(dataDir(), "config.json")); err == nil {
		_ = json.Unmarshal(b, c)
	}
	c.normalize()
	setLang(c.Lang)
	return c
}

func (c *Config) normalize() {
	c.Broker = strings.TrimSpace(c.Broker)
	c.MqUser = strings.TrimSpace(c.MqUser)
	if c.Port <= 0 || c.Port > 65535 {
		c.Port = 1883
	}
	c.Node = nodeID(c.Node)
	if c.Node == "" {
		c.Node = nodeID(sys.Hostname())
	}
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		c.Name = sys.Hostname()
	}
	c.User, c.Domain = strings.TrimSpace(c.User), strings.TrimSpace(c.Domain)
	if c.SleepMin < 5 {
		c.SleepMin = 5
	}
	if c.SleepAct != "shutdown" {
		c.SleepAct = "sleep"
	}
	if c.Apps == nil {
		c.Apps = []App{}
	}
}

func saveConfig(c *Config) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	_ = os.MkdirAll(dataDir(), 0o755)
	b, _ := json.MarshalIndent(c, "", "  ")
	return writeFileAtomic(filepath.Join(dataDir(), "config.json"), b)
}

// nodeID: a safe MQTT topic / entity name – lower case letters, digits, - and _.
func nodeID(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-_")
}

func (c *Config) mqPass() string {
	if c.PassEnc == "" {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(c.PassEnc)
	if err != nil {
		return ""
	}
	p, err := unprotect(raw)
	if err != nil {
		return ""
	}
	return string(p)
}

func (c *Config) setMqPass(t string) error {
	if t == "" {
		c.PassEnc = ""
		return nil
	}
	enc, err := protect([]byte(t))
	if err != nil {
		return err
	}
	c.PassEnc = base64.StdEncoding.EncodeToString(enc)
	return nil
}

// sameLink: the MQTT connection does not have to be made again (only names, programs … changed).
func (c *Config) sameLink(o *Config) bool {
	return c.Broker == o.Broker && c.Port == o.Port && c.TLS == o.TLS && c.MqUser == o.MqUser && c.PassEnc == o.PassEnc && c.Node == o.Node
}

func (c *Config) topic(sub string) string { return "leolink/" + c.Node + "/" + sub }

func loadState() *State {
	s := &State{}
	if b, err := os.ReadFile(filepath.Join(dataDir(), "state.json")); err == nil {
		_ = json.Unmarshal(b, s)
	}
	return s
}

func saveState(s *State) error {
	_ = os.MkdirAll(dataDir(), 0o755)
	b, _ := json.MarshalIndent(s, "", "  ")
	return writeFileAtomic(filepath.Join(dataDir(), "state.json"), b)
}

func userDir() string {
	if d := os.Getenv("LEOLINK_USERDIR"); d != "" {
		return d
	}
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		local = os.TempDir()
	}
	return filepath.Join(local, "LeoLink")
}

func loadAgentState() *AgentState {
	s := &AgentState{}
	if b, err := os.ReadFile(filepath.Join(userDir(), "agent.json")); err == nil {
		_ = json.Unmarshal(b, s)
	}
	return s
}

func saveAgentState(s *AgentState) error {
	_ = os.MkdirAll(userDir(), 0o755)
	b, _ := json.MarshalIndent(s, "", "  ")
	return writeFileAtomic(filepath.Join(userDir(), "agent.json"), b)
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	var err error
	for i := 0; i < 10; i++ {
		if err = os.Rename(tmp, path); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

// ---------------------------------------------------------------------------
// log: %ProgramData%\LeoLink\leolink.log (the boot task has no window – the log is how you see what happened)

var logMu sync.Mutex

// logPath: the service and the settings window write to %ProgramData%\LeoLink\leolink.log,
// the agent (no administrator rights) to %LOCALAPPDATA%\LeoLink\agent.log.
func logPath(agent bool) string {
	if agent {
		return filepath.Join(userDir(), "agent.log")
	}
	return filepath.Join(dataDir(), "leolink.log")
}

func logf(format string, a ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	p := logPath(runMode == "agent")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if st, err := os.Stat(p); err == nil && st.Size() > 1<<20 {
		_ = os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s [%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), runMode, fmt.Sprintf(T(format), a...))
}

func logTail(p string, n int) []string {
	b, _ := os.ReadFile(p)
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if len(lines) == 1 && lines[0] == "" {
		return []string{}
	}
	return lines
}
