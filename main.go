package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/jasonlvhit/gocron"
	"net/http"
	"os"
	"time"
)

const (
	ENABLE             = "on"
	DISABLE            = "off"
	RelaysServiceHost  = "RELAYS_SERVICE_HOST"
	SensorsServiceHost = "SENSORS_SERVICE_HOST"
	RulesServiceHost   = "RULES_SERVICE_HOST"
)

type Sensors struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    []struct {
		Pin         int       `json:"pin"`
		Dec         string    `json:"dec"`
		Temperature float32   `json:"temperature"`
		Humidity    float32   `json:"humidity"`
		CreateAt    time.Time `json:"create_at"`
		UpdateAt    time.Time `json:"update_at"`
	} `json:"data"`
}

type RelayStatus struct {
	Relay []struct {
		ID    int `json:"id"`
		State int `json:"state"`
	} `json:"relays"`
}

type Rules struct {
	RuleSensors []struct {
		Pin         int     `json:"pin"`
		Dec         string  `json:"dec"`
		RelayID     int     `json:"relay_id"`
		Temperature float32 `json:"temperature"`
		Enable      bool    `json:"enable"`
	} `json:"sensors"`
}

type Circuits struct {
	Circuit []struct {
		Name           string          `json:"name"`
		Temperature    int             `json:"temperature"`
		ParentRelayID  int             `json:"parent_relay_id"`
		CircuitsRelays []CircuitRelays `json:"relays"`
	} `json:"circuits"`
}

type CircuitRelays struct {
	Pin      int    `json:"pin"`
	Dec      string `json:"dec"`
	RelayID  int    `json:"relay_id"`
	Name     string `json:"name"`
	Enable   bool   `json:"enable"`
	Schedule []struct {
		Time        string  `json:"time"`
		Temperature float32 `json:"temperature"`
	} `json:"schedule"`
}

type Switcher struct {
	ID     int    `json:"id" binding:"required"`
	Switch string `json:"switch" binding:"required"`
}

func main() {
	gocron.Every(1).Minute().Do(manageRelays)
	<-gocron.Start()
}

func manageRelays() {
	sensors, errS := getSensors()
	rules, errR := getRules()
	if errS == nil && errR == nil {
		for _, v := range sensors.Data {
			relayId, temperature, enable := getRuleByPinAndDec(rules, v.Pin, v.Dec)
			fmt.Println("relayId:", relayId, "; enable:", enable, "; [", v.Temperature, "<", temperature, "]")
			if relayId != -1 || temperature != -1 {
				statusRelay := DISABLE
				if v.Temperature < temperature && enable {
					statusRelay = ENABLE
				}
				err := sendRelayStatus(relayId, statusRelay)
				if err != nil {
					fmt.Println("sendRelayStatus:", err)
				}
			}
		}
	} else {
		fmt.Println("getSensors:", errS)
		fmt.Println("getRules:", errR)
	}
	updateCircuitParentRelayStatus()
}

func updateCircuitParentRelayStatus() {
	circuits, err := getCircuits()
	if err == nil {
		for _, c := range circuits.Circuit {
			err = sendRelayStatus(c.ParentRelayID, analyzeParentRelayStateOfCircuit(c.CircuitsRelays))
			if err != nil {
				fmt.Println("sendRelayStatus:", err)
			}
		}
	}
}

func analyzeParentRelayStateOfCircuit(circuitRelays []CircuitRelays) string {
	relayStatus, err := getRelayStatus()
	if err == nil {
		for _, c := range circuitRelays {
			for _, r := range relayStatus.Relay {
				if c.RelayID == r.ID && r.State == 1 {
					return ENABLE
				}
			}
		}
	}
	return DISABLE
}

func sendRelayStatus(relayId int, status string) error {
	uri := "http://" + getRelaysServiceHost() + "/relay"
	switcher := Switcher{ID: relayId, Switch: status}
	fmt.Printf("POST: %s BODY: %v", uri, switcher)
	body := new(bytes.Buffer)
	err := json.NewEncoder(body).Encode(switcher)
	if err != nil {
		return fmt.Errorf("%q: %v", uri, err)
	}
	resp, err := http.Post(uri, "application/json; charset=utf-8", body)
	if err != nil {
		return fmt.Errorf("cannot fetch URL %q: %v", uri, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected http POST status: %s", resp.Status)
	}

	return nil
}

func getRuleByPinAndDec(rules Rules, pin int, dec string) (int, float32, bool) {
	for _, s := range rules.RuleSensors {
		if pin == s.Pin && dec == s.Dec {
			return s.RelayID, s.Temperature, s.Enable
		}
	}
	return -1, -1, false
}

func getSensors() (sensors Sensors, err error) {
	err = getJSON("http://"+getSensorsServiceHost(), &sensors)
	return
}

func getRules() (rules Rules, err error) {
	err = getJSON("http://"+getRulesServiceHost()+"/sensors", &rules)
	return
}

func getCircuits() (rules Circuits, err error) {
	err = getJSON("http://"+getRulesServiceHost()+"/rules", &rules)
	return
}

func getRelayStatus() (relayStatus RelayStatus, err error) {
	err = getJSON("http://"+getRelaysServiceHost()+"/status", &relayStatus)
	return
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

func getRelaysServiceHost() string {
	return os.Getenv(RelaysServiceHost)
}

func getSensorsServiceHost() string {
	return os.Getenv(SensorsServiceHost)
}

func getRulesServiceHost() string {
	return os.Getenv(RulesServiceHost)
}
