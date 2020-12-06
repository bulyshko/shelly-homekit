package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/brutella/hc"
	"github.com/brutella/hc/accessory"
	"github.com/brutella/hc/log"
	"net/url"
	"os"
	"path"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Shelly struct {
	ID    string `json:"mac"`
	Name  string `json:"id"`
	Model string `json:"model"`
}

func (d *Shelly) IsSupported() bool {
	return d.Relays() > 0
}

func (d *Shelly) Relays() int {
	switch d.Model {
	case "SHSW-1", "SHSW-PM", "SHSW-L":
		return 1
	case "SHSW-25":
		return 2
	}
	return 0
}

type Device struct {
	accessory *accessory.Switch
	transport hc.Transport
}

func addRelayIndex(str string, index int) string {
	if index == 0 {
		return str
	}
	return str + "-" + strconv.Itoa(index+1)
}

func main() {
	devices := map[string]*Device{}

	pin := flag.String("pin", "", "PIN used to pair Shellies with HomeKit")
	broker := flag.String("broker", "", "MQTT broker")
	dir := flag.String("data", "", "Path to data directory")
	verbose := flag.Bool("verbose", false, "Whether or not log output is displayed")

	flag.Parse()

	log.Info.Enable()

	if *verbose {
		log.Debug.Enable()
	}

	ctx := context.Background()

	uri, err := url.Parse(*broker)
	if err != nil {
		log.Info.Panic(err)
	}

	opts := mqtt.NewClientOptions()
	opts.SetClientID("ShellyBridge")
	opts.AddBroker(fmt.Sprintf("tcp://%s", uri.Host))

	if uri.User.Username() != "" {
		opts.SetUsername(uri.User.Username())

		password, passwordSet := uri.User.Password()
		if passwordSet {
			opts.SetPassword(password)
		}
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Info.Println("Unable to connect to the MQTT broker")
		log.Info.Panic(token.Error())
	}

	if token := client.Subscribe("shellies/announce", 0, func(client mqtt.Client, msg mqtt.Message) {
		shelly := new(Shelly)
		if err := json.Unmarshal(msg.Payload(), shelly); err != nil {
			log.Info.Panic(err)
		}

		if !shelly.IsSupported() {
			return
		}

		for i := 0; i < shelly.Relays(); i++ {
			id := addRelayIndex(shelly.ID, i)
			if _, found := devices[id]; found {
				continue
			}

			ac := accessory.NewSwitch(accessory.Info{
				Name:  addRelayIndex(shelly.Name, i),
				Model: shelly.Model,
			})

			ac.Switch.On.OnValueRemoteUpdate(func(relay int) func(on bool) {
				return func(on bool) {
					message := "off"
					if on == true {
						message = "on"
					}
					client.Publish("shellies/"+shelly.Name+"/relay/"+strconv.Itoa(relay)+"/command", 0, true, message)
				}
			}(i))

			if token := client.Subscribe("shellies/"+shelly.Name+"/relay/"+strconv.Itoa(i), 0, func(client mqtt.Client, msg mqtt.Message) {
				ac.Switch.On.SetValue(string(msg.Payload()) == "on")
			}); token.Wait() && token.Error() != nil {
				log.Info.Panic(token.Error())
			}

			transport, err := hc.NewIPTransport(hc.Config{
				Pin:         *pin,
				StoragePath: path.Join(*dir, id),
			}, ac.Accessory)
			if err != nil {
				log.Info.Panic(err)
			}

			go func() {
				transport.Start()
			}()

			devices[id] = &Device{ac, transport}
		}
	}); token.Wait() && token.Error() != nil {
		log.Info.Panic(token.Error())
	}

	client.Publish("shellies/command", 0, false, "announce")

	hc.OnTermination(func() {
		for _, device := range devices {
			<-device.transport.Stop()
		}

		time.Sleep(100 * time.Millisecond)
		os.Exit(1)
	})

	<-ctx.Done()
}
