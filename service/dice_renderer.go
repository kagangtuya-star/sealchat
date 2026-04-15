package service

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"

	ds "github.com/sealdice/dicescript"
	htmlparser "golang.org/x/net/html"
	htmlatom "golang.org/x/net/html/atom"

	"sealchat/model"
)

var (
	diceCommandPattern    = regexp.MustCompile(`(?i)(?:[\.。．｡]rh?[^\s　,，。！？!?;；:：]*)`)
	diceBracePattern      = regexp.MustCompile(`\{([^{}]+)\}`)
	incompleteDicePattern = regexp.MustCompile(`(?i)(\b\d*)d\b`)
	hiddenDicePattern     = regexp.MustCompile(`(?i)[\.。．｡]rh`)
	multiDicePattern      = regexp.MustCompile(`^\s*(\d+)\s*#\s*(.*)$`)
	replayDetailPattern   = regexp.MustCompile(`\[(\d+)d(\d+)=(\d+(?:\+\d+)*)\]`)
	replayFormulaPattern  = regexp.MustCompile(`^\s*(\d+)d(\d+)(?:([+-])(\d+))?\s*$`)
)

const (
	defaultDiceExprFallback = "d20"
	diceIconSVG             = `<span class="dice-chip__icon" aria-hidden="true">🎲</span>`
	maxMultiDiceCount       = 100
)

// DiceRenderResult 处理后的内容
type DiceRenderResult struct {
	Content  string
	Rolls    []*model.MessageDiceRollModel
	IsHidden bool // 是否为暗骰 (.rh 命令)
}

// LooksLikeTipTapJSON 判断内容是否为富文本payload，避免服务器端直接解析
func LooksLikeTipTapJSON(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || trimmed[0] != '{' {
		return false
	}
	return strings.Contains(trimmed, `"type":"doc"`)
}

// NormalizeDefaultDiceExpr 规范化频道默认骰配置
func NormalizeDefaultDiceExpr(raw string) (string, error) {
	candidate := strings.ToLower(strings.TrimSpace(raw))
	if candidate == "" {
		return defaultDiceExprFallback, nil
	}
	if !strings.HasPrefix(candidate, "d") {
		candidate = "d" + candidate
	}
	sidesPart := candidate[1:]
	if sidesPart == "" {
		return "", errors.New("默认骰面数不能为空")
	}
	value, err := strconv.Atoi(sidesPart)
	if err != nil || value <= 0 {
		return "", errors.New("默认骰需为正整数")
	}
	if value > 100000 {
		return "", errors.New("默认骰面数过大")
	}
	return fmt.Sprintf("d%d", value), nil
}

// RenderDiceContent 在HTML字符串中识别骰子表达式并渲染为dice-chip
func RenderDiceContent(content string, defaultDiceExpr string, existing []*model.MessageDiceRollModel) (*DiceRenderResult, error) {
	if LooksLikeTipTapJSON(content) {
		return &DiceRenderResult{Content: content, Rolls: nil, IsHidden: false}, nil
	}
	wrapper := &htmlparser.Node{Type: htmlparser.ElementNode, DataAtom: htmlatom.Div, Data: "div"}
	nodes, err := htmlparser.ParseFragment(strings.NewReader(content), wrapper)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		wrapper.AppendChild(node)
	}
	renderer := newDiceRenderer(defaultDiceExpr, existing)
	renderer.walk(wrapper)
	isHidden := containsHiddenDiceCommand(content)

	if !renderer.modified {
		return &DiceRenderResult{Content: content, Rolls: renderer.rolls, IsHidden: isHidden}, nil
	}

	var buf bytes.Buffer
	for child := wrapper.FirstChild; child != nil; child = child.NextSibling {
		if err := htmlparser.Render(&buf, child); err != nil {
			return nil, err
		}
	}
	return &DiceRenderResult{Content: buf.String(), Rolls: renderer.rolls, IsHidden: isHidden}, nil
}

