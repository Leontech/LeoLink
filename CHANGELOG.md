# Changelog

## 0.1.0 – 2026-10-29

First version.

- The PC in Home Assistant over MQTT discovery: buttons (sleep, shut down, restart, lock, monitor off / on, mute, Dropbox off / on, Leonzilla mode on / off), *Keep awake* switch, *Sleep after* / *Turn off the screen after* selects, sensors (on, signed-in user, boot time, CPU, RAM, free disk space, Leonzilla, Dropbox, idle time, Leonzilla mode, automatic sleep) and a *Message on the screen* notify entity.
- Leonzilla mode: wake from Home Assistant (Wake-on-LAN + a request that waits on the broker), one automatic sign-in when nobody is signed in (password as an LSA secret, `AutoLogonCount=1`, switched off right after the sign-in or after 10 minutes), start of the chosen programs, screen lock.
- Automatic sleep / shut down after a remote session; waits for running Leonzilla jobs and requests from the phone, ends as soon as somebody uses the PC.
- Settings window in English and Czech (switch in the top bar; the language also names the entities in Home Assistant and the log), built-in guide, *About* with links to GitHub, the latest version, bug reports and Ko-fi.
- Settings window: MQTT, automatic sign-in (password check, Windows Hello-only warning), programs, automatic sleep, generated Home Assistant script and dashboard card, install / uninstall (Program Files + two scheduled tasks), log.
