package application

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// EmbeddingDimensions 是当前数据库 vector(1536) 与模型响应共同遵守的固定维度。
// 修改它不能只改这一处，还必须同步数据库列、迁移脚本、Embedding 配置和相关测试。
const EmbeddingDimensions = 1536

// htmlTagPattern 是 Go MVP 的轻量 HTML 文本提取规则。
// 它只用于现有简单场景，不试图替代完整 HTML 解析器；复杂格式应在未来通过明确适配器扩展。
var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

// ProcessDocument 显式执行一个文档的读取、解析、分块、向量化和原子替换。
//
// 整个流程先在事务外完成所有慢操作和完整性校验，只有结果齐全后才调用仓储替换事务。
// 任何中途失败都会尽力把文档改为 failed，但失败回写错误不会覆盖最初的处理错误。
func (s *Service) ProcessDocument(ctx context.Context, docID string, options domain.ChunkOptions) (domain.Document, error) {
	// 清理复制或 URL 参数中可能出现的首尾空格，仓储只接收稳定 ID。
	docID = strings.TrimSpace(docID)
	// 空 ID 无法抢占具体文档，必须在任何数据库更新前拒绝。
	if docID == "" {
		return domain.Document{}, fmt.Errorf("%w: document id is empty", domain.ErrInvalidArgument)
	}

	// 在修改数据库状态前先验证选项。非法请求不应该把原本可处理的文档留在 processing 状态。
	normalizedOptions, err := options.Normalize()
	// 非法阈值直接返回，文档状态仍保持原样。
	if err != nil {
		return domain.Document{}, err
	}

	// TryMarkProcessing 必须由 PostgreSQL 使用条件 UPDATE 原子实现。
	// acquired=false 表示另一个请求已经占有处理权，本请求不能继续读取对象或调用模型。
	document, acquired, err := s.repo.TryMarkProcessing(ctx, docID)
	// 数据库更新或扫描失败时不能假设已经取得处理权。
	if err != nil {
		return domain.Document{}, fmt.Errorf("acquire document processing: %w", err)
	}
	// 未抢占成功通常表示文档正在被另一个请求处理，后续外部调用全部跳过。
	if !acquired {
		return domain.Document{}, domain.ErrAlreadyRunning
	}

	// 取得处理权后执行真正的读取、切块、向量化和提交主流程。
	completed, err := s.processAfterAcquire(ctx, document, normalizedOptions)
	// 任意主流程错误都触发尽力失败回写，同时把原始错误返回给调用方。
	if err != nil {
		s.recordProcessingFailure(ctx, document, err)
		return domain.Document{}, err
	}
	// 只有仓储替换事务成功后才返回 completed 文档。
	return completed, nil
}

// processAfterAcquire 执行已经取得处理权之后的主流程。
// 单独拆出这个函数可以清楚地区分“并发抢占”与“准备并提交结果”两个职责。
func (s *Service) processAfterAcquire(ctx context.Context, document domain.Document, options domain.ChunkOptions) (domain.Document, error) {
	// 第一步从对象存储取回原文件。数据库事务此时尚未开始，网络等待不会长期占用数据库锁。
	content, err := s.storage.Get(ctx, document.SourceURI)
	// 对象不存在、网络失败或 URI 非法都会终止本轮处理。
	if err != nil {
		return domain.Document{}, fmt.Errorf("read stored document: %w", err)
	}

	// 第二步根据文件类型提取纯文本。当前 MVP 对 pdf/doc/docx 明确返回“不支持”，不会假装解析成功。
	text, err := extractText(content, document.FileType, document.FileName)
	// 不支持的文件类型或解析失败不能继续生成虚假空分块。
	if err != nil {
		return domain.Document{}, fmt.Errorf("extract document text: %w", err)
	}

	// 第三步执行确定性的内存分块。纯空白内容会得到空结果，必须在调用模型前失败。
	chunks, err := ChunkText(text, options)
	// 算法参数或内部校验错误保留操作上下文后返回。
	if err != nil {
		return domain.Document{}, fmt.Errorf("chunk document text: %w", err)
	}
	// 空白文档得到空切片，不能调用模型或进入替换事务。
	if len(chunks) == 0 {
		return domain.Document{}, fmt.Errorf("parsed chunks are empty")
	}

	// 算法只生成内容和位置；应用用例在这里补齐持久化所需的 ID 与归属信息。
	texts := make([]string, len(chunks))
	// 按下标原地补齐每个分块，保证 texts、chunks 和 vectors 始终使用同一顺序。
	for index := range chunks {
		// 每个分块使用独立 ID，向量表沿用该 ID 建立一一对应关系。
		chunks[index].ID = s.newID()
		// 知识库归属来自已抢占文档，不能由分块算法自行猜测。
		chunks[index].KbID = document.KbID
		// 文档归属同样来自聚合根。
		chunks[index].DocumentID = document.ID
		// 当前 MVP 没有 tokenizer，沿用旧行为，用 Unicode 字符数作为近似 token 数。
		chunks[index].TokenCount = chunks[index].CharCount
		// 补齐持久化字段后再次验证，防止不完整分块进入 Embedding 结果配对。
		if err := chunks[index].Validate(); err != nil {
			return domain.Document{}, fmt.Errorf("validate chunk %d: %w", index, err)
		}
		// 单独建立文本切片，外部模型不需要知道领域 Chunk 的其他字段。
		texts[index] = chunks[index].Content
	}

	// 第四步批量调用 Embedding。批量顺序必须与 chunks 顺序一致，后面才能按相同下标配对写库。
	vectors, err := s.embedder.EmbedTexts(ctx, texts)
	// 网络、鉴权或模型错误直接终止，仓储仍保留旧分块和向量。
	if err != nil {
		return domain.Document{}, fmt.Errorf("embed document chunks: %w", err)
	}
	// 向量数量必须与分块数量相同，否则按下标配对会错位。
	if len(vectors) != len(chunks) {
		return domain.Document{}, fmt.Errorf("embedding count %d does not match chunk count %d", len(vectors), len(chunks))
	}
	// 逐个检查固定维度，不能只验证第一个向量就假设整批都正确。
	for index, vector := range vectors {
		// 任意向量不是 1536 维时，整批结果都不能进入数据库事务。
		if len(vector) != EmbeddingDimensions {
			return domain.Document{}, fmt.Errorf("embedding %d dimensions = %d, want %d", index, len(vector), EmbeddingDimensions)
		}
	}

	// 所有外部结果都完整后，先由聚合生成合法 completed 状态。
	completed, err := document.MarkCompleted(len(chunks), options.AsMap(), s.now())
	// 聚合转换失败说明业务状态不一致，不能让仓储直接拼 completed SQL。
	if err != nil {
		return domain.Document{}, fmt.Errorf("complete document aggregate: %w", err)
	}

	// 最后才进入仓储事务：旧 chunk/vector 删除、新数据写入和文档完成状态必须一起提交或一起回滚。
	if err := s.repo.ReplaceDocumentChunks(ctx, completed, chunks, vectors); err != nil {
		return domain.Document{}, fmt.Errorf("replace document chunks: %w", err)
	}
	// 事务提交成功后，返回值与数据库中的 completed 状态保持一致。
	return completed, nil
}

