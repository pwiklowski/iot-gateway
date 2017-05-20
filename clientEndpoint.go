package main

import (
	"encoding/json"
	"log"

	"container/list"

	"github.com/tidwall/gjson"
	"gopkg.in/kataras/iris.v6/adaptors/websocket"
)

type ClientConnectionServer struct {
	WebSocketServer websocket.Server
	HubConnections  *list.List
}

type ResponseIotHubDevices struct {
	Uuid    string       `json:"uuid"`
	Name    string       `json:"name"`
	Devices []*IotDevice `json:"devices"`
}

//New client connection server
func NewClientEndpoint(hubConnections *list.List) *ClientConnectionServer {
	server := ClientConnectionServer{}
	server.HubConnections = hubConnections

	server.WebSocketServer = websocket.New(websocket.Config{
		Endpoint:       "/connectClient",
		MaxMessageSize: 102400,
	})
	server.WebSocketServer.OnConnection(func(c websocket.Connection) {
		onClientConnect(c, server.HubConnections)
	})
	return &server
}

func onClientConnect(c websocket.Connection, hubConnections *list.List) {
	log.Println("New client connection", c.ID())
	newConnection := &ClientConnection{
		Connection: c,
	}

	c.OnMessage(func(messageBytes []byte) {
		message := string(messageBytes)
		messageJson := gjson.Parse(message)

		mid := gjson.Get(message, "mid").Int()

		eventName := messageJson.Get("name").String()

		log.Println("Event: " + eventName)
		log.Println("Event: " + messageJson.String())
		if eventName == "RequestAuthorize" {
			token := messageJson.Get("payload.token").String()
			userInfo, err := GetUserInfo(token, getAuthData(AUTH_WEB))
			if err != nil {
				log.Println(err)
				return
			}
			if userInfo.Username == "" {
				log.Println("Connection not authorized")
				sendResponse(newConnection, mid, "ResponseAuthorize", `{"status":"error"}`)
				c.Disconnect()
				return
			}
			log.Println("New connection authorized for " + userInfo.Username)

			newConnection.Username = userInfo.Username

			sendResponse(newConnection, mid, "ResponseAuthorize", `{"status":"ok"}`)

		} else if eventName == "RequestGetDevices" {
			handleGetDeviceList(newConnection, hubConnections, mid)
		}

	})

	c.OnDisconnect(func() {
		log.Println("Connection with ID: " + c.ID() + " has been disconnected!")
	})
}

func handleGetDeviceList(conn *ClientConnection, hubConnections *list.List, mid int64) {
	var devicesList []ResponseIotHubDevices

	for e := hubConnections.Front(); e != nil; e = e.Next() {
		con := e.Value.(*ClientConnection)
		log.Println(con)
		if con.Username != "" && con.Username == conn.Username {
			devices := ResponseIotHubDevices{}
			devices.Uuid = con.Uuid
			devices.Name = con.Name

			for d := con.DeviceList.Front(); d != nil; d = d.Next() {
				device := d.Value.(*IotDevice)
				devices.Devices = append(devices.Devices, device)
			}
			devicesList = append(devicesList, devices)
		}
	}

	devs, _ := json.Marshal(devicesList)
	sendResponse(conn, mid, "ResponseGetDevices", `{"hubs":`+string(devs)+`}`)
}
