package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/knowledge"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Storage struct {
	client *s3.Client
	bucket string
}

func NewS3(ctx context.Context, cfg config.StorageConfig) (*S3Storage, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("RUSTFS_BUCKET_NAME is empty")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})
	return &S3Storage{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3Storage) Put(ctx context.Context, input knowledge.StoredObjectInput) (knowledge.StoredObject, error) {
	key := s.key(input.FileName)
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(input.Content),
		ContentType: aws.String(input.ContentType),
	})
	if err != nil {
		return knowledge.StoredObject{}, err
	}
	return knowledge.StoredObject{
		URI:         "s3://" + s.bucket + "/" + key,
		FileName:    input.FileName,
		ContentType: input.ContentType,
		Size:        int64(len(input.Content)),
	}, nil
}

func (s *S3Storage) Get(ctx context.Context, uri string) ([]byte, error) {
	bucket, key, err := parseS3URI(uri)
	if err != nil {
		return nil, err
	}
	if bucket == "" {
		bucket = s.bucket
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (s *S3Storage) Delete(ctx context.Context, uri string) error {
	bucket, key, err := parseS3URI(uri)
	if err != nil {
		return err
	}
	if bucket == "" {
		bucket = s.bucket
	}
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *S3Storage) key(fileName string) string {
	datePath := time.Now().Format("2006/01/02")
	return path.Join(datePath, fmt.Sprintf("%d-%s", time.Now().UnixNano(), path.Base(fileName)))
}

func parseS3URI(uri string) (string, string, error) {
	value := strings.TrimPrefix(uri, "s3://")
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", "", fmt.Errorf("invalid s3 uri %q", uri)
	}
	return parts[0], parts[1], nil
}
