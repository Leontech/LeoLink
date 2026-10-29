package main

// Connection to the MQTT broker (Mosquitto in Home Assistant) and the Home Assistant MQTT discovery:
// the service announces the PC as a device with buttons, sensors, a switch and selects, so nothing has to be
// written into configuration.yaml. Topics:
//
//	leolink/<node>/status   online / offline (retained; last will of the service – the PC is on / off)
//	leolink/<node>/agent    online / offline (retained; last will of the agent – somebody is signed in)
//	leolink/<node>/sys      JSON from the service (signed-in user, boot time, disks, CPU, RAM, power settings)
//	leolink/<node>/session  JSON from the agent (Leonzilla mode, Leonzilla, Dropbox, idle time, auto sleep)
//	leolink/<node>/cmd      commands from the buttons (sleep, shutdown, lock, monitor_off …)
//	leolink/<node>/wake     "wake in Leonzilla mode" from the Home Assistant script (retained, waits for the PC)
//	leolink/<node>/set/…    switch / selects (keep awake, sleep after, screen off after)
//	leolink/<node>/notify   a message shown on the PC screen

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type handler func(topic string, payload []byte, retained bool)

type Link struct {
	cfg    *Config
	role   string // "service" | "agent" | "gui"
	c      mqtt.Client
	mu     sync.Mutex
	up     bool
	closed bool // replaced by a new connection (after sleep) – its late callbacks do nothing
}

func (c *Config) brokerURL() string {
	scheme := "tcp"
	if c.TLS {
		scheme = "ssl"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, c.Broker, c.Port)
}

func (c *Config) mqttOptions(clientID string) *mqtt.ClientOptions {
	o := mqtt.NewClientOptions().AddBroker(c.brokerURL()).SetClientID(clientID).
		SetUsername(c.MqUser).SetPassword(c.mqPass()).
		SetKeepAlive(20 * time.Second).SetPingTimeout(10 * time.Second).SetConnectTimeout(10 * time.Second).
		SetCleanSession(true).SetOrderMatters(false).SetWriteTimeout(10 * time.Second)
	if c.TLS {
		o.SetTLSConfig(&tls.Config{InsecureSkipVerify: true}) // a broker in the home network usually has a self-signed certificate
	}
	return o
}

// dial connects in the background and keeps reconnecting. onConnect runs after every (re)connection.
func dial(cfg *Config, role string, subs map[string]handler, onConnect func(*Link)) *Link {
	l := &Link{cfg: cfg, role: role}
	o := cfg.mqttOptions("leolink-" + cfg.Node + "-" + role).
		SetAutoReconnect(true).SetConnectRetry(true).SetConnectRetryInterval(10 * time.Second).SetMaxReconnectInterval(30 * time.Second)
	will := cfg.topic(role)
	if role == "service" {
		will = cfg.topic("status")
	}
	o.SetWill(will, "offline", 1, true)
	o.SetOnConnectHandler(func(c mqtt.Client) {
		if l.isClosed() {
			return // the old connection reconnected by itself just before it was replaced
		}
		l.setUp(true)
		logf("MQTT připojeno (%s)", cfg.brokerURL())
		l.pub(will, "online", true)
		for t, h := range subs {
			h := h
			c.Subscribe(t, 1, func(_ mqtt.Client, m mqtt.Message) { h(m.Topic(), m.Payload(), m.Retained()) })
		}
		if onConnect != nil {
			go func() {
				if !l.isClosed() {
					onConnect(l)
				}
			}()
		}
	})
	o.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		l.setUp(false)
		if l.isClosed() {
			return
		}
		logf("MQTT spojení ztraceno: %v", err)
	})
	l.c = mqtt.NewClient(o)
	if cfg.Broker == "" {
		logf("MQTT broker není nastavený – otevři LeoLink a vyplň Home Assistant / MQTT")
		return l
	}
	l.c.Connect() // ConnectRetry: keeps trying in the background
	return l
}

func (l *Link) isClosed() bool { l.mu.Lock(); defer l.mu.Unlock(); return l.closed }

func (l *Link) setUp(v bool) { l.mu.Lock(); l.up = v; l.mu.Unlock() }
func (l *Link) Up() bool     { l.mu.Lock(); defer l.mu.Unlock(); return l.up }

