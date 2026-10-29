package main

// Settings window: a small web UI (ui/) served on 127.0.0.1 with a random port and token, shown in WebView2.
// Runs as administrator (LSA secret, scheduled tasks, Program Files).

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed ui
var uiFS embed.FS

type gui struct {
	mu    sync.Mutex
	token string
	cfg   *Config
	first bool // no config.json yet – nothing saved
}

func runGUI() {
	if !isElevated() {
		if relaunchElevated() {
			return
		}
		fatalBox(T("LeoLink potřebuje oprávnění správce (heslo pro automatické přihlášení, naplánované úlohy)."))
		return
	}
	g := &gui{}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	g.token = hex.EncodeToString(b)
	_, err := os.Stat(filepath.Join(dataDir(), "config.json"))
	g.first = err != nil
	g.cfg = loadConfig()
	if g.first {
		acc := accountInfo()
		g.cfg.User, g.cfg.Domain = acc.User, acc.Domain
		g.cfg.Apps = detectApps()
	}
	port := os.Getenv("LEOLINK_PORT")
	if port == "" {
		port = "0"
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		fatalBox("LeoLink: " + err.Error())
		return
	}
	go (&http.Server{Handler: g.handler(ln.Addr().String()), ReadHeaderTimeout: 10 * time.Second}).Serve(ln)
	url := fmt.Sprintf("http://%s/?t=%s", ln.Addr().String(), g.token)
	if os.Getenv("LEOLINK_HEADLESS") == "1" {
		fmt.Println(url)
		select {}
	}
	openWindow(url)
}

func (g *gui) handler(host string) http.Handler {
	sub, _ := fs.Sub(uiFS, "ui")
	files := http.FileServer(http.FS(sub))
	api := map[string]func(*http.Request) (any, error){
		"GET /api/state":         g.hState,
		"POST /api/config":       g.hConfig,
		"POST /api/mqtt/test":    g.hMqttTest,
		"POST /api/password":     g.hPassword,
		"POST /api/passwordless": g.hPasswordless,
		"POST /api/install":      g.hInstall,
		"POST /api/uninstall":    g.hUninstall,
		"POST /api/cmd":          g.hCmd,
		"GET /api/ha":            g.hHA,
		"GET /api/log":           g.hLog,
		"POST /api/pick":         g.hPick,
		"POST /api/lang":         g.hLang,
		"POST /api/openurl":      g.hOpenURL,
		"GET /api/apps/detect":   func(*http.Request) (any, error) { return detectApps(), nil },
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host { // DNS rebinding
			http.Error(w, "forbidden", 403)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			files.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("X-Token") != g.token {
			http.Error(w, "forbidden", 403)
			return
		}
		h := api[r.Method+" "+r.URL.Path]
		if h == nil {
			http.NotFound(w, r)
			return
		}
		res, err := h(r)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err != nil {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(res)
	})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v)
}

// ---------------------------------------------------------------------------

type nic struct {
	Name  string `json:"name"`
	MAC   string `json:"mac"`
	IP    string `json:"ip"`
	Bcast string `json:"bcast"`
}

// nics: network adapters usable for Wake-on-LAN (up, IPv4 in a private network, not virtual / VPN).
var nics = realNics

func realNics() []nic {
	var out []nic
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		low := strings.ToLower(i.Name)
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 || len(i.HardwareAddr) != 6 ||
			strings.Contains(low, "wireguard") || strings.Contains(low, "vethernet") || strings.Contains(low, "virtual") ||
			strings.Contains(low, "vmware") || strings.Contains(low, "bluetooth") || strings.Contains(low, "tailscale") {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil || !ipn.IP.IsPrivate() {
				continue
			}
			ip4 := ipn.IP.To4()
			bc := make(net.IP, 4)
			for k := 0; k < 4; k++ {
				bc[k] = ip4[k] | ^ipn.Mask[len(ipn.Mask)-4+k]
			}
			out = append(out, nic{i.Name, strings.ToUpper(i.HardwareAddr.String()), ip4.String(), bc.String()})
			break
		}
	}
	sort.SliceStable(out, func(a, b int) bool { // wired first
		return strings.Contains(strings.ToLower(out[a].Name), "ethernet") && !strings.Contains(strings.ToLower(out[b].Name), "ethernet")
	})
	return out
}

