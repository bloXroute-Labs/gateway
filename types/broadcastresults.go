package types

import "fmt"

// BroadcastResults represents broadcast msg summery results
type BroadcastResults struct {
	RelevantPeers, NotOpenPeers, DisabledPeers, ExcludedPeers, ErrorPeers, SentPeers, SentGatewayPeers int
}

// String returns string of broadcast result
func (br BroadcastResults) String() string {
	return fmt.Sprintf("relevant %v, excluded %v, disabled %v, sent %v, gateways %v, errored %v",
		br.RelevantPeers, br.ExcludedPeers, br.DisabledPeers, br.SentPeers, br.SentGatewayPeers, br.NotOpenPeers+br.ErrorPeers)
}
