package fanout

import "time"

const (
	maxIPCount     = 100
	maxWorkerCount = 32
	minWorkerCount = 2
	maxTimeout     = 2 * time.Second
	defaultTimeout = 30 * time.Second
	readTimeout    = 2 * time.Second
	tcptlc         = "tcp-tls"
)
