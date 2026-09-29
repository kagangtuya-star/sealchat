package ttsprovider

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/fasthttp/websocket"
)

const (
	aliyunWSPath                = "/api-ws/v1/inference"
	aliyunWSCompletionLookahead = 20 * time.Millisecond
)

var errAliyunWSProtocol = errors.New("tts websocket protocol violation")

type aliyunWSHeader struct {
	Action     string `json:"action,omitempty"`
	TaskID     string `json:"task_id"`
	Streaming  string `json:"streaming,omitempty"`
	Event      string `json:"event,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Attributes *struct {
		RequestUUID string `json:"request_uuid"`
	} `json:"attributes,omitempty"`
}

type aliyunWSParameters struct {
	TextType    string  `json:"text_type"`
	Voice       string  `json:"voice"`
	Format      string  `json:"format,omitempty"`
	SampleRate  int     `json:"sample_rate,omitempty"`
	BitRate     int     `json:"bit_rate,omitempty"`
	Instruction string  `json:"instruction,omitempty"`
	Rate        float64 `json:"rate"`
	Pitch       float64 `json:"pitch"`
	Volume      int     `json:"volume"`
	Seed        int     `json:"seed"`
}

type aliyunWSServerEvent struct {
	Header  aliyunWSHeader `json:"header"`
	Payload struct {
		Output struct {
			Type string `json:"type"`
		} `json:"output"`
		Usage *struct {
			Characters *int64 `json:"characters"`
		} `json:"usage"`
	} `json:"payload"`
}

// SynthesizeSegments sends every segment through one Qwen-Audio-TTS duplex
// synthesis task. Binary frames form one task-level audio stream and are copied
// to sink in their provider order.
func (c *Client) SynthesizeSegments(ctx context.Context, model string, input Input, segments []string, sink io.Writer) (Result, error) {
	if len(segments) < 2 || strings.Join(segments, "") != input.Text {
		return Result{}, errors.New("tts segments do not match input text")
	}
	total := 0
	for _, segment := range segments {
		n := utf8.RuneCountInString(segment)
		if n == 0 || n > 20000 {
			return Result{}, errors.New("tts segment length is invalid")
		}
		total += n
		if total > 200000 {
			return Result{}, errors.New("tts segmented input is too long")
		}
	}

	endpoint, err := aliyunWebSocketEndpoint(c.SynthesisEndpoint)
	if err != nil {
		return Result{}, err
	}
	taskID, err := aliyunTaskID()
	if err != nil {
		return Result{}, err
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
	}

	dialer := *websocket.DefaultDialer
	if c.HTTP != nil {
		if c.HTTP.Timeout > 0 && c.HTTP.Timeout < dialer.HandshakeTimeout {
			dialer.HandshakeTimeout = c.HTTP.Timeout
		}
		if transport, ok := c.HTTP.Transport.(*http.Transport); ok {
			dialer.NetDialContext = transport.DialContext
			dialer.Proxy = transport.Proxy
			if transport.TLSClientConfig != nil {
				dialer.TLSClientConfig = transport.TLSClientConfig.Clone()
			}
		}
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.APIKey)
	conn, response, err := dialer.DialContext(ctx, endpoint, header)
	if err != nil {
		if response != nil {
			defer response.Body.Close()
			return Result{}, readProviderError(response)
		}
		return Result{}, err
	}
	defer conn.Close()
	conn.SetReadLimit(MaxAudioBytes + 1)
	cancelWatch := make(chan struct{})
	defer close(cancelWatch)
	var writeMu sync.Mutex
	var taskStarted atomic.Bool
	var cancelSent sync.Once
	writeJSON := func(value any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(value)
	}
	sendCancel := func() {
		cancelSent.Do(func() {
			if !taskStarted.Load() {
				return
			}
			cancel := struct {
				Header  aliyunWSHeader `json:"header"`
				Payload struct {
					Input struct {
						Directive string `json:"directive"`
					} `json:"input"`
				} `json:"payload"`
			}{}
			cancel.Header = aliyunWSHeader{Action: "finish-task", TaskID: taskID, Streaming: "duplex"}
			cancel.Payload.Input.Directive = "cancel"
			_ = writeJSON(cancel)
		})
	}
	go func() {
		select {
		case <-ctx.Done():
			sendCancel()
			_ = conn.Close()
		case <-cancelWatch:
		}
	}()

	run := struct {
		Header  aliyunWSHeader `json:"header"`
		Payload struct {
			TaskGroup  string             `json:"task_group"`
			Task       string             `json:"task"`
			Function   string             `json:"function"`
			Model      string             `json:"model"`
			Input      struct{}           `json:"input"`
			Parameters aliyunWSParameters `json:"parameters"`
		} `json:"payload"`
	}{}
	run.Header = aliyunWSHeader{Action: "run-task", TaskID: taskID, Streaming: "duplex"}
	run.Payload.TaskGroup = "audio"
	run.Payload.Task = "tts"
	run.Payload.Function = "SpeechSynthesizer"
	run.Payload.Model = model
	run.Payload.Parameters = aliyunWSParameters{
		TextType: "PlainText", Voice: input.Voice, Format: input.Format,
		SampleRate: input.SampleRate, BitRate: input.BitRate,
		Instruction: input.Instruction, Rate: input.Rate, Pitch: input.Pitch,
		Volume: input.Volume, Seed: input.Seed,
	}
	if err = writeJSON(run); err != nil {
		return Result{}, aliyunWSContextError(ctx, err)
	}

	var result Result
	started, err := aliyunWaitForTaskStart(ctx, conn, taskID, &result)
	if err != nil {
		return result, err
	}
	if !started {
		return result, ErrIncomplete
	}
	taskStarted.Store(true)

	writeDone := make(chan error, 1)
	sessionDone := make(chan struct{})
	defer close(sessionDone)
	go func() {
		for _, segment := range segments {
			select {
			case <-ctx.Done():
				writeDone <- ctx.Err()
				return
			case <-sessionDone:
				return
			default:
			}
			message := struct {
				Header  aliyunWSHeader `json:"header"`
				Payload struct {
					Input struct {
						Text string `json:"text"`
					} `json:"input"`
				} `json:"payload"`
			}{}
			message.Header = aliyunWSHeader{Action: "continue-task", TaskID: taskID, Streaming: "duplex"}
			message.Payload.Input.Text = segment
			if writeErr := writeJSON(message); writeErr != nil {
				writeDone <- aliyunWSContextError(ctx, writeErr)
				_ = conn.Close()
				return
			}
		}
		select {
		case <-ctx.Done():
			writeDone <- ctx.Err()
			return
		case <-sessionDone:
			return
		default:
		}
		finish := struct {
			Header  aliyunWSHeader `json:"header"`
			Payload struct {
				Input struct{} `json:"input"`
			} `json:"payload"`
		}{}
		finish.Header = aliyunWSHeader{Action: "finish-task", TaskID: taskID, Streaming: "duplex"}
		writeErr := writeJSON(finish)
		writeDone <- aliyunWSContextError(ctx, writeErr)
		if writeErr != nil {
			_ = conn.Close()
		}
	}()

	pendingAudio := false
	for {
		kind, data, readErr := conn.ReadMessage()
		if readErr != nil {
			select {
			case writeErr := <-writeDone:
				if writeErr != nil {
					return result, writeErr
				}
			default:
			}
			return result, aliyunWSContextError(ctx, readErr)
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		switch kind {
		case websocket.BinaryMessage:
			if !pendingAudio {
				return result, errAliyunWSProtocol
			}
			pendingAudio = false
			if result.Bytes+int64(len(data)) > MaxAudioBytes {
				return result, errors.New("tts audio exceeds spool limit")
			}
			n, writeErr := sink.Write(data)
			result.Bytes += int64(n)
			if writeErr != nil {
				return result, writeErr
			}
			if n != len(data) {
				return result, io.ErrShortWrite
			}
		case websocket.TextMessage:
			var event aliyunWSServerEvent
			if json.Unmarshal(data, &event) != nil || event.Header.TaskID != taskID {
				return result, errAliyunWSProtocol
			}
			aliyunUpdateWSResult(&result, event)
			switch event.Header.Event {
			case "result-generated":
				switch event.Payload.Output.Type {
				case "sentence-begin", "sentence-end":
				case "sentence-synthesis":
					if pendingAudio {
						return result, errAliyunWSProtocol
					}
					pendingAudio = true
				default:
					return result, errAliyunWSProtocol
				}
			case "task-failed":
				return result, &ProviderError{Code: event.Header.ErrorCode, RequestID: result.RequestID}
			case "task-finished":
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				if pendingAudio {
					return result, errAliyunWSProtocol
				}
				result.UsageConfirmed = event.Payload.Usage != nil && event.Payload.Usage.Characters != nil && *event.Payload.Usage.Characters >= 0
				select {
				case writeErr := <-writeDone:
					if writeErr != nil {
						if ctx.Err() != nil {
							result.UsageConfirmed = false
						}
						return result, writeErr
					}
				case <-ctx.Done():
					result.UsageConfirmed = false
					return result, ctx.Err()
				}
				result.Complete = true
				return aliyunConfirmNoTrailingFrame(ctx, conn, result)
			default:
				return result, errAliyunWSProtocol
			}
		default:
			return result, errAliyunWSProtocol
		}
	}
}

func aliyunWaitForTaskStart(ctx context.Context, conn *websocket.Conn, taskID string, result *Result) (bool, error) {
	for {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return false, aliyunWSContextError(ctx, err)
		}
		if kind != websocket.TextMessage {
			return false, errAliyunWSProtocol
		}
		var event aliyunWSServerEvent
		if json.Unmarshal(data, &event) != nil || event.Header.TaskID != taskID {
			return false, errAliyunWSProtocol
		}
		aliyunUpdateWSResult(result, event)
		switch event.Header.Event {
		case "task-started":
			return true, nil
		case "task-failed":
			return false, &ProviderError{Code: event.Header.ErrorCode, RequestID: result.RequestID}
		default:
			return false, errAliyunWSProtocol
		}
	}
}

func aliyunUpdateWSResult(result *Result, event aliyunWSServerEvent) {
	if event.Header.Attributes != nil && event.Header.Attributes.RequestUUID != "" {
		result.RequestID = event.Header.Attributes.RequestUUID
	}
	if event.Payload.Usage != nil && event.Payload.Usage.Characters != nil && *event.Payload.Usage.Characters >= 0 && *event.Payload.Usage.Characters > result.Characters {
		result.Characters = *event.Payload.Usage.Characters
	}
}

func aliyunConfirmNoTrailingFrame(ctx context.Context, conn *websocket.Conn, result Result) (Result, error) {
	if ctx.Err() != nil {
		result.Complete = false
		result.UsageConfirmed = false
		return result, ctx.Err()
	}
	if err := conn.SetReadDeadline(time.Now().Add(aliyunWSCompletionLookahead)); err != nil {
		if ctx.Err() != nil {
			result.Complete = false
			result.UsageConfirmed = false
			return result, ctx.Err()
		}
		return result, err
	}
	_, _, err := conn.ReadMessage()
	if err == nil {
		return result, errors.New("tts data after terminal event")
	}
	if ctx.Err() != nil {
		result.Complete = false
		result.UsageConfirmed = false
		return result, ctx.Err()
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() || websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) || errors.Is(err, io.EOF) {
		return result, nil
	}
	// The successful task-finished event is authoritative; a connection-level
	// close after it cannot make the completed task safe to retry.
	return result, nil
}

func aliyunWSContextError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func aliyunWebSocketEndpoint(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil {
		return "", errors.New("tts websocket endpoint is invalid")
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "wss", "ws":
	default:
		return "", errors.New("tts websocket endpoint is invalid")
	}
	u.Path = aliyunWSPath
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func aliyunTaskID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
