package cmd

import (
	"testing"
)

func TestMapCmdExists(t *testing.T) {
	if mapCmd == nil {
		t.Fatal("mapCmd is nil")
	}
	if mapCmd.Use != "map" {
		t.Fatalf("expected mapCmd.Use == %q, got %q", "map", mapCmd.Use)
	}
}

func TestBootstrapAliasExists(t *testing.T) {
	if bootstrapAliasCmd == nil {
		t.Fatal("bootstrapAliasCmd is nil")
	}
	if !bootstrapAliasCmd.Hidden {
		t.Fatal("bootstrapAliasCmd should be hidden")
	}
	if bootstrapAliasCmd.Use != "bootstrap" {
		t.Fatalf("expected bootstrapAliasCmd.Use == %q, got %q", "bootstrap", bootstrapAliasCmd.Use)
	}
}

func TestMapCmdRegisteredInRoot(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "map" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("mapCmd (Use=map) not registered in rootCmd")
	}
}

func TestBootstrapAliasRegisteredInRoot(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "bootstrap" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("bootstrapAliasCmd (Use=bootstrap) not registered in rootCmd")
	}
}
