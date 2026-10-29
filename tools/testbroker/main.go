package main

// Test MQTT broker + tiny client for testing LeoLink on Linux (no Windows needed):
//   go build -o tb . && ./tb serve 18830   (user leon, password tajne)

// tb serve PORT | tb pub PORT topic payload [retain] | tb sub PORT filter seconds
import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
)

func cli(port string) paho.Client {
	o := paho.NewClientOptions().AddBroker("tcp://127.0.0.1:" + port).SetUsername("leon").SetPassword("tajne").SetClientID(fmt.Sprint("tb", time.Now().UnixNano()))
	c := paho.NewClient(o)
	if t := c.Connect(); t.Wait() && t.Error() != nil {
		log.Fatal(t.Error())
	}
	return c
}

func main() {
	switch os.Args[1] {
	case "serve":
		s := mqtt.New(&mqtt.Options{InlineClient: true})
		_ = s.AddHook(new(auth.Hook), &auth.Options{Ledger: &auth.Ledger{
			Auth: auth.AuthRules{{Username: "leon", Password: "tajne", Allow: true}},
			ACL:  auth.ACLRules{{Username: "leon", Filters: auth.Filters{"#": auth.ReadWrite}}},
		}})
		if err := s.AddListener(listeners.NewTCP(listeners.Config{ID: "t", Address: ":" + os.Args[2]})); err != nil {
			log.Fatal(err)
		}
		go s.Serve()
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
	case "pub":
		c := cli(os.Args[2])
		c.Publish(os.Args[3], 1, len(os.Args) > 5 && os.Args[5] == "retain", os.Args[4]).Wait()
		c.Disconnect(100)
	case "sub":
		c := cli(os.Args[2])
		c.Subscribe(os.Args[3], 1, func(_ paho.Client, m paho.Message) {
			fmt.Printf("%s r=%t %s\n", m.Topic(), m.Retained(), m.Payload())
		}).Wait()
		n, _ := strconv.Atoi(os.Args[4])
		time.Sleep(time.Duration(n) * time.Second)
	}
}
