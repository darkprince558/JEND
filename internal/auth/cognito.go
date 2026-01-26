package auth

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentity"
)

// CognitoProvider implements aws.CredentialsProvider for Unauthenticated Identities
type CognitoProvider struct {
	Client         *cognitoidentity.Client
	IdentityPoolID string
	identityID     string // Cached Identity ID
}

// NewCognitoProvider creates a provider that exchanges Pool ID for temp creds
func NewCognitoProvider(cfg aws.Config, poolID string) *CognitoProvider {
	return &CognitoProvider{
		Client:         cognitoidentity.NewFromConfig(cfg),
		IdentityPoolID: poolID,
	}
}

// Retrieve returns the set of credentials
func (p *CognitoProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	// 1. Get Identity ID if not cached (or if creds expired, but ID usually persists? ID persists, Creds expire)
	// For simplicity, we get ID every time or cache it. Caching is better for rate limits.
	if p.identityID == "" {
		idOutput, err := p.Client.GetId(ctx, &cognitoidentity.GetIdInput{
			IdentityPoolId: aws.String(p.IdentityPoolID),
		})
		if err != nil {
			return aws.Credentials{}, fmt.Errorf("failed to get cognito identity id: %w", err)
		}
		p.identityID = *idOutput.IdentityId
	}

	// 2. Get Credentials
	credsOutput, err := p.Client.GetCredentialsForIdentity(ctx, &cognitoidentity.GetCredentialsForIdentityInput{
		IdentityId: aws.String(p.identityID),
	})
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("failed to get credentials for identity: %w", err)
	}

	if credsOutput.Credentials == nil {
		return aws.Credentials{}, fmt.Errorf("empty credentials from cognito")
	}

	return aws.Credentials{
		AccessKeyID:     *credsOutput.Credentials.AccessKeyId,
		SecretAccessKey: *credsOutput.Credentials.SecretKey,
		SessionToken:    *credsOutput.Credentials.SessionToken,
		Source:          "CognitoIdentity",
		CanExpire:       true,
		Expires:         *credsOutput.Credentials.Expiration,
	}, nil
}
