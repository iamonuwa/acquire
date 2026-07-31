// Package payloadstore persists content-addressed payloads in Cloudflare R2.
package payloadstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// Store persists and retrieves objects.
type Store interface {
	Put(ctx context.Context, key string, body []byte) error
	Get(ctx context.Context, key string) (body []byte, found bool, err error)
}

// StatusKey is the object key for the published status file.
const StatusKey = "status/current.json"

// RawKey is the object key for a raw payload.
func RawKey(sourceID, hash string) string { return "raw/" + sourceID + "/" + hash }

// NormKey is the object key for normalized text.
func NormKey(sourceID, hash string) string { return "norm/" + sourceID + "/" + hash }

var requiredEnv = []string{"R2_ACCOUNT_ID", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY", "R2_BUCKET"}

// RequireSecrets reports missing R2_* environment variables. Call before any
// network use so a misconfiguration fails before the first request.
func RequireSecrets() error {
	var missing []string
	for _, k := range requiredEnv {
		if os.Getenv(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("payloadstore: missing required env: %s", strings.Join(missing, ", "))
	}
	return nil
}

// R2 is an R2-backed Store.
type R2 struct {
	client *s3.Client
	bucket string
}

// NewR2 builds an R2 store from the R2_* environment. Call RequireSecrets first.
func NewR2(ctx context.Context) (*R2, error) {
	accountID := os.Getenv("R2_ACCOUNT_ID")
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("auto"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			os.Getenv("R2_ACCESS_KEY_ID"), os.Getenv("R2_SECRET_ACCESS_KEY"), "")),
	)
	if err != nil {
		return nil, fmt.Errorf("payloadstore: load aws config: %w", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID))
	})
	return &R2{client: client, bucket: os.Getenv("R2_BUCKET")}, nil
}

// Put writes body at key. Content-addressed keys make this idempotent.
func (r *R2) Put(ctx context.Context, key string, body []byte) error {
	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(body),
	})
	if err != nil {
		return fmt.Errorf("payloadstore: put %s: %w", key, err)
	}
	return nil
}

// Get reads the object at key. A missing key returns found=false, not an error.
func (r *R2) Get(ctx context.Context, key string) ([]byte, bool, error) {
	out, err := r.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && (ae.ErrorCode() == "NoSuchKey" || ae.ErrorCode() == "NotFound") {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("payloadstore: get %s: %w", key, err)
	}
	defer out.Body.Close()
	b, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, false, fmt.Errorf("payloadstore: read %s: %w", key, err)
	}
	return b, true, nil
}
