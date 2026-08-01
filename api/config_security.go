package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"sealchat/utils"
)

func sanitizeConfigForClient(cfg *utils.AppConfig) utils.AppConfig {
	ret := sanitizeConfigForAdmin(cfg)
	ret.AI.Providers = nil
	return ret
}

func sanitizeConfigForAdmin(cfg *utils.AppConfig) utils.AppConfig {
	if cfg == nil {
		return utils.AppConfig{}
	}
	ret := *cfg
	if len(cfg.AI.Providers) > 0 {
		ret.AI.Providers = append([]utils.AIProviderConfig(nil), cfg.AI.Providers...)
	}
	ret.RegisterInviteRequired = strings.TrimSpace(cfg.RegisterInviteCode) != ""
	ret.RegisterInviteCode = ""
	ret.TheaterActivationCode = ""

	// log upload token
	ret.LogUpload.Token = ""

	// s3 credentials
	ret.Storage.S3.AccessKey = ""
	ret.Storage.S3.SecretKey = ""
	ret.Storage.S3.SessionToken = ""

	// captcha secrets
	ret.Captcha.Turnstile.SecretKey = ""
	ret.Captcha.Signup.Turnstile.SecretKey = ""
	ret.Captcha.Signin.Turnstile.SecretKey = ""
	ret.Captcha.PasswordReset.Turnstile.SecretKey = ""
	ret.Audio.ImportDir = ""
	ret.Certificate.ZeroSSLAPIKey = ""
	ret.Certificate.ZeroSSLEABMACKey = ""
	for i := range ret.AI.Providers {
		ret.AI.Providers[i].APIKey = ""
	}

	return ret
}

func mergeConfigForWrite(current *utils.AppConfig, incoming *utils.AppConfig) *utils.AppConfig {
	if incoming == nil {
		if current == nil {
			return &utils.AppConfig{}
		}
		out := *current
		return &out
	}
	if current == nil {
		out := *incoming
		return &out
	}

	out := *incoming

	// Always keep server-only DSN if incoming is empty.
	if strings.TrimSpace(out.DSN) == "" {
		out.DSN = current.DSN
	}

	// Preserve secrets if incoming is empty (GET /api/v1/config is sanitized).
	if strings.TrimSpace(out.LogUpload.Token) == "" {
		out.LogUpload.Token = current.LogUpload.Token
	}
	if strings.TrimSpace(out.Storage.S3.AccessKey) == "" {
		out.Storage.S3.AccessKey = current.Storage.S3.AccessKey
	}
	if strings.TrimSpace(out.Storage.S3.SecretKey) == "" {
		out.Storage.S3.SecretKey = current.Storage.S3.SecretKey
	}
	if strings.TrimSpace(out.Storage.S3.SessionToken) == "" {
		out.Storage.S3.SessionToken = current.Storage.S3.SessionToken
	}

	if strings.TrimSpace(out.Captcha.Turnstile.SecretKey) == "" {
		out.Captcha.Turnstile.SecretKey = current.Captcha.Turnstile.SecretKey
	}
	if strings.TrimSpace(out.Captcha.Signup.Turnstile.SecretKey) == "" {
		out.Captcha.Signup.Turnstile.SecretKey = current.Captcha.Signup.Turnstile.SecretKey
	}
	if strings.TrimSpace(out.Captcha.Signin.Turnstile.SecretKey) == "" {
		out.Captcha.Signin.Turnstile.SecretKey = current.Captcha.Signin.Turnstile.SecretKey
	}
	if strings.TrimSpace(out.Captcha.PasswordReset.Turnstile.SecretKey) == "" {
		out.Captcha.PasswordReset.Turnstile.SecretKey = current.Captcha.PasswordReset.Turnstile.SecretKey
	}
	if strings.TrimSpace(out.Audio.ImportDir) == "" {
		out.Audio.ImportDir = current.Audio.ImportDir
	}
	if strings.TrimSpace(out.Proxy.ProxyHeader) == "" {
		out.Proxy.ProxyHeader = current.Proxy.ProxyHeader
	}
	if len(out.Proxy.TrustedProxies) == 0 && len(current.Proxy.TrustedProxies) > 0 {
		out.Proxy.TrustedProxies = append([]string(nil), current.Proxy.TrustedProxies...)
	}
	if strings.TrimSpace(out.Certificate.ZeroSSLAPIKey) == "" {
		out.Certificate.ZeroSSLAPIKey = current.Certificate.ZeroSSLAPIKey
	}
	if strings.TrimSpace(out.Certificate.ZeroSSLEABMACKey) == "" {
		out.Certificate.ZeroSSLEABMACKey = current.Certificate.ZeroSSLEABMACKey
	}
	if len(out.AI.Providers) == 0 && len(current.AI.Providers) > 0 {
		out.AI.Providers = append([]utils.AIProviderConfig(nil), current.AI.Providers...)
	}
	if len(out.AI.Providers) > 0 && len(current.AI.Providers) > 0 {
		currentKeys := make(map[string]string, len(current.AI.Providers))
		for _, provider := range current.AI.Providers {
			currentKeys[strings.TrimSpace(provider.ID)] = provider.APIKey
		}
		for i := range out.AI.Providers {
			if strings.TrimSpace(out.AI.Providers[i].APIKey) != "" {
				continue
			}
			id := strings.TrimSpace(out.AI.Providers[i].ID)
			if id == "" {
				continue
			}
			if apiKey, ok := currentKeys[id]; ok {
				out.AI.Providers[i].APIKey = apiKey
			}
		}
	}

	return &out
}

// mergeConfigPatchForWrite applies only JSON fields present in the request.
// This keeps server-only and newly added configuration fields when older
// clients submit a partial AppConfig snapshot.
func mergeConfigPatchForWrite(current *utils.AppConfig, raw []byte) (*utils.AppConfig, error) {
	if current == nil {
		current = &utils.AppConfig{}
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, fmt.Errorf("配置请求体必须为对象")
	}

	out := *current
	if err := applyJSONConfigPatch(reflect.ValueOf(&out).Elem(), payload); err != nil {
		return nil, err
	}
	return mergeConfigForWrite(current, &out), nil
}

func applyJSONConfigPatch(dst reflect.Value, payload map[string]json.RawMessage) error {
	typ := dst.Type()
	for i := 0; i < typ.NumField(); i++ {
		fieldType := typ.Field(i)
		field := dst.Field(i)
		if !field.CanSet() {
			continue
		}

		name := fieldType.Tag.Get("json")
		if comma := strings.IndexByte(name, ','); comma >= 0 {
			name = name[:comma]
		}
		if name == "-" {
			continue
		}
		if name == "" {
			name = fieldType.Name
		}

		rawValue, ok := payload[name]
		if !ok {
			continue
		}

		if field.Kind() == reflect.Struct && !bytes.Equal(bytes.TrimSpace(rawValue), []byte("null")) {
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(rawValue, &nested); err == nil && nested != nil {
				if err := applyJSONConfigPatch(field, nested); err != nil {
					return err
				}
				continue
			}
		}

		field.Set(reflect.Zero(field.Type()))
		if err := json.Unmarshal(rawValue, field.Addr().Interface()); err != nil {
			return fmt.Errorf("配置字段 %s 无效: %w", name, err)
		}
	}
	return nil
}