// pub publishes and waits (max 5 s). v: string, []byte or anything for JSON.
func (l *Link) pub(topic string, v any, retain bool) error {
	if l == nil || l.c == nil || !l.c.IsConnectionOpen() {
		return errors.New(T("MQTT není připojené"))
	}
	var payload []byte
	switch x := v.(type) {
	case string:
		payload = []byte(x)
	case []byte:
		payload = x
	default:
		payload, _ = json.Marshal(x)
	}
	t := l.c.Publish(topic, 1, retain, payload)
	if !t.WaitTimeout(5 * time.Second) {
		return errors.New(T("MQTT: odeslání nedoběhlo"))
	}
	return t.Error()
}

func (l *Link) close(offline bool) {
	if l == nil || l.c == nil {
		return
	}
	if offline && l.c.IsConnectionOpen() {
		t := l.cfg.topic(l.role)
		if l.role == "service" {
			t = l.cfg.topic("status")
		}
		_ = l.pub(t, "offline", true)
	}
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	l.c.Disconnect(300)
}

// testBroker: one connection attempt (settings window, "Vyzkoušet").
func testBroker(cfg *Config) error {
	if cfg.Broker == "" {
		return errors.New(T("vyplň adresu brokeru"))
	}
	c := mqtt.NewClient(cfg.mqttOptions(fmt.Sprintf("leolink-%s-test-%d", cfg.Node, time.Now().UnixNano()%100000)))
	t := c.Connect()
	if !t.WaitTimeout(12 * time.Second) {
		return errors.New(T("broker neodpověděl do 12 s – zkontroluj adresu a port"))
	}
	if err := t.Error(); err != nil {
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "not authorized") || strings.Contains(strings.ToLower(msg), "bad user name") {
			return errors.New(T("broker odmítl jméno nebo heslo"))
		}
		return err
	}
	c.Disconnect(100)
	return nil
}

// publishOnce: a single message from the settings window (test buttons).
func publishOnce(cfg *Config, topic string, payload string, retain bool) error {
	c := mqtt.NewClient(cfg.mqttOptions(fmt.Sprintf("leolink-%s-gui-%d", cfg.Node, time.Now().UnixNano()%100000)))
	t := c.Connect()
	if !t.WaitTimeout(12*time.Second) || t.Error() != nil {
		if t.Error() != nil {
			return t.Error()
		}
		return errors.New(T("broker neodpověděl"))
	}
	defer c.Disconnect(200)
	p := c.Publish(topic, 1, retain, payload)
	p.WaitTimeout(5 * time.Second)
	return p.Error()
}

// ---------------------------------------------------------------------------
// "wake in Leonzilla mode": the Home Assistant script publishes the current time (Unix seconds) retained,
// so it waits on the broker while the PC is off. Only a fresh request counts (±15 min) – a request whose
// Wake-on-LAN got lost must not sign the user in automatically days later, on a normal start.

const wakeMaxAge = 15 * time.Minute