// recordProcessingFailure 尽力保存最近一次处理错误，同时坚持“原始错误优先”。
func (s *Service) recordProcessingFailure(ctx context.Context, document domain.Document, cause error) {
	// 状态变化仍由领域聚合完成，仓储只负责持久化最终结果，不能在 SQL 中重新拼业务规则。
	failed, transitionErr := document.MarkFailed(cause.Error(), s.now())
	// 极少数状态转换失败只记录附加错误，原处理错误仍由 ProcessDocument 返回。
	if transitionErr != nil {
		s.logger.ErrorContext(ctx, "build failed document status",
			"document_id", document.ID,
			"processing_error", cause,
			"transition_error", transitionErr,
		)
		return
	}

	// 聚合已经成功转换为 failed 后，再尽力把完整状态写入仓储。
	if persistErr := s.repo.MarkFailed(ctx, failed); persistErr != nil {
		s.logger.ErrorContext(ctx, "persist failed document status",
			"document_id", document.ID,
			"processing_error", cause,
			"persist_error", persistErr,
		)
	}
}

// extractText 把当前 MVP 支持的文件内容转换为纯文本。
// 不支持的 Office/PDF 类型返回明确错误，调用链会把该错误写入 metadata.lastError。
func extractText(content []byte, fileType string, fileName string) (string, error) {
	// 优先使用数据库记录的文件类型，并统一去点号、空格和大小写。
	extension := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(fileType), "."))
	// 旧数据没有 FileType 时，从原始文件名提取扩展名作为兼容回退。
	if extension == "" {
		extension = strings.ToLower(strings.TrimPrefix(filepath.Ext(fileName), "."))
	}

	// 按扩展名选择当前 MVP 明确支持的轻量解析策略。
	switch extension {
	case "txt", "md", "markdown":
		// 纯文本和 Markdown 保留原始 UTF-8 内容，不做语法渲染。
		return string(content), nil
	case "html", "htm":
		// HTML 使用轻量标签剥离规则得到可分块文本。
		return stripHTML(string(content)), nil
	case "pdf", "doc", "docx":
		// 二进制文档尚无解析器，明确报错比把乱码当文本更安全。
		return "", fmt.Errorf("%s parsing is not implemented in Go MVP", extension)
	default:
		// 保留旧 MVP 的兼容行为：未知但已由上传策略允许的文本类型按原始 UTF-8 内容处理。
		return string(content), nil
	}
}

// stripHTML 去掉简单标签并把连续空白折叠成一个空格。
func stripHTML(input string) string {
	// 先把所有简单标签替换为空格，避免标签两侧文字意外粘连。
	withoutTags := htmlTagPattern.ReplaceAllString(input, " ")
	// Fields 按连续空白切词，Join 再用单个空格连接，得到稳定纯文本。
	return strings.Join(strings.Fields(withoutTags), " ")
}
