package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Entry 是一次审计写入。
type Entry struct {
	Category   string
	Action     string
	Level      string
	Result     string
	ObjectType string
	ObjectID   string
	Summary    string
	Before     any
	After      any
}

// Filter 是审计查询条件。
type Filter struct {
	Category string
	Action   string
	Level    string
	Actor    string
	From     *time.Time
	To       *time.Time
	Page     int
	Size     int
}

// Output 是审计列表出参。
type Output struct {
	ID         string          `json:"id"`
	Category   string          `json:"category"`
	Action     string          `json:"action"`
	Level      string          `json:"level"`
	Result     string          `json:"result"`
	ActorID    string          `json:"actor_id,omitempty"`
	ActorEmail string          `json:"actor_email,omitempty"`
	ObjectType string          `json:"object_type"`
	ObjectID   string          `json:"object_id"`
	Summary    string          `json:"summary"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	RequestID  string          `json:"request_id,omitempty"`
	IP         string          `json:"ip,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Service 负责租户内审计写入与查询。
type Service struct {
	db tenancy.DB
}

// NewService 创建审计服务。
func NewService(db tenancy.DB) *Service {
	return &Service{db: db}
}

// Record 写入一条审计；调用方通常忽略返回值。
func (s *Service) Record(ctx context.Context, entry Entry) error {
	scope, err := tenancy.ScopeFrom(ctx)
	if err != nil || scope.TenantID == "" {
		return err
	}

	record := &Log{
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
		ActorEmail: scope.ActorEmail,
	}
	if scope.UserID != "" {
		actorID := scope.UserID
		record.ActorID = &actorID
	}
	if entry.Before != nil {
		if raw, err := json.Marshal(entry.Before); err == nil {
			value := string(raw)
			record.BeforeData = &value
		}
	}
	if entry.After != nil {
		if raw, err := json.Marshal(entry.After); err == nil {
			value := string(raw)
			record.AfterData = &value
		}
	}

	return s.db.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Create(record)
	})
}

// List 分页返回当前租户的审计日志。
func (s *Service) List(ctx context.Context, filter Filter) ([]Output, int64, error) {
	page, size := normalizePage(filter.Page, filter.Size)

	var rows []Log
	var total int64

	err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		query := tx.Model(&Log{})
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
			query = query.Where("(LOWER(actor_email) = ? OR actor_id::text = ?)", filter.Actor, filter.Actor)
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
		return nil, 0, apperr.Wrap(err, apperr.CodeInternal, "list tenant audit logs")
	}

	out := make([]Output, 0, len(rows))
	for _, row := range rows {
		item := Output{
			ID:         row.ID,
			Category:   row.Category,
			Action:     row.Action,
			Level:      row.Level,
			Result:     row.Result,
			ActorEmail: row.ActorEmail,
			ObjectType: row.ObjectType,
			ObjectID:   row.ObjectID,
			Summary:    row.Summary,
			RequestID:  row.RequestID,
			IP:         row.IP,
			CreatedAt:  row.CreatedAt,
		}
		if row.ActorID != nil {
			item.ActorID = *row.ActorID
		}
		if row.BeforeData != nil {
			item.Before = json.RawMessage(*row.BeforeData)
		}
		if row.AfterData != nil {
			item.After = json.RawMessage(*row.AfterData)
		}
		out = append(out, item)
	}

	return out, total, nil
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
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