func RenderDiceContentWithPreviousMessage(content string, defaultDiceExpr string, previousContent string, cacheKey string, rollMore func(string) []int) (*DiceRenderResult, error) {
	snapshot, err := loadDiceReplaySnapshot(previousContent, cacheKey)
	if err != nil {
		return nil, err
	}
	if LooksLikeTipTapJSON(content) {
		return &DiceRenderResult{Content: content, Rolls: nil, IsHidden: false}, nil
	}
	wrapper := &htmlparser.Node{Type: htmlparser.ElementNode, DataAtom: htmlatom.Div, Data: "div"}
	nodes, err := htmlparser.ParseFragment(strings.NewReader(content), wrapper)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		wrapper.AppendChild(node)
	}
	renderer := newDiceReplayRenderer(defaultDiceExpr, snapshot, rollMore)
	renderer.walk(wrapper)
	isHidden := containsHiddenDiceCommand(content)

	if !renderer.modified {
		return &DiceRenderResult{Content: content, Rolls: renderer.rolls, IsHidden: isHidden}, nil
	}

	var buf bytes.Buffer
	for child := wrapper.FirstChild; child != nil; child = child.NextSibling {
		if err := htmlparser.Render(&buf, child); err != nil {
			return nil, err
		}
	}
	return &DiceRenderResult{Content: buf.String(), Rolls: renderer.rolls, IsHidden: isHidden}, nil
}

func newDiceRenderer(defaultDiceExpr string, existing []*model.MessageDiceRollModel) *diceRenderer {
	normalized, err := NormalizeDefaultDiceExpr(defaultDiceExpr)
	if err != nil || normalized == "" {
		normalized = defaultDiceExprFallback
	}
	sides := ""
	if len(normalized) > 1 {
		sides = normalized[1:]
	}
	existingMap := map[string]*model.MessageDiceRollModel{}
	for _, roll := range existing {
		if roll == nil {
			continue
		}
		key := fmt.Sprintf("%d|%s", roll.RollIndex, strings.ToLower(strings.TrimSpace(roll.Formula)))
		existingMap[key] = roll
	}
	return &diceRenderer{
		defaultDiceExpr:  normalized,
		defaultDiceSides: sides,
		existing:         existingMap,
		replayEntries:    map[int]DiceReplayEntry{},
		rolls:            []*model.MessageDiceRollModel{},
	}
}

func newDiceReplayRenderer(defaultDiceExpr string, snapshot *DiceReplaySnapshot, rollMore func(string) []int) *diceRenderer {
	renderer := newDiceRenderer(defaultDiceExpr, nil)
	if snapshot != nil {
		for _, entry := range snapshot.Entries {
			renderer.replayEntries[entry.RollIndex] = entry
		}
	}
	renderer.rollMore = rollMore
	return renderer
}

type diceRenderer struct {
	defaultDiceExpr  string
	defaultDiceSides string
	existing         map[string]*model.MessageDiceRollModel
	replayEntries    map[int]DiceReplayEntry
	rollMore         func(string) []int
	rolls            []*model.MessageDiceRollModel
	modified         bool
}

func (r *diceRenderer) walk(node *htmlparser.Node) {
	if node.Type == htmlparser.ElementNode {
		if strings.EqualFold(node.Data, "script") || strings.EqualFold(node.Data, "style") {
			return
		}
		if hasDiceChipClass(node) {
			return
		}
	}
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == htmlparser.TextNode {
			if r.processTextNode(child) {
				r.modified = true
			}
		} else {
			r.walk(child)
		}
		child = next
	}
}

func hasDiceChipClass(node *htmlparser.Node) bool {
	if node.Type != htmlparser.ElementNode {
		return false
	}
	for _, attr := range node.Attr {
		if attr.Key == "class" && strings.Contains(attr.Val, "dice-chip") {
			return true
		}
	}
	return false
}

