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
	// fake 保存所有 AWS SDK 入参，并预设一段 GetObject 返回内容。
	fake := &fakeS3Client{getContent: []byte("stored content")}
	// 固定时间使对象键中的日期和时间部分可重复，不受测试运行时刻影响。
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

	// Delete 同样解析 URI，并把 bucket/key 分开交给 SDK。
	if err := storage.Delete(context.Background(), stored.URI); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if fake.deleteBucket != "knowledge" || fake.deleteKey != fake.putKey {
		t.Fatalf("DeleteObject fields = bucket:%q key:%q", fake.deleteBucket, fake.deleteKey)
	}
}

// TestParseS3URIRejectsMalformedValues 验证缺少协议、bucket 或 key 的字符串不会被悄悄接受。
func TestParseS3URIRejectsMalformedValues(t *testing.T) {
	// 每个值分别覆盖空串、缺少协议、缺少 key、缺少 bucket 和错误协议。
	for _, value := range []string{"", "bucket/key", "s3://bucket", "s3:///key", "http://bucket/key"} {
		if _, _, err := parseS3URI(value); err == nil {
			t.Fatalf("parseS3URI(%q) 应返回错误", value)
		}
	}

	// 合法 URI 应准确拆出 bucket 和完整多级 key。
	bucket, key, err := parseS3URI("s3://knowledge/path/to/file.txt")
	if err != nil || bucket != "knowledge" || key != "path/to/file.txt" {
		t.Fatalf("parseS3URI() = (%q, %q, %v)", bucket, key, err)
	}
}

// fakeS3Client 只记录适配器传给 AWS SDK 的字段，模拟对象服务而不发起网络请求。
type fakeS3Client struct {
	// put 开头字段记录 PutObject 收到的 bucket、key、内容类型和完整正文。
	putBucket      string
	putKey         string
	putContentType string
	putContent     []byte
	// get 开头字段记录 GetObject 入参和要返回给适配器的正文。
	getBucket  string
	getKey     string
	getContent []byte
	// delete 开头字段记录 DeleteObject 最终收到的对象定位信息。
	deleteBucket string
	deleteKey    string
}

func (fake *fakeS3Client) PutObject(
	_ context.Context,
	input *s3.PutObjectInput,
	_ ...func(*s3.Options),
) (*s3.PutObjectOutput, error) {
	// SDK 使用 *string 表示可选字段，统一转成普通字符串便于断言。
	fake.putBucket = stringValue(input.Bucket)
	fake.putKey = stringValue(input.Key)
	fake.putContentType = stringValue(input.ContentType)
	// 读完请求体保存副本，确认适配器没有丢失或改写上传内容。
	fake.putContent, _ = io.ReadAll(input.Body)
	// 返回空成功响应，测试不需要模拟 S3 的其他元数据。
	return &s3.PutObjectOutput{}, nil
}

// GetObject 记录对象位置，并用可关闭 Reader 返回预设正文。
func (fake *fakeS3Client) GetObject(
	_ context.Context,
	input *s3.GetObjectInput,
	_ ...func(*s3.Options),
) (*s3.GetObjectOutput, error) {
	// 保存 bucket/key，证明适配器已经正确解析 s3:// URI。
	fake.getBucket = stringValue(input.Bucket)
	fake.getKey = stringValue(input.Key)
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(fake.getContent))}, nil
}

// DeleteObject 记录对象位置并模拟幂等删除成功。
func (fake *fakeS3Client) DeleteObject(
	_ context.Context,
	input *s3.DeleteObjectInput,
	_ ...func(*s3.Options),
) (*s3.DeleteObjectOutput, error) {
	// 删除输入必须与之前 Put 生成的 bucket/key 完全一致。
	fake.deleteBucket = stringValue(input.Bucket)
	fake.deleteKey = stringValue(input.Key)
	return &s3.DeleteObjectOutput{}, nil
}

// stringValue 安全读取 AWS SDK 使用的 *string，nil 时返回空字符串便于断言。
func stringValue(value *string) string {
	// nil 表示 SDK 字段没有赋值，转成空串后断言信息更直观。
	if value == nil {
		return ""
	}
	return *value
}