func (g *gui) hState(*http.Request) (any, error) {
	g.mu.Lock()
	c := *g.cfg
	first := g.first
	g.mu.Unlock()
	has := c.PassEnc != ""
	c.PassEnc = ""
	self, _ := os.Executable()
	inst := installInfo()
	return map[string]any{
		"version": appVersion, "cfg": c, "hasMqPass": has, "first": first, "lang": lang(),
		"hasPassword": sys.HasPassword(), "autologonActive": sys.AutologonActive(),
		"passwordless": passwordlessRequired(), "account": accountInfo(), "hostname": sys.Hostname(),
		"nics": nics(), "install": inst, "self": self,
		"service":    readHeartbeat(filepath.Join(dataDir(), "service.json")),
		"agent":      readHeartbeat(filepath.Join(userDir(), "agent_alive.json")),
		"pending":    loadState().PendingLogon,
		"lzActivity": fileExists(filepath.Join(os.Getenv("LOCALAPPDATA"), "Leonzilla", "activity.json")),
	}, nil
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

type cfgIn struct {
	Config
	MqPass      string `json:"mqPassNew"`
	MqPassClear bool   `json:"mqPassClear"`
}

// merged: settings from the window on top of the saved ones (the MQTT password stays unless a new one comes).
func (g *gui) merged(r *http.Request) (*Config, error) {
	var in cfgIn
	if err := readJSON(r, &in); err != nil {
		return nil, err
	}
	g.mu.Lock()
	old := *g.cfg
	g.mu.Unlock()
	c := in.Config
	c.PassEnc = old.PassEnc
	if in.MqPassClear {
		c.PassEnc = ""
	}
	if in.MqPass != "" {
		if err := c.setMqPass(in.MqPass); err != nil {
			return nil, err
		}
	}
	apps := []App{}
	for _, a := range c.Apps {
		a.Name, a.Path, a.Args = strings.TrimSpace(a.Name), strings.TrimSpace(a.Path), strings.TrimSpace(a.Args)
		if a.Path == "" && a.Name == "" {
			continue
		}
		if a.Name == "" {
			a.Name = strings.TrimSuffix(filepath.Base(a.Path), filepath.Ext(a.Path))
		}
		apps = append(apps, a)
	}
	c.Apps = apps
	c.normalize()
	return &c, nil
}

func (g *gui) hConfig(r *http.Request) (any, error) {
	c, err := g.merged(r)
	if err != nil {
		return nil, err
	}
	if c.Autologon && c.User == "" {
		return nil, errors.New(T("vyplň uživatele pro automatické přihlášení"))
	}
	if err := saveConfig(c); err != nil {
		return nil, err
	}
	g.mu.Lock()
	g.cfg, g.first = c, false
	g.mu.Unlock()
	logf("nastavení uloženo")
	return g.hState(r)
}

func (g *gui) hMqttTest(r *http.Request) (any, error) {
	c, err := g.merged(r)
	if err != nil {
		return nil, err
	}
	if err := testBroker(c); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func (g *gui) hPassword(r *http.Request) (any, error) {
	var in struct {
		User, Domain, Pass string
		Clear              bool
	}
	if err := readJSON(r, &in); err != nil {
		return nil, err
	}
	if in.Clear {
		if err := sys.StorePassword(""); err != nil {
			return nil, err
		}
		logf("heslo pro automatické přihlášení smazáno")
		return g.hState(r)
	}
	if strings.TrimSpace(in.User) == "" || in.Pass == "" {
		return nil, errors.New(T("vyplň uživatele a heslo"))
	}
	if err := sys.VerifyPassword(strings.TrimSpace(in.User), strings.TrimSpace(in.Domain), in.Pass); err != nil {
		return nil, fmt.Errorf(T("Windows heslo nepřijal: %v"), err)
	}
	if err := sys.StorePassword(in.Pass); err != nil {
		return nil, fmt.Errorf(T("heslo nejde uložit: %v"), err)
	}
	logf("heslo pro automatické přihlášení ověřeno a uloženo (%s)", in.User)
	return g.hState(r)
}

func (g *gui) hPasswordless(r *http.Request) (any, error) {
	if err := setPasswordlessOff(); err != nil {
		return nil, err
	}
	logf("vypnuto „Vyžadovat přihlášení přes Windows Hello pro účty Microsoft“")
	return g.hState(r)
}

func (g *gui) hInstall(r *http.Request) (any, error) {
	g.mu.Lock()
	c := *g.cfg
	first := g.first
	g.mu.Unlock()
	if first {
		if err := saveConfig(&c); err != nil {
			return nil, err
		}
		g.mu.Lock()
		g.first = false
		g.mu.Unlock()
	}
	if err := doInstall(); err != nil {
		logf("instalace: %v", err)
		return nil, err
	}
	logf("nainstalováno %s", appVersion)
	return g.hState(r)
}

func (g *gui) hUninstall(r *http.Request) (any, error) {
	g.mu.Lock()
	c := *g.cfg
	g.mu.Unlock()
	var warn []string
	if c.Broker != "" {
		if err := removeDiscovery(&c); err != nil {
			warn = append(warn, err.Error())
		}
	}
	if err := doUninstall(); err != nil {
		return nil, err
	}
	if loadState().PendingLogon || sys.AutologonActive() && loadState().RemoteBoot.After(time.Now().Add(-time.Hour)) {
		_ = sys.ClearAutologon()
	}
	_ = sys.StorePassword("")
	logf("odinstalováno")
	st, _ := g.hState(r)
	m := st.(map[string]any)
	m["warn"] = strings.Join(warn, "; ")
	return m, nil
}

// hCmd: test buttons – the same message Home Assistant would send.
func (g *gui) hCmd(r *http.Request) (any, error) {
	var in struct{ Cmd string }
	if err := readJSON(r, &in); err != nil {
		return nil, err
	}
	g.mu.Lock()
	c := *g.cfg
	g.mu.Unlock()
	var err error
	if in.Cmd == "wake" {
		err = publishOnce(&c, c.topic("wake"), fmt.Sprint(time.Now().Unix()), true)
	} else {
		err = publishOnce(&c, c.topic("cmd"), in.Cmd, false)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func (g *gui) hLog(*http.Request) (any, error) {
	return map[string]any{"service": logTail(logPath(false), 300), "agent": logTail(logPath(true), 300)}, nil
}

// hLang: the language is saved at once (the window reloads); unsaved edits of other settings stay unsaved.
func (g *gui) hLang(r *http.Request) (any, error) {
	var in struct{ Lang string }
	if err := readJSON(r, &in); err != nil {
		return nil, err
	}
	if in.Lang != "cs" && in.Lang != "en" {
		return nil, errors.New("cs / en")
	}
	g.mu.Lock()
	g.cfg.Lang = in.Lang
	first := g.first
	g.mu.Unlock()
	if !first {
		disk := loadConfig() // (sets the language of the file – set the new one after it)
		disk.Lang = in.Lang
		if err := saveConfig(disk); err != nil {
			return nil, err
		}
	}
	setLang(in.Lang)
	return map[string]any{"ok": true}, nil
}

func (g *gui) hOpenURL(r *http.Request) (any, error) {
	var in struct{ URL string }
	if err := readJSON(r, &in); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(in.URL, "https://") && !strings.HasPrefix(in.URL, "http://") {
		return nil, errors.New("URL")
	}
	return map[string]any{"ok": true}, openURL(in.URL)
}

func (g *gui) hPick(r *http.Request) (any, error) {
	var in struct{ Start string }
	_ = readJSON(r, &in)
	p, err := pickFile(T("Vyber program"), in.Start)
	if err != nil {
		return nil, err
	}
	return map[string]string{"path": p}, nil
}

func (g *gui) hHA(r *http.Request) (any, error) {
	g.mu.Lock()
	c := *g.cfg
	g.mu.Unlock()
	mac, bc := r.URL.Query().Get("mac"), r.URL.Query().Get("bcast")
	return map[string]string{"script": haScript(&c, mac, bc), "card": haCard(&c), "scriptId": "script." + scriptID(&c)}, nil
}

// ---------------------------------------------------------------------------
// Home Assistant snippets

var deaccent = strings.NewReplacer("á", "a", "č", "c", "ď", "d", "é", "e", "ě", "e", "í", "i", "ň", "n", "ó", "o", "ř", "r",
	"š", "s", "ť", "t", "ú", "u", "ů", "u", "ý", "y", "ž", "z", "ä", "a", "ö", "o", "ü", "u", "ß", "ss", "ł", "l", "ś", "s", "ż", "z", "ź", "z", "ą", "a", "ę", "e", "ć", "c", "ń", "n")

// slug like Home Assistant makes entity ids from names.
func slug(s string) string {
	s = deaccent.Replace(strings.ToLower(s))
	var b strings.Builder
	us := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			us = false
		} else if !us {
			b.WriteByte('_')
			us = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func entityID(c *Config, comp, name string) string { return comp + "." + slug(c.Name+" "+T(name)) }

func scriptID(c *Config) string { return "leolink_" + slug(c.Node) + "_leonzilla" }

func haScript(c *Config, mac, bcast string) string {
	if mac == "" {
		mac = "AA:BB:CC:DD:EE:FF"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s:\n  alias: "+T("Probudit %s – režim Leonzilla")+"\n  icon: mdi:cellphone-link\n  mode: single\n  sequence:\n", scriptID(c), c.Name)
	b.WriteString("    # " + T("žádost počká na brokeru, než PC naběhne (LeoLink bere jen 15 min starou)") + "\n")
	fmt.Fprintf(&b, "    - action: mqtt.publish\n      data:\n        topic: %s\n        payload: \"{{ now().timestamp() | int }}\"\n        retain: true\n", c.topic("wake"))
	fmt.Fprintf(&b, "    - action: wake_on_lan.send_magic_packet\n      data:\n        mac: \"%s\"\n", mac)
	if bcast != "" {
		fmt.Fprintf(&b, "        broadcast_address: %s\n", bcast)
	}
	return b.String()
}

func haCard(c *Config) string {
	e := func(comp, name string) string { return entityID(c, comp, name) }
	btn := func(name, icon, ent string) string {
		return fmt.Sprintf("      - type: button\n        name: %s\n        icon: %s\n        tap_action:\n          action: perform-action\n          perform_action: button.press\n          target:\n            entity_id: %s\n", T(name), icon, ent)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "type: vertical-stack\ncards:\n")
	fmt.Fprintf(&b, "  - type: tile\n    entity: %s\n    name: %s\n    state_content:\n      - state\n      - last_changed\n", e("binary_sensor", "Zapnuto"), c.Name)
	fmt.Fprintf(&b, "  - type: grid\n    columns: 2\n    square: false\n    cards:\n")
	fmt.Fprintf(&b, "      - type: button\n        name: %s\n        icon: mdi:cellphone-link\n        tap_action:\n          action: perform-action\n          perform_action: script.%s\n          confirmation:\n            text: %s\n", T("Probudit – režim Leonzilla"), scriptID(c), T("Probudit PC a spustit Leonzillu?"))
	b.WriteString(btn("Uspat", "mdi:sleep", e("button", "Uspat")))
	b.WriteString(btn("Vypnout", "mdi:power", e("button", "Vypnout")))
	b.WriteString(btn("Zamknout", "mdi:lock", e("button", "Zamknout")))
	b.WriteString(btn("Monitor vypnout", "mdi:monitor-off", e("button", "Monitor vypnout")))
	b.WriteString(btn("Monitor zapnout", "mdi:monitor", e("button", "Monitor zapnout")))
	fmt.Fprintf(&b, "  - type: entities\n    entities:\n")
	for _, x := range [][2]string{{"binary_sensor", "Režim Leonzilla"}, {"sensor", "Automatické uspání"}, {"sensor", "Leonzilla"},
		{"sensor", "Dropbox"}, {"switch", "Nespát"}, {"select", "Uspat po"}, {"select", "Vypnout obrazovku po"},
		{"sensor", "Přihlášený uživatel"}, {"sensor", "CPU"}, {"sensor", "RAM"}} {
		fmt.Fprintf(&b, "      - %s\n", e(x[0], x[1]))
	}
	return b.String()
}
