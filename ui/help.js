"use strict";
/* LeoLink – guide (tab "Návod" / "Guide"). Czech and English are written separately (not through the catalog). */

const HELP = {
  cs: [
    ["co", "Co LeoLink umí", `
<p>LeoLink propojí počítač s <b>Home Assistantem</b> přes MQTT a umí ho na dálku <b>probudit připravený k práci</b>.</p>
<ul>
<li><b>PC v Home Assistantu</b> – zařízení s tlačítky (Uspat, Vypnout, Restartovat, Zamknout, Monitor vyp/zap, Ztlumit, Dropbox vyp/zap),
přepínačem <i>Nespát</i>, volbami <i>Uspat po</i> a <i>Vypnout obrazovku po</i>, senzory (zapnuto, přihlášený uživatel, CPU, RAM, volné místo,
Leonzilla, Dropbox, nečinnost) a <i>Zprávou na obrazovku</i>. Entity se v HA objeví samy, do <code>configuration.yaml</code> se nic nepíše.</li>
<li><b>Režim Leonzilla</b> – jedno tlačítko v HA PC probudí, jednou se samo přihlásí, spustí vybrané programy a zamkne obrazovku.
Pak s <a href="https://github.com/Leontech/leonzilla" data-url>Leonzillou</a> pracuješ z mobilu. Normální start dál chce PIN.</li>
<li><b>Automatické uspání</b> – po práci na dálku PC samo usne nebo se vypne.</li>
</ul>
<p>LeoLink <b>neotevírá žádný port</b> – jen se připojuje k MQTT brokeru v domácí síti.</p>`],

    ["instalace", "Instalace krok za krokem", `
<ol>
<li>Spusť <code>LeoLink.exe</code> (požádá o oprávnění správce – ukládá heslo pro přihlášení a zakládá naplánované úlohy).
  Při prvním spuštění tě provede <b>průvodce</b> (jde spustit i později: Přehled → <i>Průvodce nastavením</i>).</li>
<li><b>Home Assistant</b>: adresa MQTT brokeru (většinou IP Home Assistantu), port 1883, uživatel a heslo → <i>Vyzkoušet připojení</i> → <i>Uložit</i>.</li>
<li><b>Režim Leonzilla</b>: zaškrtni automatické přihlášení, zadej heslo k účtu Windows → <i>Ověřit a uložit</i>. Vyber programy (<i>Najít programy automaticky</i>).</li>
<li><b>Přehled</b> → <i>Nainstalovat</i>. LeoLink se zkopíruje do <code>C:\\Program Files\\LeoLink</code> a založí dvě úlohy:
  <code>LeoLink\\Service</code> (při startu PC, jako SYSTEM) a <code>LeoLink\\Agent</code> (při tvém přihlášení, bez práv správce).</li>
<li>Do Home Assistantu vlož skript ze záložky <b>Home Assistant</b> (a případně kartu).</li>
<li><b>Přehled → Vyzkoušet</b> – tlačítka pošlou stejnou zprávu jako HA.</li>
<li>U hodin je <b>ikona LeoLink</b> (stav MQTT a režimu Leonzilla, vypnout monitory, zamknout, otevřít nastavení). V nabídce Start je zástupce <b>LeoLink</b>.</li>
</ol>
<p>Novou verzi nainstaluješ stejně – spusť nové <code>LeoLink.exe</code> a klikni <i>Aktualizovat</i>. Nastavení zůstane.</p>`],

    ["ha", "Home Assistant a MQTT", `
<p><b>Broker:</b> doplněk <i>Mosquitto broker</i> v Home Assistantu a integrace MQTT (s automatickým objevováním – je zapnuté ve výchozím stavu).
Pro LeoLink můžeš použít uživatele, kterého už máš pro MQTT, nebo si v HA založit nového (Nastavení → Lidé → Uživatelé).</p>
<p><b>Zařízení</b> najdeš v Nastavení → Zařízení a služby → MQTT pod názvem PC. Tlačítka Uspat / Vypnout / Restartovat a senzory systému
obsluhuje služba (funguje i bez přihlášení), zbytek agent v přihlášené relaci.</p>
<p><b>Skript „Probudit – režim Leonzilla“</b> (záložka Home Assistant) pošle Wake-on-LAN a nechá na brokeru žádost. Wake-on-LAN potřebuje v
<code>configuration.yaml</code> řádek <code>wake_on_lan:</code>. Žádost platí 15 minut – když se PC neprobudí, později se samo nepřihlásí.</p>
<p><b>Aby šlo PC probudit ze sítě:</b></p>
<ul>
<li>BIOS / UEFI: <i>Wake on LAN</i> / <i>Power on by PCI-E</i> zapnuto, <i>ErP</i> vypnuto (jinak síťová karta ve vypnutém PC nemá proud).</li>
<li>Správce zařízení → síťová karta → Řízení spotřeby: <i>Povolit zařízení probouzet počítač</i> a <i>Jen magický paket</i>; Upřesnit: <i>Wake on Magic Packet</i>.</li>
<li>Z vypnutého stavu probouzí většina desek jen přes kabel (Wi-Fi ne).</li>
</ul>
<p><b>Vlastní automatizace:</b> zpráva na obrazovku = akce <code>notify.send_message</code> s entitou <i>Zpráva na obrazovku</i>;
Nespát / Uspat po / Vypnout obrazovku po jdou přepínat i z automatizací (např. přes noc Nespát při dlouhém buildu).</p>`],

    ["rezim", "Režim Leonzilla", `
<p>Po stisku tlačítka v Home Assistantu:</p>
<ul>
<li><b>PC je vypnuté</b> (nebo nikdo není přihlášený): služba připraví <b>jedno</b> automatické přihlášení a PC restartuje
(přihlašovací obrazovka už běží a Windows automatické přihlášení čtou jen při startu). Po přihlášení agent spustí programy a zamkne obrazovku.
Služba automatické přihlášení <b>hned vypne</b> – nejpozději po 10 minutách, i kdyby se nepovedlo. Celé to trvá zhruba 1–2 minuty.</li>
<li><b>PC spí a jsi přihlášený</b>: jen se spustí programy (obrazovka zůstane zamčená, přihlášení není potřeba).</li>
<li><b>PC běží</b>: tlačítko <i>Režim Leonzilla zapnout</i> spustí programy, které neběží.</li>
</ul>
<p><b>Heslo</b> se ukládá jako tajemství LSA (stejně jako Sysinternals Autologon) – číst ho může jen SYSTEM a správci, v registru není nikdy
čitelně. Do Winlogonu se dává jen na jedno přihlášení (<code>AutoLogonCount=1</code>).</p>
<p><b>Účet Microsoft:</b> zadej heslo k účtu Microsoft, ne PIN. Pokud je ve Windows zapnuté <i>„pro účty Microsoft povolit jen přihlášení
přes Windows Hello“</i>, Windows heslo nepřijmou – LeoLink to ukáže a umí volbu vypnout (PIN zůstane).</p>
<p><b>Programy</b> se spouštějí v pořadí ze seznamu, co už běží, se přeskočí. Leonzilla s parametrem <code>--minimized</code> naběhne jen na
hlavní panel. Když změníš heslo k Windows, ulož ho v LeoLinku znovu.</p>
<p><b>Heslo neznáš?</b> Když znáš aspoň to staré, změň ho sám přes <b>Ctrl+Alt+Del → Změnit heslo</b>. <b>Nikdy</b> ho nenastavuj znovu jako
správce (<code>net user</code>, Správa počítače, cizí účet správce) – je to reset hesla a Windows při něm ztratí klíče, kterými mají zašifrovaná
uložená hesla v prohlížečích, Správce pověření a přihlášení aplikací. LeoLink heslo jen ověřuje a ukládá, na účtu nikdy nic nemění.</p>`],

    ["uspani", "Automatické uspání", `
<p>Jen v režimu Leonzilla. PC usne (nebo se vypne – podle nastavení), když se po nastavenou dobu nic neděje:</p>
<ul>
<li>nikdo nesahá na klávesnici ani myš u PC,</li>
<li>z mobilu nepřišel žádný požadavek na Leonzillu (otevřená Leonzilla v mobilu se počítá jako práce),</li>
<li>v Leonzille neběží žádná úloha (nahrávání, build, záloha…).</li>
</ul>
<p>Jakmile někdo u PC použije klávesnici nebo myš, režim Leonzilla končí a PC samo neusne – pracuješ normálně.
Odpočet vidíš v HA (<i>Automatické uspání</i>). Leonzilla hlásí svou činnost od verze 1.11.20.</p>`],

    ["leonzilla", "LeoLink + Leonzilla", `
<p><a href="https://github.com/Leontech/leonzilla" data-url>Leonzilla</a> nahrává projekty na servery, spouští buildy, zálohuje a pushuje na GitHub.
Od verze 1.11.20 ji jde <b>ovládat z mobilu</b> (jen spouštění karet). LeoLink k tomu zajistí, že PC na dálku naběhne a po práci zase usne.</p>
<ol>
<li>Leonzilla: Nastavení aplikace → záložka <i>Vzdálený přístup</i> – zapnout, heslo, adresu otevři v mobilu a přidej na plochu. Má-li Leonzilla hlavní heslo, je po spuštění zamčená: v mobilu se nejdřív přihlásíš heslem pro vzdálený přístup a pak ji odemkneš hlavním heslem.</li>
<li>Mobil mimo domov: VPN do domácí sítě (např. WireGuard).</li>
<li>LeoLink: v programech zaškrtni Leonzillu (parametr <code>--minimized</code>).</li>
<li>Z mobilu: HA → <i>Probudit – režim Leonzilla</i> → po 1–2 minutách otevři Leonzillu a spouštěj karty.</li>
<li>Po práci PC usne samo (Automatické uspání), nebo ho uspi tlačítkem v HA.</li>
</ol>`],

    ["potize", "Řešení potíží", `
<ul>
<li><b>MQTT nepřipojeno</b> – zkontroluj adresu, port a účet (<i>Vyzkoušet připojení</i>); firewall na brokeru; u TLS port 8883.</li>
<li><b>V HA nejsou entity</b> – služba musí běžet a být připojená (Přehled). V integraci MQTT musí být zapnuté objevování (discovery).</li>
<li><b>PC se neprobudí</b> – nastavení Wake-on-LAN (záložka Home Assistant v návodu), správná MAC ve skriptu, kabel.</li>
<li><b>Nepřihlásí se samo</b> – v záznamu služby hledej „automatické přihlášení“. Ověř heslo znovu (<i>Ověřit a uložit</i>),
u účtu Microsoft vypni „jen Windows Hello“. Po 10 minutách se automatické přihlášení samo vypne.</li>
<li><b>Agent neběží</b> – spouští se při přihlášení; hned po instalaci ho spustí LeoLink sám.</li>
<li><b>Nic se nespustilo</b> – cesty v seznamu programů, záznam agenta.</li>
</ul>
<p>Záznamy: <code>C:\\ProgramData\\LeoLink\\leolink.log</code> (služba) a <code>%LOCALAPPDATA%\\LeoLink\\agent.log</code> (agent) – záložka <i>Záznam</i>.</p>
<p><b>Odinstalace</b> (Přehled → Odinstalovat): zastaví a smaže úlohy, smaže uložené heslo, odebere zařízení z Home Assistantu a program
z Program Files. Nastavení v <code>C:\\ProgramData\\LeoLink</code> zůstane pro případnou novou instalaci.</p>`],
  ],

  en: [
    ["co", "What LeoLink does", `
<p>LeoLink connects the PC to <b>Home Assistant</b> over MQTT and can <b>wake it up ready for work</b> from a distance.</p>
<ul>
<li><b>The PC in Home Assistant</b> – a device with buttons (Sleep, Shut down, Restart, Lock, Monitor off/on, Mute, Dropbox off/on),
a <i>Keep awake</i> switch, <i>Sleep after</i> and <i>Turn off the screen after</i> selects, sensors (power, signed-in user, CPU, RAM, free space,
Leonzilla, Dropbox, idle time) and a <i>Message on the screen</i>. The entities appear by themselves, nothing goes into <code>configuration.yaml</code>.</li>
<li><b>Leonzilla mode</b> – one button in HA wakes the PC, signs in once by itself, starts the chosen programs and locks the screen.
Then you work with <a href="https://github.com/Leontech/leonzilla" data-url>Leonzilla</a> from your phone. A normal start still asks for your PIN.</li>
<li><b>Automatic sleep</b> – after remote work the PC goes to sleep or shuts down by itself.</li>
</ul>
<p>LeoLink <b>opens no port</b> – it only connects to the MQTT broker in your home network.</p>`],

    ["instalace", "Installation step by step", `
<ol>
<li>Run <code>LeoLink.exe</code> (it asks for administrator rights – it stores the sign-in password and creates scheduled tasks).
  On the first start a <b>wizard</b> takes you through the setup (you can run it later too: Overview → <i>Setup wizard</i>).</li>
<li><b>Home Assistant</b>: address of the MQTT broker (usually the IP of Home Assistant), port 1883, user and password → <i>Try the connection</i> → <i>Save</i>.</li>
<li><b>Leonzilla mode</b>: tick the automatic sign-in, enter the password of your Windows account → <i>Verify and save</i>. Choose the programs (<i>Find programs automatically</i>).</li>
<li><b>Overview</b> → <i>Install</i>. LeoLink copies itself to <code>C:\\Program Files\\LeoLink</code> and creates two tasks:
  <code>LeoLink\\Service</code> (at PC start, as SYSTEM) and <code>LeoLink\\Agent</code> (at your sign-in, without administrator rights).</li>
<li>Put the script from the <b>Home Assistant</b> tab into Home Assistant (and the card, if you like).</li>
<li><b>Overview → Try</b> – the buttons send the same message as HA.</li>
<li>Next to the clock there is the <b>LeoLink icon</b> (MQTT and Leonzilla mode status, monitors off, lock, open the settings). The Start menu has a <b>LeoLink</b> shortcut.</li>
</ol>
<p>A new version installs the same way – run the new <code>LeoLink.exe</code> and click <i>Update</i>. The settings stay.</p>`],

    ["ha", "Home Assistant and MQTT", `
<p><b>Broker:</b> the <i>Mosquitto broker</i> add-on in Home Assistant and the MQTT integration (with discovery – on by default).
LeoLink can use the user you already have for MQTT, or create a new one in HA (Settings → People → Users).</p>
<p><b>The device</b> is in Settings → Devices &amp; services → MQTT under the name of the PC. Sleep / Shut down / Restart and the system sensors
are handled by the service (works without anybody signed in), the rest by the agent in the signed-in session.</p>
<p><b>The script “Wake – Leonzilla mode”</b> (Home Assistant tab) sends Wake-on-LAN and leaves a request on the broker. Wake-on-LAN needs the line
<code>wake_on_lan:</code> in <code>configuration.yaml</code>. The request is valid for 15 minutes – if the PC does not wake up, it will not sign in by itself later.</p>
<p><b>So that the PC can be woken from the network:</b></p>
<ul>
<li>BIOS / UEFI: <i>Wake on LAN</i> / <i>Power on by PCI-E</i> on, <i>ErP</i> off (otherwise the network card has no power while the PC is off).</li>
<li>Device Manager → network adapter → Power Management: <i>Allow this device to wake the computer</i> and <i>Only allow a magic packet</i>; Advanced: <i>Wake on Magic Packet</i>.</li>
<li>From the switched-off state most boards wake only over a cable (not Wi-Fi).</li>
</ul>
<p><b>Your own automations:</b> a message on the screen = the <code>notify.send_message</code> action with the <i>Message on the screen</i> entity;
Keep awake / Sleep after / Turn off the screen after can be switched from automations too (e.g. Keep awake overnight for a long build).</p>`],

    ["rezim", "Leonzilla mode", `
<p>After pressing the button in Home Assistant:</p>
<ul>
<li><b>The PC is off</b> (or nobody is signed in): the service prepares <b>one</b> automatic sign-in and restarts the PC
(the sign-in screen is already up and Windows reads the automatic sign-in only at start). After the sign-in the agent starts the programs and locks the screen.
The service <b>switches the automatic sign-in off at once</b> – after 10 minutes at the latest, even if it failed. All of it takes about 1–2 minutes.</li>
<li><b>The PC sleeps and you are signed in</b>: only the programs are started (the screen stays locked, no sign-in needed).</li>
<li><b>The PC is running</b>: the <i>Leonzilla mode on</i> button starts the programs that do not run.</li>
</ul>
<p><b>The password</b> is stored as an LSA secret (like Sysinternals Autologon) – only SYSTEM and administrators can read it, it is never readable in the
registry. It is given to Winlogon for one sign-in only (<code>AutoLogonCount=1</code>).</p>
<p><b>Microsoft account:</b> enter the Microsoft account password, not the PIN. If Windows has <i>“only allow Windows Hello sign-in for Microsoft
accounts”</i> on, Windows accept no password – LeoLink shows it and can switch the option off (your PIN stays).</p>
<p><b>Programs</b> start in the order of the list, the ones already running are skipped. Leonzilla with <code>--minimized</code> starts on the
taskbar only. When you change your Windows password, store it in LeoLink again.</p>
<p><b>Don't know the password?</b> If you know at least the old one, change it yourself with <b>Ctrl+Alt+Del → Change a password</b>. <b>Never</b>
set it again as an administrator (<code>net user</code>, Computer Management, another administrator account) – that is a password reset and Windows
lose the keys that encrypt the passwords saved in browsers, Credential Manager and the sign-ins of apps. LeoLink only checks and stores the
password, it never changes anything on the account.</p>`],

    ["uspani", "Automatic sleep", `
<p>Only in Leonzilla mode. The PC goes to sleep (or shuts down – as set) when nothing happens for the set time:</p>
<ul>
<li>nobody uses the keyboard or mouse at the PC,</li>
<li>no request came from the phone to Leonzilla (Leonzilla open on the phone counts as work),</li>
<li>no Leonzilla job runs (upload, build, backup …).</li>
</ul>
<p>As soon as somebody uses the keyboard or mouse at the PC, Leonzilla mode ends and the PC does not sleep by itself – you work normally.
The countdown is shown in HA (<i>Automatic sleep</i>). Leonzilla reports its activity since version 1.11.20.</p>`],

    ["leonzilla", "LeoLink + Leonzilla", `
<p><a href="https://github.com/Leontech/leonzilla" data-url>Leonzilla</a> uploads projects to servers, runs builds, makes backups and pushes to GitHub.
Since 1.11.20 it can be <b>used from a phone</b> (running cards only). LeoLink makes sure the PC comes up from a distance and goes to sleep afterwards.</p>
<ol>
<li>Leonzilla: App settings → the <i>Remote access</i> tab – switch on, password, open the address on the phone and add it to the home screen. If Leonzilla has a master password, it is locked after it starts: on the phone sign in with the remote-access password first, then unlock it with the master password.</li>
<li>Phone away from home: a VPN into the home network (e.g. WireGuard).</li>
<li>LeoLink: tick Leonzilla among the programs (argument <code>--minimized</code>).</li>
<li>From the phone: HA → <i>Wake – Leonzilla mode</i> → after 1–2 minutes open Leonzilla and run the cards.</li>
<li>After the work the PC goes to sleep by itself (Automatic sleep), or put it to sleep with the button in HA.</li>
</ol>`],

    ["potize", "Troubleshooting", `
<ul>
<li><b>MQTT not connected</b> – check the address, port and account (<i>Try the connection</i>); the firewall of the broker; port 8883 for TLS.</li>
<li><b>No entities in HA</b> – the service must run and be connected (Overview). Discovery must be on in the MQTT integration.</li>
<li><b>The PC does not wake up</b> – Wake-on-LAN settings (Home Assistant section of this guide), the right MAC in the script, a cable.</li>
<li><b>It does not sign in</b> – look for “automatic sign-in” in the service log. Verify the password again (<i>Verify and save</i>);
for a Microsoft account switch off “only Windows Hello”. After 10 minutes the automatic sign-in switches itself off.</li>
<li><b>The agent does not run</b> – it starts at sign-in; right after the installation LeoLink starts it itself.</li>
<li><b>Nothing started</b> – the paths in the list of programs, the agent log.</li>
</ul>
<p>Logs: <code>C:\\ProgramData\\LeoLink\\leolink.log</code> (service) and <code>%LOCALAPPDATA%\\LeoLink\\agent.log</code> (agent) – the <i>Log</i> tab.</p>
<p><b>Uninstall</b> (Overview → Uninstall): stops and deletes the tasks, deletes the stored password, removes the device from Home Assistant and the
program from Program Files. The settings in <code>C:\\ProgramData\\LeoLink</code> stay for a later installation.</p>`],
  ],
};
