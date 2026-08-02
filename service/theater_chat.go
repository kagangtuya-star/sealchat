package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	theaterActionChatRandomTable    = "chat.random-table"
	theaterRandomTableMaxDiceCount  = 100
	theaterRandomTableMaxDiceSides  = 100_000
	theaterRandomTableMaxModifier   = 1_000_000
	theaterRandomTableMaxEntries    = 1_000
	theaterRandomTableMaxTextLength = 10_000
	theaterRandomTableMaxNameLength = 128
)

var theaterSimpleDiceFormulaPattern = regexp.MustCompile(`(?i)^([1-9][0-9]*)d([1-9][0-9]*)(?:([+-])([0-9]+))?$`)

type TheaterChatSendRequest struct {
	ActorID           string
	WorldID           string
	ChannelID         string
	ClientID          string
	Content           string
	IdentityID        string
	IdentityVariantID string
	ICMode            string
}

type TheaterChatSendResult struct {
	MessageID string `json:"messageId"`
}

type TheaterChatSender interface {
	SendTheaterChat(context.Context, TheaterChatSendRequest) (*TheaterChatSendResult, error)
}

var theaterChatSenderState = struct {
	sync.RWMutex
	sender TheaterChatSender
}{}

func SetTheaterChatSender(sender TheaterChatSender) {
	theaterChatSenderState.Lock()
	theaterChatSenderState.sender = sender
	theaterChatSenderState.Unlock()
}

func getTheaterChatSender() TheaterChatSender {
	theaterChatSenderState.RLock()
	defer theaterChatSenderState.RUnlock()
	return theaterChatSenderState.sender
}

type theaterChatSendPayload struct {
	Content           string `json:"content"`
	ChannelID         string `json:"channelId"`
	IdentityID        string `json:"identityId"`
	IdentityVariantID string `json:"identityVariantId"`
	ICMode            string `json:"icMode"`
}

type theaterRandomTableEntry struct {
	Min  int    `json:"min"`
	Max  int    `json:"max"`
	Text string `json:"text"`
}

type theaterRandomTablePayload struct {
	Name    string                    `json:"name"`
	Formula string                    `json:"formula"`
	Entries []theaterRandomTableEntry `json:"entries"`
}

func parseTheaterSimpleDiceFormula(formula string) (count, sides, modifier int, err error) {
	formula = strings.TrimSpace(formula)
	matches := theaterSimpleDiceFormulaPattern.FindStringSubmatch(formula)
	if matches == nil {
		return 0, 0, 0, theaterPayloadError("chat.random-table formula 无效")
	}
	count64, countErr := strconv.ParseInt(matches[1], 10, 32)
	sides64, sidesErr := strconv.ParseInt(matches[2], 10, 32)
	if countErr != nil || sidesErr != nil || count64 > theaterRandomTableMaxDiceCount || sides64 > theaterRandomTableMaxDiceSides {
		return 0, 0, 0, theaterPayloadError("chat.random-table formula 超出限制")
	}
	modifier64 := int64(0)
	if matches[4] != "" {
		modifier64, err = strconv.ParseInt(matches[4], 10, 32)
		if err != nil || modifier64 > theaterRandomTableMaxModifier {
			return 0, 0, 0, theaterPayloadError("chat.random-table formula 超出限制")
		}
		if matches[3] == "-" {
			modifier64 = -modifier64
		}
	}
	return int(count64), int(sides64), int(modifier64), nil
}

func normalizeTheaterRandomTablePayload(payload theaterRandomTablePayload) (theaterRandomTablePayload, error) {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Formula = strings.TrimSpace(payload.Formula)
	if payload.Name == "" || utf8.RuneCountInString(payload.Name) > theaterRandomTableMaxNameLength {
		return payload, theaterPayloadError("chat.random-table name 无效")
	}
	count, sides, modifier, err := parseTheaterSimpleDiceFormula(payload.Formula)
	if err != nil {
		return payload, err
	}
	if len(payload.Entries) == 0 || len(payload.Entries) > theaterRandomTableMaxEntries {
		return payload, theaterPayloadError("chat.random-table entries 数量无效")
	}
	totalTextLength := 0
	ordered := make([]theaterRandomTableEntry, len(payload.Entries))
	for index := range payload.Entries {
		payload.Entries[index].Text = strings.TrimSpace(payload.Entries[index].Text)
		entry := payload.Entries[index]
		if entry.Min > entry.Max {
			return payload, theaterPayloadError("chat.random-table 区间倒置")
		}
		if entry.Text == "" {
			return payload, theaterPayloadError("chat.random-table 结果为空")
		}
		totalTextLength += utf8.RuneCountInString(entry.Text)
		if totalTextLength > theaterRandomTableMaxTextLength {
			return payload, theaterPayloadError("chat.random-table 文本超出限制")
		}
		ordered[index] = entry
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Min == ordered[j].Min {
			return ordered[i].Max < ordered[j].Max
		}
		return ordered[i].Min < ordered[j].Min
	})
	for index := 1; index < len(ordered); index++ {
		if ordered[index].Min <= ordered[index-1].Max {
			return payload, theaterPayloadError("chat.random-table 区间重叠")
		}
	}
	minimumRoll := count + modifier
	maximumRoll := count*sides + modifier
	coveredThrough := minimumRoll - 1
	for _, entry := range ordered {
		if entry.Max < minimumRoll {
			continue
		}
		if entry.Min > coveredThrough+1 {
			break
		}
		coveredThrough = max(coveredThrough, entry.Max)
		if coveredThrough >= maximumRoll {
			break
		}
	}
	if coveredThrough < maximumRoll {
		return payload, theaterPayloadError("chat.random-table 未完整覆盖骰值范围")
	}
	return payload, nil
}