func wakeTime(p []byte) (time.Time, bool) {
	s := strings.Trim(strings.TrimSpace(string(p)), `"`)
	if n, err := strconv.ParseFloat(s, 64); err == nil && n > 1e9 {
		return time.Unix(int64(n), 0), true
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	var j struct {
		T json.RawMessage `json:"t"`
	}
	if json.Unmarshal(p, &j) == nil && len(j.T) > 0 {
		return wakeTime(j.T)
	}
	return time.Time{}, false
}

// wakeFresh: a message delivered live counts always; a retained one only with a recent time.
func wakeFresh(p []byte, retained bool) bool {
	t, ok := wakeTime(p)
	if !ok {
		return !retained
	}
	d := time.Since(t)
	return d < wakeMaxAge && d > -wakeMaxAge
}

// ---------------------------------------------------------------------------
// Home Assistant MQTT discovery

type entity struct {
	comp, id string // component (button, sensor …), object id
	cfg      map[string]any
}

// power-setting options of the selects (seconds; 0 = never)
var timeoutOptions = []int{0, 5 * 60, 10 * 60, 15 * 60, 30 * 60, 3600, 2 * 3600, 3 * 3600, 5 * 3600}

func fmtTimeout(sec int) string {
	switch {
	case sec <= 0:
		return T("nikdy")
	case sec%3600 == 0:
		return fmt.Sprintf("%d h", sec/3600)
	case sec%60 == 0:
		return fmt.Sprintf("%d min", sec/60)
	}
	return fmt.Sprintf("%d s", sec)
}

func parseTimeout(s string) (int, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "nikdy" || s == "never" || s == "0" || s == strings.ToLower(T("nikdy")) {
		return 0, true
	}
	f := strings.Fields(s)
	if len(f) != 2 {
		return 0, false
	}
	n, err := strconv.Atoi(f[0])
	if err != nil || n < 0 {
		return 0, false
	}
	switch f[1] {
	case "h":
		return n * 3600, true
	case "min":
		return n * 60, true
	case "s":
		return n, true
	}
	return 0, false
}

func timeoutChoices(current int) []string {
	out := []string{}
	found := false
	for _, s := range timeoutOptions {
		out = append(out, fmtTimeout(s))
		found = found || s == current
	}
	if !found && current > 0 {
		out = append(out, fmtTimeout(current))
	}
	return out
}

func (c *Config) entities(disks []Disk, sleepSec, screenSec int) []entity {
	st, ag := c.topic("status"), c.topic("agent")
	availSvc := map[string]any{"availability": []map[string]any{{"topic": st}}}
	availAg := map[string]any{"availability": []map[string]any{{"topic": st}, {"topic": ag}}, "availability_mode": "all"}
	with := func(m map[string]any, avail map[string]any) map[string]any {
		for k, v := range avail {
			m[k] = v
		}
		return m
	}
	sysv := func(name, tpl, icon string) map[string]any {
		return with(map[string]any{"name": name, "state_topic": c.topic("sys"), "value_template": tpl, "icon": icon}, availSvc)
	}
	sesv := func(name, tpl, icon string) map[string]any {
		return with(map[string]any{"name": name, "state_topic": c.topic("session"), "value_template": tpl, "icon": icon}, availAg)
	}
	btn := func(name, payload, icon string, svc bool) map[string]any {
		a := availAg
		if svc {
			a = availSvc
		}
		return with(map[string]any{"name": name, "command_topic": c.topic("cmd"), "payload_press": payload, "icon": icon}, a)
	}
	es := []entity{
		{"binary_sensor", "zapnuto", map[string]any{"name": "Zapnuto", "state_topic": st, "payload_on": "online", "payload_off": "offline", "device_class": "power"}},
		{"sensor", "uzivatel", sysv("Přihlášený uživatel", "{{ value_json.user if value_json.user else '"+T("nikdo")+"' }}", "mdi:account")},
		{"sensor", "spusteno", with(map[string]any{"name": "Spuštěno", "state_topic": c.topic("sys"), "value_template": "{{ value_json.boot }}", "device_class": "timestamp"}, availSvc)},
		{"sensor", "cpu", with(map[string]any{"name": "CPU", "state_topic": c.topic("sys"), "value_template": "{{ value_json.cpu }}", "unit_of_measurement": "%", "state_class": "measurement", "icon": "mdi:cpu-64-bit"}, availSvc)},
		{"sensor", "ram", with(map[string]any{"name": "RAM", "state_topic": c.topic("sys"), "value_template": "{{ value_json.ram }}", "unit_of_measurement": "%", "state_class": "measurement", "icon": "mdi:memory"}, availSvc)},
		{"binary_sensor", "rezim_leonzilla", sesv("Režim Leonzilla", "{{ 'ON' if value_json.remote else 'OFF' }}", "mdi:cellphone-link")},
		{"sensor", "leonzilla", sesv("Leonzilla", "{{ value_json.leonzilla }}", "mdi:application-cog")},
		{"sensor", "dropbox", sesv("Dropbox", "{{ value_json.dropbox }}", "mdi:dropbox")},
		{"sensor", "necinnost", with(map[string]any{"name": "Nečinnost", "state_topic": c.topic("session"), "value_template": "{{ value_json.idle }}", "unit_of_measurement": "min", "state_class": "measurement", "icon": "mdi:timer-sand"}, availAg)},
		{"sensor", "automaticke_uspani", sesv("Automatické uspání", "{{ value_json.sleepIn }}", "mdi:sleep")},

		{"button", "uspat", btn("Uspat", "sleep", "mdi:sleep", true)},
		{"button", "vypnout", btn("Vypnout", "shutdown", "mdi:power", true)},
		{"button", "restartovat", btn("Restartovat", "restart", "mdi:restart", true)},
		{"button", "zamknout", btn("Zamknout", "lock", "mdi:lock", false)},
		{"button", "monitor_vypnout", btn("Monitor vypnout", "monitor_off", "mdi:monitor-off", false)},
		{"button", "monitor_zapnout", btn("Monitor zapnout", "monitor_on", "mdi:monitor", false)},
		{"button", "ztlumit", btn("Ztlumit / zapnout zvuk", "mute", "mdi:volume-mute", false)},
		{"button", "dropbox_vypnout", btn("Dropbox vypnout", "dropbox_off", "mdi:dropbox", false)},
		{"button", "dropbox_zapnout", btn("Dropbox zapnout", "dropbox_on", "mdi:dropbox", false)},
		{"button", "rezim_leonzilla_zapnout", btn("Režim Leonzilla zapnout", "leonzilla", "mdi:cellphone-link", false)},
		{"button", "rezim_leonzilla_ukoncit", btn("Režim Leonzilla ukončit", "normal", "mdi:cellphone-link-off", false)},

		{"switch", "nespat", with(map[string]any{"name": "Nespát", "command_topic": c.topic("set/awake"), "state_topic": c.topic("sys"),
			"value_template": "{{ 'ON' if value_json.awake else 'OFF' }}", "icon": "mdi:coffee"}, availSvc)},
		{"select", "uspat_po", with(map[string]any{"name": "Uspat po", "command_topic": c.topic("set/sleep"), "state_topic": c.topic("sys"),
			"value_template": "{{ value_json.sleepAfter }}", "options": timeoutChoices(sleepSec), "icon": "mdi:power-sleep"}, availSvc)},
		{"select", "obrazovka_vypnout_po", with(map[string]any{"name": "Vypnout obrazovku po", "command_topic": c.topic("set/screen"), "state_topic": c.topic("sys"),
			"value_template": "{{ value_json.screenAfter }}", "options": timeoutChoices(screenSec), "icon": "mdi:monitor-shimmer"}, availSvc)},
		{"notify", "zprava", with(map[string]any{"name": "Zpráva na obrazovku", "command_topic": c.topic("notify"), "icon": "mdi:message-alert"}, availAg)},
	}
	for _, d := range disks {
		es = append(es, entity{"sensor", "disk_" + strings.ToLower(d.Name), with(map[string]any{
			"name": fmt.Sprintf(T("Volné místo %s:"), d.Name), "state_topic": c.topic("sys"), "value_template": "{{ value_json.disks." + d.Name + " }}",
			"unit_of_measurement": "GB", "state_class": "measurement", "icon": "mdi:harddisk"}, availSvc)})
	}
	dev := map[string]any{"identifiers": []string{"leolink_" + c.Node}, "name": c.Name, "manufacturer": "LEOTECH", "model": "LeoLink", "sw_version": appVersion}
	origin := map[string]any{"name": "LeoLink", "sw_version": appVersion, "support_url": "https://leotech.cz"}
	for i := range es {
		if n, ok := es[i].cfg["name"].(string); ok {
			es[i].cfg["name"] = T(n)
		}
		es[i].cfg["unique_id"] = "leolink_" + c.Node + "_" + es[i].id
		es[i].cfg["device"] = dev
		es[i].cfg["origin"] = origin
	}
	return es
}

func discoveryTopic(c *Config, e entity) string {
	return "homeassistant/" + e.comp + "/leolink_" + c.Node + "/" + e.id + "/config"
}

func (l *Link) publishDiscovery(disks []Disk, sleepSec, screenSec int) {
	for _, e := range l.cfg.entities(disks, sleepSec, screenSec) {
		if err := l.pub(discoveryTopic(l.cfg, e), e.cfg, true); err != nil {
			if !l.isClosed() {
				logf("discovery %s: %v", e.id, err)
			}
			return
		}
	}
}

// removeDiscovery: the entities disappear from Home Assistant (uninstall).
func removeDiscovery(cfg *Config) error {
	c := mqtt.NewClient(cfg.mqttOptions(fmt.Sprintf("leolink-%s-gui-%d", cfg.Node, time.Now().UnixNano()%100000)))
	t := c.Connect()
	if !t.WaitTimeout(12*time.Second) || t.Error() != nil {
		return errors.New(T("broker není dostupný – entity v Home Assistantu zůstanou (smaž zařízení ručně)"))
	}
	defer c.Disconnect(200)
	disks := []Disk{}
	for _, l := range "CDEFGHIJ" {
		disks = append(disks, Disk{Name: string(l)})
	}
	for _, e := range cfg.entities(disks, 0, 0) {
		c.Publish(discoveryTopic(cfg, e), 1, true, "").WaitTimeout(3 * time.Second)
	}
	for _, t := range []string{"status", "agent", "sys", "session", "wake"} {
		c.Publish(cfg.topic(t), 1, true, "").WaitTimeout(3 * time.Second)
	}
	return nil
}