func (r *diceRenderer) processTextNode(node *htmlparser.Node) bool {
	text := node.Data
	matches := findDiceMatches(text)
	if len(matches) == 0 {
		return false
	}
	parent := node.Parent
	if parent == nil {
		return false
	}
	cursor := 0
	for _, match := range matches {
		if match.start > cursor {
			before := &htmlparser.Node{Type: htmlparser.TextNode, Data: text[cursor:match.start]}
			parent.InsertBefore(before, node)
		}
		rolls := r.buildRolls(match)
		chipHTML := buildDiceRenderedHTML(match.raw, rolls)
		fragment, err := htmlparser.ParseFragment(strings.NewReader(chipHTML), parent)
		if err != nil {
			// 插入失败时降级为原文本
			parent.InsertBefore(&htmlparser.Node{Type: htmlparser.TextNode, Data: match.raw}, node)
		} else {
			for _, frag := range fragment {
				parent.InsertBefore(frag, node)
			}
		}
		cursor = match.end
	}
	if cursor < len(text) {
		parent.InsertBefore(&htmlparser.Node{Type: htmlparser.TextNode, Data: text[cursor:]}, node)
	}
	parent.RemoveChild(node)
	return true
}

type diceTextMatch struct {
	start int
	end   int
	raw   string
	inner string
	kind  string
}

const (
	matchKindBrace   = "brace"
	matchKindCommand = "command"
)

func findDiceMatches(text string) []diceTextMatch {
	var matches []diceTextMatch
	occupied := make([]bool, len(text))

	addMatch := func(start, end int, raw, inner, kind string) {
		matches = append(matches, diceTextMatch{start: start, end: end, raw: raw, inner: inner, kind: kind})
		for i := start; i < end && i < len(occupied); i++ {
			occupied[i] = true
		}
	}

	braceLoc := diceBracePattern.FindAllStringSubmatchIndex(text, -1)
	for _, loc := range braceLoc {
		if len(loc) < 4 {
			continue
		}
		start, end := loc[0], loc[1]
		innerStart, innerEnd := loc[2], loc[3]
		if start == end {
			continue
		}
		addMatch(start, end, text[start:end], text[innerStart:innerEnd], matchKindBrace)
	}

	commandLoc := diceCommandPattern.FindAllStringIndex(text, -1)
	for _, loc := range commandLoc {
		start, end := loc[0], loc[1]
		if start == end || overlaps(occupied, start, end) {
			continue
		}
		addMatch(start, end, text[start:end], text[start:end], matchKindCommand)
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].start < matches[j].start
	})
	return matches
}

func overlaps(used []bool, start, end int) bool {
	if start < 0 {
		start = 0
	}
	if end > len(used) {
		end = len(used)
	}
	for i := start; i < end; i++ {
		if used[i] {
			return true
		}
	}
	return false
}

func (r *diceRenderer) buildRolls(match diceTextMatch) []*model.MessageDiceRollModel {
	normalized, err := r.normalizeFormula(match)
	if err != nil || normalized == "" {
		roll := r.buildErrorRoll(strings.TrimSpace(match.raw), normalized, err)
		r.rolls = append(r.rolls, roll)
		return []*model.MessageDiceRollModel{roll}
	}

	repeatCount, formula, multiErr := r.parseMultiRoll(normalized)
	if multiErr != nil {
		roll := r.buildErrorRoll(strings.TrimSpace(match.raw), normalized, multiErr)
		r.rolls = append(r.rolls, roll)
		return []*model.MessageDiceRollModel{roll}
	}
	if repeatCount <= 1 {
		roll := r.buildSingleRoll(strings.TrimSpace(match.raw), formula)
		r.rolls = append(r.rolls, roll)
		return []*model.MessageDiceRollModel{roll}
	}

	rolls := make([]*model.MessageDiceRollModel, 0, repeatCount)
	for i := 0; i < repeatCount; i++ {
		roll := r.buildSingleRoll(strings.TrimSpace(match.raw), formula)
		r.rolls = append(r.rolls, roll)
		rolls = append(rolls, roll)
	}
	return rolls
}

func (r *diceRenderer) buildErrorRoll(sourceText string, formula string, err error) *model.MessageDiceRollModel {
	index := len(r.rolls)
	roll := &model.MessageDiceRollModel{
		RollIndex:  index,
		SourceText: sourceText,
		Formula:    formula,
	}
	roll.IsError = true
	if err != nil {
		roll.ResultText = err.Error()
	} else {
		roll.ResultText = "表达式为空"
	}
	return roll
}

