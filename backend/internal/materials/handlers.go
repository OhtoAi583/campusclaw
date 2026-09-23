package materials

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"campusclaw/backend/internal/httpx"
)

// multipartOverhead 是 multipart 边界与表单字段预留的额外空间。
// 上限判定用的是文件本身，请求体整体则允许略大一点，避免把合法请求误判成超限。
const multipartOverhead = 1 << 20

// allowedExtensions 是白名单。黑名单无法穷举危险类型，因此这里只列允许项。
var allowedExtensions = map[string]string{
	".txt": "text/plain; charset=utf-8",
	".md":  "text/markdown; charset=utf-8",
}

// Handler 提供材料的读取与上传接口。
type Handler struct {
	Store          *Store
	UploadDir      string
	MaxUploadBytes int64
}

// List 返回会话所在班级的材料。租户标识只来自会话，请求上的任何班级参数都被忽略。
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	session, ok := httpx.SessionFrom(r.Context())
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	list, err := h.Store.ListByClass(r.Context(), session.ClassID)
	if err != nil {
		slog.Error("查询材料列表失败", "error", err, "class_id", session.ClassID)
		httpx.PermanentFailure(w)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": list, "class_id": session.ClassID})
}

// Detail 返回本班材料详情。先取行再核对班级；跨班与不存在对外完全同形。
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	material, ok := h.lookupOwned(w, r)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, material)
}

// File 以附件形式返回原文件，读取前同样先取行再核对班级。
// 上传目录不经 Nginx 静态暴露，这里是唯一的读取入口。
func (h *Handler) File(w http.ResponseWriter, r *http.Request) {
	material, ok := h.lookupOwned(w, r)
	if !ok {
		return
	}
	// 存储名由服务端生成，这里再用 Base 兜一层，确保拼接结果不越出上传目录。
	path := filepath.Join(h.UploadDir, filepath.Base(material.StoredName))
	file, err := os.Open(path)
	if err != nil {
		slog.Error("读取材料文件失败", "error", err, "material_id", material.ID)
		notFound(w)
		return
	}
	defer file.Close()

	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": material.OriginalName})
	w.Header().Set("Content-Type", contentTypeFor(material.OriginalName))
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, file); err != nil {
		slog.Error("写出材料文件失败", "error", err, "material_id", material.ID)
	}
}

// Upload 是教师专属接口：先做角色与内容校验，再落盘，最后在同一事务里写两张表。
// 失败时磁盘与数据库都不留残留。
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	session, ok := httpx.SessionFrom(r.Context())
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	// 角色一律取自会话。请求里带 role=teacher 之类的字段不会改变这里的判断。
	if session.Role != "teacher" {
		httpx.Fail(w, http.StatusForbidden, "forbidden")
		return
	}

	// 先限制整体请求体大小：超限会在读取完整请求体之前就被拒绝（413）。
	r.Body = http.MaxBytesReader(w, r.Body, h.MaxUploadBytes+multipartOverhead)
	if err := r.ParseMultipartForm(h.MaxUploadBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.Fail(w, http.StatusRequestEntityTooLarge, "payload_too_large")
			return
		}
		httpx.Fail(w, http.StatusBadRequest, "invalid_multipart")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "missing_file")
		return
	}
	defer file.Close()

	originalName := filepath.Base(strings.TrimSpace(header.Filename))
	ext := strings.ToLower(filepath.Ext(originalName))
	_, allowed := allowedExtensions[ext]
	if !allowed {
		httpx.Fail(w, http.StatusBadRequest, "unsupported_extension")
		return
	}

	content, err := io.ReadAll(io.LimitReader(file, h.MaxUploadBytes+1))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "unreadable_file")
		return
	}
	if int64(len(content)) > h.MaxUploadBytes {
		httpx.Fail(w, http.StatusRequestEntityTooLarge, "payload_too_large")
		return
	}
	if len(content) == 0 {
		httpx.Fail(w, http.StatusBadRequest, "empty_content")
		return
	}
	if !utf8.Valid(content) {
		httpx.Fail(w, http.StatusBadRequest, "invalid_encoding")
		return
	}

	title := titleFromFilename(originalName)
	storedName, err := newStoredName(ext)
	if err != nil {
		slog.Error("生成存储名失败", "error", err)
		httpx.PermanentFailure(w)
		return
	}

	if err := os.MkdirAll(h.UploadDir, 0o750); err != nil {
		slog.Error("创建上传目录失败", "error", err)
		httpx.PermanentFailure(w)
		return
	}
	// 客户端文件名不参与路径构造：存储名是服务端生成的随机串。
	path := filepath.Join(h.UploadDir, storedName)
	if err := os.WriteFile(path, content, 0o640); err != nil {
		slog.Error("写入上传文件失败", "error", err)
		httpx.PermanentFailure(w)
		return
	}

	material, err := h.Store.Create(r.Context(), CreateInput{
		// 归属班级只取自会话；表单里携带的 class_id 一律忽略。
		ClassID:      session.ClassID,
		Title:        title,
		OriginalName: originalName,
		StoredName:   storedName,
		SizeBytes:    int64(len(content)),
		UploadedBy:   session.UserID,
		Content:      string(content),
	})
	if err != nil {
		// 数据库写入失败：删掉刚落盘的文件，回到"无文件、无记录"的干净状态。
		if removeErr := os.Remove(path); removeErr != nil {
			slog.Error("清理上传文件失败", "error", removeErr, "path", path)
		}
		slog.Error("材料入库失败", "error", err)
		httpx.PermanentFailure(w)
		return
	}

	httpx.JSON(w, http.StatusCreated, material)
}

// lookupOwned 是"先取行再核对班级"的唯一实现，详情与下载共用。
// 跨班与不存在返回完全相同的响应体，真实原因只写日志。
func (h *Handler) lookupOwned(w http.ResponseWriter, r *http.Request) (Material, bool) {
	session, ok := httpx.SessionFrom(r.Context())
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return Material{}, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		notFound(w)
		return Material{}, false
	}
	material, err := h.Store.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		notFound(w)
		return Material{}, false
	}
	if err != nil {
		slog.Error("查询材料失败", "error", err, "material_id", id)
		httpx.PermanentFailure(w)
		return Material{}, false
	}
	if material.ClassID != session.ClassID {
		// 对外是同一个 404，审计信息留在服务端日志里。
		slog.Warn("拒绝跨班访问",
			"material_id", material.ID,
			"material_class_id", material.ClassID,
			"session_class_id", session.ClassID,
			"user_id", session.UserID,
		)
		notFound(w)
		return Material{}, false
	}
	return material, true
}

// notFound 是详情与下载共用的 404 响应，跨班与不存在逐字节一致。
func notFound(w http.ResponseWriter) {
	httpx.Fail(w, http.StatusNotFound, "not_found")
}

func contentTypeFor(name string) string {
	if ct, ok := allowedExtensions[strings.ToLower(filepath.Ext(name))]; ok {
		return ct
	}
	return "application/octet-stream"
}

// titleFromFilename 用客户端文件名生成展示标题；它只用于展示，不参与任何路径构造。
func titleFromFilename(name string) string {
	base := filepath.Base(name)
	title := strings.TrimSuffix(base, filepath.Ext(base))
	title = strings.TrimSpace(title)
	if title == "" {
		return base
	}
	return title
}

// newStoredName 生成服务端存储名，避免客户端的路径分隔符与重名问题。
func newStoredName(ext string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf) + ext, nil
}
