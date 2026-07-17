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
	// PutObject 上传对象内容。
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	// GetObject 下载对象并返回响应流。
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	// DeleteObject 删除对象，作为上传失败补偿能力。
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// S3Storage 使用 AWS SDK v2 操作 Amazon S3 或兼容协议的 RustFS 服务。
type S3Storage struct {
	// client 是真实 AWS SDK 客户端或测试 fake。
	client s3API
	// bucket 是经过校验和去空格的目标桶名。
	bucket string
	// now 用于生成按日期分组且不重复的对象键。
	now func() time.Time
}

// 编译期确认 S3 实现满足应用层对象存储端口。
var _ application.ObjectStorage = (*S3Storage)(nil)

// NewS3Storage 根据配置创建真实 AWS SDK 客户端。
// 自定义 endpoint 与 path-style 使同一实现可以连接本地 RustFS，同时仍保留标准 S3 请求模型。
func NewS3Storage(ctx context.Context, configuration platformconfig.Storage) (*S3Storage, error) {
	// 桶名为空时无法定位对象写入范围。
	if strings.TrimSpace(configuration.Bucket) == "" {
		return nil, fmt.Errorf("RUSTFS_BUCKET_NAME is required")
	}
	// Region 会参与 AWS SDK 请求签名。
	if strings.TrimSpace(configuration.Region) == "" {
		return nil, fmt.Errorf("RUSTFS_REGION is required")
	}
	// AccessKey 是静态凭据的一部分。
	if strings.TrimSpace(configuration.AccessKey) == "" {
		return nil, fmt.Errorf("RUSTFS_ACCESS_KEY is required")
	}
	// SecretKey 是静态凭据的另一部分，错误信息只提变量名。
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
	// SDK 配置加载失败时不能构造部分可用客户端。
	if err != nil {
		return nil, fmt.Errorf("load AWS SDK configuration: %w", err)
	}

	// 基于公共 AWS 配置创建 S3 服务客户端，并在闭包中应用 RustFS 兼容选项。
	client := s3.NewFromConfig(awsConfiguration, func(options *s3.Options) {
		// path-style 决定地址形如 endpoint/bucket/key，而不是 bucket.endpoint/key。
		options.UsePathStyle = configuration.UsePathStyle
		// endpoint 为空时保留 AWS SDK 默认服务发现行为。
		if endpoint := strings.TrimRight(strings.TrimSpace(configuration.Endpoint), "/"); endpoint != "" {
			// BaseEndpoint 是 AWS SDK v2 当前推荐的服务级自定义 endpoint，适用于 S3 兼容服务。
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	// 真实客户端构造后复用可测试入口完成最终依赖校验。
	return newS3Storage(client, configuration.Bucket, time.Now)
}

// newS3Storage 保存已经构造好的客户端；测试通过它注入 fake 和固定时钟。
func newS3Storage(client s3API, bucket string, now func() time.Time) (*S3Storage, error) {
	// 接口为 nil 时任何对象操作都会 panic。
	if client == nil {
		return nil, fmt.Errorf("S3 client is required")
	}
	// 测试入口也必须提供有效桶名，不能依赖生产配置已经校验。
	if strings.TrimSpace(bucket) == "" {
		return nil, fmt.Errorf("S3 bucket is required")
	}
	// 时钟缺失时无法生成唯一对象键。
	if now == nil {
		return nil, fmt.Errorf("S3 storage clock is required")
	}
	// 保存去空格桶名和稳定依赖，后续请求不再读取环境变量。
	return &S3Storage{client: client, bucket: strings.TrimSpace(bucket), now: now}, nil
}

// Put 上传完整对象，并返回标准 s3://bucket/key URI。
func (storage *S3Storage) Put(
	ctx context.Context,
	input application.StoredObjectInput,
) (application.StoredObject, error) {
	// 空内容不发送到对象服务，避免创建无意义对象。
	if len(input.Content) == 0 {
		return application.StoredObject{}, fmt.Errorf("S3 object content is empty")
	}
	// objectKey 会清理文件名并生成日期/时间戳前缀。
	key, err := storage.objectKey(input.FileName)
	if err != nil {
		return application.StoredObject{}, err
	}

	// Body 使用内存 Reader，AWS SDK 会在调用 Context 生命周期内读取完整内容。
	_, err = storage.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(storage.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(input.Content),
		ContentType: aws.String(input.ContentType),
	})
	// SDK 错误保留原始链，但不打印凭据或完整内容。
	if err != nil {
		return application.StoredObject{}, fmt.Errorf("put S3 object: %w", err)
	}

	// 返回标准 URI 和实际字节数，应用层无需认识 AWS SDK 类型。
	return application.StoredObject{
		URI:         "s3://" + storage.bucket + "/" + key,
		FileName:    input.FileName,
		ContentType: input.ContentType,
		Size:        int64(len(input.Content)),
	}, nil
}

// Get 根据 s3:// URI 下载对象，并在返回前完整读取和关闭响应流。
func (storage *S3Storage) Get(ctx context.Context, uri string) ([]byte, error) {
	// 先严格解析 URI，避免把非法 bucket/key 发送到 SDK。
	bucket, key, err := parseS3URI(uri)
	if err != nil {
		return nil, err
	}

	// 使用 URI 自带 bucket 和 key 读取对象，兼容已经持久化的稳定地址。
	output, err := storage.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	// SDK 请求失败时没有可读取响应体。
	if err != nil {
		return nil, fmt.Errorf("get S3 object: %w", err)
	}
	// 成功响应也必须包含 Body，否则无法形成完整内容。
	if output.Body == nil {
		return nil, fmt.Errorf("get S3 object returned an empty body")
	}
	// 函数返回前关闭网络响应流，允许底层连接复用。
	defer output.Body.Close()

	// 应用层当前合同需要完整 []byte，因此在适配器边界一次性读完。
	content, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("read S3 object body: %w", err)
	}
	// 返回普通字节切片，外层不再持有 SDK 响应对象。
	return content, nil
}

// Delete 根据 s3:// URI 删除对象；AWS S3 对不存在的对象通常也返回成功，适合作为上传补偿动作。
func (storage *S3Storage) Delete(ctx context.Context, uri string) error {
	// 删除前使用同一严格 URI 解析规则。
	bucket, key, err := parseS3URI(uri)
	if err != nil {
		return err
	}
	// DeleteObject 的输出当前用例不需要，只检查错误。
	if _, err := storage.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("delete S3 object: %w", err)
	}
	// nil 表示对象已经删除或服务端按幂等语义认为不存在。
	return nil
}

