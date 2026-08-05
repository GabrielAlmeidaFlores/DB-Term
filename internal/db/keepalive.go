package db

import "time"

// PingInterval is the time between keepalive pings to open connections.
// A ping is a lightweight "SELECT 1" sent to prevent the server from
// closing idle connections.
const PingInterval = 3 * time.Minute
