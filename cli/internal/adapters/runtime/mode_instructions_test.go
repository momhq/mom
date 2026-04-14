package runtime

import (
	"strings"
	"testing"
)

// LanguageInstructions tests

func TestLanguageInstructions_English(t *testing.T) {
	result := LanguageInstructions("en")
	if !strings.Contains(result, "English") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "English", result)
	}
}

func TestLanguageInstructions_Portuguese(t *testing.T) {
	result := LanguageInstructions("pt")
	if !strings.Contains(result, "Português") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "Português", result)
	}
}

func TestLanguageInstructions_Spanish(t *testing.T) {
	result := LanguageInstructions("es")
	if !strings.Contains(result, "Español") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "Español", result)
	}
}

func TestLanguageInstructions_UnknownFallsBackToEnglish(t *testing.T) {
	result := LanguageInstructions("zz")
	if !strings.Contains(result, "English") {
		t.Errorf("expected unknown language to fall back to English, got:\n%s", result)
	}
}

// ModeInstructions tests

func TestModeInstructions_Verbose(t *testing.T) {
	result := ModeInstructions("verbose")
	if !strings.Contains(result, "Verbose") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "Verbose", result)
	}
	if !strings.Contains(result, "step by step") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "step by step", result)
	}
}

func TestModeInstructions_Concise(t *testing.T) {
	result := ModeInstructions("concise")
	if !strings.Contains(result, "Concise") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "Concise", result)
	}
	if !strings.Contains(result, "no filler") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "no filler", result)
	}
}

func TestModeInstructions_Caveman(t *testing.T) {
	result := ModeInstructions("caveman")
	if !strings.Contains(result, "Caveman") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "Caveman", result)
	}
	if !strings.Contains(result, "No articles") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "No articles", result)
	}
}

func TestModeInstructions_UnknownFallsBackToConcise(t *testing.T) {
	result := ModeInstructions("unknown")
	if !strings.Contains(result, "Concise") {
		t.Errorf("expected unknown mode to fall back to Concise, got:\n%s", result)
	}
}

// AutonomyInstructions tests

func TestAutonomyInstructions_Autonomous(t *testing.T) {
	result := AutonomyInstructions("autonomous")
	if !strings.Contains(result, "Autonomous") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "Autonomous", result)
	}
	if !strings.Contains(result, "Act independently") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "Act independently", result)
	}
}

func TestAutonomyInstructions_Balanced(t *testing.T) {
	result := AutonomyInstructions("balanced")
	if !strings.Contains(result, "Balanced") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "Balanced", result)
	}
	if !strings.Contains(result, "Propose before") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "Propose before", result)
	}
}

func TestAutonomyInstructions_Supervised(t *testing.T) {
	result := AutonomyInstructions("supervised")
	if !strings.Contains(result, "Supervised") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "Supervised", result)
	}
	if !strings.Contains(result, "Confirm every") {
		t.Errorf("expected instructions to contain %q, got:\n%s", "Confirm every", result)
	}
}

func TestAutonomyInstructions_UnknownFallsBackToBalanced(t *testing.T) {
	result := AutonomyInstructions("unknown")
	if !strings.Contains(result, "Balanced") {
		t.Errorf("expected unknown autonomy to fall back to Balanced, got:\n%s", result)
	}
}
