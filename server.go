package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var pubsub = NewPubSubServer()

func Publish(conn *websocket.Conn, name string) error {
	p := pubsub.GetTopic(name, true).NewPublisher()

	for {
		var data map[string]interface{}
		err := conn.ReadJSON(&data)
		if err != nil {
			log.Println("readjson", err)
			break
		}
		// log.Println(data)
		if data["action"] == "send" {
			p.Send(data["data"])
		}
		if data["action"] == "close" {
			break
		}
	}
	log.Println("disconnect")
	return nil
}

func Subscribe(conn *websocket.Conn, name string) error {
	t := pubsub.GetTopic(name, true)
	ch := make(chan Event, 10)
	s := &SubscriberCh{Ch: ch}
	t.Subscribe(s)
	defer t.Unsubscribe(s)

	for {
		ev, ok := <-ch
		if !ok {
			break
		}

		err := conn.WriteJSON(&map[string]interface{}{"type": "event", "sender": ev.Sender, "data": ev.Data})
		if err != nil {
			log.Println("WriteJSON", err)
			break
		}
	}
	log.Println("disconnect")
	return nil
}

func parseIntDefault(str string, defvalue int) int {
	v, err := strconv.ParseInt(str, 10, 32)
	if err != nil {
		return defvalue
	}
	return int(v)
}

func wsUrl(r *http.Request, path string) string {
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		return "ws://" + r.Host + path
	}
	return "wss://" + r.Host + path
}

func initStatusHandler(r *gin.Engine) {
	r.GET("/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"_status": 200, "message": "It works!"})
	})
}

func initTopicHandler(r *gin.Engine) {
	var wsupgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	r.GET("/topic/:topic", func(c *gin.Context) {
		name := c.Param("topic")
		topic := pubsub.GetTopic(name, false)
		ws := wsUrl(c.Request, "/topic/"+name+"/")

		c.JSON(http.StatusOK, gin.H{"active": topic != nil, "publishWS": ws + "publish", "subscribeWS": ws + "subscribe"})
	})

	r.GET("/topic/:topic/publish", func(c *gin.Context) {
		conn, err := wsupgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		Publish(conn, c.Param("topic"))
	})

	r.GET("/topic/:topic/subscribe", func(c *gin.Context) {
		conn, err := wsupgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		Subscribe(conn, c.Param("topic"))
	})
}

func initHttpd() *gin.Engine {
	r := gin.Default()
	initStatusHandler(r)
	initTopicHandler(r)
	return r
}

func main() {
	port := flag.Int("p", 8080, "http port")
	flag.Parse()
	gin.SetMode(gin.ReleaseMode)
	log.Printf("start server. port: %d", *port)
	initHttpd().Run(":" + fmt.Sprint(*port))
}
