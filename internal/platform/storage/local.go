// Package storage 实现知识文档对象存储端口的本地文件和 S3/RustFS 两种适配器。
//
// application 只依赖 ObjectStorage 接口，不知道文件最终写到磁盘还是对象服务；模式选择由 app 组合根完成。
package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
)

// LocalStorage 把对象内容保存在一个受限制的本地根目录中，主要用于没有 RustFS 的开发环境。
type LocalStorage struct {
	baseDirectory string
	now           func() time.Time
}

// 编译期确认本地实现满足应用层对象存储端口。
var _ application.ObjectStorage = (*LocalStorage)(nil)

// NewLocalStorage 校验并固定本地存储根目录。
func NewLocalStorage(baseDirectory string) (*LocalStorage, error) {
	return newLocalStorage(baseDirectory, time.Now)
}

// newLocalStorage 允许测试注入固定时钟，生产调用 NewLocalStorage 即可。
func newLocalStorage(baseDirectory string, now func() time.Time) (*LocalStorage, error) {
	if strings.TrimSpace(baseDirectory) == "" {
		return nil, fmt.Errorf("LOCAL_STORAGE_DIR is required")
	}
	if now == nil {
		return nil, fmt.Errorf("local storage clock is required")
	}

	absoluteDirectory, err := filepath.Abs(baseDirectory)
	if err != nil {
		return nil, fmt.Errorf("resolve local storage directory: %w", err)
	}
	return &LocalStorage{
		baseDirectory: filepath.Clean(absoluteDirectory),
		now:           now,
	}, nil
}

// Put 在受控根目录中保存完整对象内容，并返回可供后续 Get/Delete 使用的 local:// URI。
func (storage *LocalStorage) Put(
	ctx context.Context,
	input application.StoredObjectInput,
) (application.StoredObject, error) {
	if err := ctx.Err(); err != nil {
		return application.StoredObject{}, fmt.Errorf("store local object: %w", err)
	}
	if len(input.Content) == 0 {
		return application.StoredObject{}, fmt.Errorf("local object content is empty")
	}

	fileName := filepath.Base(strings.TrimSpace(input.FileName))
	if fileName == "" || fileName == "." || fileName == string(filepath.Separator) {
		return application.StoredObject{}, fmt.Errorf("local object file name is empty")
	}
	if err := os.MkdirAll(storage.baseDirectory, 0o755); err != nil {
		return application.StoredObject{}, fmt.Errorf("create local storage directory: %w", err)
	}

	// 时间戳前缀既避免同名上传互相覆盖，也保留原始文件名，方便本地排查。
	storedName := fmt.Sprintf("%d-%s", storage.now().UnixNano(), fileName)
	storedPath := filepath.Join(storage.baseDirectory, storedName)
	if err := os.WriteFile(storedPath, input.Content, 0o644); err != nil {
		return application.StoredObject{}, fmt.Errorf("write local object: %w", err)
	}

	return application.StoredObject{
		// 使用绝对路径生成 URI，服务工作目录改变后仍能找到同一个本地文件。
		URI:         "local://" + filepath.ToSlash(storedPath),
		FileName:    input.FileName,
		ContentType: input.ContentType,
		Size:        int64(len(input.Content)),
	}, nil
}

// Get 读取 local:// URI 指向的对象，并拒绝任何逃离配置根目录的路径。
func (storage *LocalStorage) Get(ctx context.Context, uri string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("read local object: %w", err)
	}
	path, err := storage.resolveURI(uri)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read local object: %w", err)
	}
	return content, nil
}

// Delete 删除 local:// URI 指向的对象；文件已经不存在时仍视为补偿成功。
func (storage *LocalStorage) Delete(ctx context.Context, uri string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("delete local object: %w", err)
	}
	path, err := storage.resolveURI(uri)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete local object: %w", err)
	}
	return nil
}

// resolveURI 把 URI 转成规范绝对路径，并执行目录边界检查。
func (storage *LocalStorage) resolveURI(uri string) (string, error) {
	if !strings.HasPrefix(uri, "local://") {
		return "", fmt.Errorf("invalid local object URI %q", uri)
	}
	rawPath := strings.TrimSpace(strings.TrimPrefix(uri, "local://"))
	if rawPath == "" {
		return "", fmt.Errorf("invalid local object URI %q", uri)
	}

	candidate := filepath.Clean(filepath.FromSlash(rawPath))
	if !filepath.IsAbs(candidate) {
		// 兼容旧 MVP 保存的 `.data/storage/name` 相对 URI：如果它按当前工作目录解析后
		// 已经位于 baseDirectory 内就直接使用；单纯 `name` 则按根目录下对象处理。
		absoluteCandidate, err := filepath.Abs(candidate)
		if err != nil {
			return "", fmt.Errorf("resolve local object URI: %w", err)
		}
		if isPathWithin(storage.baseDirectory, absoluteCandidate) {
			candidate = absoluteCandidate
		} else {
			candidate = filepath.Join(storage.baseDirectory, candidate)
		}
	}

	absoluteCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve local object path: %w", err)
	}
	absoluteCandidate = filepath.Clean(absoluteCandidate)
	if !isPathWithin(storage.baseDirectory, absoluteCandidate) {
		return "", fmt.Errorf("local object URI escapes configured storage directory")
	}
	return absoluteCandidate, nil
}

// isPathWithin 使用 filepath.Rel 判断目标是否等于根目录或位于根目录之下。
// 不能只用字符串前缀，否则 `C:\data2` 会被误认为在 `C:\data` 内。
func isPathWithin(baseDirectory string, target string) bool {
	relative, err := filepath.Rel(baseDirectory, target)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
