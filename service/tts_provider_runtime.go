package service

import (
	"context"
	"errors"
	"io"

	"sealchat/model"
	"sealchat/pkg/ttsprovider"
	"sealchat/utils"
)

var errTTSUnsupportedProvider = errors.New("unsupported TTS provider")

// Dispatch stays at the service boundary; the leaf Client is the Aliyun adapter.
func ttsProviderSynthesize(ctx context.Context, provider utils.SpeechProviderConfig, job *model.TTSJob, snapshot TTSSnapshot, sink io.Writer) (ttsprovider.Result, error) {
	switch provider.EffectiveProviderKind() {
	case ttsprovider.ProviderAliyun:
		client := ttsprovider.Client{APIKey: provider.APIKey, SynthesisEndpoint: snapshot.Provider.SynthesisEndpoint, VoiceEndpoint: snapshot.Provider.VoiceEndpoint}
		return ttsSynthesize(ctx, &client, job, snapshot, sink)
	default:
		return ttsprovider.Result{}, errTTSUnsupportedProvider
	}
}

func ttsProviderCreateVoice(ctx context.Context, provider utils.SpeechProviderConfig, job *model.TTSJob, snapshot TTSSnapshot) (ttsprovider.VoiceResult, error) {
	switch provider.EffectiveProviderKind() {
	case ttsprovider.ProviderAliyun:
		client := ttsprovider.Client{APIKey: provider.APIKey, VoiceEndpoint: snapshot.Provider.VoiceEndpoint}
		input := ttsprovider.CreateVoiceInput{TargetModel: snapshot.Provider.Model, Prefix: "sealchat", VoicePrompt: snapshot.Description, PreviewText: snapshot.Input.Text}
		if job.Operation == "clone" {
			input.VoicePrompt, input.PreviewText = "", ""
			if snapshot.CloneLanguageHint != "" {
				input.LanguageHints = []string{snapshot.CloneLanguageHint}
			}
			input.MaxPromptAudioLength = 30
			preprocess := snapshot.ClonePreprocess
			input.EnablePreprocess = &preprocess
			// Signed public URLs are an Aliyun enrollment requirement. Future
			// adapters own their sample transport instead of inheriting it.
			var err error
			input.URL, err = TTSCloneReadURL(job)
			if err != nil {
				return ttsprovider.VoiceResult{}, err
			}
		}
		return client.CreateVoice(ctx, input)
	default:
		return ttsprovider.VoiceResult{}, errTTSUnsupportedProvider
	}
}

func ttsProviderQueryVoice(ctx context.Context, provider utils.SpeechProviderConfig, id string) (ttsprovider.VoiceResult, error) {
	switch provider.EffectiveProviderKind() {
	case ttsprovider.ProviderAliyun:
		client := ttsprovider.Client{APIKey: provider.APIKey, VoiceEndpoint: provider.VoiceEndpoint}
		return client.QueryVoice(ctx, id)
	default:
		return ttsprovider.VoiceResult{}, errTTSUnsupportedProvider
	}
}

func ttsProviderDeleteVoice(ctx context.Context, provider utils.SpeechProviderConfig, id string) (ttsprovider.VoiceResult, error) {
	switch provider.EffectiveProviderKind() {
	case ttsprovider.ProviderAliyun:
		client := ttsprovider.Client{APIKey: provider.APIKey, VoiceEndpoint: provider.VoiceEndpoint}
		return client.DeleteVoice(ctx, id)
	default:
		return ttsprovider.VoiceResult{}, errTTSUnsupportedProvider
	}
}

func ttsProviderAdoptAudio(ctx context.Context, provider utils.SpeechProviderConfig, spoolPath, audioURL string) string {
	switch provider.EffectiveProviderKind() {
	case ttsprovider.ProviderAliyun:
		client := ttsprovider.Client{APIKey: provider.APIKey}
		return ttsAdoptProviderAudio(ctx, &client, spoolPath, audioURL)
	default:
		return "provider_audio_download_failed"
	}
}

func ttsSynthesize(ctx context.Context, client *ttsprovider.Client, job *model.TTSJob, snapshot TTSSnapshot, sink io.Writer) (ttsprovider.Result, error) {
	if job.Operation == "message_synthesis" {
		segments := TTSSplitText(snapshot.Input.Text)
		if len(segments) > 1 {
			return client.SynthesizeSegments(ctx, snapshot.Provider.Model, snapshot.Input, segments, sink)
		}
	}
	return client.Synthesize(ctx, snapshot.Provider.Model, snapshot.Input, sink)
}