func (r *diceRenderer) buildSingleRoll(sourceText string, formula string) *model.MessageDiceRollModel {
	index := len(r.rolls)
	roll := &model.MessageDiceRollModel{
		RollIndex:  index,
		SourceText: sourceText,
		Formula:    formula,
	}
	key := fmt.Sprintf("%d|%s", index, strings.ToLower(strings.TrimSpace(formula)))
	if prev, ok := r.existing[key]; ok {
		roll.ResultDetail = prev.ResultDetail
		roll.ResultValueText = prev.ResultValueText
		roll.ResultText = prev.ResultText
		roll.IsError = prev.IsError
		return roll
	}
	if replayed, ok := r.tryReplayRoll(index, sourceText, formula); ok {
		return replayed
	}
	computed := r.evaluateFormula(formula)
	roll.ResultDetail = computed.ResultDetail
	roll.ResultValueText = computed.ResultValueText
	roll.ResultText = computed.ResultText
	roll.IsError = computed.IsError
	return roll
}

func (r *diceRenderer) parseMultiRoll(normalized string) (int, string, error) {
	trimmed := strings.TrimSpace(normalized)
	groups := multiDicePattern.FindStringSubmatch(trimmed)
	if len(groups) != 3 {
		return 1, trimmed, nil
	}
	repeatCount, err := strconv.Atoi(groups[1])
	if err != nil {
		return 0, trimmed, errors.New("多重掷骰次数无效")
	}
	if repeatCount <= 0 || repeatCount > maxMultiDiceCount {
		return 0, trimmed, fmt.Errorf("多重掷骰次数需为 1-%d", maxMultiDiceCount)
	}
	formula := strings.TrimSpace(groups[2])
	if formula == "" {
		formula = r.defaultDiceExpr
	}
	return repeatCount, formula, nil
}

func (r *diceRenderer) normalizeFormula(match diceTextMatch) (string, error) {
	candidate := match.inner
	if match.kind == matchKindCommand {
		candidate = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(candidate), "."))
		candidate = strings.TrimPrefix(candidate, "。")
		candidate = strings.TrimPrefix(candidate, "．")
		candidate = strings.TrimPrefix(candidate, "｡")
		candidate = strings.TrimSpace(candidate)
		if len(candidate) >= 2 && strings.HasPrefix(candidate, "rh") {
			candidate = strings.TrimSpace(candidate[2:])
			candidate = strings.TrimLeft(candidate, "/ \t\n\r　、，。")
		} else if len(candidate) >= 1 && (candidate[0] == 'r') {
			candidate = strings.TrimSpace(candidate[1:])
			// 去掉 r 后，如果开头是非字母数字字符（如 /、空格等分隔符），继续清理
			// 这样 .r/ 或 .r  都会变成空字符串，从而使用默认骰
			candidate = strings.TrimLeft(candidate, "/ \t\n\r　、，。")
		}
	}
	normalized := strings.TrimSpace(candidate)
	if normalized == "" {
		normalized = r.defaultDiceExpr
	}
	normalized = strings.ToLower(normalized)
	normalized = strings.ReplaceAll(normalized, "×", "*")
	normalized = strings.ReplaceAll(normalized, "·", "*")
	normalized = strings.ReplaceAll(normalized, "x", "*")
	normalized = strings.ReplaceAll(normalized, "，", ",")
	normalized = strings.ReplaceAll(normalized, "（", "(")
	normalized = strings.ReplaceAll(normalized, "）", ")")
	normalized = incompleteDicePattern.ReplaceAllStringFunc(normalized, func(token string) string {
		if r.defaultDiceSides == "" {
			return token
		}
		if strings.HasSuffix(strings.ToLower(token), "d") {
			return token + r.defaultDiceSides
		}
		return token
	})
	if normalized == "r" || normalized == "rd" {
		normalized = r.defaultDiceExpr
	}
	return normalized, nil
}

