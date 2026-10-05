package utils

import "fmt"

// Modes expand only to the explicit scopes below; new capabilities are opt-in.
type MCPConfig struct {
	Enabled              bool   `json:"enabled" yaml:"enabled"`
	Chat                 string `json:"chat" yaml:"chat"`
	Search               string `json:"search" yaml:"search"`
	BattleReport         string `json:"battleReport" yaml:"battleReport"`
	Clue                 string `json:"clue" yaml:"clue"`
	Glossary             string `json:"glossary" yaml:"glossary"`
	Identity             string `json:"identity" yaml:"identity"`
	Audio                string `json:"audio" yaml:"audio"`
	Note                 string `json:"note" yaml:"note"`
	File                 string `json:"file" yaml:"file"`
	BattleReportGenerate bool   `json:"battleReportGenerate" yaml:"battleReportGenerate"`
	CluePublish          bool   `json:"cluePublish" yaml:"cluePublish"`
	CallsPerMinute       int    `json:"callsPerMinute" yaml:"callsPerMinute"`
	WritesPerMinute      int    `json:"writesPerMinute" yaml:"writesPerMinute"`
}

type MCPScope struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Extra       bool   `json:"extra"`
}

var MCPScopeCatalog = []MCPScope{
	{"chat:read", "读取聊天记录", false}, {"search:read", "使用综合搜索", false},
	{"battle_report:read", "读取战报", false}, {"battle_report:write", "创建、编辑、删除世界共享战报", false},
	{"clue:read", "读取线索", false}, {"clue:write", "编辑线索内容", false},
	{"glossary:read", "读取世界术语", false}, {"glossary:write", "编辑世界术语", false},
	{"identity:read", "读取频道角色", false}, {"identity:write", "管理可委托的频道角色资料", false},
	{"audio:read", "读取音频工作台", false}, {"audio:write", "改变共享播放状态", false},
	{"note:read", "读取频道便签", false}, {"note:write", "编辑频道便签", false},
	{"file:write", "上传临时附件", false},
	{"battle_report:generate", "调用原生 AI，可能计费，并创建世界共享战报", true},
	{"clue:publish", "发布、揭示或隐藏线索，影响其他用户", true},
}

func NormalizeMCPConfig(c MCPConfig) MCPConfig {
	for _, p := range []*string{&c.Chat, &c.Search, &c.BattleReport, &c.Clue, &c.Glossary, &c.Identity, &c.Audio, &c.Note, &c.File} {
		if *p == "" {
			*p = "off"
		}
	}
	if c.CallsPerMinute == 0 {
		c.CallsPerMinute = 120
	}
	if c.WritesPerMinute == 0 {
		c.WritesPerMinute = 30
	}
	return c
}

func ValidateMCPConfig(c MCPConfig) error {
	c = NormalizeMCPConfig(c)
	for _, m := range []string{c.Chat, c.Search, c.BattleReport, c.Clue, c.Glossary, c.Identity, c.Audio, c.Note, c.File} {
		if m != "off" && m != "read" && m != "write" {
			return fmt.Errorf("MCP 模块必须为 off/read/write")
		}
	}
	if c.Chat == "write" || c.Search == "write" || c.File == "read" {
		return fmt.Errorf("MCP 聊天/搜索仅支持只读，文件仅支持上传")
	}
	if c.CallsPerMinute < 1 || c.CallsPerMinute > 10000 || c.WritesPerMinute < 1 || c.WritesPerMinute > c.CallsPerMinute {
		return fmt.Errorf("MCP 限流数值无效")
	}
	return nil
}

func (c MCPConfig) AllowedScopes() []string {
	if !c.Enabled {
		return []string{}
	}
	ret := []string{}
	for _, m := range []struct{ mode, read, write string }{
		{c.Chat, "chat:read", ""}, {c.Search, "search:read", ""}, {c.BattleReport, "battle_report:read", "battle_report:write"},
		{c.Clue, "clue:read", "clue:write"}, {c.Glossary, "glossary:read", "glossary:write"}, {c.Identity, "identity:read", "identity:write"},
		{c.Audio, "audio:read", "audio:write"}, {c.Note, "note:read", "note:write"}, {c.File, "", "file:write"},
	} {
		if (m.mode == "read" || m.mode == "write") && m.read != "" {
			ret = append(ret, m.read)
		}
		if m.mode == "write" && m.write != "" {
			ret = append(ret, m.write)
		}
	}
	if c.BattleReportGenerate && c.BattleReport == "write" {
		ret = append(ret, "battle_report:generate")
	}
	if c.CluePublish && c.Clue == "write" {
		ret = append(ret, "clue:publish")
	}
	return ret
}
