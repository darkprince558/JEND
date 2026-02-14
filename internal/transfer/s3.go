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
)

const (
	// Max 200MB
	MaxS3FileSize = 200 * 1024 * 1024
	// Bucket Name (Will be updated after deployment or passed via env)
	BucketName = "jendinfrastackv4-jendtransferbucket839f7c9a-knwjudei1o5l"
)

// UploadToS3 uploads a file to the transfer bucket and returns the object key.
func UploadToS3(ctx context.Context, filePath string, code string, identityPoolID string, region string) (string, error) {
	// 1. Validate File Size
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}
	if info.Size() > MaxS3FileSize {
		return "", fmt.Errorf("file size %d exceeds limit of %d bytes (200MB)", info.Size(), MaxS3FileSize)
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
	uploader := manager.NewUploader(client)

	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	key := fmt.Sprintf("transfers/%s/%s", code, filepath.Base(filePath))

	_, err = uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(BucketName),
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
	downloader := manager.NewDownloader(client)

	fileName := filepath.Base(key)
	outputPath := filepath.Join(outputDir, fileName)

	f, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	_, err = downloader.Download(ctx, f, &s3.GetObjectInput{
		Bucket: aws.String(BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		os.Remove(outputPath) // Cleanup
		return "", fmt.Errorf("s3 download failed: %w", err)
	}

	return outputPath, nil
}
