package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSONStringSlice 提供轻量的 JSON 数组字段映射，兼容 sqlite / mysql / postgres。
type JSONStringSlice []string

func (s JSONStringSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	data, err := json.Marshal([]string(s))
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (s *JSONStringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("unsupported JSONStringSlice source type: %T", value)
	}
	if len(raw) == 0 {
		*s = JSONStringSlice{}
		return nil
	}
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return err
	}
	*s = items
	return nil
}
