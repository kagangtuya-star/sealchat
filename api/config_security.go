package api

import (
	"strings"

	"sealchat/utils"
)

func sanitizeConfigForClient(cfg *utils.AppConfig) utils.AppConfig {
	if cfg == nil {
		return utils.AppConfig{}
	}
	ret := *cfg

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
	ret.Audio.ImportDir = ""

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
	if strings.TrimSpace(out.Audio.ImportDir) == "" {
		out.Audio.ImportDir = current.Audio.ImportDir
	}

	return &out
}
