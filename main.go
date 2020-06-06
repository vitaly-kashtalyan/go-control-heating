package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jasonlvhit/gocron"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

const (
	ENABLE    = "on"
	DISABLE   = "off"
	RulesFile = "rules.json"
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
	Enable      bool    `json:"enable"`
}

type RelaysWithSensors struct {
	Relay []RelayWithSensor `json:"relays"`
}

type RelayWithSensor struct {
	relay
	Sensor sensor `json:"sensor"`
}

type relayPatch struct {
	Pin         int     `json:"pin" binding:"required"`
	DecSensor   string  `json:"dec"`
	Temperature float32 `json:"temperature,omitempty"`
	Name        string  `json:"name,omitempty"`
	Enable      *bool   `json:"enable"`
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
	CreatedAt   time.Time `json:"create_at"`
	UpdatedAt   time.Time `json:"update_at"`
}

type RelayStatus struct {
	Status  int         `json:"status"`
	Message string      `json:"message"`
	Data    map[int]int `json:"data"`
}

func main() {
	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, getRelaysWithSensor())
	})

	r.GET("/rules", func(c *gin.Context) {
		c.JSON(http.StatusOK, getRelays())
	})

	r.PATCH("/rules", func(c *gin.Context) {
		_ = updateRelayFields(c)
		c.JSON(http.StatusNoContent, nil)
	})

	go func() {
		gocron.Every(1).Minute().Do(manageRelays)
		<-gocron.Start()
	}()
	_ = r.Run(":8083")
}

func updateRelayFields(c *gin.Context) error {
	relays := getRelays()
	var jsonBody relayPatch
	if err := c.BindJSON(&jsonBody); err == nil {
		for i := range relays.Relay {
			if jsonBody.Pin == relays.Relay[i].Pin && jsonBody.DecSensor == relays.Relay[i].DecSensor {
				if jsonBody.Name != "" {
					relays.Relay[i].Name = jsonBody.Name
				}
				if jsonBody.Temperature != 0 {
					relays.Relay[i].Temperature = jsonBody.Temperature
				}
				if jsonBody.Enable != nil {
					relays.Relay[i].Enable = *jsonBody.Enable
				}
			}
		}
		return writeObjectToJson(relays)
	} else {
		return err
	}
}

func manageRelays() {
	sensors := getSensors()
	if sensors.Message == "OK" {
		for _, v := range sensors.Data {
			relayId, temperature, enable := getRuleByPinAndDec(v.Pin, v.DecSensor)
			fmt.Println("relayId:", relayId, "; enable:", enable, "; [", v.Temperature, "<", temperature, "]")
			if relayId != -1 || temperature != -1 {
				statusRelay := DISABLE
				if v.Temperature < temperature && enable {
					statusRelay = ENABLE
				}
				_ = sendRelayStatus(relayId, statusRelay)
			}
		}
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

func getRuleByPinAndDec(pin int, dec string) (int, float32, bool) {
	relays := getRelays()
	for i := 0; i < len(relays.Relay); i++ {
		if pin == relays.Relay[i].Pin && dec == relays.Relay[i].DecSensor {
			return relays.Relay[i].Relay, relays.Relay[i].Temperature, relays.Relay[i].Enable
		}
	}
	return -1, -1, false
}

func getRelays() Relays {
	jsonFile, err := os.Open(RulesFile)
	if err != nil {
		fmt.Println(err)
	}
	defer jsonFile.Close()
	byteValue, _ := ioutil.ReadAll(jsonFile)

	var relays Relays
	_ = json.Unmarshal(byteValue, &relays)
	return relays
}
func getSensors() Sensors {
	sensors := Sensors{}
	if err := getJSON("http://192.168.0.8:8084", &sensors); err == nil {
		return sensors
	} else {
		fmt.Printf("Error getting sensors data: %v", err)
	}
	return sensors
}

func getRelaysWithSensor() RelaysWithSensors {
	var relayWithSensor []RelayWithSensor
	relays := getRelays()
	sensors := getSensors()

	for i := 0; i < len(relays.Relay); i++ {
		for _, v := range sensors.Data {
			if v.Pin == relays.Relay[i].Pin && v.DecSensor == relays.Relay[i].DecSensor {
				relayWithSensor = append(relayWithSensor, RelayWithSensor{relay: relays.Relay[i], Sensor: v})
			}
		}
	}
	return RelaysWithSensors{Relay: relayWithSensor}
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

func writeObjectToJson(data interface{}) error {
	file, _ := json.MarshalIndent(data, "", " ")
	return ioutil.WriteFile(RulesFile, file, 0644)
}
