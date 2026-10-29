module github.com/Leontech/leolink

// golang.org/x/* are resolved from their GitHub mirrors (see the replace lines),
// so the project also builds where proxy.golang.org / golang.org are not reachable.

go 1.24.7

replace golang.org/x/sys => github.com/golang/sys v0.35.0

replace golang.org/x/net => github.com/golang/net v0.43.0

replace golang.org/x/sync => github.com/golang/sync v0.16.0

replace golang.org/x/text => github.com/golang/text v0.28.0

replace golang.org/x/crypto => github.com/golang/crypto v0.41.0

require (
	github.com/eclipse/paho.mqtt.golang v1.5.0
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	golang.org/x/sys v0.35.0
)

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	golang.org/x/net v0.27.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
)
