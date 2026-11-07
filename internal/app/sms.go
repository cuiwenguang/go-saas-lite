package app

import (
	"strings"

	"log/slog"

	"github.com/cuiwenguang/go-saas-lite/pkg/sms"
)

// newSMSSender 按配置选择短信通道；当前只提供 dev 模拟发送。
func newSMSSender(cfg Config, log *slog.Logger) sms.Sender {
	switch strings.ToLower(strings.TrimSpace(cfg.SMS.Provider)) {
	case "", "dev":
		return sms.NewDevSender(log)
	default:
		return sms.NewDevSender(log)
	}
}
