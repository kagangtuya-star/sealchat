package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

const (
	TheaterPresentationSchemaVersion = 2
	MaxTheaterPortraitDecorations    = 16
)

type TheaterMediaKind string

const (
	TheaterMediaKindStaticImage   TheaterMediaKind = "static_image"
	TheaterMediaKindAnimatedImage TheaterMediaKind = "animated_image"
	TheaterMediaKindVideo         TheaterMediaKind = "video"
)

type TheaterObjectFit string

const (
	TheaterObjectFitContain TheaterObjectFit = "contain"
	TheaterObjectFitCover   TheaterObjectFit = "cover"
	TheaterObjectFitFill    TheaterObjectFit = "fill"
)

type TheaterLayerSpace string

const (
	TheaterLayerSpaceViewport TheaterLayerSpace = "viewport"
	TheaterLayerSpacePortrait TheaterLayerSpace = "portrait"
	TheaterLayerSpaceDialogue TheaterLayerSpace = "dialogue"
)

type TheaterBlendMode string

const (
	TheaterBlendModeNormal   TheaterBlendMode = "normal"
	TheaterBlendModeMultiply TheaterBlendMode = "multiply"
	TheaterBlendModeScreen   TheaterBlendMode = "screen"
	TheaterBlendModeOverlay  TheaterBlendMode = "overlay"
)

type TheaterTextAlign string

const (
	TheaterTextAlignLeft   TheaterTextAlign = "left"
	TheaterTextAlignCenter TheaterTextAlign = "center"
	TheaterTextAlignRight  TheaterTextAlign = "right"
)

type TheaterTransform struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
	Rotation float64 `json:"rotation"`
	Opacity  float64 `json:"opacity"`
	ZIndex   int     `json:"zIndex"`
}

type TheaterMediaRef struct {
	AssetID              string           `json:"assetId"`
	ResourceAttachmentID string           `json:"resourceAttachmentId"`
	FallbackAttachmentID string           `json:"fallbackAttachmentId,omitempty"`
	MIMEType             string           `json:"mimeType"`
	Kind                 TheaterMediaKind `json:"kind"`
	Width                int              `json:"width"`
	Height               int              `json:"height"`
	DurationMS           *int64           `json:"durationMs,omitempty"`
}

type TheaterVisualLayer struct {
	ID           string            `json:"id"`
	Enabled      bool              `json:"enabled"`
	Media        TheaterMediaRef   `json:"media"`
	Space        TheaterLayerSpace `json:"space"`
	Transform    TheaterTransform  `json:"transform"`
	Fit          TheaterObjectFit  `json:"fit"`
	PlaybackRate float64           `json:"playbackRate"`
	BlendMode    TheaterBlendMode  `json:"blendMode"`
}

type TheaterSpacing struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

type TheaterTextLayer struct {
	Enabled   bool             `json:"enabled"`
	Transform TheaterTransform `json:"transform"`
	FontScale float64          `json:"fontScale"`
}

type TheaterNarrationStyle struct {
	Enabled         bool    `json:"enabled"`
	BackdropColor   string  `json:"backdropColor"`
	BackdropOpacity float64 `json:"backdropOpacity"`
}

type TheaterDialogueStyle struct {
	Transform           TheaterTransform    `json:"transform"`
	Frame               *TheaterVisualLayer `json:"frame"`
	Speaker             TheaterTextLayer    `json:"speaker"`
	Content             TheaterTextLayer    `json:"content"`
	Padding             TheaterSpacing      `json:"padding"`
	NameGap             float64             `json:"nameGap"`
	TextAlign           TheaterTextAlign    `json:"textAlign"`
	ContentColor        string              `json:"contentColor"`
	CharactersPerSecond float64             `json:"charactersPerSecond"`
}

