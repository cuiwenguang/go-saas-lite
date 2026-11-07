// Package sms 定义短信发送接口；当前只提供开发用模拟发送器。
package sms

import (
	"context"
	"log/slog"
)

// Sender 发送验证码短信。
type Sender interface {
	SendCode(ctx context.Context, phone, scene, code string) error
}

// DevSender 不真正发送，只把验证码写日志，便于本地联调。
type DevSender struct {
	logger *slog.Logger
}

// NewDevSender 创建开发用发送器。
func NewDevSender(logger *slog.Logger) *DevSender {
	return &DevSender{logger: logger}
}

// SendCode 输出脱敏手机号与验证码。
func (s *DevSender) SendCode(ctx context.Context, phone, scene, code string) error {
	if s.logger != nil {
		s.logger.InfoContext(ctx, "sms code (dev)", "phone", Mask(phone), "scene", scene, "code", code)
	}

	return nil
}

// Mask 对手机号做展示脱敏。
func Mask(phone string) string {
	switch {
	case len(phone) <= 4:
		return "****"
	case len(phone) <= 7:
		return phone[:1] + "****" + phone[len(phone)-2:]
	default:
		return phone[:3] + "****" + phone[len(phone)-4:]
	}
}
