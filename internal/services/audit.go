package services

import (
	"context"
	"encoding/json"
	"log/slog"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
)

// AuditService records security- and administration-relevant actions in the audit log.
type AuditService struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewAuditService creates an audit service and optionally configures a logger for write failures.
func NewAuditService(db *gorm.DB, logger ...*slog.Logger) *AuditService {
	var log *slog.Logger
	if len(logger) > 0 {
		log = logger[0]
	}
	return &AuditService{db, log}
}

// RecordBestEffort attempts to persist an audit entry without returning failures to the caller.
func (s *AuditService) RecordBestEffort(ctx context.Context, actorID, actorType, action, entityType, entityID string, meta any, ip, ua string) {
	if s == nil {
		return
	}
	data, err := json.Marshal(meta)
	if err != nil {
		data = []byte(`{"marshal_error":true}`)
	}
	err = s.db.WithContext(ctx).Create(&domain.AuditLog{
		BaseModel:    domain.BaseModel{ID: platform.NewID("aud")},
		ActorID:      actorID,
		ActorType:    actorType,
		Action:       action,
		EntityType:   entityType,
		EntityID:     entityID,
		MetadataJSON: string(data),
		IPAddress:    ip,
		UserAgent:    ua,
	}).Error
	if err != nil && s.logger != nil {
		s.logger.Error("audit log write failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}
