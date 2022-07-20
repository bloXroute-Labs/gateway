package sdnmessage

import (
	"bytes"
	"encoding/json"
	"fmt"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// NodeModel represents metadata on a given node in the bloxroute network
type NodeModel struct {
	NodeType                  string           `json:"node_type"`
	GatewayMode               string           `json:"gateway_mode"`
	ExternalPort              int64            `json:"external_port"`
	NonSSLPort                int              `json:"non_ssl_port"`
	ExternalIP                string           `json:"external_ip"`
	Online                    bool             `json:"online"`
	SdnConnectionAlive        bool             `json:"sdn_connection_alive"`
	Network                   string           `json:"network"`
	Protocol                  string           `json:"protocol"`
	NodeID                    types.NodeID     `json:"node_id"`
	SidStart                  interface{}      `json:"sid_start"`
	SidEnd                    interface{}      `json:"sid_end"`
	NextSidStart              interface{}      `json:"next_sid_start"`
	NextSidEnd                interface{}      `json:"next_sid_end"`
	SidExpireTime             int              `json:"sid_expire_time"`
	LastPongTime              float64          `json:"last_pong_time"`
	IsGatewayMiner            bool             `json:"is_gateway_miner"`
	IsInternalGateway         bool             `json:"is_internal_gateway"`
	SourceVersion             string           `json:"source_version"`
	ProtocolVersion           interface{}      `json:"protocol_version"`
	BlockchainNetworkNum      types.NetworkNum `json:"blockchain_network_num"`
	BlockchainIP              string           `json:"blockchain_ip"`
	BlockchainPort            int              `json:"blockchain_port"`
	BlockchainPeers           string           `json:"blockchain_peers"`
	Hostname                  string           `json:"hostname"`
	SdnID                     interface{}      `json:"sdn_id"`
	OsVersion                 string           `json:"os_version"`
	Continent                 string           `json:"continent"`
	SplitRelays               bool             `json:"split_relays"`
	Country                   string           `json:"country"`
	Region                    interface{}      `json:"region"`
	Idx                       int64            `json:"idx"`
	HasFullyUpdatedTxService  bool             `json:"has_fully_updated_tx_service"`
	SyncTxsStatus             bool             `json:"sync_txs_status"`
	NodeStartTime             string           `json:"node_start_time"`
	NodePublicKey             string           `json:"node_public_key"`
	BaselineRouteRedundancy   int              `json:"baseline_route_redundancy"`
	BaselineSourceRedundancy  int              `json:"baseline_source_redundancy"`
	PrivateIP                 interface{}      `json:"private_ip"`
	Csr                       string           `json:"csr"`
	Cert                      string           `json:"cert"`
	PlatformProvider          interface{}      `json:"platform_provider"`
	AccountID                 types.AccountID  `json:"account_id"`
	LatestSourceVersion       interface{}      `json:"latest_source_version"`
	ShouldUpdateSourceVersion bool             `json:"should_update_source_version"`
	AssigningShortIds         bool             `json:"assigning_short_ids"`
	NodePrivileges            string           `json:"node_privileges"`
	FirstSeenTime             interface{}      `json:"first_seen_time"`
	IsDocker                  bool             `json:"is_docker"`
	UsingPrivateIPConnection  bool             `json:"using_private_ip_connection"`
	PrivateNode               bool             `json:"private_node"`
	ProgramName               string           `json:"program_name"`
	RelayType                 types.RelayType  `json:"relay_type"`
}

// Pack serializes a NodeModel into a buffer for sending
func (nm NodeModel) Pack() []byte {
	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(nm)
	if err != nil {
		nodeModel, _ := json.MarshalIndent(nm, "", "\t")
		log.Error(fmt.Errorf("unable to encode node model with account id %v error %v", nodeModel, err))
	}
	return buf.Bytes()
}
