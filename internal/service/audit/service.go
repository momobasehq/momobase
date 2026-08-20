package audit

import (
	"context"
	"encoding/json"
	"log/slog"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
)

// Service records security- and administration-relevant actions in the audit log.
type Service struct {
	db     *gorm.DB
	logger *slog.Logger
}

// New creates an audit service and optionally configures a logger for write failures.
func New(db *gorm.DB, logger ...*slog.Logger) *Service {
	var log *slog.Logger
	if len(logger) > 0 {
		log = logger[0]
	}
	return &Service{db, log}
}

// RecordBestEffort attempts to persist an audit entry without returning failures to the caller.
func (s *Service) RecordBestEffort(ctx context.Context, actorID, actorType, action, entityType, entityID string, meta any, ip, ua string) {
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
