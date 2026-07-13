package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"travel-agent-go/internal/knowledge"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *knowledge.Service
}

type PageResult struct {
	Records []knowledge.Document `json:"records"`
	Total   int64                `json:"total"`
	Current int                  `json:"current"`
	Size    int                  `json:"size"`
}

func NewRouter(service *knowledge.Service) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	handler := &Handler{service: service}
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, Success(gin.H{"status": "ok"}))
	})
	api := router.Group("/api/knowledge")
	api.POST("/bases/:kbID/documents/upload", handler.upload)
	api.POST("/documents/:docID/chunk", handler.startChunk)
	api.GET("/documents/:docID", handler.getDocument)
	api.GET("/documents/:docID/status", handler.getDocument)
	api.GET("/bases/:kbID/documents", handler.listDocuments)
	api.DELETE("/documents/:docID", handler.deleteDocument)
	return router
}

func (h *Handler) upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		writeError(c, err)
		return
	}
	reader, err := file.Open()
	if err != nil {
		writeError(c, err)
		return
	}
	defer reader.Close()

	doc, err := h.service.UploadDocument(c.Request.Context(), knowledge.UploadInput{
		KnowledgeBaseID: c.Param("kbID"),
		FileName:        file.Filename,
		Title:           c.PostForm("title"),
		ContentType:     file.Header.Get("Content-Type"),
		Language:        c.PostForm("language"),
		ChunkStrategy:   c.PostForm("chunkStrategy"),
		Content:         reader,
		Size:            file.Size,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, Success(doc))
}

func (h *Handler) startChunk(c *gin.Context) {
	var options knowledge.ChunkOptions
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&options); err != nil {
			writeError(c, err)
			return
		}
	}
	doc, err := h.service.StartChunk(c.Request.Context(), c.Param("docID"), options)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, Success(doc))
}

func (h *Handler) getDocument(c *gin.Context) {
	doc, err := h.service.GetDocument(c.Request.Context(), c.Param("docID"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, Success(doc))
}

func (h *Handler) listDocuments(c *gin.Context) {
	page := intQuery(c, "current", 1)
	size := intQuery(c, "size", 20)
	docs, total, err := h.service.ListDocuments(c.Request.Context(), c.Param("kbID"), page, size)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, Success(PageResult{
		Records: docs,
		Total:   total,
		Current: page,
		Size:    size,
	}))
}

func (h *Handler) deleteDocument(c *gin.Context) {
	if err := h.service.DeleteDocument(c.Request.Context(), c.Param("docID")); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, Success(nil))
}

func writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := ServiceErrorCode
	if errors.Is(err, knowledge.ErrInvalidArgument) || errors.Is(err, knowledge.ErrDuplicate) || errors.Is(err, knowledge.ErrAlreadyRunning) {
		status = http.StatusBadRequest
		code = ClientErrorCode
	}
	if errors.Is(err, knowledge.ErrNotFound) {
		status = http.StatusNotFound
		code = ClientErrorCode
	}
	c.JSON(status, Failure(code, err.Error()))
}

func intQuery(c *gin.Context, name string, fallback int) int {
	value := c.Query(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
