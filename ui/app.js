"use strict";
// LeoLink – settings window

const TOKEN = new URLSearchParams(location.search).get("t") || "";
let S = null;        // last /api/state
let cfg = null;      // edited copy of S.cfg
let saved = "";      // JSON of the saved config (dirty check)
let mqPassNew = "";
let nicIdx = 0;

const $ = (s, el = document) => el.querySelector(s);
const esc = s => String(s ?? "").replace(/[&<>"]/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));

async function api(path, body) {
  const r = await fetch(path, {
    method: body === undefined ? "GET" : "POST",
    headers: { "X-Token": TOKEN, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || r.statusText);
  return j;
}

let toastT;
function toast(msg, err) {
  const t = $("#toast");
  t.textContent = msg;
  t.className = "toast" + (err ? " err" : "");
  t.hidden = false;
  clearTimeout(toastT);
  toastT = setTimeout(() => (t.hidden = true), err ? 7000 : 3500);
}

async function busy(btn, fn) {
  const old = btn.textContent;
  btn.disabled = true;
  try { return await fn(); }
  catch (e) { toast(e.message, true); }
  finally { btn.disabled = false; btn.textContent = old; }
}

// ---------------------------------------------------------------------------

function setState(st, keepEdits) {
  S = st;
  if (!keepEdits || !cfg) {
    cfg = JSON.parse(JSON.stringify(st.cfg));
    saved = JSON.stringify(st.cfg);
    mqPassNew = "";
  }
  render();
}

function dirty() {
  return JSON.stringify(cfg) !== saved || mqPassNew !== "" || (S && S.first);
}

function markDirty() {
  $("#savebar").hidden = !dirty();
}

async function save() {
  const body = Object.assign({}, cfg, { mqPassNew });
  const st = await api("/api/config", body);
  setState(st);
  toast("Uloženo – služba a agent si nastavení načtou do pár sekund.");
}

function bind(el) {
  el.querySelectorAll("[data-k]").forEach(inp => {
    const k = inp.dataset.k;
    const set = () => {
      let v = inp.type === "checkbox" ? inp.checked : inp.value;
      if (inp.type === "number") v = parseInt(v, 10) || 0;
      cfg[k] = v;
      markDirty();
    };
    inp.addEventListener("input", set);
    inp.addEventListener("change", set);
  });
}

// ---------------------------------------------------------------------------

function pills() {
  const svc = S.service, ag = S.agent;
  const p = [];
  if (!S.install.installed) p.push(`<span class="pill warn">Nenainstalováno</span>`);
  else {
    p.push(`<span class="pill ${svc ? "ok" : "bad"}">Služba ${svc ? "běží" : "neběží"}</span>`);
    p.push(`<span class="pill ${svc && svc.connected ? "ok" : "bad"}">MQTT ${svc && svc.connected ? "připojeno" : "nepřipojeno"}</span>`);
    p.push(`<span class="pill ${ag ? "ok" : "bad"}">Agent ${ag ? "běží" : "neběží"}</span>`);
  }
  $("#pills").innerHTML = p.join("");
  $("#ver").textContent = "verze " + S.version;
  $("#lang").value = S.lang;
}

function st(ok, text, cls) {
  return `<span class="st ${cls || (ok ? "ok" : "bad")}">${text}</span>`;
}

function renderOverview() {
  const el = $('[data-pane="overview"]');
  const inst = S.install, svc = S.service, ag = S.agent;
  const newer = inst.installed && inst.version && inst.version !== S.version;
  let instTxt = inst.installed ? st(true, inst.version ? `nainstalováno – verze ${esc(inst.version)}` : "nainstalováno") : st(false, "nenainstalováno", "warn");
  if (newer) instTxt = st(false, `nainstalovaná verze ${esc(inst.version)}, toto je ${esc(S.version)}`, "warn");
  const mqttOk = !!cfg.broker;
  const autologon = cfg.autologon && S.hasPassword;
  el.innerHTML = `
  <div class="card">
    <h2>Stav</h2>
    <div class="rows">
      <span class="k">Instalace</span><span>${instTxt}</span>
      <span class="k">Služba</span><span>${!inst.installed ? st(false, "–", "") : svc ? st(true, `běží${svc.connected ? ", připojená k MQTT" : " – <b>nepřipojená k MQTT</b>"}`, svc.connected ? "ok" : "warn") : st(false, "neběží")}</span>
      <span class="k">Agent</span><span>${!inst.installed ? st(false, "–", "") : ag ? st(true, `běží${ag.connected ? "" : " – nepřipojený k MQTT"}`, ag.connected ? "ok" : "warn") : st(false, "neběží (spouští se při přihlášení)")}</span>
      <span class="k">Home Assistant</span><span>${mqttOk ? st(true, `broker ${esc(cfg.broker)}:${cfg.port}, zařízení <b>${esc(cfg.name)}</b>`) : st(false, "MQTT není nastavené – záložka Home Assistant", "warn")}</span>
      <span class="k">Automatické přihlášení</span><span>${autologon ? st(true, `připravené pro ${esc(cfg.user)} (jen v režimu Leonzilla)`) : st(false, cfg.autologon ? "chybí heslo – záložka Režim Leonzilla" : "vypnuté – z vypnutého PC se režim Leonzilla nespustí", "warn")}</span>
      <span class="k">Leonzilla</span><span>${S.lzActivity ? st(true, "hlásí, co dělá (automatické uspání čeká na běžící úlohy)") : st(false, "zatím nehlásí činnost – automatické uspání potřebuje Leonzillu 1.11.20 a novější", "warn")}</span>
    </div>
    <div class="btns">
      <button class="primary" id="install">${inst.installed ? (newer ? "Aktualizovat na " + esc(S.version) : "Přeinstalovat") : "Nainstalovat"}</button>
    </div>
    ${S.pending ? `<div class="note warn">Automatické přihlášení je právě připravené (čeká se na restart). Po přihlášení nebo do 10 minut se samo vypne.</div>` : ""}
  </div>
  <div class="card">
    <h2>Vyzkoušet</h2>
    <p class="lead">Pošle stejnou zprávu jako tlačítko v Home Assistantu (přes MQTT, služba a agent musí běžet).</p>
    <div class="btns">
      <button class="ghost" data-cmd="wake">Režim Leonzilla teď</button>
      <button class="ghost" data-cmd="normal">Režim Leonzilla ukončit</button>
      <button class="ghost" data-cmd="monitor_off">Vypnout monitory</button>
      <button class="ghost" data-cmd="lock">Zamknout</button>
    </div>
  </div>
  <div class="card">
    <h2>Jak to funguje</h2>
    <ul class="how">
      <li><b>Home Assistant</b> – LeoLink se připojí k MQTT brokeru (Mosquitto) a v HA se samo objeví zařízení <b>${esc(cfg.name)}</b>: tlačítka Uspat, Vypnout, Restartovat, Zamknout, Monitor vyp/zap, Dropbox vyp/zap, přepínač Nespát, volby Uspat po / Vypnout obrazovku po, senzory (zapnuto, uživatel, CPU, RAM, disky, Leonzilla, Dropbox) a zpráva na obrazovku. Do PC se nic neotevírá, LeoLink se jen připojuje ven.</li>
      <li><b>Režim Leonzilla</b> – skript v HA pošle žádost a Wake-on-LAN. Když je PC vypnuté, LeoLink se <b>jednou</b> přihlásí sám (PC se kvůli tomu jednou restartuje), spustí zaškrtnuté programy a zamkne obrazovku. Při normálním startu se dál přihlašuješ PINem.</li>
      <li><b>Když spí</b> s tebou přihlášeným, jen spustí programy (obrazovka zůstane zamčená).</li>
      <li><b>Automatické uspání</b> – v režimu Leonzilla PC usne (nebo se vypne), když se nějakou dobu nic neděje: nikdo u klávesnice, nic z mobilu, žádná běžící úloha Leonzilly. Jakmile někdo sáhne na myš nebo klávesnici, režim Leonzilla končí.</li>
    </ul>
  </div>
  ${inst.installed ? `<div class="danger-zone"><button class="danger" id="uninstall">Odinstalovat…</button></div>` : ""}`;
  $("#install", el).onclick = e => busy(e.target, async () => {
    if (dirty()) await save();
    e.target.textContent = "Instaluji…";
    setState(await api("/api/install", {}), true);
    toast("Nainstalováno – služba a agent běží.");
  });
  const un = $("#uninstall", el);
  if (un) un.onclick = e => {
    if (!confirm("Odinstalovat LeoLink?\n\nZastaví a smaže službu i agenta, smaže uložené heslo pro automatické přihlášení a odebere zařízení z Home Assistantu. Nastavení v ProgramData\\LeoLink zůstane.")) return;
    busy(e.target, async () => {
      const r = await api("/api/uninstall", {});
      setState(r, true);
      toast(r.warn ? "Odinstalováno. " + r.warn : "Odinstalováno.", !!r.warn);
    });
  };
  el.querySelectorAll("[data-cmd]").forEach(b => b.onclick = () => busy(b, async () => {
    await api("/api/cmd", { cmd: b.dataset.cmd });
    toast("Odesláno: " + b.textContent);
  }));
}

function renderHA() {
  const el = $('[data-pane="ha"]');
  const nics = S.nics || [];
  if (nicIdx >= nics.length) nicIdx = 0;
  el.innerHTML = `
  <div class="card">
    <h2>MQTT broker</h2>
    <p class="lead">Stejný broker, jaký používá Home Assistant (doplněk Mosquitto). Uživatel může být ten, kterého HA používá pro MQTT, nebo vlastní.</p>
    <div class="form">
      <label class="k" for="f-broker">Adresa brokeru</label>
      <div class="inline"><input type="text" id="f-broker" data-k="broker" placeholder="např. 192.168.1.10" style="max-width:260px">
        <label class="k" for="f-port">port</label><input type="number" id="f-port" data-k="port">
        <label class="chk"><input type="checkbox" data-k="tls"><span>TLS</span></label></div>
      <label class="k" for="f-user">Uživatel</label><input type="text" id="f-user" data-k="mqUser" style="max-width:260px">
      <label class="k" for="f-pass">Heslo</label>
      <div class="inline"><input type="password" id="f-pass" style="max-width:260px" placeholder="${S.hasMqPass ? "uložené – vyplň jen pro změnu" : ""}" autocomplete="new-password">
        <span class="sub">uloženo zašifrované pro tento počítač</span></div>
      <label class="k" for="f-name">Zařízení v HA</label>
      <div class="inline"><input type="text" id="f-name" data-k="name" style="max-width:260px"><span class="sub">název, pod kterým PC uvidíš v Home Assistantu</span></div>
      <label class="k" for="f-node">Téma MQTT</label>
      <div class="inline"><code>leolink/</code><input type="text" id="f-node" data-k="node" style="max-width:180px"><code>/…</code></div>
    </div>
    <div class="btns"><button class="ghost" id="mqtest">Vyzkoušet připojení</button></div>
  </div>
  <div class="card">
    <h2>Skript „Probudit – režim Leonzilla“</h2>
    <p class="lead">Zapne PC přes Wake-on-LAN a nechá na brokeru žádost o režim Leonzilla. Ta na PC počká, i když je vypnuté (platí 15 minut).</p>
    <div class="form">
      <label class="k" for="f-nic">Síťová karta</label>
      <select id="f-nic" style="max-width:520px">${nics.map((n, i) => `<option value="${i}" ${i === nicIdx ? "selected" : ""}>${esc(n.name)} – ${esc(n.mac)} (${esc(n.ip)})</option>`).join("") || `<option>nenalezena – doplň MAC ve skriptu ručně</option>`}</select>
    </div>
    <ol class="steps">
      <li>V <code>configuration.yaml</code> musí být <code>wake_on_lan:</code> (pokud už PC z HA budíš, je tam).</li>
      <li>Vlož do <code>scripts.yaml</code> (nebo Nastavení → Automatizace a scény → Skripty → Přidat → ⋮ Upravit v YAML, bez prvního řádku) a znovu načti skripty.</li>
    </ol>
    <div class="code"><textarea id="y-script" readonly rows="14"></textarea><button class="ghost" data-copy="y-script">Kopírovat</button></div>
  </div>
  <div class="card">
    <h2>Karta na řídicí panel</h2>
    <p class="lead">Entity se v Home Assistantu objeví samy, jakmile se služba připojí (Nastavení → Zařízení a služby → MQTT → <b>${esc(cfg.name)}</b>). Karta jen poskládá ta nejčastější tlačítka. Řídicí panel → Upravit → Přidat kartu → Ruční.</p>
    <div class="code"><textarea id="y-card" readonly rows="16"></textarea><button class="ghost" data-copy="y-card">Kopírovat</button></div>
  </div>`;
  bind(el);
  $("#f-pass", el).addEventListener("input", e => { mqPassNew = e.target.value; markDirty(); });
  el.querySelectorAll("[data-k]").forEach(i => {
    const v = cfg[i.dataset.k];
    if (i.type === "checkbox") i.checked = !!v; else i.value = v ?? "";
  });
  $("#mqtest", el).onclick = e => busy(e.target, async () => {
    e.target.textContent = "Zkouším…";
    await api("/api/mqtt/test", Object.assign({}, cfg, { mqPassNew }));
    toast("Broker odpověděl, jméno i heslo sedí.");
  });
  $("#f-nic", el).onchange = e => { nicIdx = +e.target.value; loadYAML(); };
  el.querySelectorAll("[data-copy]").forEach(b => b.onclick = async () => {
    const t = $("#" + b.dataset.copy);
    try { await navigator.clipboard.writeText(t.value); } catch { t.select(); document.execCommand("copy"); }
    toast("Zkopírováno");
  });
  loadYAML();
}

async function loadYAML() {
  const n = (S.nics || [])[nicIdx] || {};
  try {
    const y = await api(`/api/ha?mac=${encodeURIComponent(n.mac || "")}&bcast=${encodeURIComponent(n.bcast || "")}`);
    if ($("#y-script")) $("#y-script").value = y.script;
    if ($("#y-card")) $("#y-card").value = y.card;
  } catch (e) { /* not important */ }
}

function renderLZ() {
  const el = $('[data-pane="lz"]');
  const acc = S.account || {};
  el.innerHTML = `
  <div class="card">
    <h2>Automatické přihlášení</h2>
    <p class="lead">Jen pro režim Leonzilla z vypnutého PC (nebo když nikdo není přihlášený). Normální start dál chce PIN. Heslo je uložené jako tajemství LSA (jako u Sysinternals Autologon), nikdy ne čitelně v registru. Windows ho po jednom přihlášení samy zahodí.</p>
    <label class="chk"><input type="checkbox" data-k="autologon"><span>Při žádosti o režim Leonzilla se jednou přihlásit automaticky<small>PC se kvůli tomu jednou restartuje – přihlašovací obrazovka už by jinak běžela.</small></span></label>
    <div class="form" style="margin-top:12px">
      <label class="k" for="f-lu">Uživatel</label>
      <div class="inline"><input type="text" id="f-lu" data-k="user" style="max-width:220px">
        <label class="k" for="f-ld">doména / PC</label><input type="text" id="f-ld" data-k="domain" style="max-width:200px"></div>
      <label class="k" for="f-lp">Heslo k účtu</label>
      <div class="inline"><input type="password" id="f-lp" style="max-width:220px" autocomplete="new-password" placeholder="${S.hasPassword ? "uložené – vyplň jen pro změnu" : ""}">
        <button class="ghost" id="pwsave">Ověřit a uložit</button>
        ${S.hasPassword ? `<button class="plain" id="pwclear">Smazat heslo</button>` : ""}</div>
      <span></span><span>${S.hasPassword ? st(true, "heslo je uložené") : st(false, "heslo zatím není uložené", "warn")}</span>
    </div>
    ${acc.msa ? `<div class="note info">Účet je propojený s účtem Microsoft <b>${esc(acc.msa)}</b> – zadej <b>heslo k účtu Microsoft</b>, ne PIN. Kdyby ho Windows nepřijaly, zkus jako uživatele <code>${esc(acc.msa)}</code> a doménu <code>MicrosoftAccount</code>.</div>` : ""}
    ${S.passwordless ? `<div class="note warn">Je zapnuté <b>„Zvýšit zabezpečení – pro účty Microsoft povolit jen přihlášení přes Windows Hello“</b>. Dokud to platí, Windows heslo při přihlášení nepřijmou a automatické přihlášení nepůjde. PIN zůstane, jen se k němu přidá možnost přihlásit se heslem. <button class="ghost" id="pwless">Vypnout tuto volbu</button></div>` : ""}
    <label class="chk" style="margin-top:12px"><input type="checkbox" data-k="lockAfter"><span>Po automatickém přihlášení hned zamknout obrazovku<small>Programy běží dál, Leonzilla jde ovládat z mobilu. Doma u PC se odemkneš PINem.</small></span></label>
  </div>
  <div class="card">
    <h2>Programy pro režim Leonzilla</h2>
    <p class="lead">Spustí se, když se zapne režim Leonzilla. Co už běží, se znovu nespouští.</p>
    <table class="apps"><thead><tr><th></th><th>Název</th><th>Program</th><th>Parametry</th><th>Minimalizovat</th><th></th></tr></thead><tbody id="apps"></tbody></table>
    <div class="btns"><button class="ghost" id="appadd">+ Přidat program</button><button class="plain" id="appdetect">Najít programy automaticky</button></div>
  </div>
  <div class="card">
    <h2>Automatické uspání</h2>
    <p class="lead">Jen v režimu Leonzilla. Odpočet začíná od zapnutí režimu nebo od posledního požadavku z mobilu na Leonzillu. Při běžící úloze Leonzilly se čeká.</p>
    <label class="chk"><input type="checkbox" data-k="autoSleep"><span>Když se nic neděje</span></label>
    <div class="form" style="margin-top:10px">
      <label class="k" for="f-sm">Po</label>
      <div class="inline"><input type="number" id="f-sm" data-k="sleepMin" min="5" max="600"> minutách
        <select data-k="sleepAct" style="max-width:160px"><option value="sleep">uspat</option><option value="shutdown">vypnout</option></select></div>
    </div>
  </div>`;
  bind(el);
  el.querySelectorAll("[data-k]").forEach(i => {
    const v = cfg[i.dataset.k];
    if (i.type === "checkbox") i.checked = !!v; else i.value = v ?? "";
  });
  $("#pwsave", el).onclick = e => busy(e.target, async () => {
    const pass = $("#f-lp").value;
    e.target.textContent = "Ověřuji…";
    const r = await api("/api/password", { user: cfg.user, domain: cfg.domain, pass });
    setState(r, true);
    toast("Heslo Windows přijaly a je uložené.");
  });
  const pc = $("#pwclear", el);
  if (pc) pc.onclick = e => busy(e.target, async () => { setState(await api("/api/password", { clear: true }), true); toast("Heslo smazáno."); });
  const pl = $("#pwless", el);
  if (pl) pl.onclick = e => busy(e.target, async () => { setState(await api("/api/passwordless", {}), true); toast("Vypnuto."); });
  renderApps();
  $("#appadd", el).onclick = () => { cfg.apps.push({ on: true, name: "", path: "", args: "", min: true }); renderApps(); markDirty(); };
  $("#appdetect", el).onclick = e => busy(e.target, async () => {
    const found = await api("/api/apps/detect");
    let added = 0;
    for (const a of found) {
      if (!a.path) continue;
      if (cfg.apps.some(x => x.path.toLowerCase() === a.path.toLowerCase())) continue;
      cfg.apps.push(a); added++;
    }
    renderApps(); markDirty();
    toast(added ? `Přidáno: ${added}` : "Nic nového se nenašlo.");
  });
}

function renderApps() {
  const tb = $("#apps");
  if (!tb) return;
  tb.innerHTML = cfg.apps.map((a, i) => `<tr data-i="${i}">
    <td class="c"><input type="checkbox" data-a="on" ${a.on ? "checked" : ""} title="Spouštět"></td>
    <td class="name"><input type="text" data-a="name" value="${esc(a.name)}"></td>
    <td><div class="path"><input type="text" data-a="path" value="${esc(a.path)}" placeholder="C:\\…\\program.exe"><button class="ghost" data-pick>…</button></div></td>
    <td class="args"><input type="text" data-a="args" value="${esc(a.args)}"></td>
    <td class="c"><input type="checkbox" data-a="min" ${a.min ? "checked" : ""}></td>
    <td class="c"><button class="plain" data-del title="Odebrat">✕</button></td></tr>`).join("") ||
    `<tr><td colspan="6" class="sub">Žádné programy.</td></tr>`;
  tb.querySelectorAll("tr[data-i]").forEach(tr => {
    const a = cfg.apps[+tr.dataset.i];
    tr.querySelectorAll("[data-a]").forEach(inp => inp.addEventListener("input", () => {
      a[inp.dataset.a] = inp.type === "checkbox" ? inp.checked : inp.value; markDirty();
    }));
    tr.querySelector("[data-del]").onclick = () => { cfg.apps.splice(+tr.dataset.i, 1); renderApps(); markDirty(); };
    tr.querySelector("[data-pick]").onclick = e => busy(e.target, async () => {
      const r = await api("/api/pick", { start: a.path });
      if (r.path) {
        a.path = r.path;
        if (!a.name) a.name = r.path.split("\\").pop().replace(/\.[^.]+$/, "");
        renderApps(); markDirty();
      }
    });
  });
}

async function renderLog() {
  const el = $('[data-pane="log"]');
  if (!$(".logs", el)) {
    el.innerHTML = `<div class="card"><h2>Záznam <button class="ghost tag" id="logref">Obnovit</button></h2>
      <p class="lead">Služba (ProgramData\\LeoLink\\leolink.log) a agent (LocalAppData\\LeoLink\\agent.log).</p>
      <div class="logs"><div><b>Služba</b><div class="logbox" id="log-s" translate="no"></div></div><div><b>Agent</b><div class="logbox" id="log-a" translate="no"></div></div></div></div>`;
    $("#logref", el).onclick = renderLog;
  }
  try {
    const l = await api("/api/log");
    for (const [id, lines] of [["log-s", l.service], ["log-a", l.agent]]) {
      const box = $("#" + id);
      const atEnd = box.scrollTop + box.clientHeight >= box.scrollHeight - 20;
      box.textContent = (lines || []).join("\n") || "(prázdný)";
      if (atEnd) box.scrollTop = box.scrollHeight;
    }
  } catch (e) { /* ignore */ }
}

let helpSec = "co";
function renderHelp() {
  const el = $('[data-pane="help"]');
  const secs = HELP[S.lang === "en" ? "en" : "cs"];
  const cur = secs.find(([k]) => k === helpSec) || secs[0];
  el.innerHTML = `<div class="help" translate="no">
    <nav class="help-nav">${secs.map(([k, t]) => `<button data-h="${k}" class="${k === cur[0] ? "on" : ""}">${t}</button>`).join("")}</nav>
    <article class="card help-body"><h2>${cur[1]}</h2>${cur[2]}</article></div>`;
  el.querySelectorAll("[data-h]").forEach((b) => (b.onclick = () => { helpSec = b.dataset.h; renderHelp(); }));
}

let current = "overview";
function render() {
  pills();
  const tabs = $("#tabs");
  tabs.querySelector('[data-tab="ha"]').innerHTML = "Home Assistant" + (!S.cfg.broker ? '<span class="dot"></span>' : "");
  tabs.querySelector('[data-tab="lz"]').innerHTML = "Režim Leonzilla" + (S.cfg.autologon && !S.hasPassword ? '<span class="dot"></span>' : "");
  renderOverview();
  renderHA();
  renderLZ();
  if (current === "log") renderLog();
  renderHelp();
  markDirty();
}

$("#tabs").addEventListener("click", e => {
  const b = e.target.closest("button[data-tab]");
  if (!b) return;
  current = b.dataset.tab;
  document.querySelectorAll("#tabs button").forEach(x => x.classList.toggle("on", x === b));
  document.querySelectorAll(".pane").forEach(p => p.classList.toggle("on", p.dataset.pane === current));
  if (current === "log") renderLog();
});
$("#save").onclick = e => busy(e.target, save);
$("#discard").onclick = () => { cfg = JSON.parse(saved); mqPassNew = ""; render(); };

// status refresh: pills and overview without losing edits
setInterval(async () => {
  try {
    const st = await api("/api/state");
    S = st;
    pills();
    if (current === "overview" && !document.activeElement?.closest?.('[data-pane="overview"]')) renderOverview();
    if (current === "log") renderLog();
  } catch (e) { /* window closing */ }
}, 4000);

$("#about").onclick = () => openAbout();
$("#lang").onchange = async (e) => {
  if (dirty() && !confirm("Neuložené změny se ztratí. Přepnout jazyk?")) { e.target.value = S.lang; return; }
  try { await api("/api/lang", { lang: e.target.value }); location.reload(); } catch (err) { toast(err.message, true); }
};

api("/api/state").then(st => { I18N.setLang(st.lang); setState(st); }).catch(e => toast(e.message, true));
