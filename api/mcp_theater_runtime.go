package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sealchat/service"
)

type mcpTheaterRuntimeInput struct {
	service.TheaterScope
	Operation        string `json:"operation"`
	RendererID       string `json:"rendererId,omitempty"`
	SceneID          string `json:"sceneId,omitempty"`
	ExpectedRevision int64  `json:"expectedRevision"`
	ExecutionID      string `json:"executionId,omitempty"`
	ObjectID         string `json:"objectId,omitempty"`
	ActionID         string `json:"actionId,omitempty"`
	SequenceID       string `json:"sequenceId,omitempty"`
	MutationID       string `json:"mutationId,omitempty"`
}
type mcpTheaterViewInput struct {
	service.TheaterScope
	Operation        string                         `json:"operation"`
	RendererID       string                         `json:"rendererId,omitempty"`
	SceneID          string                         `json:"sceneId"`
	ExpectedRevision int64                          `json:"expectedRevision"`
	Camera           *service.TheaterRendererCamera `json:"camera,omitempty"`
	ObjectIDs        []string                       `json:"objectIds,omitempty"`
}
type mcpTheaterCaptureInput struct {
	service.TheaterScope
	Operation        string `json:"operation"`
	RendererID       string `json:"rendererId,omitempty"`
	SceneID          string `json:"sceneId,omitempty"`
	ExpectedRevision int64  `json:"expectedRevision"`
	CaptureID        string `json:"captureId,omitempty"`
	Mode             string `json:"mode,omitempty"`
	ObjectID         string `json:"objectId,omitempty"`
	MaxEdge          int    `json:"maxEdge,omitempty"`
	MaxBytes         int    `json:"maxBytes,omitempty"`
}

