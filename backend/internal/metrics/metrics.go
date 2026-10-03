package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	WSConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "chat_ws_connections",
		Help: "Active websocket connections on this instance",
	})
	MessagesSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chat_messages_sent_total",
	})
	MessagesDelivered = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chat_messages_delivered_total",
	})
	PubSubMessages = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chat_pubsub_messages_total",
	})
)
