package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"sealchat/utils"
)

type AudioRefSource string

const (
	AudioRefSourceDatabase AudioRefSource = "database"
	AudioRefSourceS3       AudioRefSource = "s3"
)

type AudioRef struct {
	Source    AudioRefSource
	SourceID  string
	ObjectKey string
	IsPrefix  bool
}

func BuildS3AudioRef(sourceID, objectKey string) (string, error) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" || objectKey == "" {
		return "", errors.New("audio S3 ref 参数为空")
	}
	if err := validateS3ObjectKey(objectKey); err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString([]byte(objectKey))
	return fmt.Sprintf("aud:s3:v1:%s:%s", sourceID, encoded), nil
}

func BuildS3AudioPrefixRef(sourceID, prefix string) (string, error) {
	if strings.TrimSpace(sourceID) == "" {
		return "", errors.New("audio S3 source id 为空")
	}
	normalized, err := NormalizeAudioLibraryPrefix(prefix)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString([]byte(normalized))
	return fmt.Sprintf("aud:s3p:v1:%s:%s", strings.TrimSpace(sourceID), encoded), nil
}

func ParseAudioRef(raw string) (AudioRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return AudioRef{}, errors.New("audio ref 为空")
	}
	if !strings.HasPrefix(raw, "aud:") {
		return AudioRef{Source: AudioRefSourceDatabase, ObjectKey: raw}, nil
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 5 || parts[0] != "aud" || parts[2] != "v1" {
		return AudioRef{}, errors.New("audio ref 格式无效")
	}
	switch parts[1] {
	case "s3":
		decoded, err := base64.RawURLEncoding.DecodeString(parts[4])
		if err != nil || len(decoded) == 0 {
			return AudioRef{}, errors.New("audio S3 ref 编码无效")
		}
		objectKey := string(decoded)
		if err := validateS3ObjectKey(objectKey); err != nil {
			return AudioRef{}, err
		}
		return AudioRef{Source: AudioRefSourceS3, SourceID: parts[3], ObjectKey: objectKey}, nil
	case "s3p":
		decoded, err := base64.RawURLEncoding.DecodeString(parts[4])
		if err != nil {
			return AudioRef{}, errors.New("audio S3 prefix ref 编码无效")
		}
		return AudioRef{Source: AudioRefSourceS3, SourceID: parts[3], ObjectKey: string(decoded), IsPrefix: true}, nil
	default:
		return AudioRef{}, errors.New("audio ref source 不支持")
	}
}

func validateS3ObjectKey(objectKey string) error {
	if objectKey == "" || !utf8.ValidString(objectKey) || strings.Contains(objectKey, "\\") {
		return errors.New("audio S3 object key 无效")
	}
	lower := strings.ToLower(objectKey)
	if strings.Contains(lower, "%2e") || strings.Contains(lower, "%00") {
		return errors.New("audio S3 object key 包含编码路径穿越")
	}
	for _, part := range strings.Split(objectKey, "/") {
		if part == "" || part == "." || part == ".." {
			return errors.New("audio S3 object key 包含非法路径段")
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return errors.New("audio S3 object key 包含控制字符")
			}
		}
	}
	return nil
}

func ResolveAudioRef(raw string) (AudioRef, error) {
	worldID, err := ResolveAudioLibraryWorldID("")
	if err != nil {
		return AudioRef{}, err
	}
	return ResolveAudioRefForWorld(raw, worldID)
}

func ResolveAudioRefForWorld(raw, worldID string) (AudioRef, error) {
	ref, err := ParseAudioRef(raw)
	if err != nil {
		return AudioRef{}, err
	}
	if ref.Source == AudioRefSourceDatabase {
		return ref, nil
	}
	cfg := utils.GetConfig()
	if cfg == nil {
		return AudioRef{}, errors.New("配置未初始化")
	}
	library := configuredAudioLibrary(cfg, worldID)
	if ref.SourceID != library.S3SourceID {
		return AudioRef{}, errors.New("audio ref 不属于当前 S3 source")
	}
	if ref.IsPrefix {
		prefix, normalizeErr := NormalizeAudioLibraryPrefix(ref.ObjectKey)
		if normalizeErr != nil || (prefix != "" && !isWithinAudioPrefix(library.S3Prefix, prefix)) || (prefix == "" && library.S3Prefix != "") {
			return AudioRef{}, errors.New("audio ref 不属于当前 S3 source")
		}
		ref.ObjectKey = prefix
		return ref, nil
	}
	if !isWithinAudioPrefix(library.S3Prefix, ref.ObjectKey) {
		return AudioRef{}, errors.New("audio ref 不属于当前 S3 source")
	}
	return ref, nil
}