func (dialogue *TheaterDialogueStyle) UnmarshalJSON(data []byte) error {
	type theaterDialogueStyle TheaterDialogueStyle
	value := theaterDialogueStyle{
		ContentColor:        "#F4F4F5",
		CharactersPerSecond: 6,
		Speaker:             TheaterTextLayer{FontScale: 0.85},
		Content:             TheaterTextLayer{FontScale: 1.2},
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*dialogue = TheaterDialogueStyle(value)
	return nil
}

type TheaterPresentation struct {
	SchemaVersion       int                   `json:"schemaVersion"`
	Portrait            *TheaterVisualLayer   `json:"portrait"`
	PortraitDecorations []TheaterVisualLayer  `json:"portraitDecorations"`
	Dialogue            TheaterDialogueStyle  `json:"dialogue"`
	Narration           TheaterNarrationStyle `json:"narration"`
}

// TheaterVisualStyle stores reusable layer settings without binding a template
// to one character's media asset.
type TheaterVisualStyle struct {
	Enabled      bool             `json:"enabled"`
	Transform    TheaterTransform `json:"transform"`
	Fit          TheaterObjectFit `json:"fit"`
	PlaybackRate float64          `json:"playbackRate"`
	BlendMode    TheaterBlendMode `json:"blendMode"`
}

type TheaterDialogueBoxTemplate struct {
	Transform           TheaterTransform    `json:"transform"`
	Frame               *TheaterVisualLayer `json:"frame"`
	Padding             TheaterSpacing      `json:"padding"`
	NameGap             float64             `json:"nameGap"`
	TextAlign           TheaterTextAlign    `json:"textAlign"`
	ContentColor        string              `json:"contentColor"`
	CharactersPerSecond float64             `json:"charactersPerSecond"`
}

// WorldTheaterPresentationTemplate contains independently selectable world
// defaults. Portrait media is excluded; dialogue frame media is reusable.
type WorldTheaterPresentationTemplate struct {
	Portrait *TheaterVisualStyle         `json:"portrait,omitempty"`
	Speaker  *TheaterTextLayer           `json:"speaker,omitempty"`
	Content  *TheaterTextLayer           `json:"content,omitempty"`
	Dialogue *TheaterDialogueBoxTemplate `json:"dialogue,omitempty"`
}

func (presentation *TheaterPresentation) UnmarshalJSON(data []byte) error {
	type theaterPresentation TheaterPresentation
	value := theaterPresentation{Narration: DefaultTheaterNarrationStyle()}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*presentation = TheaterPresentation(value)
	return nil
}

// TheaterPatchField preserves omitted, null, and value as distinct JSON states.
type TheaterPatchField[T any] struct {
	Set   bool
	Value *T
}

type TheaterPresentationPatch struct {
	Portrait            TheaterPatchField[TheaterVisualLayer]
	PortraitDecorations TheaterPatchField[[]TheaterVisualLayer]
	Dialogue            TheaterPatchField[TheaterDialogueStyle]
	Narration           TheaterPatchField[TheaterNarrationStyle]
}

// OptionalTheaterPresentation preserves omitted, null, and value request states.
type OptionalTheaterPresentation struct {
	Set   bool
	Value *TheaterPresentation
}

func (value *OptionalTheaterPresentation) UnmarshalJSON(data []byte) error {
	value.Set = true
	value.Value = nil
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	value.Value = new(TheaterPresentation)
	return json.Unmarshal(data, value.Value)
}

// OptionalTheaterPresentationPatch preserves omission of the whole variant patch.
type OptionalTheaterPresentationPatch struct {
	Set   bool
	Value *TheaterPresentationPatch
}

func (value *OptionalTheaterPresentationPatch) UnmarshalJSON(data []byte) error {
	value.Set = true
	value.Value = nil
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	value.Value = new(TheaterPresentationPatch)
	return json.Unmarshal(data, value.Value)
}

func DefaultTheaterTransform() TheaterTransform {
	return TheaterTransform{Width: 1, Height: 1, Opacity: 1}
}

func DefaultTheaterDialogueStyle() TheaterDialogueStyle {
	return TheaterDialogueStyle{
		Transform: TheaterTransform{X: 0.05, Y: 0.69, Width: 0.9, Height: 0.28, Opacity: 1},
		Speaker: TheaterTextLayer{
			Enabled:   true,
			Transform: TheaterTransform{X: 0.025, Y: 0.065, Width: 0.34, Height: 0.12, Opacity: 1, ZIndex: 2},
			FontScale: 0.85,
		},
		Content: TheaterTextLayer{
			Enabled:   true,
			Transform: TheaterTransform{X: 0.025, Y: 0.28, Width: 0.95, Height: 0.68, Opacity: 1, ZIndex: 2},
			FontScale: 1.2,
		},
		Padding:             TheaterSpacing{Top: 0.16, Right: 0.08, Bottom: 0.12, Left: 0.08},
		NameGap:             0.04,
		TextAlign:           TheaterTextAlignLeft,
		ContentColor:        "#F4F4F5",
		CharactersPerSecond: 6,
	}
}

func DefaultTheaterNarrationStyle() TheaterNarrationStyle {
	return TheaterNarrationStyle{BackdropColor: "#000000", BackdropOpacity: 1}
}

func DefaultTheaterPresentation() TheaterPresentation {
	return TheaterPresentation{
		SchemaVersion:       TheaterPresentationSchemaVersion,
		PortraitDecorations: []TheaterVisualLayer{},
		Dialogue:            DefaultTheaterDialogueStyle(),
		Narration:           DefaultTheaterNarrationStyle(),
	}
}

func applyTheaterVisualStyle(layer *TheaterVisualLayer, style *TheaterVisualStyle) {
	if layer == nil || style == nil {
		return
	}
	layer.Enabled = style.Enabled
	layer.Transform = style.Transform
	layer.Fit = style.Fit
	layer.PlaybackRate = style.PlaybackRate
	layer.BlendMode = style.BlendMode
}

func ApplyWorldTheaterPresentationTemplate(value TheaterPresentation, template WorldTheaterPresentationTemplate) TheaterPresentation {
	value = NormalizeTheaterPresentation(value)
	applyTheaterVisualStyle(value.Portrait, template.Portrait)
	if template.Speaker != nil {
		value.Dialogue.Speaker = *template.Speaker
	}
	if template.Content != nil {
		value.Dialogue.Content = *template.Content
	}
	if template.Dialogue != nil {
		box := template.Dialogue
		value.Dialogue.Transform = box.Transform
		value.Dialogue.Frame = cloneTheaterLayer(box.Frame)
		value.Dialogue.Padding = box.Padding
		value.Dialogue.NameGap = box.NameGap
		value.Dialogue.TextAlign = box.TextAlign
		value.Dialogue.ContentColor = box.ContentColor
		value.Dialogue.CharactersPerSecond = box.CharactersPerSecond
	}
	return NormalizeTheaterPresentation(value)
}

func ValidateWorldTheaterPresentationTemplate(template WorldTheaterPresentationTemplate) error {
	var problems []error
	validateStyle := func(style *TheaterVisualStyle, path string) {
		if style == nil {
			return
		}
		problems = appendError(problems, validateTheaterTransform(style.Transform, path+".transform"))
		if style.Fit != TheaterObjectFitCover {
			problems = append(problems, fmt.Errorf("%s.fit is invalid", path))
		}
		if style.PlaybackRate < 0.25 || style.PlaybackRate > 4 || math.IsNaN(style.PlaybackRate) || math.IsInf(style.PlaybackRate, 0) {
			problems = append(problems, fmt.Errorf("%s.playbackRate is invalid", path))
		}
		if style.BlendMode != TheaterBlendModeNormal && style.BlendMode != TheaterBlendModeMultiply && style.BlendMode != TheaterBlendModeScreen && style.BlendMode != TheaterBlendModeOverlay {
			problems = append(problems, fmt.Errorf("%s.blendMode is invalid", path))
		}
	}
	validateStyle(template.Portrait, "portrait")
	if template.Speaker != nil {
		problems = appendError(problems, validateTheaterTextTransform(template.Speaker.Transform, "speaker.transform"))
		if template.Speaker.FontScale < 0.25 || template.Speaker.FontScale > 4 {
			problems = append(problems, errors.New("speaker.fontScale is invalid"))
		}
	}
	if template.Content != nil {
		problems = appendError(problems, validateTheaterTextTransform(template.Content.Transform, "content.transform"))
		if template.Content.FontScale < 0.25 || template.Content.FontScale > 4 {
			problems = append(problems, errors.New("content.fontScale is invalid"))
		}
	}
	if template.Dialogue != nil {
		dialogue := DefaultTheaterDialogueStyle()
		dialogue.Transform = template.Dialogue.Transform
		dialogue.Frame = cloneTheaterLayer(template.Dialogue.Frame)
		dialogue.Padding = template.Dialogue.Padding
		dialogue.NameGap = template.Dialogue.NameGap
		dialogue.TextAlign = template.Dialogue.TextAlign
		dialogue.ContentColor = template.Dialogue.ContentColor
		dialogue.CharactersPerSecond = template.Dialogue.CharactersPerSecond
		problems = appendError(problems, validateTheaterDialogue(dialogue))
	}
	return errors.Join(problems...)
}

func NormalizeTheaterPresentation(value TheaterPresentation) TheaterPresentation {
	if value.SchemaVersion != TheaterPresentationSchemaVersion {
		return DefaultTheaterPresentation()
	}
	if value.PortraitDecorations == nil {
		value.PortraitDecorations = []TheaterVisualLayer{}
	}
	if value.Dialogue == (TheaterDialogueStyle{}) {
		value.Dialogue = DefaultTheaterDialogueStyle()
	} else {
		defaults := DefaultTheaterDialogueStyle()
		if value.Dialogue.ContentColor == "" {
			value.Dialogue.ContentColor = defaults.ContentColor
		}
		if value.Dialogue.CharactersPerSecond == 0 {
			value.Dialogue.CharactersPerSecond = defaults.CharactersPerSecond
		}
	}
	if value.Narration.BackdropColor == "" {
		value.Narration = DefaultTheaterNarrationStyle()
	}
	value.Dialogue = migrateLegacyDefaultTheaterDialogue(value.Dialogue)
	return value
}

func migrateLegacyDefaultTheaterDialogue(dialogue TheaterDialogueStyle) TheaterDialogueStyle {
	legacyOuter := matchesTheaterTransformSize(dialogue.Transform, 0.02, 0.69, 0.96, 0.28)
	firstRevisionOuter := matchesTheaterTransformSize(dialogue.Transform, 0.1, 0.69, 0.8, 0.28)
	secondRevisionOuter := matchesTheaterTransformSize(dialogue.Transform, 0.05, 0.69, 0.9, 0.28)
	if !legacyOuter && !firstRevisionOuter && !secondRevisionOuter {
		return dialogue
	}
	dialogue.Transform.X = 0.05
	dialogue.Transform.Width = 0.9
	speakerX := 0.075
	speakerScale := 1.0
	if legacyOuter {
		speakerX = 0.08
	}
	if secondRevisionOuter {
		speakerScale = 0.85
	}
	if matchesTheaterTransformSize(dialogue.Speaker.Transform, speakerX, 0.12, 0.34, 0.12) && dialogue.Speaker.FontScale == speakerScale {
		dialogue.Speaker.Transform.X = 0.025
		dialogue.Speaker.Transform.Y = 0.065
		dialogue.Speaker.FontScale = 0.85
	}
	contentX, contentWidth, contentScale := 0.075, 0.85, 1.2
	if legacyOuter {
		contentX, contentWidth, contentScale = 0.08, 0.84, 1
	}
	if matchesTheaterTransformSize(dialogue.Content.Transform, contentX, 0.3, contentWidth, 0.56) && dialogue.Content.FontScale == contentScale {
		dialogue.Content.Transform.X = 0.025
		dialogue.Content.Transform.Y = 0.28
		dialogue.Content.Transform.Width = 0.95
		dialogue.Content.Transform.Height = 0.68
		dialogue.Content.FontScale = 1.2
	}
	return dialogue
}

func matchesTheaterTransformSize(transform TheaterTransform, x, y, width, height float64) bool {
	return transform.X == x && transform.Y == y && transform.Width == width && transform.Height == height
}

func ResolveTheaterPresentation(base TheaterPresentation, patch *TheaterPresentationPatch) TheaterPresentation {
	resolved := NormalizeTheaterPresentation(base)
	resolved.Portrait = cloneTheaterLayer(resolved.Portrait)
	resolved.PortraitDecorations = append([]TheaterVisualLayer(nil), resolved.PortraitDecorations...)
	resolved.Dialogue.Frame = cloneTheaterLayer(resolved.Dialogue.Frame)
	if patch == nil {
		return resolved
	}
	if patch.Portrait.Set {
		resolved.Portrait = cloneTheaterLayer(patch.Portrait.Value)
	}
	if patch.PortraitDecorations.Set {
		resolved.PortraitDecorations = []TheaterVisualLayer{}
		if patch.PortraitDecorations.Value != nil {
			resolved.PortraitDecorations = append(resolved.PortraitDecorations, (*patch.PortraitDecorations.Value)...)
		}
	}
	if patch.Dialogue.Set {
		resolved.Dialogue = DefaultTheaterDialogueStyle()
		if patch.Dialogue.Value != nil {
			resolved.Dialogue = *patch.Dialogue.Value
		}
	}
	if patch.Narration.Set {
		resolved.Narration = DefaultTheaterNarrationStyle()
		if patch.Narration.Value != nil {
			resolved.Narration = *patch.Narration.Value
		}
	}
	return NormalizeTheaterPresentation(resolved)
}

func ValidateTheaterPresentation(value TheaterPresentation) error {
	var problems []error
	if value.SchemaVersion != TheaterPresentationSchemaVersion {
		problems = append(problems, fmt.Errorf("schemaVersion must be %d", TheaterPresentationSchemaVersion))
	}
	if value.Portrait != nil {
		problems = appendError(problems, validateTheaterLayer(*value.Portrait, TheaterLayerSpaceViewport, "portrait"))
	}
	if len(value.PortraitDecorations) > MaxTheaterPortraitDecorations {
		problems = append(problems, fmt.Errorf("portraitDecorations must contain at most %d layers", MaxTheaterPortraitDecorations))
	}
	seen := make(map[string]struct{}, len(value.PortraitDecorations))
	for index, layer := range value.PortraitDecorations {
		path := fmt.Sprintf("portraitDecorations[%d]", index)
		problems = appendError(problems, validateTheaterLayer(layer, TheaterLayerSpacePortrait, path))
		if _, exists := seen[layer.ID]; exists {
			problems = append(problems, fmt.Errorf("%s.id must be unique", path))
		}
		seen[layer.ID] = struct{}{}
	}
	problems = appendError(problems, validateTheaterDialogue(value.Dialogue))
	problems = appendError(problems, validateTheaterNarration(value.Narration))
	return errors.Join(problems...)
}

func ValidateTheaterPresentationPatch(patch TheaterPresentationPatch) error {
	var problems []error
	if patch.Portrait.Set && patch.Portrait.Value != nil {
		problems = appendError(problems, validateTheaterLayer(*patch.Portrait.Value, TheaterLayerSpaceViewport, "portrait"))
	}
	if patch.PortraitDecorations.Set && patch.PortraitDecorations.Value != nil {
		layers := *patch.PortraitDecorations.Value
		if len(layers) > MaxTheaterPortraitDecorations {
			problems = append(problems, fmt.Errorf("portraitDecorations must contain at most %d layers", MaxTheaterPortraitDecorations))
		}
		seen := make(map[string]struct{}, len(layers))
		for index, layer := range layers {
			path := fmt.Sprintf("portraitDecorations[%d]", index)
			problems = appendError(problems, validateTheaterLayer(layer, TheaterLayerSpacePortrait, path))
			if _, exists := seen[layer.ID]; exists {
				problems = append(problems, fmt.Errorf("%s.id must be unique", path))
			}
			seen[layer.ID] = struct{}{}
		}
	}
	if patch.Dialogue.Set && patch.Dialogue.Value != nil {
		problems = appendError(problems, validateTheaterDialogue(*patch.Dialogue.Value))
	}
	if patch.Narration.Set && patch.Narration.Value != nil {
		problems = appendError(problems, validateTheaterNarration(*patch.Narration.Value))
	}
	return errors.Join(problems...)
}

func (patch *TheaterPresentationPatch) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("theater presentation patch must be an object")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*patch = TheaterPresentationPatch{}
	for key := range fields {
		if key != "portrait" && key != "portraitDecorations" && key != "dialogue" && key != "narration" {
			return fmt.Errorf("unknown theater presentation patch field %q", key)
		}
	}
	if raw, ok := fields["portrait"]; ok {
		patch.Portrait.Set = true
		if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			patch.Portrait.Value = new(TheaterVisualLayer)
			if err := json.Unmarshal(raw, patch.Portrait.Value); err != nil {
				return fmt.Errorf("portrait: %w", err)
			}
		}
	}
	if raw, ok := fields["portraitDecorations"]; ok {
		patch.PortraitDecorations.Set = true
		if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			patch.PortraitDecorations.Value = new([]TheaterVisualLayer)
			if err := json.Unmarshal(raw, patch.PortraitDecorations.Value); err != nil {
				return fmt.Errorf("portraitDecorations: %w", err)
			}
		}
	}
	if raw, ok := fields["dialogue"]; ok {
		patch.Dialogue.Set = true
		if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			patch.Dialogue.Value = new(TheaterDialogueStyle)
			if err := json.Unmarshal(raw, patch.Dialogue.Value); err != nil {
				return fmt.Errorf("dialogue: %w", err)
			}
		}
	}
	if raw, ok := fields["narration"]; ok {
		patch.Narration.Set = true
		if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			patch.Narration.Value = new(TheaterNarrationStyle)
			if err := json.Unmarshal(raw, patch.Narration.Value); err != nil {
				return fmt.Errorf("narration: %w", err)
			}
		}
	}
	return nil
}

