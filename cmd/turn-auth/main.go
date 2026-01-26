package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// Response structure
type TurnCredentials struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	TTL      int      `json:"ttl"`
	URIs     []string `json:"uris"`
}

func handleRequest(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	secretKey := os.Getenv("TURN_SECRET_KEY")
	if secretKey == "" {
		return errorResponse(500, "Server misconfigured (missing secret)"), nil
	}

	// Dynamic TTL (default 1 hour)
	ttl := 3600
	expiration := time.Now().Add(time.Duration(ttl) * time.Second).Unix()

	// Username = expiration_timestamp : uuid (or just timestamp for simplicity)
	// Standard TURN REST API: username = timestamp:salt (or just timestamp)
	// coturn use-auth-secret format: username = timestamp
	// Wait, coturn with --use-auth-secret expects username to be a timestamp?
	// Coturn checks: username > current_time?
	// Actually, the standard algorithm is:
	// username = <expiry_timestamp>
	// password = HMAC_SHA1(username, secret_key) -> Base64
	//
	// But usually we want a unique username.
	// Coturn supports `timestamp:user_id` format if `use-auth-secret` is set.
	// Let's use `timestamp:random_id`.

	username := fmt.Sprintf("%d:jend-user", expiration)

	// HMAC-SHA1
	mac := hmac.New(sha1.New, []byte(secretKey))
	mac.Write([]byte(username))
	password := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	creds := TurnCredentials{
		Username: username,
		Password: password,
		TTL:      ttl,
		URIs: []string{
			"turn:" + os.Getenv("TURN_URI") + "?transport=udp",
			"turn:" + os.Getenv("TURN_URI") + "?transport=tcp",
		},
	}

	body, _ := json.Marshal(creds)

	return events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

func errorResponse(code int, msg string) events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{
		StatusCode: code,
		Body:       fmt.Sprintf(`{"error":"%s"}`, msg),
	}
}

func main() {
	lambda.Start(handleRequest)
}
