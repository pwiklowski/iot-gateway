package main

import (
	"container/list"
	"io/ioutil"
	"log"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	iris "gopkg.in/kataras/iris.v6"
)

const (
	NAMESPACE_CONTROL   = "Alexa.ConnectedHome.Control"
	NAMESPACE_DISCOVERY = "Alexa.ConnectedHome.Discovery"

	DISCOVER_APPLIANCES_REQUEST  = "DiscoverAppliancesRequest"
	DISCOVER_APPLIANCES_RESPONSE = "DiscoverAppliancesResponse"

	TURN_ON_REQUEST       = "TurnOnRequest"
	TURN_OFF_REQUEST      = "TurnOffRequest"
	TURN_ON_CONFIRMATION  = "TurnOnConfirmation"
	TURN_OFF_CONFIRMATION = "TurnOffConfirmation"

	SET_PERCENTAGE_REQUEST            = "SetPercentageRequest"
	SET_PERCENTAGE_CONFIRMATION       = "SetPercentageConfirmation"
	INCREMENT_PERCENTAGE_REQUEST      = "IncrementPercentageRequest"
	INCREMENT_PERCENTAGE_CONFIRMATION = "IncrementPercentageConfirmation"
	DECREMENT_PERCENTAGE_REQUEST      = "DecrementPercentageRequest"
	DECREMENT_PERCENTAGE_CONFIRMATION = "DecrementPercentageConfirmation"

	MANUFACTURER_NAME = "Wiklosoft"
)

type AlexaHeader struct {
	Namespace      string `json:"namespace"`
	Name           string `json:"name"`
	PayloadVersion string `json:"payloadVersion"`
	MessageID      string `json:"messageId"`
}
type AlexaPayload struct {
	AccessToken string `json:"accessToken"`
}

type AlexaMessage struct {
	Header  AlexaHeader  `json:"header"`
	Payload AlexaPayload `json:"payload"`
}
type AlexaDevice struct {
	ApplicanceID        string `json:"applianceId"`
	ManufacturerName    string `json:"manufacturerName"`
	ModelName           string `json:"modelName"`
	FriendlyName        string `json:"friendlyName"`
	FriendlyDescription string `json:"friendlyDescription"`
	IsReachable         bool   `json:"isReachable"`
	Version             string `json:"version"`

	Actions                    []string `json:"actions"`
	AdditionalApplianceDetails struct {
	} `json:"additionalApplianceDetails"`
}

type AlexaDiscoveryResponse struct {
	Header  AlexaHeader `json:"header"`
	Payload struct {
		DiscoveredAppliances []AlexaDevice `json:"discoveredAppliances"`
	} `json:"payload"`
}
type AlexaControlResponse struct {
	Header  AlexaHeader `json:"header"`
	Payload struct {
	} `json:"payload"`
}

type AlexaEndpoint struct {
	HubConnections *list.List
}

func NewAlexaEndpoint(app *iris.Framework, hubConnections *list.List) *AlexaEndpoint {
	endpoint := &AlexaEndpoint{}

	app.Post("/", func(c *iris.Context) {
		bodyBytes, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(iris.StatusInternalServerError, nil)
			return
		}
		body := string(bodyBytes)

		token := gjson.Get(body, "payload.accessToken").String()

		userInfo, err := GetUserInfo(token, getAuthData(AUTH_ALEXA))
		if err != nil {
			log.Println(err)
			return
		}

		handleAlexaMessage(body, hubConnections, userInfo, c)
	})
	return endpoint
}

func onTurnOnOffRequest(clientConnection *ClientConnection, device *IotDevice, value bool) {
	setDeviceValue(clientConnection, device.UUID, "/master", `{"value":`+strconv.FormatBool(value)+`}`)
}
func onSetPercentRequest(clientConnection *ClientConnection, device *IotDevice, resource string, value int64) {
	log.Println("onSetPercentRequest " + device.UUID + resource)
	resourceType := device.getVariable(resource).ResourceType
	variable := gjson.Parse(device.getVariable(resource).Value)
	log.Println("onSetPercentRequest " + resource + " variable:" + device.getVariable(resource).Value)
	if resourceType == "oic.r.light.dimming" {
		var max int64
		if !variable.Get("range").Exists() {
			max = 100
		} else {
			var err error
			max, err = strconv.ParseInt(strings.Split(variable.Get("range").String(), ",")[1], 10, 0)
			if err != nil {
				log.Println(err)
				max = 100
			}
		}

		newValue := value * max / 100

		setDeviceValue(clientConnection, device.UUID, resource, `{"dimmingSetting":`+strconv.FormatInt(newValue, 10)+`}`)
	}
}
func onChangePercentRequest(conn *ClientConnection, device *IotDevice, resource string, value int64) {
	resourceType := device.getVariable(resource).ResourceType
	variable := gjson.Parse(device.getVariable(resource).Value)

	if resourceType == "oic.r.light.dimming" {
		var max int64
		if !variable.Get("range").Exists() {
			max = 100
		} else {
			var err error
			max, err = strconv.ParseInt(strings.Split(variable.Get("range").String(), ",")[1], 10, 0)
			if err != nil {
				log.Println(err)
				max = 100
			}
		}

		diffValue := value * max / 100
		prevValue := variable.Get("dimmingSetting").Int()

		newValue := prevValue + diffValue
		log.Println("onChangePercentRequest oldValue:", prevValue, "newValue: ", newValue, " diff:", diffValue)

		if newValue > max {
			newValue = max
		}

		if newValue < 0 {
			newValue = 0
		}
		setDeviceValue(conn, device.UUID, resource, `{"dimmingSetting":`+strconv.FormatInt(newValue, 10)+`}`)
	}
}

