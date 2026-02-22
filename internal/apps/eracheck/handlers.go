package eracheck

import (
	"errors"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EraHandler struct {
	eraService *EraService
}

func NewEraHandler(eraService *EraService) *EraHandler {
	return &EraHandler{eraService: eraService}
}

func (h *EraHandler) GetQuestions(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	questions, err := h.eraService.GetQuizQuestions(appID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to retrieve questions",
		})
	}
	return c.JSON(fiber.Map{"error": false, "questions": questions})
}

func (h *EraHandler) SubmitQuiz(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	var req struct {
		Answers map[string]int `json:"answers"`
	}
	if err := c.BodyParser(&req); err != nil || len(req.Answers) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Answers are required",
		})
	}

	result, err := h.eraService.SubmitQuizAnswers(appID, userID, req.Answers)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to submit quiz",
		})
	}

	response := fiber.Map{"result": result}
	if profile, ok := EraProfiles[result.Era]; ok {
		response["profile"] = profile
	}
	if topEras := GetTopErasForResult(result.Scores); topEras != nil {
		response["top_eras"] = topEras
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"error": false, "data": response})
}

func (h *EraHandler) GetResults(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	results, err := h.eraService.GetUserResults(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to retrieve results",
		})
	}
	return c.JSON(fiber.Map{"error": false, "results": results})
}

func (h *EraHandler) GetResult(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid result ID",
		})
	}

	result, err := h.eraService.GetResultByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || err.Error() == "record not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": true, "message": "Result not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to retrieve result",
		})
	}

	response := fiber.Map{"result": result}
	if profile, ok := EraProfiles[result.Era]; ok {
		response["profile"] = profile
	}
	return c.JSON(fiber.Map{"error": false, "data": response})
}

func (h *EraHandler) ShareResult(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid result ID",
		})
	}

	if err := h.eraService.IncrementShareCount(id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to update share count",
		})
	}
	return c.JSON(fiber.Map{"error": false, "message": "Share count incremented"})
}

func (h *EraHandler) GetStats(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	stats, err := h.eraService.GetEraStats(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to retrieve stats",
		})
	}
	return c.JSON(fiber.Map{"error": false, "stats": stats})
}

// --- Challenge Handler ---

type ChallengeHandler struct {
	challengeService *ChallengeService
	streakService    *StreakService
}

func NewChallengeHandler(challengeService *ChallengeService, streakService *StreakService) *ChallengeHandler {
	return &ChallengeHandler{challengeService: challengeService, streakService: streakService}
}

func (h *ChallengeHandler) GetDailyChallenge(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	challenge, err := h.challengeService.GetDailyChallenge(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to retrieve challenge",
		})
	}
	return c.JSON(fiber.Map{"error": false, "challenge": challenge.ToPublicView()})
}

func (h *ChallengeHandler) SubmitChallenge(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	var req struct {
		Answer string `json:"answer"`
	}
	if err := c.BodyParser(&req); err != nil || req.Answer == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Answer is required",
		})
	}

	challenge, err := h.challengeService.SubmitChallengeAnswer(appID, userID, req.Answer)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": err.Error(),
		})
	}

	if err := h.streakService.UpdateStreak(appID, userID); err != nil {
		_ = err // non-critical
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"error": false, "challenge": challenge.ToPublicView()})
}

func (h *ChallengeHandler) GetHistory(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	challenges, err := h.challengeService.GetChallengeHistory(appID, userID, c.QueryInt("limit", 30))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to retrieve history",
		})
	}

	views := make([]ChallengePublicView, len(challenges))
	for i, ch := range challenges {
		views[i] = ch.ToPublicView()
	}
	return c.JSON(fiber.Map{"error": false, "challenges": views})
}

func (h *ChallengeHandler) GetStreak(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	streak, err := h.streakService.GetOrCreateStreak(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to retrieve streak",
		})
	}

	badges, err := h.streakService.GetStreakBadges(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to retrieve badges",
		})
	}

	return c.JSON(fiber.Map{"error": false, "streak": streak, "badges": badges})
}