func normalizeTheaterChatSendPayload(payload theaterChatSendPayload) (theaterChatSendPayload, error) {
	payload.Content = strings.TrimSpace(payload.Content)
	payload.ChannelID = strings.TrimSpace(payload.ChannelID)
	payload.IdentityID = strings.TrimSpace(payload.IdentityID)
	payload.IdentityVariantID = strings.TrimSpace(payload.IdentityVariantID)
	payload.ICMode = strings.ToLower(strings.TrimSpace(payload.ICMode))
	if payload.Content == "" || len(payload.Content) > 10000 {
		return payload, theaterPayloadError("chat.send content 无效")
	}
	if payload.ICMode == "" {
		payload.ICMode = "ic"
	}
	if payload.ICMode != "ic" && payload.ICMode != "ooc" {
		return payload, theaterPayloadError("chat.send icMode 无效")
	}
	if err := rejectUnsafeTheaterJSON(payload.Content); err != nil {
		return payload, err
	}
	return payload, nil
}

func sendTheaterChat(ctx context.Context, actorID, worldID, channelID, actionRequestID string, raw []byte) (*TheaterChatSendResult, error) {
	var payload theaterChatSendPayload
	if err := decodeStrictJSON(raw, &payload); err != nil {
		return nil, theaterPayloadError(err.Error())
	}
	payload, err := normalizeTheaterChatSendPayload(payload)
	if err != nil {
		return nil, err
	}
	if payload.ChannelID != "" {
		channelID = payload.ChannelID
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, theaterPayloadError("chat.send 缺少 inputChannelId")
	}
	if _, _, err := resolveTheaterScope(worldID, channelID); err != nil {
		return nil, err
	}
	sender := getTheaterChatSender()
	if sender == nil {
		return nil, errors.New("theater chat sender unavailable")
	}
	return sender.SendTheaterChat(ctx, TheaterChatSendRequest{
		ActorID: actorID, WorldID: worldID, ChannelID: channelID, ClientID: "theater:" + actionRequestID,
		Content: payload.Content, IdentityID: payload.IdentityID, IdentityVariantID: payload.IdentityVariantID, ICMode: payload.ICMode,
	})
}

func evaluateTheaterRandomTableFormula(formula string) (int, error) {
	roll := evaluateDiceFormula(formula, "")
	if roll.IsError {
		return 0, theaterPayloadError("chat.random-table 掷骰失败: " + roll.ResultText)
	}
	value, err := strconv.Atoi(strings.TrimSpace(roll.ResultValueText))
	if err != nil {
		return 0, theaterPayloadError("chat.random-table 掷骰结果不是整数")
	}
	return value, nil
}

func sendTheaterRandomTable(ctx context.Context, actorID, worldID, channelID, actionRequestID string, raw []byte) (*TheaterChatSendResult, error) {
	return sendTheaterRandomTableWithRoller(ctx, actorID, worldID, channelID, actionRequestID, raw, evaluateTheaterRandomTableFormula)
}

func sendTheaterRandomTableWithRoller(ctx context.Context, actorID, worldID, channelID, actionRequestID string, raw []byte, roller func(string) (int, error)) (*TheaterChatSendResult, error) {
	var payload theaterRandomTablePayload
	if err := decodeStrictJSON(raw, &payload); err != nil {
		return nil, theaterPayloadError("chat.random-table action payload 无效")
	}
	payload, err := normalizeTheaterRandomTablePayload(payload)
	if err != nil {
		return nil, err
	}
	result, err := roller(payload.Formula)
	if err != nil {
		return nil, err
	}
	var matched *theaterRandomTableEntry
	for index := range payload.Entries {
		entry := &payload.Entries[index]
		if result >= entry.Min && result <= entry.Max {
			matched = entry
			break
		}
	}
	if matched == nil {
		return nil, theaterPayloadError(fmt.Sprintf("chat.random-table 掷骰结果 %d 无匹配条目", result))
	}
	message, _ := json.Marshal(theaterChatSendPayload{
		Content: fmt.Sprintf("%s\n%s = %d\n%s", payload.Name, payload.Formula, result, matched.Text),
	})
	return sendTheaterChat(ctx, actorID, worldID, channelID, actionRequestID, message)
}