func (patch TheaterPresentationPatch) MarshalJSON() ([]byte, error) {
	fields := make(map[string]any, 4)
	if patch.Portrait.Set {
		fields["portrait"] = patch.Portrait.Value
	}
	if patch.PortraitDecorations.Set {
		fields["portraitDecorations"] = patch.PortraitDecorations.Value
	}
	if patch.Dialogue.Set {
		fields["dialogue"] = patch.Dialogue.Value
	}
	if patch.Narration.Set {
		fields["narration"] = patch.Narration.Value
	}
	return json.Marshal(fields)
}

func validateTheaterDialogue(dialogue TheaterDialogueStyle) error {
	var problems []error
	problems = appendError(problems, validateTheaterTransform(dialogue.Transform, "dialogue.transform"))
	problems = appendError(problems, validateTheaterTextTransform(dialogue.Speaker.Transform, "dialogue.speaker.transform"))
	problems = appendError(problems, validateTheaterTextTransform(dialogue.Content.Transform, "dialogue.content.transform"))
	if !finiteInRange(dialogue.Speaker.FontScale, 0.25, 4) {
		problems = append(problems, errors.New("dialogue.speaker.fontScale must be finite and between 0.25 and 4"))
	}
	if !finiteInRange(dialogue.Content.FontScale, 0.25, 4) {
		problems = append(problems, errors.New("dialogue.content.fontScale must be finite and between 0.25 and 4"))
	}
	if !validTheaterColor(dialogue.ContentColor) {
		problems = append(problems, errors.New("dialogue.contentColor must be a hex color"))
	}
	if !finiteInRange(dialogue.CharactersPerSecond, 1, 60) {
		problems = append(problems, errors.New("dialogue.charactersPerSecond must be finite and between 1 and 60"))
	}
	if dialogue.Frame != nil {
		problems = appendError(problems, validateTheaterLayer(*dialogue.Frame, TheaterLayerSpaceDialogue, "dialogue.frame"))
	}
	for name, value := range map[string]float64{
		"top": dialogue.Padding.Top, "right": dialogue.Padding.Right,
		"bottom": dialogue.Padding.Bottom, "left": dialogue.Padding.Left,
	} {
		if !finiteInRange(value, 0, 1) {
			problems = append(problems, fmt.Errorf("dialogue.padding.%s must be finite and between 0 and 1", name))
		}
	}
	if !finiteInRange(dialogue.NameGap, 0, 1) {
		problems = append(problems, errors.New("dialogue.nameGap must be finite and between 0 and 1"))
	}
	if dialogue.TextAlign != TheaterTextAlignLeft && dialogue.TextAlign != TheaterTextAlignCenter && dialogue.TextAlign != TheaterTextAlignRight {
		problems = append(problems, errors.New("dialogue.textAlign must be left, center, or right"))
	}
	return errors.Join(problems...)
}

