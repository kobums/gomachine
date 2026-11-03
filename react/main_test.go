package react

import (
	"gomachine/config"
	"gomachine/util"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupHome 은 ../views 를 가리키는 HOME 을 만든다 (템플릿은 $HOME/bin/buildtool 에서 읽는다).
func setupHome(t *testing.T) string {
	t.Helper()
	views, err := filepath.Abs("../views")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(views, filepath.Join(home, "bin", "buildtool")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	return t.TempDir()
}

func noteColumns(cnf config.ModelConfig) []util.Column {
	raws := []util.RawColumn{{Name: "n_id", DataType: "bigint"}, {Name: "n_title", DataType: "varchar"}, {Name: "n_memo", DataType: "varchar", Nullable: true}}
	ret := make([]util.Column, 0)
	for _, r := range raws {
		ret = append(ret, util.NewColumnFromRaw("note_tb", r, nil, cnf))
	}
	return ret
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestReactOutput(t *testing.T) {
	out := setupHome(t)
	cnf := config.ModelConfig{Language: "react", ReactModelFilePath: out}
	ProcessReact("p", "note_tb", "n", noteColumns(cnf), nil, nil, "1", "", cnf)

	for _, p := range []string{"models/note.ts", "models/contract.ts", "types/note.ts", "hooks/useNote.ts"} {
		src := readFile(t, filepath.Join(out, "src", p))
		if !strings.HasPrefix(src, util.GeneratedHeader+"\n") {
			t.Errorf("%s does not start with the generated header", p)
		}
	}
	types := readFile(t, filepath.Join(out, "src", "types", "note.ts"))
	if !strings.Contains(types, "\n  id: number;\n") {
		t.Errorf("entity id must be required:\n%s", types)
	}
	if !strings.Contains(types, "CreateNoteRequest = Omit<Note, 'id'> & Partial<Pick<Note, 'id'>>;") {
		t.Errorf("create request must make id optional:\n%s", types)
	}
	hook := readFile(t, filepath.Join(out, "src", "hooks", "useNote.ts"))
	for _, want := range []string{"import { useCallback, useState } from 'react';", "export function useNote()", "fetchOne", "remove"} {
		if !strings.Contains(hook, want) {
			t.Errorf("hook missing %q", want)
		}
	}
	api := readFile(t, filepath.Join(out, "src", "services", "api.ts"))
	if util.IsGeneratedContent(api) || !strings.Contains(api, "export function setTokenProvider(") || !strings.Contains(api, "params?: object;") {
		t.Errorf("unexpected api.ts:\n%s", api)
	}
}

// api.ts 는 사용자 파일이라 없을 때만 만든다. 생성 파일도 머리줄이 없는 사용자 파일이면 덮어쓰지 않는다.
func TestWriteIfMissingAndProtect(t *testing.T) {
	out := setupHome(t)
	cnf := config.ModelConfig{Language: "vue", VueModelFilePath: out}
	apiPath := filepath.Join(out, "src", "services", "api.ts")
	mine := "// my api\nexport const get = 1\n"
	if err := os.MkdirAll(filepath.Dir(apiPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(apiPath, []byte(mine), 0644); err != nil {
		t.Fatal(err)
	}
	typePath := filepath.Join(out, "src", "types", "note.ts")
	if err := os.MkdirAll(filepath.Dir(typePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(typePath, []byte(mine), 0644); err != nil {
		t.Fatal(err)
	}
	// 예전 머리줄(Auto-generated)이 있는 파일은 생성 파일로 보고 덮어쓴다.
	legacyPath := filepath.Join(out, "src", "composables", "useNote.ts")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("// Auto-generated Vue 3 composable for Note\n"), 0644); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		renderVue(t, cnf)
	}
	if got := readFile(t, apiPath); got != mine {
		t.Errorf("api.ts overwritten:\n%s", got)
	}
	if got := readFile(t, typePath); got != mine {
		t.Errorf("hand-written types/note.ts overwritten:\n%s", got)
	}
	if got := readFile(t, legacyPath); !strings.HasPrefix(got, util.GeneratedHeader) {
		t.Errorf("legacy generated composable not regenerated:\n%s", got)
	}
}

// renderVue 는 vue 패키지를 import 하지 않고 같은 Render 호출을 한다 (vue → react import 순환 방지).
func renderVue(t *testing.T, cnf config.ModelConfig) {
	t.Helper()
	data := BuildTemplateData("p", "note_tb", noteColumns(cnf), nil)
	views := newViews(filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "vue"))
	out := OutDir(cnf.VueModelFilePath, cnf)
	Render(views, "type.jet", filepath.Join(out, "types"), "note.ts", data, true)
	Render(views, "composable.jet", filepath.Join(out, "composables"), "useNote.ts", data, true)
	RenderAPI(views, out, true)
}

func TestTsOptions(t *testing.T) {
	out := setupHome(t)
	no := false
	cnf := config.ModelConfig{Language: "react", ReactModelFilePath: out, TsSemicolons: &no, TsOutDir: "src/gen"}
	ProcessReact("p", "note_tb", "n", noteColumns(cnf), nil, nil, "1", "", cnf)
	for _, p := range []string{"models/note.ts", "models/contract.ts", "types/note.ts", "hooks/useNote.ts", "services/api.ts"} {
		src := readFile(t, filepath.Join(out, "src", "gen", p))
		for i, line := range strings.Split(src, "\n") {
			if strings.HasSuffix(line, ";") && !strings.HasPrefix(strings.TrimSpace(line), "//") {
				t.Errorf("%s:%d keeps a semicolon: %s", p, i+1, line)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(out, "src", "models")); !os.IsNotExist(err) {
		t.Errorf("files written outside tsOutDir")
	}
}

func TestStripSemicolons(t *testing.T) {
	src := "const a = 1;\n// c;\n(b as X).y();\nlet c = 2;\n/** doc */\nexport type T = 1;\n"
	want := "const a = 1;\n// c;\n(b as X).y()\nlet c = 2\n/** doc */\nexport type T = 1\n"
	if got := StripSemicolons(src); got != want {
		t.Errorf("StripSemicolons =\n%s\nwant\n%s", got, want)
	}
}
