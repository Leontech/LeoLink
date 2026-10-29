// LeoLink – connects the PC with Home Assistant (MQTT): buttons, sensors, power, monitors, Dropbox, messages,
// and "Leonzilla mode" – wake the PC from Home Assistant, sign in once automatically, start the chosen programs.
// One program, three roles:
//
//	LeoLink.exe            settings window (asks for administrator rights)
//	LeoLink.exe --service  scheduled task at system startup, as SYSTEM (service.go)
//	LeoLink.exe --agent    scheduled task at the user's sign-in, without administrator rights (agent.go)
//
// © Leoš Vilím · leotech.cz, MIT License
package main

import (
	"os"
	"strings"
)

const appVersion = "0.1.1"

const appTitle = "LeoLink"

var runMode = "gui"

func main() {
	for _, a := range os.Args[1:] {
		switch strings.ToLower(a) {
		case "--service":
			runMode = "service"
		case "--agent":
			runMode = "agent"
		}
	}
	switch runMode {
	case "service":
		if !singleInstance("Global\\LeoLink-service") {
			return
		}
		runService()
	case "agent":
		if !singleInstance("Local\\LeoLink-agent") {
			return
		}
		runAgent()
	default:
		runGUI()
	}
}