func validateTheaterNarration(narration TheaterNarrationStyle) error {
	var problems []error
	if !validTheaterColor(narration.BackdropColor) {
		problems = append(problems, errors.New("narration.backdropColor must be a hex color"))
	}
	if !finiteInRange(narration.BackdropOpacity, 0, 1) {
		problems = append(problems, errors.New("narration.backdropOpacity must be finite and between 0 and 1"))
	}
	return errors.Join(problems...)
}

func validTheaterColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, character := range value[1:] {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}

func validateTheaterLayer(layer TheaterVisualLayer, expectedSpace TheaterLayerSpace, path string) error {
	var problems []error
	if utf8.RuneCountInString(layer.ID) < 1 || utf8.RuneCountInString(layer.ID) > 128 {
		problems = append(problems, fmt.Errorf("%s.id must contain 1 to 128 characters", path))
	}
	if layer.Space != expectedSpace {
		problems = append(problems, fmt.Errorf("%s.space must be %q", path, expectedSpace))
	}
	problems = appendError(problems, validateTheaterMedia(layer.Media, path+".media"))
	problems = appendError(problems, validateTheaterTransform(layer.Transform, path+".transform"))
	if layer.Fit != TheaterObjectFitCover {
		problems = append(problems, fmt.Errorf("%s.fit must be %q", path, TheaterObjectFitCover))
	}
	if !finiteInRange(layer.PlaybackRate, 0.25, 4) {
		problems = append(problems, fmt.Errorf("%s.playbackRate must be finite and between 0.25 and 4", path))
	}
	if layer.BlendMode != TheaterBlendModeNormal && layer.BlendMode != TheaterBlendModeMultiply && layer.BlendMode != TheaterBlendModeScreen && layer.BlendMode != TheaterBlendModeOverlay {
		problems = append(problems, fmt.Errorf("%s.blendMode is invalid", path))
	}
	return errors.Join(problems...)
}

