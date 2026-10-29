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
      <button class="ghost" id="wizard">Průvodce nastavením</button>
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
  $("#wizard", el).onclick = () => openWizard();
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
    for (const id of ["y-script", "w-script"]) if ($("#" + id)) $("#" + id).value = y.script;
    for (const id of ["y-card", "w-card"]) if ($("#" + id)) $("#" + id).value = y.card;
  } catch (e) { /* not important */ }
}

// Password for the automatic sign-in – shared by the "Leonzilla mode" tab and the wizard.
// NEVER suggest resetting the password (net user / as administrator): Windows would lose the user's DPAPI keys –
// saved browser passwords, Credential Manager, app sign-ins (incident 2026-10-29).
function pwBlock() {
  const acc = S.account || {};
  const who = `<b translate="no">${esc(cfg.user || acc.user)}</b> na počítači <b translate="no">${esc(S.hostname)}</b>`;
  const kind = acc.msa ? `účet Microsoft <b translate="no">${esc(acc.msa)}</b>` : "místní účet (bez účtu Microsoft)";
  return `
    <div class="pwbox">
      <div class="acc">Účet: ${who} – ${kind}</div>
      <label class="k" for="f-lp">Heslo, kterým se do tohoto účtu přihlašuješ ve Windows – <b>ne PIN</b></label>
      <div class="inline"><input type="password" id="f-lp" style="max-width:260px" autocomplete="new-password" placeholder="${S.hasPassword ? "uložené – vyplň jen pro změnu" : ""}">
        <button class="${S.hasPassword ? "ghost" : "primary"}" id="pwsave">Ověřit a uložit</button>
        ${S.hasPassword ? `<button class="plain" id="pwclear">Smazat heslo</button>` : ""}</div>
      <div>${S.hasPassword ? st(true, "heslo je uložené a Windows ho přijaly") : st(false, "heslo zatím není uložené", "warn")}</div>
      <ul class="pwhelp">
        ${acc.msa
          ? `<li>Je to <b>heslo k účtu Microsoft</b> – stejné jako na outlook.com nebo account.microsoft.com.</li>`
          : `<li>Je to heslo, které zadáš na přihlašovací obrazovce po kliknutí na <b>Možnosti přihlášení → ikona klíče</b>.</li>`}
        <li>PIN to být nemůže – PIN platí jen na tomhle PC a automatické přihlášení ho použít neumí.</li>
        <li>LeoLink heslo nejdřív ověří u Windows, uloží ho jen když sedí. Nic na účtu nemění.</li>
        <li><b>Heslo neznáš?</b> Když znáš aspoň to staré, změň ho sám přes <b>Ctrl+Alt+Del → Změnit heslo</b>. <b>Nikdy</b> ho nenastavuj znovu jako správce (<code>net user</code>, Správa počítače …) – Windows by tím ztratily uložená hesla v prohlížečích, ve Správci pověření i přihlášení aplikací. Bez hesla používej LeoLink jen pro ovládání z Home Assistantu.</li>
      </ul>
      <details class="adv"><summary>Upřesnit – uživatel a doména (vyplní se samy)</summary>
        <div class="form" style="margin-top:8px">
          <label class="k" for="f-lu">Uživatel</label><input type="text" id="f-lu" data-k="user" style="max-width:220px">
          <label class="k" for="f-ld">Doména / PC</label><input type="text" id="f-ld" data-k="domain" style="max-width:220px">
        </div>
        ${acc.msa ? `<p class="sub">Kdyby Windows heslo nepřijaly, zkus jako uživatele <code>${esc(acc.msa)}</code> a doménu <code>MicrosoftAccount</code>.</p>` : ""}
      </details>
    </div>
    ${S.passwordless && acc.msa ? `<div class="note warn">Je zapnuté <b>„Zvýšit zabezpečení – pro účty Microsoft povolit jen přihlášení přes Windows Hello“</b>. Dokud to platí, Windows heslo při přihlášení nepřijmou a automatické přihlášení nepůjde. PIN zůstane, jen se k němu přidá možnost přihlásit se heslem. <button class="ghost" id="pwless">Vypnout tuto volbu</button></div>` : ""}`;
}