func handleAlexaMessage(message string, hubConnections *list.List, userInfo *AuthUserData, c *iris.Context) {
	namespace := gjson.Get(message, "header.namespace").String()

	log.Println("handleAlexaMessage: " + message)
	if namespace == NAMESPACE_DISCOVERY {
		response := &AlexaDiscoveryResponse{}
		response.Header.Name = DISCOVER_APPLIANCES_RESPONSE
		response.Header.Namespace = NAMESPACE_DISCOVERY
		response.Header.PayloadVersion = "2"
		response.Header.MessageID = generateMessageUUID()

		for e := hubConnections.Front(); e != nil; e = e.Next() {
			con := e.Value.(*ClientConnection)
			log.Println(con.Username + " " + userInfo.Username)
			if userInfo.Username != "" && con.Username == userInfo.Username {
				for d := con.DeviceList.Front(); d != nil; d = d.Next() {
					device := d.Value.(*IotDevice)
					log.Println("Adding device " + device.UUID)

					if device.getVariable("/master") != nil {
						dev := AlexaDevice{
							ApplicanceID:        con.Uuid + ":" + device.UUID,
							ManufacturerName:    MANUFACTURER_NAME,
							ModelName:           "The Best Model",
							FriendlyName:        device.Name,
							FriendlyDescription: "OCF Device by Wiklosoft",
							IsReachable:         true,
							Version:             "0.1",
						}

						dev.Actions = append(dev.Actions, "turnOn")
						dev.Actions = append(dev.Actions, "turnOff")
						response.Payload.DiscoveredAppliances = append(response.Payload.DiscoveredAppliances, dev)
					}

					for _, variable := range device.Variables {
						if variable.ResourceType == "oic.r.light.dimming" {
							dev := AlexaDevice{
								ApplicanceID:        con.Uuid + ":" + device.UUID + ":" + strings.Replace(variable.Href, "/", "_", -1),
								ManufacturerName:    MANUFACTURER_NAME,
								ModelName:           "The Best Model",
								FriendlyName:        variable.Name,
								FriendlyDescription: "OCF Resource by Wiklosoft",
								IsReachable:         true,
								Version:             "0.1",
							}

							dev.Actions = append(dev.Actions, "setPercentage")
							dev.Actions = append(dev.Actions, "incrementPercentage")
							dev.Actions = append(dev.Actions, "decrementPercentage")
							response.Payload.DiscoveredAppliances = append(response.Payload.DiscoveredAppliances, dev)
						}
					}
				}
			}
		}
		log.Println(response)
		c.JSON(iris.StatusOK, response)
	} else if namespace == NAMESPACE_CONTROL {
		name := gjson.Get(message, "header.name").String()
		response := &AlexaControlResponse{}

		response.Header.Namespace = NAMESPACE_CONTROL
		response.Header.PayloadVersion = "2"
		response.Header.MessageID = generateMessageUUID()

		applianceID := strings.Split(gjson.Get(message, "payload.appliance.applianceId").String(), ":")

		connectionID := applianceID[0]
		deviceID := applianceID[1]
		resource := ""
		if len(applianceID) == 3 {
			resource = strings.Replace(applianceID[2], "_", "/", -1)
		}
		var clientConnection *ClientConnection
		for e := hubConnections.Front(); e != nil; e = e.Next() {
			if e.Value.(*ClientConnection).Uuid == connectionID {
				clientConnection = e.Value.(*ClientConnection)
			}
		}
		if clientConnection == nil {
			log.Println("Unable to found hub connection: " + connectionID)
			c.JSON(iris.StatusInternalServerError, nil)
			return
		}

		device := clientConnection.getDevice(deviceID)
		if device == nil {
			log.Println("Unable to device connection: " + deviceID)
			c.JSON(iris.StatusInternalServerError, nil)
			return
		}

		if name == TURN_ON_REQUEST {
			response.Header.Name = TURN_ON_CONFIRMATION
			onTurnOnOffRequest(clientConnection, device, true)
		} else if name == TURN_OFF_REQUEST {
			response.Header.Name = TURN_OFF_CONFIRMATION
			onTurnOnOffRequest(clientConnection, device, false)
		} else if name == SET_PERCENTAGE_REQUEST {
			response.Header.Name = SET_PERCENTAGE_CONFIRMATION
			percent := gjson.Get(message, "payload.percentageState.value").Int()
			onSetPercentRequest(clientConnection, device, resource, percent)
		} else if name == INCREMENT_PERCENTAGE_REQUEST {
			response.Header.Name = INCREMENT_PERCENTAGE_CONFIRMATION
			percent := gjson.Get(message, "payload.deltaPercentage.value").Int()
			onChangePercentRequest(clientConnection, device, resource, percent)
		} else if name == DECREMENT_PERCENTAGE_REQUEST {
			response.Header.Name = DECREMENT_PERCENTAGE_CONFIRMATION
			percent := gjson.Get(message, "payload.deltaPercentage.value").Int()
			onChangePercentRequest(clientConnection, device, resource, -percent)
		}

		log.Println(response)
		c.JSON(iris.StatusOK, response)
	}
}
