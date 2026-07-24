package storage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
)

// TestLocalStorageRoundTrip 验证本地开发模式能完整执行写入、读取和幂等删除。
func TestLocalStorageRoundTrip(t *testing.T) {
	// 准备：每个测试使用独立临时目录，运行结束由 testing 自动清理，不污染仓库的 .data。
	storage, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	content := []byte("TravelAgent local storage")

	// 执行写入并断言返回的是应用层稳定对象，而不是泄漏 os.File 等底层句柄。
	stored, err := storage.Put(context.Background(), application.StoredObjectInput{
		FileName:    "guide.md",
		ContentType: "text/markdown",
		Content:     content,
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if !strings.HasPrefix(stored.URI, "local://") || stored.Size != int64(len(content)) {
		t.Fatalf("Put() result = %#v", stored)
	}

	// 关键断言：通过 URI 读回的字节必须与上传内容完全一致。
	loaded, err := storage.Get(context.Background(), stored.URI)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !bytes.Equal(loaded, content) {
		t.Fatalf("Get() = %q, want %q", loaded, content)
	}

	// Delete 是补偿流程的一部分，重复执行不应因文件已经不存在而制造第二个错误。
	if err := storage.Delete(context.Background(), stored.URI); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := storage.Delete(context.Background(), stored.URI); err != nil {
		t.Fatalf("second Delete() error = %v", err)
	}
}

// TestLocalStorageRejectsPathOutsideBaseDirectory 验证伪造 local URI 不能读取存储根目录之外的文件。
func TestLocalStorageRejectsPathOutsideBaseDirectory(t *testing.T) {
	// baseDirectory 是适配器允许访问的唯一根目录。
	baseDirectory := t.TempDir()
	storage, err := NewLocalStorage(baseDirectory)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}

	// 在另一个临时目录放置“敏感文件”，模拟攻击者伪造越界 URI。
	outsideDirectory := t.TempDir()
	outsideFile := filepath.Join(outsideDirectory, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("should not be readable"), 0o600); err != nil {
		t.Fatalf("prepare outside file: %v", err)
	}

	// 把绝对路径包装成 local://，如果只检查协议而不检查根目录就可能被读取。
	uri := "local://" + filepath.ToSlash(outsideFile)
	// Get 必须拒绝越界路径，文件内容不能返回给调用方。
	if _, err := storage.Get(context.Background(), uri); err == nil {
		t.Fatal("Get() 应拒绝存储根目录之外的 local URI")
	}
}
