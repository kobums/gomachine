package main

import (
	"gomachine/config"
	dartcodegen "gomachine/dart"
	gocodegen "gomachine/go"
	kotlincodegen "gomachine/kotlin"
	reactcodegen "gomachine/react"
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

	dart := read("dart", "acct.dart")
	check("dart fromJson keys", unique(all(`json\['(\w+)'\]`, strings.Split(dart, "factory")[1][:strings.Index(strings.Split(dart, "factory")[1], "extra:")])), want)

	// 목록 필터 이름: Go Index, Spring @RequestParam, TypeScript SearchParams
	wantFilters := []string{"loginid", "displayName", "userid", "status", "startcreatedat", "endcreatedat", "startcreateddate", "endcreateddate"}
	rest := read("controllers", "rest", "acct.go")
	filters := regexp.MustCompile(`(?s)func ParseAcctFilters\(.*?\n}`).FindString(rest)
	check("go filters", all(`get\("(\w+)"\)`, filters), wantFilters)

	ctl := read("kt", "controller", "AcctController.kt")
	list := regexp.MustCompile(`(?s)fun getAccts\((.*?)\): ResponseEntity`).FindStringSubmatch(ctl)
	if list == nil {
		t.Fatalf("Spring list endpoint not found:\n%s", ctl)
	}
	// 매개변수 이름은 _page / {field}_start 처럼 컬럼과 겹치지 않는 이름이고, 쿼리 파라미터 이름은 @RequestParam("...") 이다.
	springParams := make([]string, 0)
	for _, p := range all(`@RequestParam\("(\w+)"`, list[1]) {
		if p != "page" && p != "pagesize" && p != "sort" && p != "orderby" {
			springParams = append(springParams, p)
		}
	}
	check("spring list params", springParams, wantFilters)

	tsFilters := make([]string, 0)
	for _, c := range ts.Columns {
		if !c.IsSearchable {
			continue
		}
		if c.IsDateType {
			tsFilters = append(tsFilters, c.StartParam, c.EndParam)
		} else {
			tsFilters = append(tsFilters, c.Key)
		}
	}
	check("ts search params", tsFilters, wantFilters)

	// 검색 경로: Go 라우터와 Spring 컨트롤러가 같은 /search/{field}?{key}= 를 만든다.
	router := read("router", "routers", "acct.go")
	goSearch := all(`group\.Get\("/acct/search/(\w+)"`, router)
	springSearch := all(`@GetMapping\("/search/(\w+)"\)`, ctl)
	check("go search paths", goSearch, springSearch)
	for _, key := range []string{"displayName", "userid", "createdAt"} {
		if !strings.Contains(router, `c.Query("`+key+`")`) {
			t.Errorf("Go search does not read query %q", key)
		}
	}
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

// tinyint / bit 는 모든 언어에서 bool 이다 (Go bool, Kotlin Boolean, TypeScript boolean, Dart bool).
func TestBoolTypesAgree(t *testing.T) {
	for _, dbType := range []string{"tinyint", "bit"} {
		raw := util.RawColumn{Name: "a_flag", DataType: dbType}
		want := map[string]string{"go": "bool", "kotlin": "Boolean", "dart": "bool"}
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
