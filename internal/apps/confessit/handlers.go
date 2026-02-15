package confessit

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type ConfessionHandler struct {
	service *ConfessionService
}

func NewConfessionHandler(service *ConfessionService) *ConfessionHandler {
	return &ConfessionHandler{service: service}
}

// --- Request DTOs ---

type CreateConfessionRequest struct {
	Content  string `json:"content"`
	Category string `json:"category"`
	Mood     string `json:"mood"`
}

type AddCommentRequest struct {
	Content string `json:"content"`
}

type ReactRequest struct {
	Emoji string `json:"emoji"`
}

// --- Protected handlers (require JWT) ---

func (h *ConfessionHandler) Create(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "message": "Unauthorized"})
	}

	var req CreateConfessionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid request"})
	}

	confession, err := h.service.CreateConfession(appID, userID, req.Content, req.Category, req.Mood)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(confession)
}

func (h *ConfessionHandler) GetFeed(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	confessions, total, err := h.service.GetFeed(appID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"confessions": confessions,
			"pagination": fiber.Map{
				"page":        page,
				"limit":       limit,
				"total":       total,
				"total_pages": (total + int64(limit) - 1) / int64(limit),
			},
		},
	})
}

func (h *ConfessionHandler) GetByCategory(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	category := c.Params("category")
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	confessions, total, err := h.service.GetByCategory(appID, category, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"confessions": confessions,
			"pagination": fiber.Map{
				"page":     page,
				"limit":    limit,
				"total":    total,
				"category": category,
			},
		},
	})
}

func (h *ConfessionHandler) Like(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "message": "Unauthorized"})
	}

	confessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid confession ID"})
	}

	if err := h.service.LikeConfession(appID, userID, confessionID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *ConfessionHandler) AddComment(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "message": "Unauthorized"})
	}

	confessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid confession ID"})
	}

	var req AddCommentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid request"})
	}

	comment, err := h.service.AddComment(appID, userID, confessionID, req.Content)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(comment)
}

func (h *ConfessionHandler) GetComments(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	confessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid confession ID"})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)

	comments, err := h.service.GetComments(appID, confessionID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"comments": comments,
			"pagination": fiber.Map{
				"page":  page,
				"limit": limit,
			},
		},
	})
}

func (h *ConfessionHandler) Share(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	confessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid confession ID"})
	}

	if err := h.service.IncrementShare(appID, confessionID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *ConfessionHandler) GetStats(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "message": "Unauthorized"})
	}

	stats, err := h.service.GetStats(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(stats)
}

func (h *ConfessionHandler) GetByID(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	confessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid confession ID"})
	}

	confession, err := h.service.GetConfession(appID, confessionID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{"data": confession})
}

func (h *ConfessionHandler) Delete(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "message": "Unauthorized"})
	}

	confessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid confession ID"})
	}

	if err := h.service.DeleteConfession(appID, userID, confessionID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Confession deleted"})
}

func (h *ConfessionHandler) React(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "message": "Unauthorized"})
	}

	confessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid confession ID"})
	}

	var req ReactRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid request"})
	}

	if err := h.service.ReactToConfession(appID, userID, confessionID, req.Emoji); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *ConfessionHandler) GetReactions(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	confessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid confession ID"})
	}

	reactions, err := h.service.GetReactions(appID, confessionID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{"data": reactions})
}

func (h *ConfessionHandler) GetMyConfessions(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "message": "Unauthorized"})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)

	confessions, total, err := h.service.GetMyConfessions(appID, userID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"confessions": confessions,
		"total":       total,
		"page":        page,
	})
}

func (h *ConfessionHandler) GetTrending(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	confessions, total, err := h.service.GetTrendingFeed(appID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": "Failed to fetch trending confessions"})
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"confessions": confessions,
			"pagination": fiber.Map{
				"page":        page,
				"limit":       limit,
				"total":       total,
				"total_pages": (total + int64(limit) - 1) / int64(limit),
			},
		},
	})
}
