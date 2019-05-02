package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/brutella/hc"
	"github.com/brutella/hc/accessory"
	"github.com/brutella/hc/log"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Config struct {
	Broker   string   `json:"broker"`
	Pin      string   `json:"pin"`
	Shellies []Shelly `json:"shellies"`
}

type Shelly struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

func main() {
	dir := flag.String("d", "", "Path to data directory")
	verbose := flag.Bool("v", false, "Whether or not log output is displayed")

	flag.Parse()

	log.Info.Enable()

	if *verbose {
		log.Debug.Enable()
	}

	var config Config

	file, err := os.Open(path.Join(*dir, "config.json"))
	if err != nil {
		log.Info.Panic(err)
	}

	defer file.Close()

	bytes, _ := ioutil.ReadAll(file)

	if err := json.Unmarshal(bytes, &config); err != nil {
		log.Info.Panic(err)
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(config.Broker)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Info.Panic(token.Error())
	}

	var accessories []*accessory.Accessory

	for i := 0; i < len(config.Shellies); i++ {
		shelly := config.Shellies[i]

		log.Info.Println("New Switch:", shelly.Name)

		ac := accessory.NewSwitch(accessory.Info{Name: shelly.Name})

		ac.Switch.On.OnValueRemoteUpdate(func(on bool) {
			message := "off"
			if on == true {
				message = "on"
			}
			client.Publish("shellies/shelly1-"+shelly.Id+"/relay/0/command", 0, true, message)
		})

		if token := client.Subscribe("shellies/shelly1-"+shelly.Id+"/relay/0", 0, func(client mqtt.Client, msg mqtt.Message) {
			ac.Switch.On.SetValue(string(msg.Payload()) == "on")
		}); token.Wait() && token.Error() != nil {
			log.Info.Panic(token.Error())
		}

		accessories = append(accessories, ac.Accessory)
	}

	bridge := accessory.NewBridge(accessory.Info{Name: "Shelly"})

	transport, err := hc.NewIPTransport(hc.Config{
		Pin:         config.Pin,
		StoragePath: path.Join(*dir, "db"),
	}, bridge.Accessory, accessories...)
	if err != nil {
		log.Info.Panic(err)
	}

	hc.OnTermination(func() {
		<-transport.Stop()

		time.Sleep(100 * time.Millisecond)
		os.Exit(1)
	})

	transport.Start()
}
