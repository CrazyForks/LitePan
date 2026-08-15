package aiorganize

import (
	"context"
	"net/url"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/settings"
)

type Config struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

func (s *Service) Config() Config {
	if s == nil || s.settings == nil {
		return Config{}
	}
	apiKey := s.settings.String(settings.KeyAIOrganizeAPIKey)
	if apiKey != "" {
		apiKey = maskAPIKey(apiKey)
	}
	return Config{
		Enabled: s.settings.Bool(settings.KeyAIOrganizeEnabled),
		BaseURL: s.settings.String(settings.KeyAIOrganizeBaseURL),
		APIKey:  apiKey,
		Model:   s.settings.String(settings.KeyAIOrganizeModel),
	}
}

func (s *Service) Update(ctx context.Context, in Config) (Config, error) {
	if s == nil || s.settings == nil {
		return Config{}, domain.Errorf(domain.CodeInternal, "AI 辅助增强配置服务未就绪")
	}
	currentKey := s.settings.String(settings.KeyAIOrganizeAPIKey)
	apiKey := strings.TrimSpace(in.APIKey)
	if isAPIKeyMask(apiKey, currentKey) {
		apiKey = currentKey
	}
	cfg := Config{
		Enabled: in.Enabled,
		BaseURL: strings.TrimSpace(in.BaseURL),
		APIKey:  apiKey,
		Model:   strings.TrimSpace(in.Model),
	}
	if err := validateConfig(cfg, cfg.Enabled); err != nil {
		return Config{}, err
	}
	if err := s.settings.Update(ctx, map[string]string{
		settings.KeyAIOrganizeEnabled: boolString(cfg.Enabled),
		settings.KeyAIOrganizeBaseURL: cfg.BaseURL,
		settings.KeyAIOrganizeAPIKey:  cfg.APIKey,
		settings.KeyAIOrganizeModel:   cfg.Model,
	}); err != nil {
		return Config{}, err
	}
	return s.Config(), nil
}

func maskAPIKey(apiKey string) string {
	return strings.Repeat("*", len([]rune(apiKey)))
}

func isAPIKeyMask(value, stored string) bool {
	return stored != "" && value == maskAPIKey(stored)
}

func validateConfig(cfg Config, requireComplete bool) error {
	if cfg.BaseURL != "" {
		u, err := url.Parse(cfg.BaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return domain.Errorf(domain.CodeValidation, "API 地址必须是有效的 HTTP/HTTPS 地址")
		}
	}
	if !requireComplete {
		return nil
	}
	if cfg.BaseURL == "" {
		return domain.Errorf(domain.CodeValidation, "请填写 API 地址")
	}
	if cfg.APIKey == "" {
		return domain.Errorf(domain.CodeValidation, "请填写 API Key")
	}
	if cfg.Model == "" {
		return domain.Errorf(domain.CodeValidation, "请填写模型名称")
	}
	return nil
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
