package metrics

import (
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
)

var statsdClient *statsd.Client

// RegisterStatsd creates a new global statsd client
func RegisterStatsd() error {
	host, port := os.Getenv("DD_AGENT_HOST"), os.Getenv("DD_DOGSTATSD_PORT")

	if host == "" || port == "" {
		log.Info("STATSD: statsd_host and statsd_port environment variables not set, ignoring metrics")
		return nil
	}

	addr := fmt.Sprintf("%s:%s", host, port)
	sd, err := statsd.New(addr)
	if err != nil {
		return fmt.Errorf("failed to create statsd client: %w", err)
	}

	statsdClient = sd
	return nil
}

const (
	sdRequestsTotal              = "cloudapi.api.requests.all.count"
	sdRequestsEliteAccount       = "cloudapi.api.requests.per_elite_account.count"
	sdForwardToBuilders          = "cloudapi.forward_to_builders.count"
	sdHandleMevBlockEventAll     = "cloudapi.internal_api.handle_mev_block_event.all.count"
	sdHandleMevBlockEventUniques = "cloudapi.internal_api.handle_mev_block_event.uniques.count"
	sdHandleMevBlockLag          = "cloudapi.internal_api.handle_mev_block_event.all.lag"
	shHTTPConnections            = "cloudapi.http.connections.all.count"
	sdWsConnections              = "cloudapi.http.connections.websockets.count"
	sdWsConnectionsDuration      = "cloudapi.http.connections.websockets.duration"
)

// Notification related metric names
const (
	SdNotificationsCreated              = "cloudapi.feed.notifications.created.count"
	SdNotificationsProcessed            = "cloudapi.feed.notifications.processed.count"
	SdNotificationsDelivered            = "cloudapi.feed.notifications.delivered.count"
	SdExtFeedNotificationsReceived      = "cloudapi.ext_feed.notifications.received.count"
	SdExtFeedNotificationsReceiveFailed = "cloudapi.ext_feed.notifications.receive_failed.count"
)

// IncrRPCRequestsTotal increments total number of incoming rpc requests
func IncrRPCRequestsTotal(networkNum uint32, rpcMethod string) {
	if statsdClient == nil {
		return
	}

	tags := []string{
		tag(NetworkNum, networkNum),
		tag(RPCMethod, rpcMethod),
	}

	if err := statsdClient.Incr(sdRequestsTotal, tags, 1); err != nil {
		log.Errorf("Failed to update metric %s: %v", sdRequestsTotal, err)
	}
}

// IncrHandleMevBlockEventAll increments total (before deduplication) number of mev block events received by grpc.
func IncrHandleMevBlockEventAll(relayIP string) {
	if statsdClient == nil {
		return
	}

	tags := []string{
		tag(MevBlockEventRelayIPTag, relayIP),
	}

	if err := statsdClient.Incr(sdHandleMevBlockEventAll, tags, 1); err != nil {
		log.Errorf("Failed to update metric %s: %v", sdHandleMevBlockEventAll, err)
	}
}

// IncrHandleMevBlockEventUniques increments unique number of mev block events received by grpc.
func IncrHandleMevBlockEventUniques(relayIP string) {
	if statsdClient == nil {
		return
	}

	tags := []string{
		tag(MevBlockEventRelayIPTag, relayIP),
	}

	if err := statsdClient.Incr(sdHandleMevBlockEventUniques, tags, 1); err != nil {
		log.Errorf("Failed to update metric %s: %v", sdHandleMevBlockEventUniques, err)
	}
}

// AddHandleMevBlockLag adds a mev block events lag
func AddHandleMevBlockLag(relayIP string, secs float64) {
	if statsdClient == nil {
		return
	}

	tags := []string{
		tag(MevBlockEventRelayIPTag, relayIP),
	}

	if err := statsdClient.Histogram(sdHandleMevBlockLag, secs, tags, 1); err != nil {
		log.Errorf("Failed to update metric %s: %v", sdHandleMevBlockLag, err)
	}
}

