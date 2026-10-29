# Changelog

## 0.1.1 – 2026-10-29

- **Setup wizard** on the first start (and later from Overview → *Setup wizard*): Home Assistant / MQTT with a connection test, Windows account and password, programs, automatic sleep, install, the script and the card for Home Assistant.
- **Icon next to the clock** (drawn by the agent): MQTT and Leonzilla mode status with the sleep countdown, monitors off, lock, open the settings, hide until the next sign-in. Install adds a **LeoLink** shortcut to the Start menu.
- Clearer password field: which account it is (local / Microsoft), "the Windows password, not the PIN", and what to do when you don't know it (change it yourself with Ctrl+Alt+Del – never reset it as an administrator, Windows would lose saved passwords). User / domain moved under *Advanced*. Saving a verified password switches the automatic sign-in on.
- The Windows Hello-only warning is shown only for Microsoft accounts.
- After the PC wakes up, the replaced MQTT connection no longer logs a failed announcement to Home Assistant; the service and agent log their start in the chosen language.
- Leonzilla mode after a "fast startup" shut-down: right after the PC comes back, Windows refuses to restart for a while (exit status 1115). LeoLink now tries again every 5 s for up to 3 minutes instead of giving up and leaving the PC on the sign-in screen.
- If somebody signs in by hand before the restart, LeoLink cancels the restart and starts the programs without locking the screen right after the PIN.
- Programs from the Microsoft Store / MSIX packages (e.g. the Claude desktop app) are started through `shell:AppsFolder\<package>!<app>`. Started directly from `C:\Program Files\WindowsApps\…\claude.exe` they ran without their package identity, which is not supported. The package path also changes with every update. *Find programs* now offers the packaged Claude first.

## 0.1.0 – 2026-10-29

First version.

- The PC in Home Assistant over MQTT discovery: buttons (sleep, shut down, restart, lock, monitor off / on, mute, Dropbox off / on, Leonzilla mode on / off), *Keep awake* switch, *Sleep after* / *Turn off the screen after* selects, sensors (on, signed-in user, boot time, CPU, RAM, free disk space, Leonzilla, Dropbox, idle time, Leonzilla mode, automatic sleep) and a *Message on the screen* notify entity.
- Leonzilla mode: wake from Home Assistant (Wake-on-LAN + a request that waits on the broker), one automatic sign-in when nobody is signed in (password as an LSA secret, `AutoLogonCount=1`, switched off right after the sign-in or after 10 minutes), start of the chosen programs, screen lock.
- Automatic sleep / shut down after a remote session; waits for running Leonzilla jobs and requests from the phone, ends as soon as somebody uses the PC.
- Settings window in English and Czech (switch in the top bar; the language also names the entities in Home Assistant and the log), built-in guide, *About* with links to GitHub, the latest version, bug reports and Ko-fi.
- Settings window: MQTT, automatic sign-in (password check, Windows Hello-only warning), programs, automatic sleep, generated Home Assistant script and dashboard card, install / uninstall (Program Files + two scheduled tasks), log.
