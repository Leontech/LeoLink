module testbroker

go 1.24.7

replace golang.org/x/sys => github.com/golang/sys v0.35.0

replace golang.org/x/net => github.com/golang/net v0.43.0

replace golang.org/x/sync => github.com/golang/sync v0.16.0

replace golang.org/x/text => github.com/golang/text v0.28.0

replace golang.org/x/crypto => github.com/golang/crypto v0.41.0

replace golang.org/x/exp => github.com/golang/exp v0.0.0-20250718183923-645b1fa84792

require (
	github.com/eclipse/paho.mqtt.golang v1.5.0
	github.com/mochi-mqtt/server/v2 v2.7.9
)

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/rs/xid v1.4.0 // indirect
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace gopkg.in/yaml.v3 => github.com/go-yaml/yaml/v3 v3.0.1

replace gopkg.in/check.v1 => github.com/go-check/check v0.0.0-20201130134442-10cb98267c6c
