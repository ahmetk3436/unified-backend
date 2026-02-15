package snapstreak

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type SnapHandler struct {
	snapService *SnapService
}

func NewSnapHandler(snapService *SnapService) *SnapHandler {
	return &SnapHandler{snapService: snapService}
}

// CreateSnap handles POST /snaps - creates a new snap with multipart/form-data image upload.
func (h *SnapHandler) CreateSnap(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	// Get the uploaded file
	file, err := c.FormFile("image")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Image file is required",
		})
	}

	// Validate file size (max 10MB)
	if file.Size > 10*1024*1024 {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Image size must be less than 10MB",
		})
	}

	// Validate content type
	contentType := file.Header.Get("Content-Type")
	validTypes := map[string]bool{
		"image/jpeg": true,
		"image/jpg":  true,
		"image/png":  true,
		"image/heic": true,
	}
	if !validTypes[contentType] {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid image format. Only JPEG, PNG, and HEIC are allowed",
		})
	}

	caption := c.FormValue("caption", "")
	filter := c.FormValue("filter", "none")

	// Generate unique filename
	fileExt := filepath.Ext(file.Filename)
	if fileExt == "" {
		fileExt = ".jpg"
	}
	filename := fmt.Sprintf("%s_%s%s", userID.String()[:8], uuid.New().String()[:8], fileExt)

	// Save the file
	uploadDir := "./uploads/snaps"
	savePath := filepath.Join(uploadDir, filename)
	if err := c.SaveFile(file, savePath); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to save image",
		})
	}

	imageURL := fmt.Sprintf("/uploads/snaps/%s", filename)

	snap, err := h.snapService.CreateSnap(appID, userID, imageURL, caption, filter)
	if err != nil {
		os.Remove(savePath)
		if errors.Is(err, ErrInvalidFilter) {
			return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to create snap",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(SnapResponse{
		ID:        snap.ID.String(),
		UserID:    snap.UserID.String(),
		ImageURL:  snap.ImageURL,
		Caption:   snap.Caption,
		Filter:    snap.Filter,
		SnapDate:  snap.SnapDate,
		LikeCount: snap.LikeCount,
		CreatedAt: snap.CreatedAt,
	})
}

// GetMySnaps handles GET /snaps - returns paginated snaps for the authenticated user.
func (h *SnapHandler) GetMySnaps(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit
	snaps, total, err := h.snapService.GetUserSnaps(appID, userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch snaps",
		})
	}

	baseURL := c.Protocol() + "://" + c.Hostname()
	snapResponses := make([]SnapResponse, len(snaps))
	for i, snap := range snaps {
		imageURL := snap.ImageURL
		if len(imageURL) > 0 && imageURL[0] == '/' {
			imageURL = baseURL + imageURL
		}
		snapResponses[i] = SnapResponse{
			ID:        snap.ID.String(),
			UserID:    snap.UserID.String(),
			ImageURL:  imageURL,
			Caption:   snap.Caption,
			Filter:    snap.Filter,
			SnapDate:  snap.SnapDate,
			LikeCount: snap.LikeCount,
			CreatedAt: snap.CreatedAt,
		}
	}

	return c.JSON(SnapsListResponse{
		Snaps: snapResponses,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// GetStreak handles GET /snaps/streak - returns the authenticated user's streak info.
func (h *SnapHandler) GetStreak(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	streak, err := h.snapService.GetStreak(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch streak",
		})
	}

	todaySnap, err := h.snapService.GetTodaySnap(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to check today's snap",
		})
	}

	return c.JSON(StreakResponse{
		CurrentStreak:    streak.CurrentStreak,
		LongestStreak:    streak.LongestStreak,
		TotalSnaps:       streak.TotalSnaps,
		LastSnapDate:     streak.LastSnapDate,
		HasSnappedToday:  todaySnap != nil,
		FreezesAvailable: streak.FreezesAvailable,
		FreezesUsed:      streak.FreezesUsed,
	})
}

// AddFreeze handles POST /snaps/streak/freeze - adds a streak freeze to the user's account.
func (h *SnapHandler) AddFreeze(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	err = h.snapService.AddStreakFreeze(appID, userID)
	if err != nil {
		if err.Error() == "maximum freezes reached (3)" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "You already have the maximum of 3 freezes",
			})
		}
		if err.Error() == "no streak record found for user" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "No streak record found. Start your streak first!",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to add streak freeze",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Streak freeze added successfully",
		"data": fiber.Map{
			"freezes_available": 1,
		},
	})
}

// GetSnapCalendar handles GET /snaps/calendar - returns an array of date strings for the user's snap activity.
func (h *SnapHandler) GetSnapCalendar(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized",
		})
	}

	days := c.QueryInt("days", 35)
	if days < 7 {
		days = 7
	}
	if days > 90 {
		days = 90
	}

	dates, err := h.snapService.GetSnapDates(appID, userID, days)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve calendar data",
		})
	}

	return c.JSON(fiber.Map{
		"dates": dates,
		"days":  days,
	})
}

// DeleteSnap handles DELETE /snaps/:id - soft deletes a snap if owned by the user.
func (h *SnapHandler) DeleteSnap(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	snapID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid snap ID",
		})
	}

	if err := h.snapService.DeleteSnap(appID, userID, snapID); err != nil {
		if errors.Is(err, ErrSnapNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
				Error: true, Message: "Snap not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to delete snap",
		})
	}

	return c.JSON(fiber.Map{"message": "Snap deleted"})
}

// LikeSnap handles POST /snaps/:id/like - increments the like count.
func (h *SnapHandler) LikeSnap(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	snapID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid snap ID",
		})
	}

	if err := h.snapService.LikeSnap(appID, snapID); err != nil {
		if errors.Is(err, ErrSnapNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
				Error: true, Message: "Snap not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to like snap",
		})
	}

	return c.JSON(fiber.Map{"message": "Snap liked"})
}
