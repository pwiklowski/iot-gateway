package main

import (
	"container/list"
	"log"
	"strconv"

	"github.com/tidwall/gjson"

	"gopkg.in/kataras/iris.v6/adaptors/websocket"
)

type AlexaConnectionEndpoint struct {
	WebSocketServer websocket.Server
	HubConnections  *list.List
}

type IotVariable struct {
	Interface    string `json:"if"`
	ResourceType string `json:"rt"`
	Href         string `json:"href"`
	Name         string `json:"n"`
	Value        string
}

type IotDevice struct {
	ID        string
	Name      string
	Variables []*IotVariable `json:"variables"`
}

func (device *IotDevice) getVariable(href string) *IotVariable {
	for _, variable := range device.Variables {
		if variable.Href == href {
			return variable
		}
	}
	return nil
}

func (connection *ClientConnection) getDevice(uuid string) *IotDevice {
	for device := connection.DeviceList.Front(); device != nil; device = device.Next() {
		if device.Value.(*IotDevice).ID == uuid {
			return device.Value.(*IotDevice)
		}
	}
	return nil
}

//New client connection server
func NewHubEndpoint(hubConnections *list.List) *ClientConnectionServer {
	server := ClientConnectionServer{}
	server.HubConnections = hubConnections

	server.WebSocketServer = websocket.New(websocket.Config{
		Endpoint:       "/connect",
		MaxMessageSize: 102400,
	})
	server.WebSocketServer.OnConnection(func(c websocket.Connection) {
		onHubConnect(c, server.HubConnections)
	})
	return &server
}

func onHubConnect(c websocket.Connection, hubConnections *list.List) {
	log.Println("New connection", c.ID())
	newConnection := &ClientConnection{
		Connection: c,
		Mid:        1,
		Callbacks:  make(map[int64]RequestCallback)}
	hubConnections.PushBack(newConnection)

	c.OnMessage(func(messageBytes []byte) {
		message := string(messageBytes)
		messageJson := gjson.Parse(message)

		mid := gjson.Get(message, "mid").Int()

		callback := newConnection.Callbacks[mid]
		if callback != nil {
			callback(message)
			delete(newConnection.Callbacks, mid)
		}

		eventName := messageJson.Get("name").String()

		log.Println("Event: " + eventName)
		log.Println("Event: " + messageJson.String())
		if eventName == "RequestAuthorize" {
			token := messageJson.Get("payload.token").String()
			userInfo, err := GetUserInfo(token, getAuthData(AUTH_HUB))
			if err != nil {
				log.Println(err)
				return
			}
			if userInfo.Username == "" {
				log.Println("Connection not authorized")
				c.Disconnect()
				return
			}
			log.Println("New connection authorized for " + userInfo.Username)
			sendRequest(newConnection, `{"name":"RequestGetDevices"}`, func(response string) {
				parseDeviceList(newConnection, response)
			})
			newConnection.Username = userInfo.Username
			newConnection.Uuid = messageJson.Get("payload.uuid").String()
			newConnection.Name = messageJson.Get("payload.name").String()
		} else if eventName == "EventDeviceListUpdate" {
			parseDeviceList(newConnection, message)
		} else if eventName == "EventValueUpdate" {
			handleValueUpdate(newConnection, messageJson)
		}
	})

	c.OnDisconnect(func() {
		for e := hubConnections.Front(); e != nil; e = e.Next() {
			con := e.Value.(*ClientConnection)
			if con.Connection.ID() == c.ID() {
				hubConnections.Remove(e)
				break
			}
		}
		log.Println("Connection with ID: " + c.ID() + " has been disconnected!")
	})

}

func handleValueUpdate(conn *ClientConnection, message gjson.Result) {

	deviceID := message.Get("payload.di").String()
	resourceID := message.Get("payload.resource").String()
	value := message.Get("payload.value").String()

	log.Println("handleValueUpdate " + deviceID + " " + resourceID)

	device := conn.getDevice(deviceID)
	if device == nil {
		log.Println("Unable to find device with ID" + deviceID)
		return
	}
	device.getVariable(resourceID).Value = value

	log.Println("handleValueUpdate " + conn.getDevice(deviceID).getVariable(resourceID).Value)
}
func parseDeviceList(conn *ClientConnection, message string) {
	devices := gjson.Get(message, "payload.devices").Array()
	//Add new devices
	for _, deviceData := range devices {
		deviceID := deviceData.Get("id").String()

		if conn.getDevice(deviceID) != nil {
			continue
		}
		log.Println("Add new device id" + deviceID)
		d := &IotDevice{
			ID:   deviceID,
			Name: deviceData.Get("name").String(),
		}

		sendRequest(conn, `{"name":"RequestSubscribeDevice", "uuid":"`+d.ID+`"}`, nil)

		for _, variableData := range deviceData.Get("variables").Array() {
			v := &IotVariable{
				Href:         variableData.Get("href").String(),
				Name:         variableData.Get("n").String(),
				Interface:    variableData.Get("if").String(),
				ResourceType: variableData.Get("rt").String(),
				Value:        variableData.Get("values").String(),
			}
			d.Variables = append(d.Variables, v)
		}
		conn.DeviceList.PushBack(d)
	}
	deviceIDs := gjson.Get(message, "payload.devices.#.id").Array()

	for device := conn.DeviceList.Front(); device != nil; device = device.Next() {
		found := false
		for _, deviceID := range deviceIDs {
			if device.Value.(*IotDevice).ID == deviceID.String() {
				found = true
			}
		}
		if !found {
			log.Println("Remove device id" + device.Value.(*IotDevice).ID)

			sendRequest(conn, `{"name":"RequestUnsubscribeDevice", "uuid":"`+device.Value.(*IotDevice).ID+`"}`, nil)
			conn.DeviceList.Remove(device)
		}
	}
}
func sendRequest(conn *ClientConnection, payload string, callback RequestCallback) {
	log.Println("sendRequest " + payload)
	if callback != nil {
		conn.Callbacks[conn.Mid] = callback
	}
	conn.Connection.EmitMessage([]byte(`{ "mid":` + strconv.FormatInt(conn.Mid, 10) + `, "payload":` + payload + `}`))
	conn.Mid++
}

func sendResponse(conn *ClientConnection, mid int64, name string, payload string) {
	log.Println("sendRequest " + payload)
	conn.Connection.EmitMessage([]byte(`{ "mid":` + strconv.FormatInt(mid, 10) + `,"name":"` + name + `", "payload":` + payload + `}`))
}
