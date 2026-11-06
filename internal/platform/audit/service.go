package audit

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Service 负责写入与查询平台操作审计。
type Service struct {
	platform tenancy.PlatformDB
}

// NewService 创建审计服务。
func NewService(platform tenancy.PlatformDB) *Service {
	return &Service{platform: platform}
}

// Record 写入一条审计。
// 调用方通常忽略返回值：审计写失败不能阻断主流程。
func (s *Service) Record(ctx context.Context, entry Entry) error {
	record := Record{
		ID:         uuid.Must(uuid.NewV7()).String(),
		Category:   entry.Category,
		Action:     entry.Action,
		Level:      defaultString(entry.Level, LevelInfo),
		Result:     defaultString(entry.Result, ResultSuccess),
		ObjectType: entry.ObjectType,
		ObjectID:   entry.ObjectID,
		Summary:    truncate(entry.Summary, 250),
		RequestID:  response.RequestIDFrom(ctx),
		IP:         response.ClientIPFrom(ctx),
	}

	actorType, actorID, actorEmail := entry.ActorType, entry.ActorID, entry.ActorEmail
	if scope, err := tenancy.ScopeFrom(ctx); err == nil {
		if actorType == "" {
			if scope.Actor == tenancy.ActorPlatform {
				actorType = ActorAdmin
			} else {
				actorType = ActorSystem
			}
		}
		if actorID == "" {
			actorID = scope.UserID
		}
		if actorEmail == "" {
			actorEmail = scope.ActorEmail
		}
	}
	record.ActorType = defaultString(actorType, ActorSystem)
	if actorID != "" {
		record.ActorID = &actorID
	}
	if actorEmail != "" {
		record.ActorEmail = &actorEmail
	}

	if entry.Before != nil {
		if raw, err := json.Marshal(entry.Before); err == nil {
			text := string(raw)
			record.Before = &text
		}
	}
	if entry.After != nil {
		if raw, err := json.Marshal(entry.After); err == nil {
			text := string(raw)
			record.After = &text
		}
	}

	return s.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Create(&record)
	})
}

// List 按条件分页查询，时间倒序。
func (s *Service) List(ctx context.Context, filter Filter) ([]Output, int64, error) {
	page, size := normalizePage(filter.Page, filter.Size)

	var rows []Record
	var total int64

	err := s.platform.Read(ctx, func(tx tenancy.Tx) error {
		query := tx.Model(&Record{})
		if filter.Category != "" {
			query = query.Where("category = ?", filter.Category)
		}
		if filter.Action != "" {
			query = query.Where("action = ?", filter.Action)
		}
		if filter.Level != "" {
			query = query.Where("level = ?", filter.Level)
		}
		if filter.Actor != "" {
			query = query.Where("actor_email = ? OR actor_id::text = ?", filter.Actor, filter.Actor)
		}
		if filter.ObjectID != "" {
			query = query.Where("object_id = ?", filter.ObjectID)
		}
		if filter.From != nil {
			query = query.Where("created_at >= ?", *filter.From)
		}
		if filter.To != nil {
			query = query.Where("created_at <= ?", *filter.To)
		}

		if err := query.Count(&total); err != nil {
			return err
		}

		return query.Order("created_at DESC").Limit(size).Offset((page - 1) * size).Find(&rows)
	})
	if err != nil {
		return nil, 0, apperr.Wrap(err, apperr.CodeInternal, "list audit logs")
	}

	out := make([]Output, 0, len(rows))
	for _, row := range rows {
		out = append(out, toOutput(row))
	}

	return out, total, nil
}

func toOutput(row Record) Output {
	out := Output{
		ID:         row.ID,
		Category:   row.Category,
		Action:     row.Action,
		Level:      row.Level,
		Result:     row.Result,
		ActorType:  row.ActorType,
		ObjectType: row.ObjectType,
		ObjectID:   row.ObjectID,
		Summary:    row.Summary,
		RequestID:  row.RequestID,
		IP:         row.IP,
		CreatedAt:  row.CreatedAt,
	}
	if row.ActorEmail != nil {
		out.ActorEmail = *row.ActorEmail
	}
	if row.Before != nil {
		out.Before = json.RawMessage(*row.Before)
	}
	if row.After != nil {
		out.After = json.RawMessage(*row.After)
	}

	return out
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	return page, size
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}

	return value[:limit]
}
