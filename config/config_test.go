package config

import (
	"reflect"
	"testing"
)

func TestEnabledLanguages(t *testing.T) {
	tests := []struct {
		name string
		cnf  ModelConfig
		want []string
	}{
		{"default", ModelConfig{}, []string{"go"}},
		{"legacy language", ModelConfig{Language: "spring"}, []string{"kotlin"}},
		{"legacy comma list", ModelConfig{Language: "react, go"}, []string{"go", "react"}},
		{"toggle map", ModelConfig{Languages: map[string]bool{"go": true, "react": true, "spring": false}}, []string{"go", "react"}},
		{"map wins over language", ModelConfig{Language: "dart", Languages: map[string]bool{"go": true}}, []string{"go"}},
		{"all", ModelConfig{Languages: map[string]bool{"all": true, "go": false}}, []string{"go", "kotlin", "dart", "react", "vue"}},
		{"vue", ModelConfig{Languages: map[string]bool{"vue": true, "react": true}}, []string{"react", "vue"}},
		{"all off", ModelConfig{Languages: map[string]bool{"go": false}}, []string{}},
		{"unsupported skipped", ModelConfig{Languages: map[string]bool{"svelte": true, "flutter": true}}, []string{"dart"}},
	}
	for _, tt := range tests {
		if got := tt.cnf.EnabledLanguages(); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestResolveLanguagesArg(t *testing.T) {
	if got := ResolveLanguages([]string{"all"}); len(got) != len(SupportedLanguages) {
		t.Errorf("all: got %v", got)
	}
	if got := ResolveLanguages([]string{"Go", "go", "spring"}); !reflect.DeepEqual(got, []string{"go", "kotlin"}) {
		t.Errorf("dedupe: got %v", got)
	}
}
