package service

import (
	"encoding/json"
	"sealchat/model"
)

// Gateway and event history carry a bounded summary. The normalized Plan lives
// only in the mutation record for fingerprint/audit; clients reload by revision.
func TheaterMutationEventPayload(mutation model.TheaterMutationModel) json.RawMessage {
	if mutation.Type != TheaterMutationDesignApply {
		return normalizedRawJSON(mutation.PayloadJSON, `{}`)
	}
	var result TheaterMutationResult
	var summary TheaterDesignSummary
	if json.Unmarshal([]byte(mutation.ResultJSON), &result) != nil || json.Unmarshal(result.Payload, &summary) != nil {
		return json.RawMessage(`{}`)
	}
	raw, _ := json.Marshal(map[string]any{"stepCount": summary.StepCount, "scenesCreated": len(summary.ScenesCreated), "scenesUpdated": len(summary.ScenesUpdated), "scenesDeleted": len(summary.ScenesDeleted), "objectsCreated": len(summary.ObjectsCreated), "objectsUpdated": len(summary.ObjectsUpdated), "objectsDeleted": len(summary.ObjectsDeleted), "layoutCount": len(summary.Layout), "warningCount": len(summary.Warnings)})
	return raw
}
