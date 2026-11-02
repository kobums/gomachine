package main

import (
	"gomachine/config"
	gocodegen "gomachine/go"
	kotlincodegen "gomachine/kotlin"
	reactcodegen "gomachine/react"
	rustcodegen "gomachine/rust"
	"gomachine/util"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// passwd 와 password 컬럼이 같이 있는 테이블: 두 컬럼 모두 응답(조인 extra 포함)에서 빠지고, 목록 필터 / 검색 / 정렬에서 빠지고,
// 저장할 때 해시한다 (Go, Spring, Rust). TypeScript 필터도 같다.
func TestPasswordColumnsHidden(t *testing.T) {
	out := setupOut(t)
	userRaws := []util.RawColumn{
		{Name: "u_id", DataType: "bigint"},
		{Name: "u_email", DataType: "varchar"},
		{Name: "u_passwd", DataType: "varchar"},
		{Name: "u_password", DataType: "varchar"},
		{Name: "u_level", DataType: "int"},
		{Name: "u_createddate", DataType: "datetime"},
	}
	postRaws := []util.RawColumn{
		{Name: "p_id", DataType: "bigint"},
		{Name: "p_user", DataType: "bigint"},
		{Name: "p_title", DataType: "varchar"},
	}
	util.StaticColumns["user_tb"] = userRaws
	t.Cleanup(func() { delete(util.StaticColumns, "user_tb") })
	userGpa := &config.Gpa{Name: "user"}
	postGpa := &config.Gpa{Name: "post", Join: []config.GpaJoin{{Name: "user", Column: "user"}}}
	columns := func(table string, raws []util.RawColumn, gpa *config.Gpa, lang string) []util.Column {
		ret := make([]util.Column, 0)
		for _, r := range raws {
			ret = append(ret, util.NewColumnFromRaw(table, r, gpa, config.ModelConfig{Language: lang}))
		}
		return ret
	}
	gen := func(table string, prefix string, raws []util.RawColumn, gpa *config.Gpa) {
		gocodegen.ProcessGo("testproj", table, prefix, columns(table, raws, gpa, "go"), nil, gpa, "1", "jwt", config.ModelConfig{Language: "go", GoModelFilePath: out + "/"})
		kotlincodegen.ProcessKotlin("testproj", table, prefix, columns(table, raws, gpa, "kotlin"), nil, gpa, "1", "jwt", config.ModelConfig{Language: "kotlin", KotlinModelFilePath: filepath.Join(out, "kt")})
		rustcodegen.ProcessRust("testproj", table, prefix, columns(table, raws, gpa, "rust"), nil, gpa, "1", "jwt", config.ModelConfig{Language: "rust", RustModelFilePath: filepath.Join(out, "rust")})
	}
	gen("user_tb", "u", userRaws, userGpa)
	gen("post_tb", "p", postRaws, postGpa)
	ts := reactcodegen.BuildTemplateData("testproj", "user_tb", columns("user_tb", userRaws, userGpa, "react"), userGpa)

	// 목록 필터 / 검색 경로에 비밀번호가 없다
	wantFilters := "email,level,startcreateddate,endcreateddate"
	for lang, got := range listFilters(t, out, "user_tb", userGpa, ts) {
		if strings.Join(got, ",") != wantFilters {
			t.Errorf("%s filters = %v, want %s", lang, got, wantFilters)
		}
	}
	router := readOut(t, out, "router", "routers", "user.go")
	ctl := readOut(t, out, "kt", "controller", "UserController.kt")
	rustHandler := readOut(t, out, "rust", "src", "handlers", "user.rs")
	for name, src := range map[string]string{"go router": router, "spring controller": ctl, "rust handler": rustHandler} {
		if regexp.MustCompile(`/search/passw`).MatchString(src) {
			t.Errorf("%s has a password search path", name)
		}
	}

	// Go: 출력 구조체의 두 컬럼 모두 json:"-", 정렬 목록에 없음, Insert / Insertbatch / Update 에서 둘 다 해시
	goModel := readOut(t, out, "models", "user.go")
	goStruct := regexp.MustCompile(`(?s)type User struct \{(.*?)\n\}`).FindStringSubmatch(goModel)
	if goStruct == nil {
		t.Fatalf("Go struct not found:\n%s", goModel)
	}
	for _, f := range []string{"Passwd", "Password"} {
		if !regexp.MustCompile(`\n\s*` + f + `\s+string\s+` + "`json:\"-\"`").MatchString(goStruct[1]) {
			t.Errorf("Go User.%s is not hidden:\n%s", f, goStruct[1])
		}
	}
	goOrder := regexp.MustCompile(`(?s)var orderColumnsUser = map\[string\]string\{(.*?)\n\}`).FindStringSubmatch(goModel)
	if goOrder == nil || strings.Contains(goOrder[1], "passw") {
		t.Errorf("Go order columns include a password: %v", goOrder)
	}
	goRest := readOut(t, out, "controllers", "rest", "user.go")
	for _, f := range []string{"Passwd", "Password"} {
		for _, expr := range []string{"jwt.GeneratePasswd(item." + f + ")", "jwt.GeneratePasswd((*item)[i]." + f + ")", "jwt.GeneratePasswd(merged." + f + ")"} {
			if !strings.Contains(goRest, expr) {
				t.Errorf("Go rest does not hash %s: %s", f, expr)
			}
		}
	}

	// Spring: 응답 클래스에 없고, 정렬 이름에 없고, 저장할 때 해시
	kt := readOut(t, out, "kt", "entity", "User.kt")
	resp := regexp.MustCompile(`(?s)data class UserResponse\((.*?)\n\s*val extra`).FindStringSubmatch(kt)
	if resp == nil {
		t.Fatalf("Kotlin response not found:\n%s", kt)
	}
	if strings.Contains(strings.ToLower(resp[1]), "passw") {
		t.Errorf("Kotlin UserResponse has a password:\n%s", resp[1])
	}
	sorts := regexp.MustCompile(`(?s)SORT_FIELDS = mapOf\((.*?)\n\s*\)`).FindStringSubmatch(ctl)
	if sorts == nil || strings.Contains(sorts[1], "passw") {
		t.Errorf("Spring sort fields include a password: %v", sorts)
	}
	service := readOut(t, out, "kt", "service", "UserService.kt")
	for _, f := range []string{"passwd", "password"} {
		if !strings.Contains(service, "request."+f+")") && !strings.Contains(service, "request."+f+"?.takeIf") {
			t.Errorf("Spring service does not hash %s", f)
		}
	}

	// Rust: 두 필드 모두 skip_serializing, 정렬 이름에 없음, 저장할 때 해시, 조인 extra 에서도 숨김
	rustModel := readOut(t, out, "rust", "src", "models", "user.rs")
	for _, f := range []string{"passwd", "password"} {
		if !strings.Contains(rustModel, `#[serde(rename = "`+f+`", skip_serializing)]`) {
			t.Errorf("Rust User.%s is not hidden", f)
		}
		if strings.Contains(rustModel, `"`+f+`" |`) || strings.Contains(rustModel, `"u_`+f+`" =>`) {
			t.Errorf("Rust order columns include %s", f)
		}
		if !strings.Contains(rustHandler, "item."+f+" = password::generate_passwd_blocking(") || !strings.Contains(rustHandler, "merged."+f+" = password::generate_passwd_blocking(") {
			t.Errorf("Rust handler does not hash %s", f)
		}
	}
	rustPost := readOut(t, out, "rust", "src", "models", "post.rs")
	join := regexp.MustCompile(`(?s)fn join_user\(.*?\n\}`).FindString(rustPost)
	if join == "" {
		t.Fatalf("Rust join_user not found:\n%s", rustPost)
	}
	for _, f := range []string{"passwd", "password"} {
		if regexp.MustCompile(`m\.insert\("` + f + `"`).MatchString(join) {
			t.Errorf("Rust joined extra includes %s:\n%s", f, join)
		}
	}
}
