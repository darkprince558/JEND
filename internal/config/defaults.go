package config

import (
	"os"
	"time"
)

// AWS / Infrastructure Defaults
const (
	DefaultRegion         = "us-east-1"
	DefaultIdentityPoolID = "us-east-1:6b98c4f2-9fea-4591-8b0f-f34be0a4da23"
	DefaultIoTEndpoint    = "a10ofg7qwmr003-ats.iot.us-east-1.amazonaws.com"
	DefaultAPIEndpoint    = "https://ei6hnj0udh.execute-api.us-east-1.amazonaws.com"
	DefaultS3Bucket       = "jendinfrastackv4-jendtransferbucket839f7c9a-knwjudei1o5l"
	DefaultSTUNServer     = "stun:stun.l.google.com:19302"
)

// Transfer Tuning
const (
	ChunkSize          = 1024 * 64         // 64KB data chunks
	MaxS3FileSize      = 200 * 1024 * 1024 // 200MB S3 upload limit
	ParallelThreshold  = 100 * 1024 * 1024 // 100MB threshold for parallel download
	MaxTextSize        = 1 * 1024 * 1024   // 1MB text content limit
	DefaultConcurrency = 4                 // Default parallel QUIC streams
)

// Network
const (
	MaxConnectionRetries         = 10
	DefaultLocalDiscoveryTimeout = 2 * time.Second
)

// IdentityPoolID returns the configured Identity Pool ID,
// falling back to the default if JEND_IDENTITY_POOL_ID is not set.
func IdentityPoolID() string {
	if id := os.Getenv("JEND_IDENTITY_POOL_ID"); id != "" {
		return id
	}
	return DefaultIdentityPoolID
}
