package test

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"sync"
	"time"
)

/* main
run main.go first.
*/

// ClientResponse - client response
type ClientResponse struct {
	Jsonrpc string      `json:"JSONRPC"`
	ID      string      `json:"id"`
	Result  interface{} `json:"result"`
}

// ClientRequest - client request
type ClientRequest struct {
	ClientID string        `json:"id"`
	Method   string        `json:"method"`
	Params   []interface{} `json:"params"`
}

func main() {
	colorRed := "\033[31m"
	colorGreen := "\033[32m"
	colorYellow := "\033[33m"

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		testClient(colorRed)
		wg.Done()
	}()
	go func() {
		testClient(colorGreen)
		wg.Done()
	}()
	go func() {
		testClient(colorYellow)
		wg.Done()
	}()
	wg.Wait()
}

func testClient(color string) {
	dialer := websocket.DefaultDialer
	var ws *websocket.Conn

	ws, _, err := dialer.Dial("ws://localhost:1234/", nil)
	if err != nil {
		log.Println(color, err)
	}
	defer ws.Close()

	//subscribe
	writeMessage(color, ws, []byte(`{"id": "1", "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash"]}]})`))
	msg := readMessage(color, ws)
	clientRes := getClientResponse(color, msg)
	idFromServer := clientRes.Result

	//print new tx
	for start := time.Now(); time.Since(start) < time.Second*5; {
		_, p, err := ws.ReadMessage()
		if err != nil {
			log.Fatal(err)
		}
		log.Println(color, string(p))
	}

	//unsubscribe
	request := createUnsubscribeRequest(color, idFromServer)
	log.Println(color, "sending unsubscribe request with id:", idFromServer)
	writeMessage(color, ws, request)
	//msg = readMessage(color, ws)
}

func createUnsubscribeRequest(color string, idFromServer interface{}) []byte {
	var id []interface{}
	id = append(id, idFromServer)
	clientReq := &ClientRequest{
		"1",
		"unsubscribe",
		id,
	}
	response, err := json.Marshal(clientReq)
	if err != nil {
		log.Fatal(color, err)
	}
	return response
}

func getClientResponse(color string, msg []byte) (clientResponse ClientResponse) {
	serverRes := ClientResponse{}
	err := json.Unmarshal(msg, &serverRes)
	if err != nil {
		log.Fatal(color, err)
	}
	return serverRes
}

func readMessage(color string, ws *websocket.Conn) []byte {
	_, msg, err := ws.ReadMessage()
	if err != nil {
		log.Fatal(color, err)
	}
	log.Println(color, string(msg))
	return msg
}

func writeMessage(color string, ws *websocket.Conn, response []byte) {
	if err := ws.WriteMessage(websocket.TextMessage, response); err != nil {
		log.Fatal(color, err)
	}
}