// objectKey 使用 UTC 日期和纳秒时间戳生成不会因原始同名文件而互相覆盖的对象键。
func (storage *S3Storage) objectKey(fileName string) (string, error) {
	// Base 去掉用户文件名中的目录部分，阻止通过路径控制对象前缀。
	baseName := filepath.Base(strings.TrimSpace(fileName))
	if baseName == "" || baseName == "." || baseName == string(filepath.Separator) {
		return "", fmt.Errorf("S3 object file name is empty")
	}
	// 所有对象键使用 UTC，避免部署节点时区不同导致目录漂移。
	now := storage.now().UTC()
	// 日期目录便于按天查看对象，纳秒前缀避免同名上传覆盖。
	return path.Join(
		now.Format("2006/01/02"),
		fmt.Sprintf("%d-%s", now.UnixNano(), baseName),
	), nil
}

// parseS3URI 严格拆分 s3://bucket/key，防止把 HTTP URL 或缺少 bucket 的值发给 SDK。
func parseS3URI(uri string) (string, string, error) {
	// 只接受 S3Storage 生成的 s3:// 协议。
	if !strings.HasPrefix(uri, "s3://") {
		return "", "", fmt.Errorf("invalid S3 URI %q", uri)
	}
	// 去掉协议后剩余内容应为 bucket/key。
	value := strings.TrimPrefix(uri, "s3://")
	// SplitN 只切第一个斜杠，key 内部可以继续包含目录。
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid S3 URI %q", uri)
	}
	// 返回独立 bucket 和 key，供三个 SDK 操作复用。
	return parts[0], parts[1], nil
}
