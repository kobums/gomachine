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
	check("dart fromJson keys", all(`json\['(\w+)'\] (?:as|==)`, strings.Split(dart, "factory")[1][:strings.Index(strings.Split(dart, "factory")[1], "extra:")]), want)

	// 목록 필터 이름: Go Index, Spring @RequestParam, TypeScript SearchParams
	wantFilters := []string{"loginid", "displayName", "userid", "status", "startcreatedat", "endcreatedat", "startcreateddate", "endcreateddate"}
	rest := read("controllers", "rest", "acct.go")
	filters := regexp.MustCompile(`(?s)func \(c \*AcctController\) filters\(\).*?\n}`).FindString(rest)
	check("go filters", all(`c\.Get[if64]*\("(\w+)"\)`, filters), wantFilters)

	ctl := read("kt", "controller", "AcctController.kt")
	list := regexp.MustCompile(`(?s)fun getAccts\((.*?)\): ResponseEntity`).FindStringSubmatch(ctl)
	if list == nil {
		t.Fatalf("Spring list endpoint not found:\n%s", ctl)
	}
	springParams := make([]string, 0)
	for _, p := range all(`@RequestParam\([^)]*\)(?: @DateTimeFormat\([^)]*\))? (\w+):`, list[1]) {
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
