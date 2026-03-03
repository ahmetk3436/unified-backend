package moodpulse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const (
	moodPhotoMaxBytes = 10 * 1024 * 1024 // 10 MB
	moodAudioMaxBytes = 25 * 1024 * 1024 // 25 MB

	moodOpenAIWhisperURL = "https://api.openai.com/v1/audio/transcriptions"
	moodBaseUploadURL    = "https://api.vexellabspro.com"
)

var moodAllowedPhotoMIME = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/heic": "heic",
}

var moodAllowedAudioMIME = map[string]string{
	"audio/m4a":  "m4a",
	"audio/mpeg": "mp3",
	"audio/wav":  "wav",
	"audio/aac":  "aac",
	"audio/mp4":  "m4a",
	"audio/webm": "webm",
}

// UploadHandler handles file upload and transcription endpoints for moodpulse.
type UploadHandler struct {
	openAIAPIKey string
	aiTimeout    time.Duration
	uploadsRoot  string // absolute path to uploads directory on disk
}

func NewUploadHandler(openAIAPIKey string, aiTimeout time.Duration, uploadsRoot string) *UploadHandler {
	return &UploadHandler{
		openAIAPIKey: openAIAPIKey,
		aiTimeout:    aiTimeout,
		uploadsRoot:  uploadsRoot,
	}
}

// UploadPhoto handles POST /moods/upload-photo
// Accepts multipart/form-data with a "photo" field.
// Validates size (max 10MB) and MIME type, saves to disk, returns URL.
func (h *UploadHandler) UploadPhoto(c *fiber.Ctx) error {
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	fileHeader, err := c.FormFile("photo")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "photo field is required",
		})
	}

	if fileHeader.Size > moodPhotoMaxBytes {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "photo exceeds maximum size of 10MB",
		})
	}

	f, err := fileHeader.Open()
	if err != nil {
		slog.Error("[moodpulse] photo upload: open file failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "failed to read uploaded file",
		})
	}
	defer f.Close()

	// Read full content (bounded by size check above).
	data, err := io.ReadAll(io.LimitReader(f, moodPhotoMaxBytes+1))
	if err != nil {
		slog.Error("[moodpulse] photo upload: read failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "failed to read uploaded file",
		})
	}
	if int64(len(data)) > moodPhotoMaxBytes {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "photo exceeds maximum size of 10MB",
		})
	}

	// Validate MIME type via Content-Type header AND magic bytes.
	contentType := fileHeader.Header.Get("Content-Type")
	// Normalise: strip parameters (e.g. "image/jpeg; charset=...")
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	ext, allowed := moodAllowedPhotoMIME[contentType]
	if !allowed {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "unsupported image type; allowed: jpeg, png, webp, heic",
		})
	}

	if !validateMoodPhotoMagic(data, contentType) {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "file content does not match declared image type",
		})
	}

	// Generate UUID filename — never use user-supplied filename.
	filename := uuid.New().String() + "." + ext
	// filepath.Base is a no-op on a pure UUID.ext string, but belt-and-suspenders.
	filename = filepath.Base(filename)

	// Store under moodpulse/photos/{user_id}/ to namespace by user.
	userIDStr := userID.String()
	savePath := filepath.Join(h.uploadsRoot, "moodpulse", "photos", userIDStr, filename)
	if err := moodSaveFile(savePath, data); err != nil {
		slog.Error("[moodpulse] photo upload: save failed", "path", savePath, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "failed to save photo",
		})
	}

	url := fmt.Sprintf("%s/uploads/moodpulse/photos/%s/%s", moodBaseUploadURL, userIDStr, filename)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"url": url})
}

// Transcribe handles POST /moods/transcribe
// Accepts multipart/form-data with an "audio" field.
// Validates size (max 25MB) and MIME type, calls OpenAI Whisper, returns transcript.
func (h *UploadHandler) Transcribe(c *fiber.Ctx) error {
	_, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	if h.openAIAPIKey == "" {
		slog.Error("[moodpulse] transcribe: OPENAI_API_KEY not configured")
		return c.Status(fiber.StatusServiceUnavailable).JSON(dto.ErrorResponse{
			Error: true, Message: "transcription service not available",
		})
	}

	fileHeader, err := c.FormFile("audio")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "audio field is required",
		})
	}

	if fileHeader.Size > moodAudioMaxBytes {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "audio exceeds maximum size of 25MB",
		})
	}

	f, err := fileHeader.Open()
	if err != nil {
		slog.Error("[moodpulse] transcribe: open file failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "failed to read uploaded file",
		})
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, moodAudioMaxBytes+1))
	if err != nil {
		slog.Error("[moodpulse] transcribe: read failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "failed to read uploaded file",
		})
	}
	if int64(len(data)) > moodAudioMaxBytes {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "audio exceeds maximum size of 25MB",
		})
	}

	// Validate MIME type via Content-Type header.
	contentType := fileHeader.Header.Get("Content-Type")
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	ext, allowed := moodAllowedAudioMIME[contentType]
	if !allowed {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "unsupported audio type; allowed: m4a, mp3, wav, aac, mp4, webm",
		})
	}

	transcript, err := h.callWhisper(data, ext)
	if err != nil {
		slog.Error("[moodpulse] transcribe: OpenAI Whisper call failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "transcription failed",
		})
	}

	return c.JSON(fiber.Map{
		"transcript": transcript,
	})
}

// callWhisper sends the audio bytes to OpenAI Whisper and returns the transcript.
// Uses POST https://api.openai.com/v1/audio/transcriptions with model=whisper-1.
func (h *UploadHandler) callWhisper(audioData []byte, ext string) (string, error) {
	// Build a safe filename — never send the user-supplied filename.
	safeFilename := "audio." + ext

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile("file", safeFilename)
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := fw.Write(audioData); err != nil {
		return "", fmt.Errorf("write audio data: %w", err)
	}
	// Add required model field.
	if err := mw.WriteField("model", "whisper-1"); err != nil {
		return "", fmt.Errorf("write model field: %w", err)
	}
	mw.Close()

	timeout := h.aiTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	httpClient := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodPost, moodOpenAIWhisperURL, &buf)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+h.openAIAPIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024)) // 1MB response cap
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI Whisper returned status %d", resp.StatusCode)
	}

	// Parse OpenAI Whisper response.
	// Response shape: {"text": "transcribed text here"}
	var whisperResp struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &whisperResp); err != nil {
		return "", fmt.Errorf("parse OpenAI Whisper response: %w", err)
	}

	return strings.TrimSpace(whisperResp.Text), nil
}

// validateMoodPhotoMagic checks that the file bytes match the declared MIME type.
func validateMoodPhotoMagic(data []byte, mimeType string) bool {
	if len(data) < 4 {
		return false
	}
	switch mimeType {
	case "image/jpeg":
		return len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF
	case "image/png":
		return len(data) >= 4 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47
	case "image/webp":
		// RIFF....WEBP
		return len(data) >= 12 &&
			data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
			data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50
	case "image/heic":
		// HEIC has an ftyp box at offset 4: bytes 4-7 == "ftyp"
		return len(data) >= 8 &&
			data[4] == 0x66 && data[5] == 0x74 && data[6] == 0x79 && data[7] == 0x70
	}
	return false
}

// moodSaveFile writes data to disk at the given path, creating parent directories as needed.
func moodSaveFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	return os.WriteFile(path, data, 0644)
}