func validateTheaterMedia(media TheaterMediaRef, path string) error {
	var problems []error
	for name, value := range map[string]string{
		"assetId": media.AssetID, "resourceAttachmentId": media.ResourceAttachmentID,
	} {
		if utf8.RuneCountInString(value) < 1 || utf8.RuneCountInString(value) > 128 {
			problems = append(problems, fmt.Errorf("%s.%s must contain 1 to 128 characters", path, name))
		}
	}
	if utf8.RuneCountInString(media.FallbackAttachmentID) > 128 {
		problems = append(problems, fmt.Errorf("%s.fallbackAttachmentId must contain at most 128 characters", path))
	}
	if media.MIMEType != "image/png" && media.MIMEType != "image/webp" && media.MIMEType != "video/webm" {
		problems = append(problems, fmt.Errorf("%s.mimeType is invalid", path))
	}
	if media.Kind != TheaterMediaKindStaticImage && media.Kind != TheaterMediaKindAnimatedImage && media.Kind != TheaterMediaKindVideo {
		problems = append(problems, fmt.Errorf("%s.kind is invalid", path))
	}
	mediaMatchesKind := (media.Kind == TheaterMediaKindVideo && media.MIMEType == "video/webm") ||
		(media.Kind == TheaterMediaKindAnimatedImage && (media.MIMEType == "image/webp" || media.MIMEType == "video/webm")) ||
		(media.Kind == TheaterMediaKindStaticImage && (media.MIMEType == "image/png" || media.MIMEType == "image/webp"))
	if !mediaMatchesKind {
		problems = append(problems, fmt.Errorf("%s.mimeType does not match kind", path))
	}
	if media.Width < 1 || media.Width > 4096 || media.Height < 1 || media.Height > 4096 {
		problems = append(problems, fmt.Errorf("%s dimensions must be between 1 and 4096", path))
	}
	if media.DurationMS != nil && (*media.DurationMS < 0 || *media.DurationMS > 60_000) {
		problems = append(problems, fmt.Errorf("%s.durationMs must be between 0 and 60000", path))
	}
	return errors.Join(problems...)
}

