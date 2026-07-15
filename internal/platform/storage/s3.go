package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
	platformconfig "github.com/luqingjia/TravelAgent/internal/platform/config"
)

// s3API 是 S3Storage 真正用到的 AWS SDK 方法集合。
// 使用小接口后，单元测试可以传入 fake，不需要启动 RustFS 或访问公网。
type s3API interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// S3Storage 使用 AWS SDK v2 操作 Amazon S3 或兼容协议的 RustFS 服务。
type S3Storage struct {
	client s3API
	bucket string
	now    func() time.Time
}

// 编译期确认 S3 实现满足应用层对象存储端口。
var _ application.ObjectStorage = (*S3Storage)(nil)

// NewS3Storage 根据配置创建真实 AWS SDK 客户端。
// 自定义 endpoint 与 path-style 使同一实现可以连接本地 RustFS，同时仍保留标准 S3 请求模型。
func NewS3Storage(ctx context.Context, configuration platformconfig.Storage) (*S3Storage, error) {
	if strings.TrimSpace(configuration.Bucket) == "" {
		return nil, fmt.Errorf("RUSTFS_BUCKET_NAME is required")
	}
	if strings.TrimSpace(configuration.Region) == "" {
		return nil, fmt.Errorf("RUSTFS_REGION is required")
	}
	if strings.TrimSpace(configuration.AccessKey) == "" {
		return nil, fmt.Errorf("RUSTFS_ACCESS_KEY is required")
	}
	if strings.TrimSpace(configuration.SecretKey) == "" {
		return nil, fmt.Errorf("RUSTFS_SECRET_KEY is required")
	}

	// 显式静态凭据适合 RustFS；配置值只交给签名器，错误和日志都不能输出密钥。
	awsConfiguration, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(configuration.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(configuration.AccessKey, configuration.SecretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS SDK configuration: %w", err)
	}

	client := s3.NewFromConfig(awsConfiguration, func(options *s3.Options) {
		options.UsePathStyle = configuration.UsePathStyle
		if endpoint := strings.TrimRight(strings.TrimSpace(configuration.Endpoint), "/"); endpoint != "" {
			// BaseEndpoint 是 AWS SDK v2 当前推荐的服务级自定义 endpoint，适用于 S3 兼容服务。
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	return newS3Storage(client, configuration.Bucket, time.Now)
}

// newS3Storage 保存已经构造好的客户端；测试通过它注入 fake 和固定时钟。
func newS3Storage(client s3API, bucket string, now func() time.Time) (*S3Storage, error) {
	if client == nil {
		return nil, fmt.Errorf("S3 client is required")
	}
	if strings.TrimSpace(bucket) == "" {
		return nil, fmt.Errorf("S3 bucket is required")
	}
	if now == nil {
		return nil, fmt.Errorf("S3 storage clock is required")
	}
	return &S3Storage{client: client, bucket: strings.TrimSpace(bucket), now: now}, nil
}

// Put 上传完整对象，并返回标准 s3://bucket/key URI。
func (storage *S3Storage) Put(
	ctx context.Context,
	input application.StoredObjectInput,
) (application.StoredObject, error) {
	if len(input.Content) == 0 {
		return application.StoredObject{}, fmt.Errorf("S3 object content is empty")
	}
	key, err := storage.objectKey(input.FileName)
	if err != nil {
		return application.StoredObject{}, err
	}

	_, err = storage.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(storage.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(input.Content),
		ContentType: aws.String(input.ContentType),
	})
	if err != nil {
		return application.StoredObject{}, fmt.Errorf("put S3 object: %w", err)
	}

	return application.StoredObject{
		URI:         "s3://" + storage.bucket + "/" + key,
		FileName:    input.FileName,
		ContentType: input.ContentType,
		Size:        int64(len(input.Content)),
	}, nil
}

// Get 根据 s3:// URI 下载对象，并在返回前完整读取和关闭响应流。
func (storage *S3Storage) Get(ctx context.Context, uri string) ([]byte, error) {
	bucket, key, err := parseS3URI(uri)
	if err != nil {
		return nil, err
	}

	output, err := storage.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get S3 object: %w", err)
	}
	if output.Body == nil {
		return nil, fmt.Errorf("get S3 object returned an empty body")
	}
	defer output.Body.Close()

	content, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("read S3 object body: %w", err)
	}
	return content, nil
}

// Delete 根据 s3:// URI 删除对象；AWS S3 对不存在的对象通常也返回成功，适合作为上传补偿动作。
func (storage *S3Storage) Delete(ctx context.Context, uri string) error {
	bucket, key, err := parseS3URI(uri)
	if err != nil {
		return err
	}
	if _, err := storage.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("delete S3 object: %w", err)
	}
	return nil
}

// objectKey 使用 UTC 日期和纳秒时间戳生成不会因原始同名文件而互相覆盖的对象键。
func (storage *S3Storage) objectKey(fileName string) (string, error) {
	baseName := filepath.Base(strings.TrimSpace(fileName))
	if baseName == "" || baseName == "." || baseName == string(filepath.Separator) {
		return "", fmt.Errorf("S3 object file name is empty")
	}
	now := storage.now().UTC()
	return path.Join(
		now.Format("2006/01/02"),
		fmt.Sprintf("%d-%s", now.UnixNano(), baseName),
	), nil
}

// parseS3URI 严格拆分 s3://bucket/key，防止把 HTTP URL 或缺少 bucket 的值发给 SDK。
func parseS3URI(uri string) (string, string, error) {
	if !strings.HasPrefix(uri, "s3://") {
		return "", "", fmt.Errorf("invalid S3 URI %q", uri)
	}
	value := strings.TrimPrefix(uri, "s3://")
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid S3 URI %q", uri)
	}
	return parts[0], parts[1], nil
}
