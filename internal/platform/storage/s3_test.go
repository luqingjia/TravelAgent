package storage

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
)

// TestS3StorageDelegatesObjectOperations 验证 S3 适配器正确构造 bucket/key，并保持 s3:// URI 语义。
func TestS3StorageDelegatesObjectOperations(t *testing.T) {
	fake := &fakeS3Client{getContent: []byte("stored content")}
	fixedTime := time.Date(2026, time.July, 14, 12, 0, 0, 123, time.UTC)
	storage, err := newS3Storage(fake, "knowledge", func() time.Time { return fixedTime })
	if err != nil {
		t.Fatalf("newS3Storage() error = %v", err)
	}

	// Put 应去掉上传文件名中的目录部分，并生成带日期的稳定对象键。
	stored, err := storage.Put(context.Background(), application.StoredObjectInput{
		FileName:    "folder/guide.md",
		ContentType: "text/markdown",
		Content:     []byte("upload"),
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if fake.putBucket != "knowledge" || !strings.HasSuffix(fake.putKey, "-guide.md") || fake.putContentType != "text/markdown" {
		t.Fatalf("PutObject fields = bucket:%q key:%q type:%q", fake.putBucket, fake.putKey, fake.putContentType)
	}
	if !bytes.Equal(fake.putContent, []byte("upload")) || stored.URI != "s3://knowledge/"+fake.putKey {
		t.Fatalf("Put() result = %#v, body = %q", stored, fake.putContent)
	}

	// Get 使用 URI 中的 bucket/key，而不是把整个 URI 当成对象键。
	content, err := storage.Get(context.Background(), stored.URI)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !bytes.Equal(content, fake.getContent) || fake.getBucket != "knowledge" || fake.getKey != fake.putKey {
		t.Fatalf("GetObject fields = bucket:%q key:%q content:%q", fake.getBucket, fake.getKey, content)
	}

	if err := storage.Delete(context.Background(), stored.URI); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if fake.deleteBucket != "knowledge" || fake.deleteKey != fake.putKey {
		t.Fatalf("DeleteObject fields = bucket:%q key:%q", fake.deleteBucket, fake.deleteKey)
	}
}

// TestParseS3URIRejectsMalformedValues 验证缺少协议、bucket 或 key 的字符串不会被悄悄接受。
func TestParseS3URIRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{"", "bucket/key", "s3://bucket", "s3:///key", "http://bucket/key"} {
		if _, _, err := parseS3URI(value); err == nil {
			t.Fatalf("parseS3URI(%q) 应返回错误", value)
		}
	}

	bucket, key, err := parseS3URI("s3://knowledge/path/to/file.txt")
	if err != nil || bucket != "knowledge" || key != "path/to/file.txt" {
		t.Fatalf("parseS3URI() = (%q, %q, %v)", bucket, key, err)
	}
}

// fakeS3Client 只记录适配器传给 AWS SDK 的字段，模拟对象服务而不发起网络请求。
type fakeS3Client struct {
	putBucket      string
	putKey         string
	putContentType string
	putContent     []byte
	getBucket      string
	getKey         string
	getContent     []byte
	deleteBucket   string
	deleteKey      string
}

func (fake *fakeS3Client) PutObject(
	_ context.Context,
	input *s3.PutObjectInput,
	_ ...func(*s3.Options),
) (*s3.PutObjectOutput, error) {
	fake.putBucket = stringValue(input.Bucket)
	fake.putKey = stringValue(input.Key)
	fake.putContentType = stringValue(input.ContentType)
	fake.putContent, _ = io.ReadAll(input.Body)
	return &s3.PutObjectOutput{}, nil
}

func (fake *fakeS3Client) GetObject(
	_ context.Context,
	input *s3.GetObjectInput,
	_ ...func(*s3.Options),
) (*s3.GetObjectOutput, error) {
	fake.getBucket = stringValue(input.Bucket)
	fake.getKey = stringValue(input.Key)
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(fake.getContent))}, nil
}

func (fake *fakeS3Client) DeleteObject(
	_ context.Context,
	input *s3.DeleteObjectInput,
	_ ...func(*s3.Options),
) (*s3.DeleteObjectOutput, error) {
	fake.deleteBucket = stringValue(input.Bucket)
	fake.deleteKey = stringValue(input.Key)
	return &s3.DeleteObjectOutput{}, nil
}

// stringValue 安全读取 AWS SDK 使用的 *string，nil 时返回空字符串便于断言。
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
