package service

import (
	"encoding/json"
	"regexp"
	"strings"

	"sealchat/model"
)

// ChannelEmbedTopicPattern is the Channel Embed events.publish/subscribe topic
// syntax. Theater embed event bindings use the same exact-topic grammar.
var ChannelEmbedTopicPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

const (
	TheaterMaxEmbedEventBindings       = 16
	TheaterMaxEmbedEventBindingActions = 16
)

// TheaterEmbedEventBinding maps one exact Channel Embed topic to saved
// StageAction IDs of the same iframe object. It is configuration data only;
// an event payload never selects actions.
type TheaterEmbedEventBinding struct {
	Topic     string   `json:"topic"`
	ActionIDs []string `json:"actionIds"`
}

// theaterObjectEmbedEventBindings decodes metadata.embedEventBindings. present
// reports whether the key exists, independent of its validity.
func theaterObjectEmbedEventBindings(metadataRaw []byte) ([]TheaterEmbedEventBinding, bool, error) {
	if len(metadataRaw) == 0 {
		return nil, false, nil
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		// Legacy non-object metadata cannot carry bindings; leave it untouched.
		return nil, false, nil
	}
	raw, ok := metadata["embedEventBindings"]
	if !ok {
		return nil, false, nil
	}
	var bindings []TheaterEmbedEventBinding
	if err := decodeStrictJSON(raw, &bindings); err != nil {
		return nil, true, theaterPayloadError("embedEventBindings 无效")
	}
	return bindings, true, nil
}

// validateTheaterEmbedEventBindings validates the final object state, so a
// binding can never reference an action that is not saved on the same object.
func validateTheaterEmbedEventBindings(kind string, metadataRaw, actionsRaw []byte) error {
	bindings, present, err := theaterObjectEmbedEventBindings(metadataRaw)
	if err != nil || !present {
		return err
	}
	if kind != "iframe" {
		return theaterPayloadError("embedEventBindings 仅支持 iframe 对象")
	}
	if len(bindings) > TheaterMaxEmbedEventBindings {
		return theaterPayloadError("embedEventBindings 数量超限")
	}
	actionIDs := map[string]struct{}{}
	if len(actionsRaw) > 0 {
		var actions []theaterStoredAction
		if err := json.Unmarshal(actionsRaw, &actions); err != nil {
			return theaterPayloadError("object actions 无效")
		}
		for _, action := range actions {
			actionIDs[action.ID] = struct{}{}
		}
	}
	topics := map[string]struct{}{}
	for _, binding := range bindings {
		if !ChannelEmbedTopicPattern.MatchString(binding.Topic) {
			return theaterPayloadError("embedEventBindings topic 无效")
		}
		if _, exists := topics[binding.Topic]; exists {
			return theaterPayloadError("embedEventBindings topic 重复")
		}
		topics[binding.Topic] = struct{}{}
		if len(binding.ActionIDs) == 0 || len(binding.ActionIDs) > TheaterMaxEmbedEventBindingActions {
			return theaterPayloadError("embedEventBindings actionIds 数量无效")
		}
		seen := map[string]struct{}{}
		for _, actionID := range binding.ActionIDs {
			if err := validateTheaterID(actionID, "embedEventBindings actionId"); err != nil {
				return err
			}
			if _, exists := seen[actionID]; exists {
				return theaterPayloadError("embedEventBindings actionId 重复")
			}
			seen[actionID] = struct{}{}
			if _, exists := actionIDs[actionID]; !exists {
				return theaterPayloadError("embedEventBindings 引用了不存在的动作")
			}
		}
	}
	return nil
}

// theaterObjectCanRunSavedAction separates "owns executable saved actions" from
// stage click semantics. drawing/text/image/button keep their click behavior; an
// iframe runs only actions referenced by its embed event bindings and never
// becomes a pointer click or object.trigger target.
func theaterObjectCanRunSavedAction(object *model.TheaterObjectModel, actionID string) bool {
	if object == nil {
		return false
	}
	if isTheaterActionTargetKind(object.Kind) {
		return true
	}
	if object.Kind != "iframe" {
		return false
	}
	actionID = strings.TrimSpace(actionID)
	bindings, _, err := theaterObjectEmbedEventBindings([]byte(object.MetadataJSON))
	if err != nil || actionID == "" {
		return false
	}
	for _, binding := range bindings {
		for _, boundID := range binding.ActionIDs {
			if boundID == actionID {
				return true
			}
		}
	}
	return false
}
