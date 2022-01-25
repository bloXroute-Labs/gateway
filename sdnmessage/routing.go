package sdnmessage

// SDNRoutingConfig represents configuration sent down from the SDN intended for controlling routing behavior
// Currently, the relay proxy is expected to only use TransactionPaidDelay
type SDNRoutingConfig struct {
	HighLatencyThreshold                 int  `json:"high_latency_threshold_ms"`
	UltraHighLatencyThreshold            int  `json:"ultra_high_latency_threshold_ms"`
	LowBandwidthThroughputThreshold      int  `json:"low_bandwidth_threshold_sent_throughput_bytes"`
	UltraLowBandwidthThroughputThreshold int  `json:"ultra_low_bandwidth_threshold_sent_throughput_bytes"`
	LowBandwidthBacklogThreshold         int  `json:"low_bandwidth_threshold_backlog_bytes"`
	UltraLowBandwidthBacklogThreshold    int  `json:"ultra_low_bandwidth_threshold_backlog_bytes"`
	ConnectionHealthUpdateInterval       int  `json:"connection_health_update_interval_s"`
	EnableRoutingTables                  bool `json:"enable_routing_tables"`
	MaxTransactionElapsedTime            int  `json:"max_transaction_elapsed_time_s"`
	BroadcastAllChinaRelays              bool `json:"broadcast_to_all_china_relays"`
	NoRoutesToSameCountry                bool `json:"no_routes_to_same_country"`
	UnpaidTransactionPropagationDelay    int  `json:"unpaid_transaction_propagation_delay_ms"`
	TransactionPropagationDelay          int  `json:"transaction_propagation_delay_ms"`
	CENProcessingDelay                   int  `json:"cen_processing_delay_ms"`
	DirectRouteBuffer                    int  `json:"direct_route_buffer_ms"`
	ATRNoPrivateIPForwardingRoutes       bool `json:"atr_no_private_ip_forwarding_routes"`
}
