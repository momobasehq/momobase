package services

import (
	"encoding/json"
	"log/slog"

	"gorm.io/gorm"

	"momobase/internal/domain"
	"momobase/internal/platform"
)

type AuditService struct {
	db     *gorm.DB
	logger *slog.Logger
}

func NewAuditService(db *gorm.DB, logger ...*slog.Logger) *AuditService {
	var log *slog.Logger
	if len(logger) > 0 {
		log = logger[0]
	}
	return &AuditService{db, log}
}
func (s *AuditService) RecordBestEffort(actorID, actorType, action, entityType, entityID string, meta any, ip, ua string) {
	if s == nil {
		return
	}
	data, err := json.Marshal(meta)
	if err != nil {
		data = []byte(`{"marshal_error":true}`)
	}
	err = s.db.Create(&domain.AuditLog{BaseModel: domain.BaseModel{ID: platform.NewID("aud")}, ActorID: actorID, ActorType: actorType, Action: action, EntityType: entityType, EntityID: entityID, MetadataJSON: string(data), IPAddress: ip, UserAgent: ua}).Error
	if err != nil && s.logger != nil {
		s.logger.Error("audit log write failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}
