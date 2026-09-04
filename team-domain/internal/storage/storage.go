package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-domain/internal/config"
)

// StorageClient provides presigned upload URLs and public access URLs for product media.
type StorageClient interface {
	GetPresignedUploadURL(ctx context.Context, contentType, filename string) (uploadURL, imageKey, publicURL string, err error)
}

// S3Storage generates standard S3 SigV4 presigned PUT URLs compatible with MinIO and AWS S3.
type S3Storage struct {
	cfg config.Storage
}

// NewS3Storage creates a new S3Storage instance from configuration.
func NewS3Storage(cfg config.Storage) *S3Storage {
	return &S3Storage{cfg: cfg}
}

// GetPresignedUploadURL generates a presigned S3 PUT URL valid for 15 minutes.
func (s *S3Storage) GetPresignedUploadURL(
	_ context.Context,
	contentType, filename string,
) (uploadURL, imageKey, publicURL string, err error) {
	ext := filepath.Ext(filename)
	if ext == "" {
		switch contentType {
		case "image/jpeg", "image/jpg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		case "image/webp":
			ext = ".webp"
		case "image/gif":
			ext = ".gif"
		default:
			ext = ".jpg"
		}
	}
	imageKey = fmt.Sprintf("%s%s", uuid.New().String(), ext)

	scheme := "http"
	if s.cfg.UseSSL {
		scheme = "https"
	}
	endpoint := strings.TrimRight(s.cfg.Endpoint, "/")
	bucket := s.cfg.Bucket
	region := s.cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")
	expires := 900 // 15 minutes

	host := endpoint
	if strings.Contains(host, "://") {
		u, err := url.Parse(host)
		if err == nil {
			host = u.Host
		}
	}

	canonicalURI := fmt.Sprintf("/%s/%s", bucket, imageKey)
	credential := fmt.Sprintf("%s/%s/%s/s3/aws4_request", s.cfg.AccessKey, dateStamp, region)

	query := url.Values{}
	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", credential)
	query.Set("X-Amz-Date", amzDate)
	query.Set("X-Amz-Expires", fmt.Sprintf("%d", expires))
	query.Set("X-Amz-SignedHeaders", "host")

	canonicalQuery := query.Encode()

	canonicalHeaders := fmt.Sprintf("host:%s\n", host)
	signedHeaders := "host"
	payloadHash := "UNSIGNED-PAYLOAD"

	canonicalRequest := fmt.Sprintf(
		"PUT\n%s\n%s\n%s\n%s\n%s",
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	)

	canonicalRequestHash := sha256Hex([]byte(canonicalRequest))

	stringToSign := fmt.Sprintf(
		"AWS4-HMAC-SHA256\n%s\n%s/%s/s3/aws4_request\n%s",
		amzDate,
		dateStamp,
		region,
		canonicalRequestHash,
	)

	signingKey := getSignatureKey(s.cfg.SecretKey, dateStamp, region, "s3")
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	query.Set("X-Amz-Signature", signature)

	uploadURL = fmt.Sprintf("%s://%s%s?%s", scheme, host, canonicalURI, query.Encode())

	publicBase := strings.TrimRight(s.cfg.PublicBaseURL, "/")
	if publicBase != "" {
		publicURL = fmt.Sprintf("%s/%s", publicBase, imageKey)
	} else {
		publicURL = fmt.Sprintf("%s://%s/%s/%s", scheme, host, bucket, imageKey)
	}

	return uploadURL, imageKey, publicURL, nil
}

func hmacSHA256(key []byte, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func getSignatureKey(key, dateStamp, regionName, serviceName string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+key), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(regionName))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}