func validateTheaterTransform(transform TheaterTransform, path string) error {
	return validateTheaterTransformWithYMinimum(transform, path, -1)
}

func validateTheaterTextTransform(transform TheaterTransform, path string) error {
	return validateTheaterTransformWithYMinimum(transform, path, math.Inf(-1))
}

func validateTheaterTransformWithYMinimum(transform TheaterTransform, path string, minimumY float64) error {
	var problems []error
	if !finiteInRange(transform.X, -1, 2) {
		problems = append(problems, fmt.Errorf("%s.x must be finite and between -1 and 2", path))
	}
	if !finiteInRange(transform.Y, minimumY, 2) {
		if math.IsInf(minimumY, -1) {
			problems = append(problems, fmt.Errorf("%s.y must be finite and at most 2", path))
		} else {
			problems = append(problems, fmt.Errorf("%s.y must be finite and between -1 and 2", path))
		}
	}
	for name, value := range map[string]float64{
		"width": transform.Width, "height": transform.Height,
	} {
		if !finiteInRange(value, 0.01, 3) {
			problems = append(problems, fmt.Errorf("%s.%s must be finite and between 0.01 and 3", path, name))
		}
	}
	if !finiteInRange(transform.Rotation, -180, 180) {
		problems = append(problems, fmt.Errorf("%s.rotation must be finite and between -180 and 180", path))
	}
	if !finiteInRange(transform.Opacity, 0, 1) {
		problems = append(problems, fmt.Errorf("%s.opacity must be finite and between 0 and 1", path))
	}
	if transform.ZIndex < -100 || transform.ZIndex > 100 {
		problems = append(problems, fmt.Errorf("%s.zIndex must be between -100 and 100", path))
	}
	return errors.Join(problems...)
}

func finiteInRange(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimum && value <= maximum
}

func appendError(problems []error, problem error) []error {
	if problem != nil {
		return append(problems, problem)
	}
	return problems
}

func cloneTheaterLayer(layer *TheaterVisualLayer) *TheaterVisualLayer {
	if layer == nil {
		return nil
	}
	clone := *layer
	if layer.Media.DurationMS != nil {
		duration := *layer.Media.DurationMS
		clone.Media.DurationMS = &duration
	}
	return &clone
}
