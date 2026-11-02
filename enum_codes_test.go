package main

import (
	"gomachine/config"
	gocodegen "gomachine/go"
	kotlincodegen "gomachine/kotlin"
	reactcodegen "gomachine/react"
	rustcodegen "gomachine/rust"
	"gomachine/util"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// 같은 gpa.map 으로 만든 Go 상수, Kotlin enum, TypeScript 상수의 코드가 같은지 확인한다 (코드 = data 위치).
func TestEnumCodesAgree(t *testing.T) {
	views, err := filepath.Abs("views")
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

	out := t.TempDir()
	for _, d := range []string{"models", "controllers/rest", "router/routers", "kt"} {
		if err := os.MkdirAll(filepath.Join(out, d), 0755); err != nil {
			t.Fatal(err)
		}
	}

	gpa := &config.Gpa{Name: "evt", Map: []config.GpaMap{
		{Name: "status", Data: []string{"", "wait:Wait", "", "done:Done", "plain"}},
		{Name: "kind", Data: []string{"first:First", "second:Second"}},
	}}
	raws := []util.RawColumn{{Name: "e_id", DataType: "bigint"}, {Name: "e_status", DataType: "int"}, {Name: "e_kind", DataType: "int", Nullable: true}}
	columns := func(lang string) []util.Column {
		cnf := config.ModelConfig{Language: lang}
		ret := make([]util.Column, 0)
		for _, r := range raws {
			ret = append(ret, util.NewColumnFromRaw("evt_tb", r, gpa, cnf))
		}
		return ret
	}

	goCnf := config.ModelConfig{Language: "go", GoModelFilePath: out + "/"}
	gocodegen.ProcessGo("testproj", "evt_tb", "e", columns("go"), nil, gpa, "1", "", goCnf)
	ktCnf := config.ModelConfig{Language: "kotlin", KotlinModelFilePath: filepath.Join(out, "kt")}
	kotlincodegen.ProcessKotlin("testproj", "evt_tb", "e", columns("kotlin"), nil, gpa, "1", "", ktCnf)

	goSrc, err := os.ReadFile(filepath.Join(out, "models", "evt", "evt.go"))
	if err != nil {
		t.Fatal(err)
	}
	ktSrc, err := os.ReadFile(filepath.Join(out, "kt", "enums", "evt", "Enums.kt"))
	if err != nil {
		t.Fatal(err)
	}
	ts := reactcodegen.BuildTemplateData("testproj", "evt_tb", columns("react"), gpa)
	rustcodegen.ProcessRust("testproj", "evt_tb", "e", columns("rust"), nil, gpa, "1", "", config.ModelConfig{Language: "rust", RustModelFilePath: filepath.Join(out, "rust")})
	rustSrc, err := os.ReadFile(filepath.Join(out, "rust", "src", "models", "evt.rs"))
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]map[string]int{
		"Status": {"Wait": 1, "Done": 3, "plain": 4},
		"Kind":   {"First": 0, "Second": 1},
	}
	for typeName, codes := range want {
		// Go: 상수 값과 라벨 배열
		goConst := regexp.MustCompile(`(?m)^\s*`+typeName+`\w+ `+typeName+` = (\d+)$`).FindAllStringSubmatch(string(goSrc), -1)
		labels := regexp.MustCompile(`var ` + typeName + `s = \[\]string\{(.*)\}`).FindStringSubmatch(string(goSrc))
		if labels == nil {
			t.Fatalf("Go label array for %s not found:\n%s", typeName, goSrc)
		}
		goLabels := regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`).FindAllStringSubmatch(labels[1], -1)
		goCodes := map[string]int{}
		for _, m := range goConst {
			code, _ := strconv.Atoi(m[1])
			goCodes[goLabels[code][1]] = code
		}

		// Kotlin: NAME(code, "label")
		ktBlock := regexp.MustCompile(`(?s)enum class ` + typeName + `\(.*?\n;`).FindString(string(ktSrc))
		ktCodes := map[string]int{}
		for _, m := range regexp.MustCompile(`(?m)^\s+\w+\((\d+), "(.*)"\),$`).FindAllStringSubmatch(ktBlock, -1) {
			if m[2] == "" {
				continue // NONE
			}
			code, _ := strconv.Atoi(m[1])
			ktCodes[m[2]] = code
		}

		// TypeScript
		tsCodes := map[string]int{}
		for _, e := range ts.Enums {
			if util.CamelCase(e.Name) != typeName {
				continue
			}
			for _, d := range e.Data {
				if d.Key != "" {
					tsCodes[d.Label] = d.Code
				}
			}
		}

		// Rust: 연관 상수 pub const WAIT: Status = Status(1) 와 LABELS 배열
		rustLabels := regexp.MustCompile(`(?s)impl ` + typeName + ` \{.*?pub const LABELS: &'static \[&'static str\] = &\[(.*?)\];`).FindStringSubmatch(string(rustSrc))
		if rustLabels == nil {
			t.Fatalf("Rust LABELS for %s not found:\n%s", typeName, rustSrc)
		}
		rustLabelList := regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`).FindAllStringSubmatch(rustLabels[1], -1)
		rustCodes := map[string]int{}
		for _, m := range regexp.MustCompile(`(?m)^\s+pub const \w+: `+typeName+` = `+typeName+`\((\d+)\);$`).FindAllStringSubmatch(string(rustSrc), -1) {
			code, _ := strconv.Atoi(m[1])
			rustCodes[rustLabelList[code][1]] = code
		}

		for name, got := range map[string]map[string]int{"go": goCodes, "kotlin": ktCodes, "ts": tsCodes, "rust": rustCodes} {
			if len(got) != len(codes) {
				t.Errorf("%s %s codes = %v, want %v", name, typeName, got, codes)
				continue
			}
			for label, code := range codes {
				if got[label] != code {
					t.Errorf("%s %s %q = %d, want %d", name, typeName, label, got[label], code)
				}
			}
		}
	}

	// Rust 는 enum 컬럼을 newtype(util.RustEnumType) 으로 쓴다. 코드는 다른 언어와 같은 util.EnumCode 다.
	for _, c := range columns("rust") {
		want := map[string]string{"e_id": "i64", "e_status": "Status", "e_kind": "Kind"}[c.Column]
		if c.Type != want {
			t.Errorf("rust %s type = %q, want %q", c.Column, c.Type, want)
		}
	}

	// NONE 은 data[0] 이 비어 있을 때만 있다.
	if !regexp.MustCompile(`enum class Status\(.*\) \{\n\s+NONE\(0, ""\),`).Match(ktSrc) {
		t.Errorf("Status should have NONE(0):\n%s", ktSrc)
	}
	if regexp.MustCompile(`(?s)enum class Kind\(.*?NONE\(0`).Match(ktSrc) {
		t.Errorf("Kind must not have NONE:\n%s", ktSrc)
	}
}
