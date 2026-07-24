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
	// baseDirectory 是经过 Abs 和 Clean 的受控绝对根目录。
	baseDirectory string
	// now 用于生成不重复文件名前缀，测试可注入固定时间。
	now func() time.Time
}

// 编译期确认本地实现满足应用层对象存储端口。
var _ application.ObjectStorage = (*LocalStorage)(nil)

// NewLocalStorage 校验并固定本地存储根目录。
func NewLocalStorage(baseDirectory string) (*LocalStorage, error) {
	// 生产入口使用真实系统时钟，具体校验委托给可测试构造函数。
	return newLocalStorage(baseDirectory, time.Now)
}

// newLocalStorage 允许测试注入固定时钟，生产调用 NewLocalStorage 即可。
func newLocalStorage(baseDirectory string, now func() time.Time) (*LocalStorage, error) {
	// 根目录为空时无法建立安全路径边界。
	if strings.TrimSpace(baseDirectory) == "" {
		return nil, fmt.Errorf("LOCAL_STORAGE_DIR is required")
	}
	// 时钟缺失时无法生成稳定对象名，构造阶段直接拒绝。
	if now == nil {
		return nil, fmt.Errorf("local storage clock is required")
	}

	// 转成绝对路径后，服务工作目录变化也不会改变对象定位。
	absoluteDirectory, err := filepath.Abs(baseDirectory)
	if err != nil {
		return nil, fmt.Errorf("resolve local storage directory: %w", err)
	}
	// Clean 消除多余分隔符和 . 段，后续边界比较都使用规范路径。
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
	// 在触碰文件系统前检查调用方 Context 是否已经取消。
	if err := ctx.Err(); err != nil {
		return application.StoredObject{}, fmt.Errorf("store local object: %w", err)
	}
	// 空内容没有存储价值，也不能形成有效知识文档。
	if len(input.Content) == 0 {
		return application.StoredObject{}, fmt.Errorf("local object content is empty")
	}

	// Base 去掉调用方文件名中的目录部分，阻止通过 ../ 指定任意写入位置。
	fileName := filepath.Base(strings.TrimSpace(input.FileName))
	// 空名、当前目录或根分隔符都不是有效对象文件名。
	if fileName == "" || fileName == "." || fileName == string(filepath.Separator) {
		return application.StoredObject{}, fmt.Errorf("local object file name is empty")
	}
	// 目录不存在时递归创建，权限允许服务读写、其他用户只读执行。
	if err := os.MkdirAll(storage.baseDirectory, 0o755); err != nil {
		return application.StoredObject{}, fmt.Errorf("create local storage directory: %w", err)
	}

	// 时间戳前缀既避免同名上传互相覆盖，也保留原始文件名，方便本地排查。
	storedName := fmt.Sprintf("%d-%s", storage.now().UnixNano(), fileName)
	// Join 始终把安全文件名放到受控根目录下。
	storedPath := filepath.Join(storage.baseDirectory, storedName)
	// 0644 允许服务所有者读写，其他用户只读，不授予执行权限。
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
	// 已取消请求不再执行磁盘读取。
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("read local object: %w", err)
	}
	// URI 必须先经过协议和根目录边界校验。
	path, err := storage.resolveURI(uri)
	if err != nil {
		return nil, err
	}
	// 一次性读取完整文件，应用层后续会进行文本解析和分块。
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read local object: %w", err)
	}
	// 返回独立字节切片，不暴露文件描述符给应用层。
	return content, nil
}

// Delete 删除 local:// URI 指向的对象；文件已经不存在时仍视为补偿成功。
func (storage *LocalStorage) Delete(ctx context.Context, uri string) error {
	// 调用方已经取消时不再执行补偿删除。
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("delete local object: %w", err)
	}
	// 删除和读取共用同一 URI 安全解析逻辑。
	path, err := storage.resolveURI(uri)
	if err != nil {
		return err
	}
	// 不存在表示目标已经被删除，补偿目标已经达到，因此按成功处理。
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete local object: %w", err)
	}
	// nil 表示文件已不存在或刚刚成功删除。
	return nil
}

// resolveURI 把 URI 转成规范绝对路径，并执行目录边界检查。
func (storage *LocalStorage) resolveURI(uri string) (string, error) {
	// 只接受本适配器生成的 local:// 协议，避免误解析 HTTP 或 S3 地址。
	if !strings.HasPrefix(uri, "local://") {
		return "", fmt.Errorf("invalid local object URI %q", uri)
	}
	// 去掉协议后得到文件路径文本，并清理首尾空格。
	rawPath := strings.TrimSpace(strings.TrimPrefix(uri, "local://"))
	if rawPath == "" {
		return "", fmt.Errorf("invalid local object URI %q", uri)
	}

	// FromSlash 允许 URI 始终使用正斜杠，再转换成当前操作系统分隔符。
	candidate := filepath.Clean(filepath.FromSlash(rawPath))
	// 旧数据可能保存相对路径，因此需要在安全边界内做兼容解析。
	if !filepath.IsAbs(candidate) {
		// 兼容旧 MVP 保存的 `.data/storage/name` 相对 URI：如果它按当前工作目录解析后
		// 已经位于 baseDirectory 内就直接使用；单纯 `name` 则按根目录下对象处理。
		absoluteCandidate, err := filepath.Abs(candidate)
		if err != nil {
			return "", fmt.Errorf("resolve local object URI: %w", err)
		}
		// 已经按当前工作目录落在根目录内的旧路径可以直接使用。
		if isPathWithin(storage.baseDirectory, absoluteCandidate) {
			candidate = absoluteCandidate
		} else {
			// 单纯对象名等其他相对值按存储根目录下路径解释。
			candidate = filepath.Join(storage.baseDirectory, candidate)
		}
	}

	// 无论输入原本是否绝对，都转换成最终绝对路径再做一次边界检查。
	absoluteCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve local object path: %w", err)
	}
	// Clean 消除可能影响 Rel 判断的冗余路径段。
	absoluteCandidate = filepath.Clean(absoluteCandidate)
	// 任何逃离根目录的路径都拒绝，阻止目录穿越读取和删除。
	if !isPathWithin(storage.baseDirectory, absoluteCandidate) {
		return "", fmt.Errorf("local object URI escapes configured storage directory")
	}
	// 返回已确认位于根目录内的规范绝对路径。
	return absoluteCandidate, nil
}

// isPathWithin 使用 filepath.Rel 判断目标是否等于根目录或位于根目录之下。
// 不能只用字符串前缀，否则 `C:\data2` 会被误认为在 `C:\data` 内。
func isPathWithin(baseDirectory string, target string) bool {
	// Rel 计算从根目录到目标的相对路径，可跨平台正确处理盘符和分隔符。
	relative, err := filepath.Rel(baseDirectory, target)
	if err != nil {
		return false
	}
	// 等于 . 表示目标就是根目录；不以 .. 开头表示目标位于根目录之下。
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
