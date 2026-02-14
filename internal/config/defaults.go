package config

import "os"

const (
	DefaultRegion         = "us-east-1"
	DefaultIdentityPoolID = "us-east-1:6b98c4f2-9fea-4591-8b0f-f34be0a4da23"
	DefaultIoTEndpoint    = "a10ofg7qwmr003-ats.iot.us-east-1.amazonaws.com"
)

// IdentityPoolID returns the configured Identity Pool ID,
// falling back to the default if JEND_IDENTITY_POOL_ID is not set.
func IdentityPoolID() string {
	if id := os.Getenv("JEND_IDENTITY_POOL_ID"); id != "" {
		return id
	}
	return DefaultIdentityPoolID
}
