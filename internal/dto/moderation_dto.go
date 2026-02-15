package dto

import "github.com/google/uuid"

type CreateReportRequest struct {
	ContentType string `json:"content_type"`
	ContentID   string `json:"content_id"`
	Reason      string `json:"reason"`
}

type ActionReportRequest struct {
	Status    string `json:"status"`
	AdminNote string `json:"admin_note"`
}

type BlockUserRequest struct {
	BlockedID uuid.UUID `json:"blocked_id"`
}
