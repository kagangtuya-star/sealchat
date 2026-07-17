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
}

type TheaterDialogueStyle struct {
	Transform TheaterTransform    `json:"transform"`
	Frame     *TheaterVisualLayer `json:"frame"`
	Speaker   TheaterTextLayer    `json:"speaker"`
	Content   TheaterTextLayer    `json:"content"`
	Padding   TheaterSpacing      `json:"padding"`
	NameGap   float64             `json:"nameGap"`
	TextAlign TheaterTextAlign    `json:"textAlign"`
}

type TheaterPresentation struct {
	SchemaVersion       int                  `json:"schemaVersion"`
	Portrait            *TheaterVisualLayer  `json:"portrait"`
	PortraitDecorations []TheaterVisualLayer `json:"portraitDecorations"`
	Dialogue            TheaterDialogueStyle `json:"dialogue"`
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
		Transform: TheaterTransform{X: 0.02, Y: 0.69, Width: 0.96, Height: 0.28, Opacity: 1},
		Speaker: TheaterTextLayer{
			Enabled:   true,
			Transform: TheaterTransform{X: 0.08, Y: 0.12, Width: 0.34, Height: 0.12, Opacity: 1, ZIndex: 2},
		},
		Content: TheaterTextLayer{
			Enabled:   true,
			Transform: TheaterTransform{X: 0.08, Y: 0.30, Width: 0.84, Height: 0.56, Opacity: 1, ZIndex: 2},
		},
		Padding:   TheaterSpacing{Top: 0.16, Right: 0.08, Bottom: 0.12, Left: 0.08},
		NameGap:   0.04,
		TextAlign: TheaterTextAlignLeft,
	}
}

func DefaultTheaterPresentation() TheaterPresentation {
	return TheaterPresentation{
		SchemaVersion:       TheaterPresentationSchemaVersion,
		PortraitDecorations: []TheaterVisualLayer{},
		Dialogue:            DefaultTheaterDialogueStyle(),
	}
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
	}
	return value
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
		if key != "portrait" && key != "portraitDecorations" && key != "dialogue" {
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
	return nil
}

func (patch TheaterPresentationPatch) MarshalJSON() ([]byte, error) {
	fields := make(map[string]any, 3)
	if patch.Portrait.Set {
		fields["portrait"] = patch.Portrait.Value
	}
	if patch.PortraitDecorations.Set {
		fields["portraitDecorations"] = patch.PortraitDecorations.Value
	}
	if patch.Dialogue.Set {
		fields["dialogue"] = patch.Dialogue.Value
	}
	return json.Marshal(fields)
}

func validateTheaterDialogue(dialogue TheaterDialogueStyle) error {
	var problems []error
	problems = appendError(problems, validateTheaterTransform(dialogue.Transform, "dialogue.transform"))
	problems = appendError(problems, validateTheaterTransform(dialogue.Speaker.Transform, "dialogue.speaker.transform"))
	problems = appendError(problems, validateTheaterTransform(dialogue.Content.Transform, "dialogue.content.transform"))
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
	var problems []error
	for name, value := range map[string]float64{
		"x": transform.X, "y": transform.Y,
	} {
		if !finiteInRange(value, -1, 2) {
			problems = append(problems, fmt.Errorf("%s.%s must be finite and between -1 and 2", path, name))
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
