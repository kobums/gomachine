package config

import (
	"os"
	"path/filepath"
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
		{"all", ModelConfig{Languages: map[string]bool{"all": true, "go": false}}, []string{"go", "kotlin", "dart", "react", "vue", "rust"}},
		{"rust", ModelConfig{Languages: map[string]bool{"rust": true, "go": true}}, []string{"go", "rust"}},
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

func TestParseStrict(t *testing.T) {
	// 문법 오류 / 타입 오류는 에러다 (반쯤 읽은 설정으로 계속하지 않는다).
	for name, src := range map[string]string{
		"syntax":       `{"database": "db", "table": [}`,
		"type":         `{"database": "db", "table": [{"name": "user", "session": {"key": "id"}}]}`,
		"top-level":    `{"auth": 1}`,
		"not object":   `[1, 2]`,
		"primary type": `{"table": [{"name": "user", "primary": "id"}]}`,
	} {
		if _, _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}

	// 모르는 key 는 경고만 (위치 포함), 나머지 설정은 읽는다.
	cnf, warnings, err := Parse([]byte(`{"database": "db", "auth": "jwt", "extra": 1,
		"table": [{"name": "user", "sesion": [{"key": "id", "column": "id"}], "join": [{"name": "a", "colum": "b"}]}],
		"languages": {"go": true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if cnf.Database != "db" || cnf.Auth != "jwt" || len(cnf.Gpa) != 1 || cnf.Gpa[0].Name != "user" {
		t.Errorf("config not parsed: %+v", cnf)
	}
	want := []string{"unknown key extra (ignored)", "unknown key table[0].join[0].colum (ignored)", "unknown key table[0].sesion (ignored)"}
	if !reflect.DeepEqual(warnings, want) {
		t.Errorf("warnings = %q, want %q", warnings, want)
	}

	// 대소문자만 다른 key 는 encoding/json 처럼 받는다.
	if _, warnings, err := Parse([]byte(`{"Database": "db", "table": [{"Name": "x"}]}`)); err != nil || len(warnings) != 0 {
		t.Errorf("case-insensitive keys: %v %v", warnings, err)
	}
}

func TestLoadMissing(t *testing.T) {
	if _, _, err := Load(t.TempDir()); err == nil {
		t.Error("missing model.json: want error")
	}
}

func TestOutputPathRust(t *testing.T) {
	cnf := ModelConfig{RustModelFilePath: "../proj/proj_rust"}
	if got := cnf.OutputPath("rust"); got != "../proj/proj_rust" {
		t.Errorf("rust output path = %q", got)
	}
	if got := NormalizeLanguage(" Rust "); got != "rust" {
		t.Errorf("NormalizeLanguage(Rust) = %q", got)
	}
}

func TestGoModule(t *testing.T) {
	dir := t.TempDir()
	if got := GoModule(dir); got != "" {
		t.Errorf("no go.mod: got %q", got)
	}
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("// comment\nmodule example.com/app // x\n\ngo 1.22\n"), 0644)
	if got := GoModule(dir); got != "example.com/app" {
		t.Errorf("got %q", got)
	}
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module \"quoted/app\"\n"), 0644)
	if got := GoModule(dir); got != "quoted/app" {
		t.Errorf("quoted: got %q", got)
	}
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("modules x\n"), 0644)
	if got := GoModule(dir); got != "" {
		t.Errorf("modules: got %q", got)
	}
}

func TestResolvePaths(t *testing.T) {
	c := ModelConfig{KotlinModelFilePath: "spring/src/main/kotlin/com/x", ReactModelFilePath: "/abs/web", VueModelFilePath: ""}
	c.ResolvePaths("proj")
	if c.GoModelFilePath != "proj" {
		t.Errorf("empty go path: got %q", c.GoModelFilePath)
	}
	if c.KotlinModelFilePath != filepath.Join("proj", "spring/src/main/kotlin/com/x") {
		t.Errorf("kotlin: got %q", c.KotlinModelFilePath)
	}
	if c.ReactModelFilePath != "/abs/web" || c.VueModelFilePath != "" {
		t.Errorf("abs/empty changed: %q %q", c.ReactModelFilePath, c.VueModelFilePath)
	}
	c = ModelConfig{GoModelFilePath: "./backend/"}
	c.ResolvePaths("proj")
	if c.GoModelFilePath != filepath.Join("proj", "backend") {
		t.Errorf("go: got %q", c.GoModelFilePath)
	}
}

func TestNewOptionDefaults(t *testing.T) {
	var c ModelConfig
	if c.GetSpringBoot() != 4 || !c.UseTsSemicolons() || c.GetTsOutDir() != "src" {
		t.Errorf("defaults: boot=%d semi=%v out=%q", c.GetSpringBoot(), c.UseTsSemicolons(), c.GetTsOutDir())
	}
	off := false
	c = ModelConfig{SpringBoot: 3, TsSemicolons: &off, TsOutDir: "/src/gen/"}
	if c.GetSpringBoot() != 3 || c.UseTsSemicolons() || c.GetTsOutDir() != "src/gen" {
		t.Errorf("set: boot=%d semi=%v out=%q", c.GetSpringBoot(), c.UseTsSemicolons(), c.GetTsOutDir())
	}
	c, warnings, err := Parse([]byte(`{"springBoot": 3, "tsSemicolons": false, "tsOutDir": "src/gen"}`))
	if err != nil || len(warnings) > 0 || c.GetSpringBoot() != 3 || c.UseTsSemicolons() || c.GetTsOutDir() != "src/gen" {
		t.Errorf("parse: %v %v %+v", err, warnings, c)
	}
}
