package transfer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/darkprince558/jend/internal/auth"
	jendcfg "github.com/darkprince558/jend/internal/config"
)

// UploadToS3 uploads a file to the transfer bucket and returns the object key.
func UploadToS3(ctx context.Context, filePath string, code string, identityPoolID string, region string) (string, error) {
	// 1. Validate File Size
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}
	if info.Size() > jendcfg.MaxS3FileSize {
		return "", fmt.Errorf("file size %d exceeds limit of %d bytes (200MB)", info.Size(), jendcfg.MaxS3FileSize)
	}

	// 2. Load Config with Cognito Credentials
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return "", fmt.Errorf("failed to load aws config: %w", err)
	}
	credsProvider := auth.NewCognitoProvider(cfg, identityPoolID)
	cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region), config.WithCredentialsProvider(credsProvider))
	if err != nil {
		return "", fmt.Errorf("failed to load aws config with cognito: %w", err)
	}

	// 3. Upload
	client := s3.NewFromConfig(cfg)
	//nolint:staticcheck // feature/s3/manager is deprecated but we're keeping it for now
	uploader := manager.NewUploader(client)

	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	key := fmt.Sprintf("transfers/%s/%s", code, filepath.Base(filePath))

	//nolint:staticcheck // feature/s3/manager is deprecated but we're keeping it for now
	_, err = uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(jendcfg.DefaultS3Bucket),
		Key:    aws.String(key),
		Body:   f,
	})
	if err != nil {
		return "", fmt.Errorf("s3 upload failed: %w", err)
	}

	return key, nil
}

// DownloadFromS3 downloads a file from S3 to the output directory.
func DownloadFromS3(ctx context.Context, key string, outputDir string, identityPoolID string, region string) (string, error) {
	// 1. Load Config
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return "", fmt.Errorf("failed to load aws config: %w", err)
	}
	credsProvider := auth.NewCognitoProvider(cfg, identityPoolID)
	cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region), config.WithCredentialsProvider(credsProvider))
	if err != nil {
		return "", fmt.Errorf("failed to load aws config with cognito: %w", err)
	}

	// 2. Download
	client := s3.NewFromConfig(cfg)
	//nolint:staticcheck // feature/s3/manager is deprecated but we're keeping it for now
	downloader := manager.NewDownloader(client)

	fileName := filepath.Base(key)
	outputPath := filepath.Join(outputDir, fileName)

	f, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = f.Close() }()

	//nolint:staticcheck // feature/s3/manager is deprecated but we're keeping it for now
	_, err = downloader.Download(ctx, f, &s3.GetObjectInput{
		Bucket: aws.String(jendcfg.DefaultS3Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		_ = os.Remove(outputPath) // Cleanup
		return "", fmt.Errorf("s3 download failed: %w", err)
	}

	return outputPath, nil
}