function bindPw(el) {
  $("#pwsave", el).onclick = e => busy(e.target, async () => {
    const pass = $("#f-lp", el).value;
    if (!pass) throw new Error("Zadej heslo.");
    e.target.textContent = "Ověřuji…";
    const r = await api("/api/password", { user: cfg.user, domain: cfg.domain, pass });
    setState(r, true);
    if (!cfg.autologon) {
      cfg.autologon = true; // the password is there for exactly this
      if (!S.first) await save(); else render();
    }
    toast("Heslo Windows přijaly a je uložené. Automatické přihlášení je zapnuté.");
  });
  const pc = $("#pwclear", el);
  if (pc) pc.onclick = e => busy(e.target, async () => { setState(await api("/api/password", { clear: true }), true); toast("Heslo smazáno."); });
  const pl = $("#pwless", el);
  if (pl) pl.onclick = e => busy(e.target, async () => { setState(await api("/api/passwordless", {}), true); toast("Vypnuto."); });
}

function renderLZ() {
  const el = $('[data-pane="lz"]');
  el.innerHTML = `
  <div class="card">
    <h2>Automatické přihlášení</h2>
    <p class="lead">Jen pro režim Leonzilla z vypnutého PC (nebo když nikdo není přihlášený). Normální start dál chce PIN. Heslo je uložené jako tajemství LSA (jako u Sysinternals Autologon), nikdy ne čitelně v registru. Windows ho po jednom přihlášení samy zahodí.</p>
    <label class="chk"><input type="checkbox" data-k="autologon"><span>Při žádosti o režim Leonzilla se jednou přihlásit automaticky<small>PC se kvůli tomu jednou restartuje – přihlašovací obrazovka už by jinak běžela.</small></span></label>
    ${pwBlock()}
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
  bindPw(el);
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


// ---------------------------------------------------------------------------
// Wizard – the first start (no config.json yet) and "Run the wizard again" on the overview.

let wizOn = false, wizStep = 0, wizMqOk = false, wizAuto = null, wizDone = false;
const WIZ = ["Vítej", "Home Assistant", "Účet Windows", "Programy", "Automatické uspání", "Instalace", "Skript a karta"];

function openWizard() {
  wizOn = true; wizStep = 0; wizMqOk = !!(S.service && S.service.connected); wizAuto = cfg.autologon && S.hasPassword ? true : null; wizDone = false;
  document.body.classList.add("wizard-on");
  renderWizard();
}

function closeWizard() {
  wizOn = false;
  document.body.classList.remove("wizard-on");
  render();
}

function wizCanNext() {
  switch (wizStep) {
    case 1: return wizMqOk;
    case 2: return wizAuto === false || (wizAuto === true && S.hasPassword);
    case 5: return wizDone;
    default: return true;
  }
}

function renderWizard() {
  const el = $("#wiz");
  const nics = S.nics || [];
  if (nicIdx >= nics.length) nicIdx = 0;
  const steps = WIZ.map((t, i) => `<li class="${i === wizStep ? "on" : i < wizStep ? "done" : ""}"><span>${i + 1}</span>${t}</li>`).join("");
  let body = "";
  switch (wizStep) {
    case 0:
      body = `<h2>Vítej v LeoLinku</h2>
        <p>LeoLink propojí tohle PC s Home Assistantem a umí ho na dálku probudit tak, aby na něm šlo pracovat z mobilu přes Leonzillu. Průvodce tě provede nastavením – zabere to pár minut.</p>
        <ul class="how">
          <li><b>Home Assistant</b> – PC se v HA objeví jako zařízení: uspat, vypnout, zamknout, monitory, Dropbox, senzory.</li>
          <li><b>Režim Leonzilla</b> – tlačítko v HA PC probudí, jednou se samo přihlásí, spustí vybrané programy a zamkne obrazovku. Normální start dál chce PIN.</li>
          <li><b>Automatické uspání</b> – po práci na dálku PC samo usne.</li>
        </ul>
        <p class="sub">Jazyk okna přepneš vpravo nahoře. Všechno půjde později změnit v záložkách.</p>`;
      break;
    case 1:
      body = `<h2>Připojení k Home Assistantu (MQTT)</h2>
        <p>LeoLink se připojuje k MQTT brokeru, který používá Home Assistant – většinou doplněk <b>Mosquitto</b>. Adresa je adresa Home Assistantu, port 1883. Uživatel a heslo: ten, kterého používá HA pro MQTT, nebo vlastní uživatel vytvořený v HA (Nastavení → Lidé → Uživatelé).</p>
        <div class="form">
          <label class="k" for="w-broker">Adresa brokeru</label>
          <div class="inline"><input type="text" id="w-broker" data-k="broker" placeholder="např. 192.168.1.10" style="max-width:240px">
            <label class="k" for="w-port">port</label><input type="number" id="w-port" data-k="port">
            <label class="chk"><input type="checkbox" data-k="tls"><span>TLS</span></label></div>
          <label class="k" for="w-user">Uživatel</label><input type="text" id="w-user" data-k="mqUser" style="max-width:240px">
          <label class="k" for="w-pass">Heslo</label><input type="password" id="w-pass" style="max-width:240px" autocomplete="new-password" placeholder="${S.hasMqPass ? "uložené – vyplň jen pro změnu" : ""}">
          <label class="k" for="w-name">Název PC v HA</label><input type="text" id="w-name" data-k="name" style="max-width:240px">
        </div>
        <div class="btns"><button class="primary" id="w-mqtest">Vyzkoušet připojení</button>
          <span id="w-mqst">${wizMqOk ? st(true, "připojení funguje") : st(false, "nejdřív vyzkoušej připojení", "warn")}</span></div>`;
      break;
    case 2:
      body = `<h2>Účet Windows – automatické přihlášení</h2>
        <p>Když PC probudíš v režimu Leonzilla z <b>vypnutého</b> stavu, nikdo není přihlášený. Aby se programy mohly spustit, LeoLink se jednou přihlásí sám – k tomu potřebuje heslo k tvému účtu. Normální start dál chce PIN.</p>
        <div class="choice">
          <label class="chk"><input type="radio" name="w-auto" value="1" ${wizAuto === true ? "checked" : ""}><span>Ano, chci režim Leonzilla i z vypnutého PC<small>zadám heslo k účtu Windows</small></span></label>
          <label class="chk"><input type="radio" name="w-auto" value="0" ${wizAuto === false ? "checked" : ""}><span>Ne, stačí mi ovládání z Home Assistantu<small>režim Leonzilla pak funguje jen ze spánku, když jsem přihlášený</small></span></label>
        </div>
        ${wizAuto ? `<div class="card inner">${pwBlock()}</div>` : ""}`;
      break;
    case 3:
      body = `<h2>Programy pro režim Leonzilla</h2>
        <p>Zaškrtnuté programy se spustí, když zapneš režim Leonzilla. Co už běží, se znovu nespouští. Seznam doplníš nebo upravíš později v záložce <b>Režim Leonzilla</b>.</p>
        <div class="wapps">${cfg.apps.map((a, i) => `<label class="chk"><input type="checkbox" data-wa="${i}" ${a.on ? "checked" : ""}><span>${esc(a.name || a.path)}<small translate="no">${esc(a.path)}${a.args ? " " + esc(a.args) : ""}</small></span></label>`).join("") || `<p class="sub">Žádný z obvyklých programů (Claude, Dropbox, Leonzilla) se nenašel.</p>`}</div>
        <div class="btns"><button class="ghost" id="w-detect">Najít programy znovu</button></div>`;
      break;
    case 4:
      body = `<h2>Automatické uspání po práci na dálku</h2>
        <p>Jen v režimu Leonzilla: když se nastavenou dobu nic neděje (nikdo u PC, nic z mobilu, žádná úloha Leonzilly), PC usne nebo se vypne. Jakmile u PC někdo sáhne na klávesnici nebo myš, režim Leonzilla končí a samo nic neusne.</p>
        <label class="chk"><input type="checkbox" data-k="autoSleep"><span>Uspávat automaticky</span></label>
        <div class="form" style="margin-top:10px">
          <label class="k" for="w-sm">Po</label>
          <div class="inline"><input type="number" id="w-sm" data-k="sleepMin" min="5" max="600"> minutách
            <select data-k="sleepAct" style="max-width:160px"><option value="sleep">uspat</option><option value="shutdown">vypnout</option></select></div>
        </div>`;
      break;
    case 5:
      body = `<h2>Instalace</h2>
        <p>LeoLink se zkopíruje do <code>C:\\Program Files\\LeoLink</code> a založí dvě naplánované úlohy: <b>službu</b> (běží pořád, i bez přihlášení – MQTT, napájení, automatické přihlášení) a <b>agenta</b> (běží po tvém přihlášení – programy, zamykání, monitory, ikona u hodin). Do nabídky Start přidá zástupce <b>LeoLink</b>.</p>
        <div class="rows">
          <span class="k">Home Assistant</span><span>${st(wizMqOk, `broker ${esc(cfg.broker)}:${cfg.port}, zařízení <b>${esc(cfg.name)}</b>`)}</span>
          <span class="k">Automatické přihlášení</span><span>${cfg.autologon && S.hasPassword ? st(true, `zapnuté pro ${esc(cfg.user)}`) : st(false, "vypnuté", "warn")}</span>
          <span class="k">Programy</span><span>${esc(cfg.apps.filter(a => a.on).map(a => a.name).join(", ") || "žádné")}</span>
          <span class="k">Automatické uspání</span><span>${cfg.autoSleep ? `po ${cfg.sleepMin} min – ${cfg.sleepAct === "shutdown" ? "vypnout" : "uspat"}` : "vypnuté"}</span>
        </div>
        <div class="btns"><button class="primary" id="w-install">${wizDone ? "Nainstalováno ✓" : S.install.installed ? "Uložit a přeinstalovat" : "Uložit a nainstalovat"}</button></div>`;
      break;
    case 6:
      body = `<h2>Skript a karta pro Home Assistant</h2>
        <p>Zařízení <b>${esc(cfg.name)}</b> se v Home Assistantu objevilo samo (Nastavení → Zařízení a služby → MQTT). Zbývá skript pro probuzení v režimu Leonzilla:</p>
        <ol class="steps">
          <li>V <code>configuration.yaml</code> musí být řádek <code>wake_on_lan:</code> (když už PC z HA budíš, je tam).</li>
          <li>Vlož skript do <code>scripts.yaml</code> (nebo Nastavení → Automatizace a scény → Skripty → Přidat → ⋮ Upravit v YAML, bez prvního řádku) a znovu načti skripty.</li>
          <li>Kartu přidáš na řídicí panel: Upravit → Přidat kartu → Ruční.</li>
        </ol>
        <div class="form"><label class="k" for="w-nic">Síťová karta</label>
          <select id="w-nic" style="max-width:520px">${nics.map((n, i) => `<option value="${i}" ${i === nicIdx ? "selected" : ""}>${esc(n.name)} – ${esc(n.mac)} (${esc(n.ip)})</option>`).join("") || `<option>nenalezena – doplň MAC ve skriptu ručně</option>`}</select></div>
        <h3>Skript</h3><div class="code"><textarea id="w-script" readonly rows="11"></textarea><button class="ghost" data-copy="w-script">Kopírovat</button></div>
        <h3>Karta</h3><div class="code"><textarea id="w-card" readonly rows="10"></textarea><button class="ghost" data-copy="w-card">Kopírovat</button></div>
        <p class="sub">Obojí najdeš i později v záložce Home Assistant. U hodin teď najdeš ikonu LeoLink – stav a rychlé volby.</p>`;
      break;
  }
  const last = wizStep === WIZ.length - 1;
  el.innerHTML = `<div class="wiz-in">
    <ol class="wsteps">${steps}</ol>
    <div class="card wbody">${body}</div>
    <div class="wfoot">
      ${S.first ? "" : `<button class="plain" id="w-close">Zavřít průvodce</button>`}
      <span style="flex:1"></span>
      ${wizStep > 0 ? `<button class="ghost" id="w-back">Zpět</button>` : ""}
      <button class="primary" id="w-next" ${wizCanNext() ? "" : "disabled"}>${last ? "Hotovo" : wizStep === 0 ? "Začít" : "Další"}</button>
    </div></div>`;

  // values + binding
  bind(el);
  el.querySelectorAll("[data-k]").forEach(i => {
    const v = cfg[i.dataset.k];
    if (i.type === "checkbox") i.checked = !!v; else i.value = v ?? "";
  });
  const go = d => { wizStep += d; renderWizard(); el.scrollTop = 0; };
  $("#w-next", el).onclick = () => (last ? closeWizard() : go(1));
  const back = $("#w-back", el); if (back) back.onclick = () => go(-1);
  const cl = $("#w-close", el); if (cl) cl.onclick = () => { if (!dirty() || confirm("Neuložené změny zůstanou neuložené. Zavřít průvodce?")) closeWizard(); };

  if (wizStep === 1) {
    const mqInputs = el.querySelectorAll("input");
    mqInputs.forEach(i => i.addEventListener("input", () => { wizMqOk = false; $("#w-mqst").innerHTML = st(false, "nejdřív vyzkoušej připojení", "warn"); $("#w-next").disabled = true; }));
    $("#w-pass", el).addEventListener("input", e => { mqPassNew = e.target.value; markDirty(); });
    $("#w-mqtest", el).onclick = e => busy(e.target, async () => {
      e.target.textContent = "Zkouším…";
      try {
        await api("/api/mqtt/test", Object.assign({}, cfg, { mqPassNew }));
        wizMqOk = true;
        $("#w-mqst").innerHTML = st(true, "připojení funguje");
        $("#w-next").disabled = false;
      } catch (err) {
        $("#w-mqst").innerHTML = st(false, "nepřipojeno: " + esc(err.message));
        throw err;
      }
    });
  }
  if (wizStep === 2) {
    el.querySelectorAll('[name="w-auto"]').forEach(r => r.onchange = () => {
      wizAuto = r.value === "1";
      if (!wizAuto) cfg.autologon = false;
      renderWizard();
    });
    if (wizAuto) bindPw(el);
  }
  if (wizStep === 3) {
    el.querySelectorAll("[data-wa]").forEach(c => c.onchange = () => { cfg.apps[+c.dataset.wa].on = c.checked; });
    $("#w-detect", el).onclick = e => busy(e.target, async () => {
      const found = await api("/api/apps/detect");
      for (const a of found) if (a.path && !cfg.apps.some(x => x.path.toLowerCase() === a.path.toLowerCase())) cfg.apps.push(a);
      renderWizard();
    });
  }
  if (wizStep === 5) {
    $("#w-install", el).onclick = e => busy(e.target, async () => {
      if (wizDone) return;
      e.target.textContent = "Ukládám…";
      await save();
      e.target.textContent = "Instaluji…";
      setState(await api("/api/install", {}), true);
      wizDone = true;
      renderWizard();
      toast("Nainstalováno – služba a agent běží.");
    });
  }
  if (wizStep === 6) {
    $("#w-nic", el).onchange = e => { nicIdx = +e.target.value; loadYAML(); };
    el.querySelectorAll("[data-copy]").forEach(b => b.onclick = async () => {
      const t = $("#" + b.dataset.copy);
      try { await navigator.clipboard.writeText(t.value); } catch { t.select(); document.execCommand("copy"); }
      toast("Zkopírováno");
    });
    loadYAML();
  }
}

let current = "overview";
function render() {
  pills();
  if (wizOn) { renderWizard(); return; }
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
    if (wizOn) return;
    if (current === "overview" && !document.activeElement?.closest?.('[data-pane="overview"]')) renderOverview();
    if (current === "log") renderLog();
  } catch (e) { /* window closing */ }
}, 4000);

$("#about").onclick = () => openAbout();
$("#lang").onchange = async (e) => {
  if (dirty() && !confirm("Neuložené změny se ztratí. Přepnout jazyk?")) { e.target.value = S.lang; return; }
  try { await api("/api/lang", { lang: e.target.value }); location.reload(); } catch (err) { toast(err.message, true); }
};

api("/api/state").then(st => { I18N.setLang(st.lang); setState(st); if (st.first) openWizard(); }).catch(e => toast(e.message, true));
