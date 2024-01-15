package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/jasonlvhit/gocron"
	log "github.com/sirupsen/logrus"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
)

const (
	ENABLE             = "on"
	DISABLE            = "off"
	RelaysServiceHost  = "RELAYS_SERVICE_HOST"
	SensorsServiceHost = "SENSORS_SERVICE_HOST"
	RulesServiceHost   = "RULES_SERVICE_HOST"
)

type Sensors struct {
	Results []struct {
		StatementID int `json:"statement_id"`
		Series      []struct {
			Name    string          `json:"name"`
			Columns []string        `json:"columns"`
			Values  [][]interface{} `json:"values"`
		} `json:"series"`
	} `json:"results"`
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
		Temperature    float32         `json:"temperature"`
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

func init() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)
	log.SetReportCaller(true)
	log.SetFormatter(&log.TextFormatter{
		ForceColors:     true,
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			return "->", fmt.Sprintf("[%10s:%4d]", filepath.Base(f.File), f.Line)
		},
	})
	log.Info("initialization application")
}

func main() {
	log.Info("run cron job every minutes")
	gocron.Every(1).Minute().Do(manageRelays)
	<-gocron.Start()
}

func manageRelays() {
	rules, err := getRules()
	if err == nil {
		for _, r := range rules.RuleSensors {
			currentTemp, err := getCurrentTemperatureByPinAndDec(r.Pin, r.Dec)
			if err != nil {
				log.Error("error reading sensors: ", err)
			}
			relayId, temperature, enable := getRuleByPinAndDec(rules, r.Pin, r.Dec)
			log.Info("relayId:", relayId, "; enable:", enable, "; [", currentTemp, "<", temperature, "]")
			if relayId != -1 || temperature != -1 || currentTemp != -1 {
				statusRelay := DISABLE
				if currentTemp < temperature && enable {
					statusRelay = ENABLE
				}
				err := sendRelayStatus(relayId, statusRelay)
				if err != nil {
					log.Error("error sending relay status: ", err)
				}
			}
		}
	} else {
		log.Error("error parsing rules: ", err)
	}
	updateCircuitParentRelayStatus()
}

func getCurrentTemperatureByPinAndDec(pin int, dec string) (temp float32, err error) {
	temp = -1
	sensors, err := getSensors(pin, dec)
	if err != nil {
		return
	}
	t, err := getFloat(sensors.Results[0].Series[0].Values[0][1])
	if err != nil {
		return
	} else {
		temp = float32(t)
	}
	return
}

func getFloat(unk interface{}) (float64, error) {
	v := reflect.ValueOf(unk)
	v = reflect.Indirect(v)
	if !v.Type().ConvertibleTo(reflect.TypeOf(float64(0))) {
		return 0, fmt.Errorf("cannot convert %v to float64", v.Type())
	}
	fv := v.Convert(reflect.TypeOf(float64(0)))
	return fv.Float(), nil
}

func updateCircuitParentRelayStatus() {
	circuits, err := getCircuits()
	if err == nil {
		for _, c := range circuits.Circuit {
			err = sendRelayStatus(c.ParentRelayID, analyzeParentRelayStateOfCircuit(c.CircuitsRelays))
			if err != nil {
				log.Error("error sending relay status: ", err)
			}
		}
	} else {
		log.Error("error getting Circuits: ", err)
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
	} else {
		log.Error("error getting relay status:", err)
	}
	return DISABLE
}

func sendRelayStatus(relayId int, status string) error {
	uri := "http://" + getRelaysServiceHost() + "/relay"
	switcher := Switcher{ID: relayId, Switch: status}
	log.Info("send POST: "+uri+"; body=", switcher)
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

func getSensors(pin int, dec string) (sensors Sensors, err error) {
	var decParam = ""
	if len(dec) != 0 {
		decParam = " AND dec='" + dec + "'"
	}
	params := url.Values{}
	params.Add("u", "telegraf")
	params.Add("p", "telegraf")
	params.Add("db", "telegraf")
	params.Add("q", fmt.Sprintf("SELECT MOVING_AVERAGE(value,8) FROM climate_sensors WHERE (pin='%d'%s AND topic='home/sensors/temperature') ORDER BY time DESC LIMIT 1;", pin, decParam))
	err = getJSON("http://"+getSensorsServiceHost()+"/query?"+params.Encode(), &sensors)
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
