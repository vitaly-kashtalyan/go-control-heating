package main

import (
	"encoding/json"
	"fmt"
	"github.com/jasonlvhit/gocron"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

const (
	ENABLE  = "on"
	DISABLE = "off"
)

type Relays struct {
	Relay []relay `json:"relays"`
}

type relay struct {
	Pin         int     `json:"pin"`
	DecSensor   string  `json:"dec"`
	Temperature float32 `json:"temperature"`
	Relay       int     `json:"relay"`
	Name        string  `json:"name"`
}

type Sensors struct {
	Status  int      `json:"status"`
	Message string   `json:"message"`
	Data    []sensor `json:"data"`
}

type sensor struct {
	Pin         int       `json:"pin"`
	DecSensor   string    `json:"dec"`
	Temperature float32   `json:"temperature"`
	Humidity    float32   `json:"humidity"`
	CreatedAt   time.Time `json:"date"`
}

type RelayStatus struct {
	Status  int         `json:"status"`
	Message string      `json:"message"`
	Data    map[int]int `json:"data"`
}

func main() {
	s := gocron.NewScheduler()
	s.Every(1).Minute().Do(manageRelays)
	<-s.Start()
}

func manageRelays() {
	sensors := Sensors{}
	if err := getJSON("http://192.168.0.8:8084", &sensors); err == nil {
		if sensors.Message == "OK" {
			for _, v := range sensors.Data {
				relayId, temperature := getRuleByPinAndDec(v.Pin, v.DecSensor)
				if relayId != -1 || temperature != -1 {
					statusRelay := DISABLE
					if v.Temperature < temperature {
						statusRelay = ENABLE
					}
					_ = sendRelayStatus(relayId, statusRelay)
				}
			}
		}
	} else {
		fmt.Printf("manageRelays: %v", err)
	}
	_ = sendRelayStatus(0, getFloorState())
}

func getFloorState() string {
	relayStatus := RelayStatus{}
	if err := getJSON("http://192.168.0.8:8082/status", &relayStatus); err == nil {
		if relayStatus.Status == http.StatusOK {
			for key, num := range relayStatus.Data {
				if key%2 == 0 && key > 0 && num == 1 {
					return ENABLE
				}
			}
		}
	}
	return DISABLE
}

func sendRelayStatus(relayId int, status string) error {
	url := fmt.Sprintf("http://192.168.0.8:8082/relays/%s/%d", status, relayId)
	fmt.Printf("GET: %s\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("cannot fetch URL %q: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected http GET status: %s", resp.Status)
	}

	return nil
}

func getRuleByPinAndDec(pin int, dec string) (int, float32) {
	jsonFile, err := os.Open("rules.json")
	if err != nil {
		fmt.Println(err)
	}
	defer jsonFile.Close()
	byteValue, _ := ioutil.ReadAll(jsonFile)

	var relays Relays
	_ = json.Unmarshal(byteValue, &relays)

	for i := 0; i < len(relays.Relay); i++ {
		if pin == relays.Relay[i].Pin && dec == relays.Relay[i].DecSensor {
			return relays.Relay[i].Relay, relays.Relay[i].Temperature
		}
	}
	return -1, -1
}

func getJSON(url string, result interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("cannot fetch URL %q: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected http GET status: %s", resp.Status)
	}

	err = json.NewDecoder(resp.Body).Decode(result)
	if err != nil {
		return fmt.Errorf("cannot decode JSON: %v", err)
	}
	return nil
}
