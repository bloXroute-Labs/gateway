package websocket

import (
	"encoding/json"
	"fmt"
	log "github.com/bloXroute-Labs/gateway/logger"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/gorilla/websocket"
	"strconv"
	"sync"
)

// Ethereum is a websocket connection to an Ethereum RPC interface
type Ethereum struct {
	hostAndPort                   string
	ws                            *websocket.Conn
	newPendingTransactionsChannel chan *types.BxTransaction
	wg                            *sync.WaitGroup
}

// EthereumResponseOfHash represents the Ethereum RPC response
type EthereumResponseOfHash struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// NewEthereum builds a new connection to an Ethereum RPC websocket
func NewEthereum(hostAndPort string, transactions chan *types.BxTransaction, wg *sync.WaitGroup) *Ethereum {
	newEthereum := &Ethereum{
		hostAndPort:                   hostAndPort,
		newPendingTransactionsChannel: transactions,
		wg:                            wg,
	}
	return newEthereum
}

// Start connects to the Ethereum node and starts its main event loop
func (e Ethereum) Start() error {
	e.run()
	return nil
}

func (e Ethereum) run() {
	log.Info("starting websocket client")
	defer e.wg.Done()

	var ws *websocket.Conn = nil
	var sendingAttempt int = 0

	for {
		//connect to websocket node
		ws = e.connect(e.hostAndPort)

		sendingAttempt++

		hashTransactionRequest := `{"id": "` + strconv.Itoa(sendingAttempt) + `", "method": "` + "eth_subscribe" + `", "params": ["` + "newPendingTransactions" + `"]}`
		err := ws.WriteMessage(websocket.TextMessage, []byte(hashTransactionRequest))
		if err != nil {
			log.Error(err)
			continue
		}
		for {
			_, hashOfNewTransaction, err := ws.ReadMessage()
			if err != nil {
				log.Error(err)
				break
			}
			ethResponse := EthereumResponseOfHash{}
			err = json.Unmarshal(hashOfNewTransaction, &ethResponse)
			if err != nil {
				log.Error(err)
				break
			}
			if ethResponse.Params != nil {
				res := make(map[string]interface{})
				hash := make(map[string]string)
				result := ethResponse.Params.(map[string]interface{})

				res["subscription"] = result["subscription"].(string)
				hash["txHash"] = result["result"].(string)
				res["result"] = hash
				ethResponse = EthereumResponseOfHash{
					JSONRPC: ethResponse.JSONRPC,
					Method:  "subscribe",
					Params:  res,
				}
				newHashTx, err := json.Marshal(ethResponse)
				if err != nil {
					log.Error(err)
					break
				}
				//TODO ask eth node for content if not exist
				//e.newPendingTransactionsChannel <- newHashTx
				if false {
					fmt.Println(newHashTx)
				}
			}
			//TODO tx service from hash to content - common
			//get content of transaction
			//var connection = web3.NewWeb3(providers.NewHTTPProvider("63.34.22.69:8545", 10, false))
			//fmt.Println(connection)
			//pointer, err := e.GetTransactionByHash("nn")
			//fmt.Println(pointer.Data)
		}
		log.Info("reconnect")
	}
}

//func (e websocket) GetTransactionByHash(hash string) (*dto.TransactionResponse, error) {
//
//	var provider providers.ProviderInterface
//	params := make([]string, 1)
//	params[0] = hash
//
//	pointer := &dto.RequestResult{}
//
//	err := provider.SendRequest(pointer, "eth_getTransactionByHash", params)
//
//	if err != nil {
//		fmt.Println(err)
//	}
//	return pointer.ToTransactionResponse()
//}

func (e Ethereum) connect(ipAndPort string) *websocket.Conn {
	dialer := websocket.DefaultDialer
	var ws *websocket.Conn

	for {
		wsSubscriber, _, err := dialer.Dial(ipAndPort, nil)
		if err == nil {
			ws = wsSubscriber
			break
		}
		log.Error(err)
	}
	return ws
}