func loadDiceReplaySnapshot(previousContent string, cacheKey string) (*DiceReplaySnapshot, error) {
	if cacheKey != "" {
		if cached, ok := globalDiceReplayCache.Get(cacheKey); ok {
			return cached, nil
		}
	}
	snapshot, err := ParseDiceReplaySnapshotFromContent(previousContent)
	if err != nil {
		return nil, err
	}
	if cacheKey != "" && snapshot != nil {
		globalDiceReplayCache.Set(cacheKey, snapshot)
	}
	return snapshot, nil
}

func ParseDiceReplaySnapshotFromContent(content string) (*DiceReplaySnapshot, error) {
	if strings.TrimSpace(content) == "" || !strings.Contains(content, "dice-chip") {
		return &DiceReplaySnapshot{}, nil
	}
	wrapper := &htmlparser.Node{Type: htmlparser.ElementNode, DataAtom: htmlatom.Div, Data: "div"}
	nodes, err := htmlparser.ParseFragment(strings.NewReader(content), wrapper)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		wrapper.AppendChild(node)
	}
	entries := make([]DiceReplayEntry, 0)
	var walk func(node *htmlparser.Node)
	walk = func(node *htmlparser.Node) {
		if node == nil {
			return
		}
		if entry, ok := parseDiceReplayEntry(node); ok {
			entries = append(entries, entry)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(wrapper)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].RollIndex < entries[j].RollIndex
	})
	return &DiceReplaySnapshot{Entries: entries}, nil
}

func parseDiceReplayEntry(node *htmlparser.Node) (DiceReplayEntry, bool) {
	if node == nil || node.Type != htmlparser.ElementNode || !strings.EqualFold(node.Data, "span") {
		return DiceReplayEntry{}, false
	}
	className := ""
	entry := DiceReplayEntry{RollIndex: -1}
	for _, attr := range node.Attr {
		key := strings.ToLower(strings.TrimSpace(attr.Key))
		switch key {
		case "class":
			className = attr.Val
		case "data-dice-roll-index":
			if value, err := strconv.Atoi(strings.TrimSpace(attr.Val)); err == nil {
				entry.RollIndex = value
			}
		case "data-dice-source":
			entry.SourceText = html.UnescapeString(attr.Val)
		case "data-dice-formula":
			entry.Formula = html.UnescapeString(attr.Val)
		case "data-dice-result-detail":
			entry.DetailText = html.UnescapeString(attr.Val)
		case "data-dice-result-value":
			entry.ValueText = html.UnescapeString(attr.Val)
		case "data-dice-result-text":
			entry.ResultText = html.UnescapeString(attr.Val)
		}
	}
	if !strings.Contains(className, "dice-chip") || entry.RollIndex < 0 || strings.TrimSpace(entry.Formula) == "" {
		return DiceReplayEntry{}, false
	}
	entry.Samples = parseReplaySamples(entry.DetailText)
	return entry, true
}

func parseReplaySamples(detail string) []DiceReplaySample {
	matches := replayDetailPattern.FindStringSubmatch(strings.TrimSpace(detail))
	if len(matches) != 4 {
		return nil
	}
	parts := strings.Split(matches[3], "+")
	samples := make([]DiceReplaySample, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil
		}
		samples = append(samples, DiceReplaySample{Value: value})
	}
	return samples
}

type replayableFormula struct {
	DiceCount int
	DiceSides int
	Modifier  int
}

func parseReplayableFormula(expr string) (replayableFormula, bool) {
	matches := replayFormulaPattern.FindStringSubmatch(strings.TrimSpace(expr))
	if len(matches) != 5 {
		return replayableFormula{}, false
	}
	diceCount, err := strconv.Atoi(matches[1])
	if err != nil || diceCount <= 0 {
		return replayableFormula{}, false
	}
	diceSides, err := strconv.Atoi(matches[2])
	if err != nil || diceSides <= 0 {
		return replayableFormula{}, false
	}
	modifier := 0
	if matches[4] != "" {
		value, err := strconv.Atoi(matches[4])
		if err != nil {
			return replayableFormula{}, false
		}
		modifier = value
		if matches[3] == "-" {
			modifier = -modifier
		}
	}
	return replayableFormula{DiceCount: diceCount, DiceSides: diceSides, Modifier: modifier}, true
}

