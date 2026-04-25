package watcher

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/momhq/mom/cli/internal/recorder"
)

// WindsurfAdapter parses Windsurf JSONL transcript lines.
// Windsurf writes one JSON object per line to ~/.windsurf/transcripts/{trajectory_id}.jsonl
// with the schema:
//
//	{ "status": "done", "type": "user_input",       "user_input":       { "user_response": "..." } }
//	{ "status": "done", "type": "planner_response", "planner_response": { "response": "..." } }
//	{ "status": "done", "type": "code_action",      "code_action":      { ... } }   ← skipped
//	{ "status": "done", "type": "command_action",   "command_action":   { ... } }   ← skipped
//
// Only user_input and planner_response are ingested; all other types are dropped.
type WindsurfAdapter struct{}

// NewWindsurfAdapter returns a new WindsurfAdapter.
func NewWindsurfAdapter() *WindsurfAdapter {
	return &WindsurfAdapter{}
}

func (a *WindsurfAdapter) Name() string { return "windsurf" }

// windsurfTranscriptLine is the minimal subset of a Windsurf JSONL line
// that the adapter needs to inspect.
type windsurfTranscriptLine struct {
	Type            string                  `json:"type"`
	Status          string                  `json:"status"`
	UserInput       *windsurfUserInput      `json:"user_input,omitempty"`
	PlannerResponse *windsurfPlannerResponse `json:"planner_response,omitempty"`
}

type windsurfUserInput struct {
	UserResponse string `json:"user_response"`
}

type windsurfPlannerResponse struct {
	Response string `json:"response"`
}

// ParseLine implements Adapter. It parses one JSONL line and returns a
// RawEntry if the line contains user_input or planner_response content.
func (a *WindsurfAdapter) ParseLine(line []byte, sessionID string) (recorder.RawEntry, bool) {
	line = trimLine(line)
	if len(line) == 0 {
		return recorder.RawEntry{}, false
	}

	var tl windsurfTranscriptLine
	if err := json.Unmarshal(line, &tl); err != nil {
		return recorder.RawEntry{}, false
	}

	switch tl.Type {
	case "user_input":
		if tl.UserInput == nil {
			return recorder.RawEntry{}, false
		}
		text := strings.TrimSpace(tl.UserInput.UserResponse)
		if text == "" {
			return recorder.RawEntry{}, false
		}
		return recorder.RawEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Event:     "watch-user",
			Text:      text,
			SessionID: sessionID,
		}, true

	case "planner_response":
		if tl.PlannerResponse == nil {
			return recorder.RawEntry{}, false
		}
		text := strings.TrimSpace(tl.PlannerResponse.Response)
		if text == "" {
			return recorder.RawEntry{}, false
		}
		return recorder.RawEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Event:     "watch-assistant",
			Text:      text,
			SessionID: sessionID,
		}, true

	default:
		// Drop: code_action, command_action, file-history-snapshot, hook_progress, etc.
		return recorder.RawEntry{}, false
	}
}
