package main

import (
	"github.com/twinj/uuid"

	"container/list"

	"gopkg.in/kataras/iris.v6"
	"gopkg.in/kataras/iris.v6/adaptors/httprouter"
	"gopkg.in/kataras/iris.v6/adaptors/websocket"
)

type HubConnection struct {
	Username   string
	Connection websocket.Connection
	DeviceList list.List

	Callbacks map[int64]RequestCallback
	Mid       int64
	Uuid      string
	Name      string
}

type RequestCallback func(string)

type IotPayload struct {
	Request string `json:"request"`
}
type IotMessage struct {
	Mid     int        `json:"mid"`
	Payload IotPayload `json:"payload"`
	Event   string     `json:"event"`
}

type IotDevices struct {
	Devices []IotDevice `json:"devices"`
}

type EventDeviceListMessage struct {
	Mid     int        `json:"mid"`
	Payload IotDevices `json:"payload"`
	Event   string     `json:"event"`
}

func generateMessageUUID() string {
	return uuid.NewV4().String()
}
func setDeviceValue(clientConnection *HubConnection, deviceID string, resourceID string, valueObject string) {
	sendRequest(clientConnection, "RequestSetValue", `{"di":"`+deviceID+`","resource":"`+resourceID+`", "value":`+valueObject+`}`, nil)
}

func main() {
	hubConnections := list.New()
	webClietnConnections := list.New()
	app := iris.New()
	app.Adapt(iris.DevLogger(), httprouter.New())

	clientConnectionServer := NewClientEndpoint(hubConnections, webClietnConnections)
	app.Adapt(clientConnectionServer.WebSocketServer)

	hubConnectionServer := NewHubEndpoint(hubConnections, clientConnectionServer)
	app.Adapt(hubConnectionServer.WebSocketServer)


	alexaEndpoint := NewAlexaEndpoint(app, hubConnections)
	_ = alexaEndpoint

	app.Listen(":12345")
}