func formatReplayableFormula(item replayableFormula) string {
	base := fmt.Sprintf("%dd%d", item.DiceCount, item.DiceSides)
	switch {
	case item.Modifier > 0:
		return fmt.Sprintf("%s+%d", base, item.Modifier)
	case item.Modifier < 0:
		return fmt.Sprintf("%s%d", base, item.Modifier)
	default:
		return base
	}
}

func (r *diceRenderer) tryReplayRoll(index int, sourceText string, formula string) (*model.MessageDiceRollModel, bool) {
	entry, ok := r.replayEntries[index]
	if !ok {
		return nil, false
	}
	oldFormula, ok := parseReplayableFormula(entry.Formula)
	if !ok || len(entry.Samples) < oldFormula.DiceCount {
		return nil, false
	}
	newFormula, ok := parseReplayableFormula(formula)
	if !ok || oldFormula.DiceSides != newFormula.DiceSides {
		return nil, false
	}
	values := make([]int, 0, newFormula.DiceCount)
	reuseCount := oldFormula.DiceCount
	if newFormula.DiceCount < reuseCount {
		reuseCount = newFormula.DiceCount
	}
	for i := 0; i < reuseCount; i++ {
		values = append(values, entry.Samples[i].Value)
	}
	if newFormula.DiceCount > len(values) {
		appended, ok := r.rollAdditionalDice(newFormula.DiceCount-len(values), newFormula.DiceSides)
		if !ok {
			return nil, false
		}
		values = append(values, appended...)
	}
	total := newFormula.Modifier
	baseTotal := 0
	valueParts := make([]string, 0, len(values))
	for _, value := range values {
		baseTotal += value
		total += value
		valueParts = append(valueParts, strconv.Itoa(value))
	}
	detailText := fmt.Sprintf("%d[%dd%d=%s]", baseTotal, newFormula.DiceCount, newFormula.DiceSides, strings.Join(valueParts, "+"))
	switch {
	case newFormula.Modifier > 0:
		detailText = fmt.Sprintf("%s+%d", detailText, newFormula.Modifier)
	case newFormula.Modifier < 0:
		detailText = fmt.Sprintf("%s%d", detailText, newFormula.Modifier)
	}
	roll := &model.MessageDiceRollModel{
		RollIndex:       index,
		SourceText:      sourceText,
		Formula:         formatReplayableFormula(newFormula),
		ResultDetail:    detailText,
		ResultValueText: strconv.Itoa(total),
		ResultText:      fmt.Sprintf("%s = %d", formatReplayableFormula(newFormula), total),
	}
	return roll, true
}

