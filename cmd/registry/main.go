package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var (
	svc       *dynamodb.Client
	tableName string
)

func init() {
	tableName = os.Getenv("TABLE_NAME")
	if tableName == "" {
		log.Println("TABLE_NAME env var is empty, defaulting to JendRegistry")
		tableName = "JendRegistry"
	}

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	svc = dynamodb.NewFromConfig(cfg)
}

// RegistryItem represents the data stored in DynamoDB
type RegistryItem struct {
	Code      string   `json:"code" dynamodbav:"code"`
	IP        string   `json:"ip" dynamodbav:"ip"`
	Port      int      `json:"port" dynamodbav:"port"`
	Endpoints []string `json:"endpoints,omitempty" dynamodbav:"endpoints,omitempty"` // For candidates
	PublicKey string   `json:"public_key,omitempty" dynamodbav:"public_key,omitempty"`
	ExpiresAt int64    `json:"expires_at" dynamodbav:"expires_at"` // TTL
}

// Handler handles the API Gateway requests
func Handler(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	log.Printf("Processing request %s %s", request.RequestContext.HTTP.Method, request.RequestContext.HTTP.Path)

	method := request.RequestContext.HTTP.Method
	// Normalize path if needed, but for now we assume strict routing from API Gateway
	// Or check request.RawPath

	switch method {
	case "POST":
		sourceIP := request.RequestContext.HTTP.SourceIP
		return handleRegister(ctx, request.Body, sourceIP)
	case "GET":
		// Expecting /lookup/{code}
		// In HTTP API, path parameters are available in request.PathParameters
		code := request.PathParameters["code"]
		if code == "" {
			return errorResponse(400, "Missing code parameter"), nil
		}
		return handleLookup(ctx, code)
	default:
		return errorResponse(405, "Method Not Allowed"), nil
	}
}

func handleRegister(ctx context.Context, body string, sourceIP string) (events.APIGatewayV2HTTPResponse, error) {
	var item RegistryItem
	if err := json.Unmarshal([]byte(body), &item); err != nil {
		return errorResponse(400, "Invalid JSON body"), nil
	}

	if item.Code == "" {
		return errorResponse(400, "Code is required"), nil
	}

	// Auto-detect IP if not provided (useful for NAT)
	if item.IP == "" {
		item.IP = sourceIP
	}

	// Set TTL to 10 minutes from now (configurable)
	item.ExpiresAt = time.Now().Add(10 * time.Minute).Unix()

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		log.Printf("Failed to marshal item: %v", err)
		return errorResponse(500, "Internal Server Error"), nil
	}

	_, err = svc.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      av,
	})

	if err != nil {
		log.Printf("Failed to put item into DynamoDB: %v", err)
		return errorResponse(500, "Failed to save record"), nil
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Body:       `{"message": "Registered successfully"}`,
		Headers:    map[string]string{"Content-Type": "application/json"},
	}, nil
}

func handleLookup(ctx context.Context, code string) (events.APIGatewayV2HTTPResponse, error) {
	out, err := svc.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"code": &types.AttributeValueMemberS{Value: code},
		},
	})
	if err != nil {
		log.Printf("Failed to get item: %v", err)
		return errorResponse(500, "Failed to lookup code"), nil
	}

	if out.Item == nil {
		return errorResponse(404, "Code not found"), nil
	}

	var item RegistryItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		log.Printf("Failed to unmarshal item: %v", err)
		return errorResponse(500, "Internal Server Error"), nil
	}

	responseBody, _ := json.Marshal(item)
	return events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Body:       string(responseBody),
		Headers:    map[string]string{"Content-Type": "application/json"},
	}, nil
}

// Helper functions
func errorResponse(statusCode int, message string) events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{
		StatusCode: statusCode,
		Body:       fmt.Sprintf(`{"error": "%s"}`, message),
		Headers:    map[string]string{"Content-Type": "application/json"},
	}
}

func main() {
	lambda.Start(Handler)
}
