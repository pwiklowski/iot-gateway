package main

import (
	"encoding/json"
	"log"

	"container/list"

	"github.com/tidwall/gjson"
	"gopkg.in/kataras/iris.v6/adaptors/websocket"
)

type ClientConnectionServer struct {
	WebSocketServer      websocket.Server
	HubConnections       *list.List
	WebClientConnections *list.List
}

type ResponseIotHubDevices struct {
	Uuid    string       `json:"uuid"`
	Name    string       `json:"name"`
	Devices []*IotDevice `json:"devices"`
}

type WebClientSubscription struct {
	Uuid    string `json:"uuid"`
	HubUuid string `json:"uuid"`
}

type WebClientConnection struct {
	Username      string
	Connection    websocket.Connection
	Subscriptions *list.List
}

func (server *ClientConnectionServer) getHubConnection(hubUUID string) *HubConnection {
	for e := server.HubConnections.Front(); e != nil; e = e.Next() {
		con := e.Value.(*HubConnection)
		if con.Uuid == hubUUID {
			return con
		}
	}
	return nil
}

func (server *ClientConnectionServer) notifyDeviceListChange() {
	for e := server.WebClientConnections.Front(); e != nil; e = e.Next() {
		con := e.Value.(*WebClientConnection)
		devicesList := createDeviceList(con, server.HubConnections)
		devs, _ := json.Marshal(devicesList)
		sendResponse(con.Connection, -1, "EventDeviceListUpdate", `{"hubs":`+string(devs)+`}`)
	}
}
func (server *ClientConnectionServer) notifyDeviceResourceChange(hubUUID string, uuid string) {
	log.Println("notifyDeviceResourceChange" + uuid)
	for e := server.WebClientConnections.Front(); e != nil; e = e.Next() {
		con := e.Value.(*WebClientConnection)
		log.Println("web client connection sub count=", con.Subscriptions.Len())
		for e := con.Subscriptions.Front(); e != nil; e = e.Next() {
			s := e.Value.(*WebClientSubscription)
			if s.Uuid == uuid && s.HubUuid == hubUUID {
				server.sendDeviceUpdateEvent(con, uuid, hubUUID)
			}
		}

	}
}

//New client connection server
func NewClientEndpoint(hubConnections *list.List, webClientConnections *list.List) *ClientConnectionServer {
	server := ClientConnectionServer{}
	server.HubConnections = hubConnections
	server.WebClientConnections = webClientConnections

	server.WebSocketServer = websocket.New(websocket.Config{
		Endpoint:       "/connectClient",
		MaxMessageSize: 102400,
	})
	server.WebSocketServer.OnConnection(func(c websocket.Connection) {
		server.onClientConnect(c, server.HubConnections)
	})
	return &server
}

func (server *ClientConnectionServer) onClientConnect(c websocket.Connection, hubConnections *list.List) {
	log.Println("New web client connection", c.ID())
	newConnection := &WebClientConnection{
		Connection:    c,
		Subscriptions: list.New(),
	}

	server.WebClientConnections.PushBack(newConnection)

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
				sendResponse(newConnection.Connection, mid, "ResponseAuthorize", `{"status":"error"}`)
				c.Disconnect()
				return
			}
			log.Println("New connection authorized for " + userInfo.Username)

			newConnection.Username = userInfo.Username

			sendResponse(newConnection.Connection, mid, "ResponseAuthorize", `{"status":"ok"}`)

		} else if eventName == "RequestGetDevices" {
			server.handleGetDeviceList(newConnection, mid)
		} else if eventName == "RequestSubscribeDevice" {
			server.handleRequestSubscribeDevice(newConnection, messageJson.Get("payload.uuid").String(), messageJson.Get("payload.hubUuid").String())
		} else if eventName == "RequestUnsubscribeDevice" {
			server.handleRequestUnsubscribeDevice(newConnection, messageJson.Get("payload.uuid").String(), messageJson.Get("payload.hubUuid").String())
		}

	})

	c.OnDisconnect(func() {
		log.Println("Connection with ID: " + c.ID() + " has been disconnected!")
		for e := server.WebClientConnections.Front(); e != nil; e = e.Next() {
			con := e.Value.(*WebClientConnection)
			if con.Connection.ID() == c.ID() {
				server.WebClientConnections.Remove(e)
				break
			}
		}
	})
}

func (server *ClientConnectionServer) sendDeviceUpdateEvent(conn *WebClientConnection, uuid string, hubUuid string) {
	hubConnection := server.getHubConnection(hubUuid)
	if hubConnection != nil {
		device := hubConnection.getDevice(uuid)
		deviceData, _ := json.Marshal(device)
		sendResponse(conn.Connection, -1, "EventDeviceUpdate", string(deviceData))
	}
}

func (server *ClientConnectionServer) handleRequestSubscribeDevice(conn *WebClientConnection, uuid string, hubUuid string) {
	log.Println("Add subscribe " + uuid + " " + hubUuid)
	//todo check if client can subscribe to device on hub

	sub := &WebClientSubscription{
		Uuid:    uuid,
		HubUuid: hubUuid,
	}
	conn.Subscriptions.PushBack(sub)

	server.sendDeviceUpdateEvent(conn, uuid, hubUuid)
}

func (server *ClientConnectionServer) handleRequestUnsubscribeDevice(conn *WebClientConnection, uuid string, hubUuid string) {
	for e := conn.Subscriptions.Front(); e != nil; e = e.Next() {
		s := e.Value.(*WebClientSubscription)
		if s.Uuid == uuid && s.HubUuid == hubUuid {
			conn.Subscriptions.Remove(e)
			return
		}
	}
}

func createDeviceList(conn *WebClientConnection, hubConnections *list.List) []ResponseIotHubDevices {
	var devicesList []ResponseIotHubDevices

	for e := hubConnections.Front(); e != nil; e = e.Next() {
		con := e.Value.(*HubConnection)
		log.Println(con)
		if con.Username != "" && con.Username == conn.Username {
			devices := ResponseIotHubDevices{}
			devices.Uuid = con.Uuid //hub data
			devices.Name = con.Name //hub data

			for d := con.DeviceList.Front(); d != nil; d = d.Next() {
				device := d.Value.(*IotDevice)
				device.HubUUID = con.Uuid
				devices.Devices = append(devices.Devices, device)
			}
			devicesList = append(devicesList, devices)
		}
	}
	return devicesList
}

func (server *ClientConnectionServer) handleGetDeviceList(conn *WebClientConnection, mid int64) {
	devicesList := createDeviceList(conn, server.HubConnections)
	devs, _ := json.Marshal(devicesList)
	sendResponse(conn.Connection, mid, "ResponseGetDevices", `{"hubs":`+string(devs)+`}`)
}