func (r *diceRenderer) rollAdditionalDice(count int, sides int) ([]int, bool) {
	if count <= 0 || sides <= 0 {
		return []int{}, true
	}
	if r.rollMore != nil {
		expr := fmt.Sprintf("%dd%d", count, sides)
		values := r.rollMore(expr)
		if len(values) >= count {
			return append([]int(nil), values[:count]...), true
		}
	}
	values := make([]int, 0, count)
	for i := 0; i < count; i++ {
		rolled := r.evaluateFormula(fmt.Sprintf("1d%d", sides))
		value, err := strconv.Atoi(strings.TrimSpace(rolled.ResultValueText))
		if err != nil {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

func (r *diceRenderer) evaluateFormula(expr string) *model.MessageDiceRollModel {
	roll := &model.MessageDiceRollModel{Formula: expr}
	vm := ds.NewVM()
	vm.Config.EnableDiceWoD = true
	vm.Config.EnableDiceCoC = true
	vm.Config.EnableDiceFate = true
	vm.Config.EnableDiceDoubleCross = true
	vm.Config.DisableStmts = true
	vm.Config.OpCountLimit = 30000
	if r.defaultDiceSides != "" {
		vm.Config.DefaultDiceSideExpr = fmt.Sprintf("面数 ?? %s", r.defaultDiceSides)
	}
	if err := vm.Run(expr); err != nil {
		roll.IsError = true
		roll.ResultText = err.Error()
		return roll
	}
	if vm.Ret != nil {
		roll.ResultValueText = vm.Ret.ToString()
	}
	detail := strings.TrimSpace(vm.GetDetailText())
	roll.ResultDetail = detail
	if roll.ResultValueText != "" {
		roll.ResultText = fmt.Sprintf("%s = %s", expr, roll.ResultValueText)
	} else {
		roll.ResultText = expr
	}
	if !roll.IsError && roll.ResultDetail == "" && roll.ResultValueText != "" {
		roll.ResultDetail = fmt.Sprintf("[%s=%s]", expr, roll.ResultValueText)
	}
	return roll
}

func buildDiceChipHTML(roll *model.MessageDiceRollModel) string {
	classes := []string{"dice-chip"}
	if roll.IsError {
		classes = append(classes, "dice-chip--error")
	}
	builder := &strings.Builder{}
	fmt.Fprintf(builder, `<span class="%s" data-dice-roll-index="%d" data-dice-source="%s" data-dice-formula="%s"`,
		strings.Join(classes, " "), roll.RollIndex, html.EscapeString(roll.SourceText), html.EscapeString(roll.Formula))
	if roll.ResultText != "" {
		builder.WriteString(` data-dice-result-text="`)
		builder.WriteString(html.EscapeString(roll.ResultText))
		builder.WriteString(`"`)
	}
	if roll.ResultDetail != "" {
		builder.WriteString(` data-dice-result-detail="`)
		builder.WriteString(html.EscapeString(roll.ResultDetail))
		builder.WriteString(`"`)
	}
	if roll.ResultValueText != "" {
		builder.WriteString(` data-dice-result-value="`)
		builder.WriteString(html.EscapeString(roll.ResultValueText))
		builder.WriteString(`"`)
	}
	if roll.IsError {
		builder.WriteString(` data-dice-error="true"`)
	}
	builder.WriteString(">")
	formulaText := roll.Formula
	if roll.ResultDetail != "" {
		formulaText = roll.ResultDetail
	}
	resultText := roll.ResultValueText
	if resultText == "" {
		resultText = roll.ResultText
	}
	builder.WriteString(diceIconSVG)
	builder.WriteString(`<span class="dice-chip__formula">`)
	builder.WriteString(html.EscapeString(strings.TrimSpace(formulaText)))
	builder.WriteString(`</span>`)
	if !roll.IsError {
		builder.WriteString(`<span class="dice-chip__equals">=</span>`)
	}
	builder.WriteString(`<span class="dice-chip__result">`)
	if roll.IsError {
		builder.WriteString(html.EscapeString(strings.TrimSpace(roll.ResultText)))
	} else if strings.TrimSpace(resultText) != "" {
		builder.WriteString(html.EscapeString(strings.TrimSpace(resultText)))
	} else {
		builder.WriteString("?")
	}
	builder.WriteString(`</span></span>`)
	return builder.String()
}

func buildDiceRenderedHTML(sourceText string, rolls []*model.MessageDiceRollModel) string {
	if len(rolls) <= 1 {
		if len(rolls) == 0 {
			return html.EscapeString(sourceText)
		}
		return buildDiceChipHTML(rolls[0])
	}
	builder := &strings.Builder{}
	fmt.Fprintf(builder, `<span class="dice-roll-group" data-dice-source="%s">`, html.EscapeString(strings.TrimSpace(sourceText)))
	for _, roll := range rolls {
		if roll == nil {
			continue
		}
		builder.WriteString(buildDiceChipHTML(roll))
	}
	builder.WriteString(`</span>`)
	return builder.String()
}

// containsHiddenDiceCommand 检测内容中是否包含暗骰命令
func containsHiddenDiceCommand(content string) bool {
	return hiddenDicePattern.MatchString(content)
}

// ContainsHiddenDiceCommand 提供给外部使用的暗骰检测
func ContainsHiddenDiceCommand(content string) bool {
	return containsHiddenDiceCommand(content)
}
