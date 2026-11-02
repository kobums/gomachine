package main

import (
	"gomachine/config"
	dartcodegen "gomachine/dart"
	gocodegen "gomachine/go"
	kotlincodegen "gomachine/kotlin"
	reactcodegen "gomachine/react"
	rustcodegen "gomachine/rust"
	"gomachine/util"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// 같은 컬럼으로 만든 Go json 태그, Kotlin 응답 프로퍼티, TypeScript key, Dart json key 가 같은지,
// 목록 필터 / 검색 이름이 Go, Spring, TypeScript 에서 같은지 확인한다.
func TestJSONKeysAgree(t *testing.T) {
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
	for _, d := range []string{"models", "controllers/rest", "router/routers", "kt", "dart"} {
		if err := os.MkdirAll(filepath.Join(out, d), 0755); err != nil {
			t.Fatal(err)
		}
	}

	gpa := &config.Gpa{Name: "acct", Map: []config.GpaMap{{Name: "status", Data: []string{"", "use:Use"}}}}
	raws := []util.RawColumn{
		{Name: "a_id", DataType: "bigint"},
		{Name: "a_loginid", DataType: "varchar"},      // 한 단어
		{Name: "a_display_name", DataType: "varchar"}, // 여러 단어
		{Name: "a_userId", DataType: "bigint"},        // camelCase 컬럼
		{Name: "a_status", DataType: "int"},           // enum
		{Name: "a_created_at", DataType: "datetime"},  // 여러 단어 날짜
		{Name: "a_createddate", DataType: "datetime"}, // 한 단어 날짜
	}
	want := []string{"id", "loginid", "displayName", "userid", "status", "createdAt", "createddate"}
	columns := func(lang string) []util.Column {
		cnf := config.ModelConfig{Language: lang}
		ret := make([]util.Column, 0)
		for _, r := range raws {
			ret = append(ret, util.NewColumnFromRaw("acct_tb", r, gpa, cnf))
		}
		return ret
	}

	gocodegen.ProcessGo("testproj", "acct_tb", "a", columns("go"), nil, gpa, "1", "", config.ModelConfig{Language: "go", GoModelFilePath: out + "/"})
	kotlincodegen.ProcessKotlin("testproj", "acct_tb", "a", columns("kotlin"), nil, gpa, "1", "", config.ModelConfig{Language: "kotlin", KotlinModelFilePath: filepath.Join(out, "kt")})
	dartcodegen.ProcessDart("testproj", "acct_tb", "a", columns("dart"), nil, gpa, "1", "", config.ModelConfig{Language: "dart", DartModelFilePath: filepath.Join(out, "dart") + "/"})
	ts := reactcodegen.BuildTemplateData("testproj", "acct_tb", columns("react"), gpa)
	rustcodegen.ProcessRust("testproj", "acct_tb", "a", columns("rust"), nil, gpa, "1", "", config.ModelConfig{Language: "rust", RustModelFilePath: filepath.Join(out, "rust")})

	read := func(path ...string) string {
		b, err := os.ReadFile(filepath.Join(append([]string{out}, path...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	all := func(re string, src string) []string {
		ret := make([]string, 0)
		for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(src, -1) {
			ret = append(ret, m[1])
		}
		return ret
	}
	check := func(name string, got []string, want []string) {
		t.Helper()
		g := append([]string{}, got...)
		w := append([]string{}, want...)
		sort.Strings(g)
		sort.Strings(w)
		if strings.Join(g, ",") != strings.Join(w, ",") {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}

	// JSON key
	goModel := read("models", "acct.go")
	goStruct := regexp.MustCompile(`(?s)type Acct struct \{(.*?)\n\}`).FindStringSubmatch(goModel)
	if goStruct == nil {
		t.Fatalf("Go struct not found:\n%s", goModel)
	}
	goKeys := all("`json:\"([^\"]+)\"`", goStruct[1])
	check("go json tags", goKeys[:len(goKeys)-1], want) // 마지막은 extra

	kt := read("kt", "entity", "Acct.kt")
	resp := regexp.MustCompile(`(?s)data class AcctResponse\((.*?)\n\s*val extra`).FindStringSubmatch(kt)
	if resp == nil {
		t.Fatalf("Kotlin response not found:\n%s", kt)
	}
	check("kotlin response properties", all(`(?m)^\s+val (\w+):`, resp[1]), want)

	tsKeys := make([]string, 0)
	for _, c := range ts.Columns {
		tsKeys = append(tsKeys, c.Key)
	}
	check("ts keys", tsKeys, want)

	// Rust: struct 필드의 serde rename (extra 는 rename 없음)
	rustModel := read("rust", "src", "models", "acct.rs")
	rustStruct := regexp.MustCompile(`(?s)pub struct Acct \{(.*?)\n\}`).FindStringSubmatch(rustModel)
	if rustStruct == nil {
		t.Fatalf("Rust struct not found:\n%s", rustModel)
	}
	check("rust serde rename", all(`#\[serde\(rename = "(\w+)"`, rustStruct[1]), want)

	dart := read("dart", "acct.dart")
	check("dart fromJson keys", unique(all(`json\['(\w+)'\]`, strings.Split(dart, "factory")[1][:strings.Index(strings.Split(dart, "factory")[1], "extra:")])), want)

	// 목록 필터 이름: Go Index, Spring @RequestParam, TypeScript SearchParams
	wantFilters := []string{"loginid", "displayName", "userid", "status", "startcreatedat", "endcreatedat", "startcreateddate", "endcreateddate"}
	for lang, got := range listFilters(t, out, "acct_tb", gpa, ts) {
		check(lang+" filters", got, wantFilters)
	}
	ctl := read("kt", "controller", "AcctController.kt")

	// 검색 경로: Go 라우터와 Spring 컨트롤러가 같은 /search/{field}?{key}= 를 만든다.
	router := read("router", "routers", "acct.go")
	goSearch := all(`group\.Get\("/acct/search/(\w+)"`, router)
	springSearch := all(`@GetMapping\("/search/(\w+)"\)`, ctl)
	check("go search paths", goSearch, springSearch)
	rustHandler := read("rust", "src", "handlers", "acct.rs")
	check("rust search paths", all(`\.route\("/acct/search/(\w+)"`, rustHandler), goSearch)
	for _, key := range []string{"displayName", "userid", "createdAt"} {
		if !strings.Contains(router, `c.Query("`+key+`")`) {
			t.Errorf("Go search does not read query %q", key)
		}
		if !strings.Contains(rustHandler, `q.get("`+key+`")`) {
			t.Errorf("Rust search does not read query %q", key)
		}
	}
}

// filterLangs 는 listFilters 가 목록 필터 이름을 꺼내는 언어다. 새 백엔드는 여기에 추출 함수를 더한다
// (out 은 생성 디렉터리, ts 는 TypeScript 템플릿 데이터).
var filterLangs = map[string]func(t *testing.T, out string, model string, ts reactcodegen.ReactTemplateData) []string{
	"go": func(t *testing.T, out string, model string, ts reactcodegen.ReactTemplateData) []string {
		rest := readOut(t, out, "controllers", "rest", strings.ToLower(model)+".go")
		filters := regexp.MustCompile(`(?s)func Parse` + model + `Filters\(.*?\n}`).FindString(rest)
		if filters == "" {
			t.Fatalf("Go Parse%sFilters not found:\n%s", model, rest)
		}
		return submatches(`get\("(\w+)"\)`, filters)
	},
	"spring": func(t *testing.T, out string, model string, ts reactcodegen.ReactTemplateData) []string {
		ctl := readOut(t, out, "kt", "controller", model+"Controller.kt")
		list := regexp.MustCompile(`(?s)fun get` + model + `s\((.*?)\): ResponseEntity`).FindStringSubmatch(ctl)
		if list == nil {
			t.Fatalf("Spring list endpoint not found:\n%s", ctl)
		}
		// 매개변수 이름은 _page / {field}_start 처럼 컬럼과 겹치지 않는 이름이고, 쿼리 파라미터 이름은 @RequestParam("...") 이다.
		ret := make([]string, 0)
		for _, p := range submatches(`@RequestParam\("(\w+)"`, list[1]) {
			if p != "page" && p != "pagesize" && p != "sort" && p != "orderby" {
				ret = append(ret, p)
			}
		}
		return ret
	},
	"rust": func(t *testing.T, out string, model string, ts reactcodegen.ReactTemplateData) []string {
		src := readOut(t, out, "rust", "src", "handlers", strings.ToLower(model)+".rs")
		filters := regexp.MustCompile(`(?s)pub fn parse_filters\(.*?\n}`).FindString(src)
		if filters == "" {
			t.Fatalf("Rust parse_filters not found:\n%s", src)
		}
		return submatches(`q\.get\("(\w+)"\)`, filters)
	},
	"ts": func(t *testing.T, out string, model string, ts reactcodegen.ReactTemplateData) []string {
		ret := make([]string, 0)
		for _, c := range ts.Columns {
			if !c.IsSearchable {
				continue
			}
			if c.IsDateType {
				ret = append(ret, c.StartParam, c.EndParam)
			} else {
				ret = append(ret, c.Key)
			}
		}
		return ret
	},
}

// listFilters 는 이미 생성한 table 의 목록 필터 파라미터 이름을 언어마다 꺼낸다.
func listFilters(t *testing.T, out string, table string, gpa *config.Gpa, ts reactcodegen.ReactTemplateData) map[string][]string {
	t.Helper()
	model := util.ModelName(table)
	ret := map[string][]string{}
	for lang, f := range filterLangs {
		ret[lang] = f(t, out, model, ts)
	}
	return ret
}

func readOut(t *testing.T, out string, path ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{out}, path...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func submatches(re string, src string) []string {
	ret := make([]string, 0)
	for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(src, -1) {
		ret = append(ret, m[1])
	}
	return ret
}

// gpa.primary 컬럼(id 없음)은 id 처럼 목록 필터가 없다 (Go, Spring, TypeScript 가 같다).
// 정수 primary 가 유일한 숫자 컬럼이면 Go rest 는 strconv 를 import 하지 않는다 (쓰지 않는 import 는 컴파일 오류).
func TestPrimaryFilterAgree(t *testing.T) {
	out := setupOut(t)
	gpa := &config.Gpa{Name: "code", Primary: []string{"code"}}
	raws := []util.RawColumn{
		{Name: "c_code", DataType: "int"},
		{Name: "c_name", DataType: "varchar"},
		{Name: "c_created_at", DataType: "datetime"},
		{Name: "c_data", DataType: "blob"},
	}
	columns := func(lang string) []util.Column {
		ret := make([]util.Column, 0)
		for _, r := range raws {
			ret = append(ret, util.NewColumnFromRaw("code_tb", r, gpa, config.ModelConfig{Language: lang}))
		}
		return ret
	}
	gocodegen.ProcessGo("testproj", "code_tb", "c", columns("go"), nil, gpa, "1", "", config.ModelConfig{Language: "go", GoModelFilePath: out + "/"})
	kotlincodegen.ProcessKotlin("testproj", "code_tb", "c", columns("kotlin"), nil, gpa, "1", "", config.ModelConfig{Language: "kotlin", KotlinModelFilePath: filepath.Join(out, "kt")})
	ts := reactcodegen.BuildTemplateData("testproj", "code_tb", columns("react"), gpa)
	rustcodegen.ProcessRust("testproj", "code_tb", "c", columns("rust"), nil, gpa, "1", "", config.ModelConfig{Language: "rust", RustModelFilePath: filepath.Join(out, "rust")})

	want := "name,startcreatedat,endcreatedat"
	for lang, got := range listFilters(t, out, "code_tb", gpa, ts) {
		if strings.Join(got, ",") != want {
			t.Errorf("%s filters = %v, want %s", lang, got, want)
		}
	}
	if rest := readOut(t, out, "controllers", "rest", "code.go"); strings.Contains(rest, `"strconv"`) != strings.Contains(rest, "strconv.") {
		t.Errorf("Go rest strconv import does not match its use:\n%s", rest)
	}
}

// setupOut 은 views 를 가리키는 HOME 과 생성 디렉터리를 만든다.
func setupOut(t *testing.T) string {
	t.Helper()
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
	for _, d := range []string{"models", "controllers/rest", "router/routers", "kt", "dart"} {
		if err := os.MkdirAll(filepath.Join(out, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// unique 는 순서를 지키며 중복을 뺀다 (Dart fromJson 은 null 검사로 같은 key 를 두 번 읽는다).
func unique(list []string) []string {
	ret := make([]string, 0, len(list))
	seen := map[string]bool{}
	for _, v := range list {
		if !seen[v] {
			seen[v] = true
			ret = append(ret, v)
		}
	}
	return ret
}

// tinyint / bit 는 모든 언어에서 bool 이다 (Go bool, Kotlin Boolean, TypeScript boolean, Dart bool, Rust bool).
func TestBoolTypesAgree(t *testing.T) {
	for _, dbType := range []string{"tinyint", "bit"} {
		raw := util.RawColumn{Name: "a_flag", DataType: dbType}
		want := map[string]string{"go": "bool", "kotlin": "Boolean", "dart": "bool", "rust": "bool"}
		for lang, w := range want {
			if got := util.NewColumnFromRaw("acct_tb", raw, nil, config.ModelConfig{Language: lang}).Type; got != w {
				t.Errorf("%s %s = %q, want %q", lang, dbType, got, w)
			}
		}
		col := util.NewColumnFromRaw("acct_tb", raw, nil, config.ModelConfig{Language: "react"})
		ts := reactcodegen.BuildTemplateData("testproj", "acct_tb", []util.Column{col}, nil)
		if got := ts.Columns[0].TypeScriptType; got != "boolean" {
			t.Errorf("typescript %s = %q, want boolean", dbType, got)
		}
	}
}

// 컬럼 이름이 생성 코드의 지역 변수 / 매개변수(page, sort, order, spec, result ...)와 같아도 Spring 컨트롤러 이름이 겹치지 않는지,
// Kotlin / Spring 타입과 같은 테이블 이름에는 접미사가 붙는지 확인한다 (컴파일은 kotlinc 로 따로 확인했다).
func TestKotlinReservedNames(t *testing.T) {
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

	names := []string{"id", "page", "pagesize", "sort", "orderby", "order", "spec", "result", "response", "value", "root", "cb", "predicates",
		"log", "request", "count", "size", "e", "it", "start", "end", "extra", "success"}
	raws := make([]util.RawColumn, 0)
	for _, n := range names {
		typ := "int"
		if n == "id" {
			typ = "bigint"
		} else if n == "start" {
			typ = "datetime"
		}
		raws = append(raws, util.RawColumn{Name: "l_" + n, DataType: typ})
	}
	gen := func(table string, raws []util.RawColumn, auth string) {
		cnf := config.ModelConfig{Language: "kotlin", KotlinModelFilePath: out, Auth: auth}
		cols := make([]util.Column, 0)
		for _, r := range raws {
			cols = append(cols, util.NewColumnFromRaw(table, r, nil, cnf))
		}
		kotlincodegen.ProcessKotlin("testproj", table, "l", cols, nil, nil, "1", auth, cnf)
	}
	gen("lst_tb", raws, "jwt")
	gen("page_tb", raws[:2], "")

	src, err := os.ReadFile(filepath.Join(out, "controller", "LstController.kt"))
	if err != nil {
		t.Fatal(err)
	}
	ctl := string(src)
	list := regexp.MustCompile(`(?s)fun getLsts\((.*?)\): ResponseEntity`).FindStringSubmatch(ctl)
	if list == nil {
		t.Fatalf("list endpoint not found:\n%s", ctl)
	}
	// 매개변수 이름은 모두 다르다.
	seen := map[string]bool{}
	for _, p := range regexp.MustCompile(`\) (\w+): `).FindAllStringSubmatch(list[1], -1) {
		if seen[p[1]] {
			t.Errorf("duplicate parameter %s", p[1])
		}
		seen[p[1]] = true
	}
	for _, p := range []string{"_page", "_pagesize", "_sort", "_orderby", "page", "sort", "order", "start_start", "start_end"} {
		if !seen[p] {
			t.Errorf("parameter %s not found in %v", p, seen)
		}
	}
	// 함수 안의 지역 변수는 _ 로 시작한다 (컬럼 매개변수와 겹치지 않는다).
	for _, m := range regexp.MustCompile(`(?m)^ {8,}val (\w+)`).FindAllStringSubmatch(ctl, -1) {
		if !strings.HasPrefix(m[1], "_") {
			t.Errorf("local %s does not start with _", m[1])
		}
	}
	// extra 컬럼은 extra2
	if !strings.Contains(ctl, `@RequestParam("extra2", required = false) extra2: Int?`) {
		t.Error("extra column should be extra2")
	}
	if _, err := os.Stat(filepath.Join(out, "entity", "PageEntity.kt")); err != nil {
		t.Errorf("page_tb should generate PageEntity: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "controller", "PageEntityController.kt")); !strings.Contains(string(b), `@RequestMapping("/api/page")`) {
		t.Error("PageEntity URL should stay /api/page")
	}
}