// IncrRPCRequestsElites increments number of incoming rpc requests made by elite users
func IncrRPCRequestsElites(networkNum uint32, rpcMethod string, accountID string) {
	if statsdClient == nil {
		return
	}

	tags := []string{
		tag(NetworkNum, networkNum),
		tag(RPCMethod, rpcMethod),
		tag(AccountID, accountID),
	}

	if err := statsdClient.Incr(sdRequestsEliteAccount, tags, 1); err != nil {
		log.Errorf("Failed to update metric %s: %v", sdRequestsEliteAccount, err)
	}
}

// IncrFeedNotification increments number of feed notifications for given network and feed name
func IncrFeedNotification(networkNum uint32, name string, trigger string) {
	tags := []string{
		tag(FeedName, name),
		tag(NetworkNum, networkNum),
	}
	Incr(trigger, tags)
}

// IncrFeedNotificationDelivered increments number of feed notifications delivered for given network, feed name and account id
func IncrFeedNotificationDelivered(networkNum uint32, name, accountID string) {
	tags := []string{
		tag(FeedName, name),
		tag(NetworkNum, networkNum),
		tag(AccountID, accountID),
	}
	Incr(SdNotificationsDelivered, tags)
}

// Incr serves as a generic func for incrementing given metric
func Incr(name string, tags []string) {
	if statsdClient == nil {
		return
	}

	go func() {
		if err := statsdClient.Incr(name, tags, 1); err != nil {
			log.Errorf("Failed to update metric %s: %v", name, err)
		}
	}()
}

// IncrForwardToBuilders increments number of forwarding to builders
func IncrForwardToBuilders(endpoint string, method string, success bool) {
	if statsdClient == nil {
		return
	}

	status := Error
	if success {
		status = Ok
	}

	tags := []string{
		tag(BuilderEndpoint, endpoint),
		tag(Status, status),
		tag(RPCMethod, method),
	}

	if err := statsdClient.Incr(sdForwardToBuilders, tags, 1); err != nil {
		log.Errorf("Failed to update metric %s: %v", sdForwardToBuilders, err)
	}
}

// IncrForwardToExtBuilders increments number of forwarding to external BSC builders
func IncrForwardToExtBuilders(endpoint, method, accountID string, httpStatusCode int, rpcStatusCode int64, success bool) {
	if statsdClient == nil {
		return
	}

	status := Error
	if success {
		status = Ok
	}

	tags := []string{
		tag(BuilderEndpoint, endpoint),
		tag(Status, status),
		tag(RPCMethod, method),
		tag(AccountID, accountID),
		tag(RPCStatusCode, rpcStatusCode),
		tag(HTTPStatusCode, httpStatusCode),
	}

	if err := statsdClient.Incr(sdForwardToBuilders, tags, 1); err != nil {
		log.Errorf("Failed to update metric %s: %v", sdForwardToBuilders, err)
	}
}

// GaugeHTTPConnections measures number of all http connections and active websocket connections at the moment.
func GaugeHTTPConnections(all float64, wsOnly float64) {
	if statsdClient == nil {
		return
	}

	if err := statsdClient.Gauge(shHTTPConnections, all, nil, 1); err != nil {
		log.Errorf("Failed to update metric %s: %v", shHTTPConnections, err)
	}
	if err := statsdClient.Gauge(sdWsConnections, wsOnly, nil, 1); err != nil {
		log.Errorf("Failed to update metric %s: %v", sdWsConnections, err)
	}
}

// AddWsConnectionDuration measures duration of websocket connections
func AddWsConnectionDuration(dur time.Duration) {
	if statsdClient == nil {
		return
	}

	if err := statsdClient.Histogram(sdWsConnectionsDuration, dur.Seconds(), nil, 1); err != nil {
		log.Errorf("Failed to update metric %s: %v", sdWsConnectionsDuration, err)
	}
}

func tag(name string, val any) string {
	return fmt.Sprintf("%s:%v", name, val)
}