func mcpTheaterRuntimeTarget(ctx context.Context, a *service.MCPActor, s service.TheaterScope, sceneID string, revision int64) (*service.TheaterSnapshotResult, service.TheaterScope, error) {
	snap, scope, err := mcpTheaterSnapshot(ctx, a, s, false)
	if err != nil {
		return nil, scope, err
	}
	if revision < 0 || snap.Revision != revision {
		return nil, scope, &service.TheaterError{Code: service.TheaterErrorRevisionConflict, Message: "请重新读取当前 revision", HTTPStatus: 409, Details: map[string]any{"currentRevision": snap.Revision}}
	}
	if snap.Snapshot.ActiveSceneID == nil || sceneID != *snap.Snapshot.ActiveSceneID {
		return nil, scope, mcpFailure("renderer_scene_mismatch", "仅支持当前可见场景")
	}
	return snap, scope, nil
}
func mcpTheaterControl(ctx context.Context, a *service.MCPActor, in mcpTheaterRuntimeInput) (any, error) {
	if err := mcpTheaterOperationFields(in, in.Operation, map[string]string{
		"execution_status": "executionId", "cancel_execution": "executionId", "apply_scene": "sceneId",
		"trigger_action": "rendererId sceneId objectId actionId", "trigger_sequence": "rendererId sceneId sequenceId",
	}); err != nil {
		return nil, err
	}
	if in.Operation != "apply_scene" && in.MutationID != "" {
		return nil, mcpFailure("invalid_argument", "仅 apply_scene 接受 mutationId")
	}
	scope, err := service.NormalizeTheaterMCPScope(a.User.ID, in.TheaterScope)
	if err != nil {
		return nil, err
	}
	switch in.Operation {
	case "execution_status":
		return mcpTheaterTask(a, scope, in.ExecutionID, "execute", false)
	case "cancel_execution":
		return mcpTheaterTask(a, scope, in.ExecutionID, "execute", true)
	case "apply_scene":
		return mcpTheaterMutation(ctx, a, mcpTheaterWriteInput{TheaterScope: scope, Operation: "apply", MutationID: in.MutationID, ExpectedRevision: in.ExpectedRevision}, service.TheaterMutationSceneApply, map[string]any{"sceneId": in.SceneID}, nil)
	case "trigger_action", "trigger_sequence":
		if _, _, err := mcpTheaterRuntimeTarget(ctx, a, scope, in.SceneID, in.ExpectedRevision); err != nil {
			return nil, err
		}
		if in.Operation == "trigger_action" && (in.ObjectID == "" || in.SequenceID != "") {
			return nil, mcpFailure("invalid_argument", "trigger_action 需要 objectId，可指定 actionId")
		}
		if in.Operation == "trigger_sequence" && (in.SequenceID == "" || in.ObjectID != "" || in.ActionID != "") {
			return nil, mcpFailure("invalid_argument", "trigger_sequence 需要 sequenceId")
		}
		execution, err := service.PrepareTheaterMCPExecution(ctx, a, scope, in.SceneID, in.ObjectID, in.ActionID, in.SequenceID, in.ExpectedRevision)
		if err != nil {
			return nil, err
		}
		token, _ := ctx.Value(mcpCredentialContextKey{}).(string)
		resource, _ := ctx.Value(mcpResourceContextKey{}).(string)
		if token == "" {
			return nil, mcpFailure("unauthorized", "执行任务需要有效 MCP 凭证")
		}
		return service.TheaterRenderers.Start(a.User.ID, a.CredentialID, scope, in.RendererID, in.SceneID, in.ExpectedRevision, "execute", execution.Plan, execution.StepIDs(), func(stepCtx context.Context, executionID, stepID string, revision int64) (any, error) {
			actor, err := service.AuthenticateMCPCredential(token, mcpConfigSnapshot().MCP, resource)
			if err != nil {
				return nil, err
			}
			if actor.User.ID != a.User.ID || actor.CredentialID != a.CredentialID {
				return nil, service.ErrMCPScopeDenied
			}
			needsAudio, audioErr := execution.StepNeedsAudio(scope, stepID)
			if audioErr != nil {
				return nil, audioErr
			}
			if needsAudio {
				if !actor.Allows("audio:write") {
					return nil, service.ErrMCPScopeDenied
				}
				if err := mcpAudioAccess(actor, scope.WorldID, scope.InputChannelID); err != nil {
					return nil, err
				}
			}
			result, err := execution.Execute(stepCtx, actor, scope, executionID, stepID, revision)
			if err != nil {
				return nil, err
			}
			// Effects are played by the selected renderer after this acknowledged step;
			// do not also broadcast them, which would execute twice on that renderer.
			if result.Clue != nil {
				broadcastWorldClueChanged(scope.WorldID, result.Clue.ClueID, "upsert", result.Clue.Revision)
				if result.Clue.PublishSeq > 0 && len(result.Clue.RecipientIDs) > 0 {
					broadcastWorldCluePublished(scope.WorldID, result.Clue.ClueID, result.Clue.PublishSeq, result.Clue.RecipientIDs)
				}
			}
			return result, nil
		})
	default:
		return nil, mcpFailure("invalid_argument", "operation 必须为 apply_scene/trigger_action/trigger_sequence/execution_status/cancel_execution")
	}
}
func mcpTheaterView(ctx context.Context, a *service.MCPActor, in mcpTheaterViewInput) (any, error) {
	if err := mcpTheaterOperationFields(in, in.Operation, map[string]string{
		"get": "rendererId sceneId", "camera_set": "rendererId sceneId camera", "fit_scene": "rendererId sceneId",
		"focus_object": "rendererId sceneId objectIds", "select_objects": "rendererId sceneId objectIds", "clear_selection": "rendererId sceneId",
	}); err != nil {
		return nil, err
	}
	switch in.Operation {
	case "get", "camera_set", "fit_scene", "focus_object", "select_objects", "clear_selection":
	default:
		return nil, mcpFailure("invalid_argument", "view operation 无效")
	}
	if in.Operation != "get" && !a.Allows("theater:control") {
		return nil, service.ErrMCPScopeDenied
	}
	snap, scope, err := mcpTheaterRuntimeTarget(ctx, a, in.TheaterScope, in.SceneID, in.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	if in.Operation == "camera_set" && in.Camera == nil {
		return nil, mcpFailure("invalid_argument", "camera 必填")
	}
	if len(in.ObjectIDs) > 200 || (in.Operation == "focus_object" && len(in.ObjectIDs) != 1) {
		return nil, mcpFailure("invalid_argument", "objectIds 数量无效")
	}
	for _, id := range in.ObjectIDs {
		if !mcpTheaterCurrentObject(snap, in.SceneID, id) {
			return nil, mcpFailure("not_found", "对象不可见")
		}
	}
	task, err := service.TheaterRenderers.Start(a.User.ID, a.CredentialID, scope, in.RendererID, in.SceneID, in.ExpectedRevision, in.Operation, map[string]any{"camera": in.Camera, "objectIds": in.ObjectIDs}, nil, nil)
	if err != nil {
		return nil, err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()
	return service.TheaterRenderers.Wait(waitCtx, a.User.ID, a.CredentialID, scope, task.RequestID)
}
func mcpTheaterCapture(ctx context.Context, a *service.MCPActor, in mcpTheaterCaptureInput) (any, error) {
	if err := mcpTheaterOperationFields(in, in.Operation, map[string]string{
		"status": "captureId", "cancel": "captureId", "capture": "rendererId sceneId mode objectId maxEdge maxBytes",
	}); err != nil {
		return nil, err
	}
	scope, err := service.NormalizeTheaterMCPScope(a.User.ID, in.TheaterScope)
	if err != nil {
		return nil, err
	}
	var task *service.TheaterRendererTask
	switch in.Operation {
	case "status":
		task, err = mcpTheaterTask(a, scope, in.CaptureID, "capture", false)
	case "cancel":
		task, err = mcpTheaterTask(a, scope, in.CaptureID, "capture", true)
	case "capture":
		snap, _, targetErr := mcpTheaterRuntimeTarget(ctx, a, scope, in.SceneID, in.ExpectedRevision)
		if targetErr != nil {
			return nil, targetErr
		}
		if in.Mode == "" {
			in.Mode = "viewport"
		}
		if in.Mode != "viewport" && in.Mode != "object" {
			return nil, mcpFailure("invalid_argument", "mode 必须为 viewport/object")
		}
		if in.Mode == "viewport" && in.ObjectID != "" {
			return nil, mcpFailure("invalid_argument", "仅 object 模式接受 objectId")
		}
		if in.Mode == "object" {
			if !mcpTheaterCurrentObject(snap, in.SceneID, in.ObjectID) {
				return nil, mcpFailure("not_found", "截图对象不存在或不可见")
			}
		}
		if in.MaxEdge == 0 {
			in.MaxEdge = 1600
		}
		if in.MaxBytes == 0 {
			in.MaxBytes = 2 << 20
		}
		if in.MaxEdge < 64 || in.MaxEdge > 1600 || in.MaxBytes < 1024 || in.MaxBytes > 2<<20 {
			return nil, mcpFailure("invalid_argument", "maxEdge 范围 64..1600；maxBytes 范围 1024..2097152")
		}
		task, err = service.TheaterRenderers.Start(a.User.ID, a.CredentialID, scope, in.RendererID, in.SceneID, in.ExpectedRevision, "capture", map[string]any{"mode": in.Mode, "objectId": in.ObjectID, "maxEdge": in.MaxEdge, "maxBytes": in.MaxBytes}, nil, nil)
	default:
		return nil, mcpFailure("invalid_argument", "operation 必须为 capture/status/cancel")
	}
	if err != nil {
		return nil, err
	}
	if task.Capture == nil {
		return map[string]any{"captureId": task.RequestID, "task": task}, nil
	}
	return mcpTheaterCaptureResult(task.Capture)
}
func mcpTheaterCurrentObject(snapshot *service.TheaterSnapshotResult, sceneID, objectID string) bool {
	if _, ok := snapshot.Snapshot.PersistentObjects[objectID]; ok {
		return true
	}
	scene, ok := snapshot.Snapshot.Scenes[sceneID]
	if !ok {
		return false
	}
	_, ok = scene.Objects[objectID]
	return ok
}
func mcpTheaterTask(a *service.MCPActor, scope service.TheaterScope, id, operation string, cancel bool) (*service.TheaterRendererTask, error) {
	t, err := service.TheaterRenderers.Status(a.User.ID, a.CredentialID, scope, id)
	if err != nil {
		return nil, err
	}
	if t.Operation() != operation {
		return nil, mcpFailure("task_not_found", "任务类型不匹配")
	}
	if cancel {
		return service.TheaterRenderers.Cancel(a.User.ID, a.CredentialID, scope, id)
	}
	return t, nil
}
func mcpTheaterCaptureResult(c *service.TheaterCaptureResult) (any, error) {
	data, err := base64.StdEncoding.DecodeString(c.Data)
	if err != nil {
		return nil, err
	}
	metadata := *c
	metadata.Data = ""
	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{StructuredContent: json.RawMessage(raw), Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}, &mcp.ImageContent{Data: data, MIMEType: c.MimeType}}}, nil
}
