package gocodegen

import (
	"gomachine/config"
	"gomachine/util"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// genRest 는 views 를 HOME 으로 잡고 테이블 하나를 생성해 rest 컨트롤러 내용을 돌려준다.
func genRest(t *testing.T, table string, raws []util.RawColumn, gpa *config.Gpa, auth string, adminLevel string) string {
	t.Helper()
	views, err := filepath.Abs("../views")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "bin"), 0755)
	if err := os.Symlink(views, filepath.Join(home, "bin", "buildtool")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	out := t.TempDir()
	cnf := config.ModelConfig{Language: "go", GoModelFilePath: out, Auth: auth, AdminLevel: adminLevel}
	cols := make([]util.Column, 0)
	for _, r := range raws {
		cols = append(cols, util.NewColumnFromRaw(table, r, gpa, cnf))
	}
	ProcessGo("testproj", table, util.GetPrefix(raws[0].Name), cols, nil, gpa, "1", auth, cnf)
	generatedRoutes = nil
	b, err := os.ReadFile(filepath.Join(out, "controllers", "rest", util.FileName(table)+".go"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// fn 은 컨트롤러 메서드 본문이다.
func fn(t *testing.T, src string, name string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)\nfunc (\(c \*\w+\) )?` + name + `\(.*?\n}\n`).FindString(src)
	if m == "" {
		t.Fatalf("func %s not found:\n%s", name, src)
	}
	return m
}

var postRaws = []util.RawColumn{{Name: "p_id", DataType: "bigint"}, {Name: "p_user", DataType: "bigint"}, {Name: "p_title", DataType: "varchar"}, {Name: "p_createddate", DataType: "datetime"}}

func TestReadPolicyCode(t *testing.T) {
	sess := []config.SessionPair{{Key: "id", Column: "user"}}
	methods := []string{"FindByUser", "UpdateTitleById"}
	for _, tt := range []struct {
		read          string
		login, filter bool
	}{
		{"", true, true}, // owner (기본)
		{"login", true, false},
		{"public", false, false},
	} {
		src := genRest(t, "post_tb", postRaws, &config.Gpa{Name: "post", Read: tt.read, Session: sess, Method: methods}, "jwt", "")
		for _, name := range []string{"Read", "Index", "Count", "Search", "FindByUser"} {
			body := fn(t, src, name)
			if got := strings.Contains(body, "c.Session == nil"); got != tt.login {
				t.Errorf("read %q %s: login check %v, want %v", tt.read, name, got, tt.login)
			}
		}
		if got := strings.Contains(fn(t, src, "filters"), "c.Session.Id"); got != tt.filter {
			t.Errorf("read %q: session filter %v, want %v", tt.read, got, tt.filter)
		}
		if got := strings.Contains(fn(t, src, "Read"), "item.User != "); got != tt.filter {
			t.Errorf("read %q: Read ownership %v, want %v", tt.read, got, tt.filter)
		}
		// 쓰기는 정책과 상관없이 로그인 + 소유권 검사
		if w := fn(t, src, "UpdateTitleById"); !strings.Contains(w, "c.Session == nil") || !strings.Contains(w, "p_user <> ? or p_user is null") {
			t.Errorf("read %q: write checks missing:\n%s", tt.read, w)
		}
		if w := fn(t, src, "Update"); !strings.Contains(w, "existing.User != ") {
			t.Errorf("read %q: Update ownership missing", tt.read)
		}
	}
}

func TestUserSelfReadCode(t *testing.T) {
	raws := []util.RawColumn{{Name: "u_id", DataType: "bigint"}, {Name: "u_email", DataType: "varchar"}, {Name: "u_level", DataType: "int", HasDefault: true, Default: "1"}}
	src := genRest(t, "user_tb", raws, &config.Gpa{Name: "user", Read: "public", Method: []string{"FindByLevel"}}, "jwt", "3")
	if !strings.Contains(fn(t, src, "Read"), "!c.isAdmin() && id != c.Session.Id") {
		t.Error("Read: no self check")
	}
	self := `if !c.isAdmin() {
		args = append(args, models.Where{Column: "id", Value: c.Session.Id, Compare: "="})`
	for _, name := range []string{"filters", "Search"} {
		if !strings.Contains(fn(t, src, name), self) {
			t.Errorf("%s: no self filter:\n%s", name, fn(t, src, name))
		}
	}
	if !strings.Contains(fn(t, src, "FindByLevel"), "manager.FindByLevel(level, readArgs...)") {
		t.Error("FindByLevel: no self args")
	}
	// 보호 컬럼 Insert 는 DB 기본값
	if !strings.Contains(fn(t, src, "Insert"), "item.Level = 1") {
		t.Errorf("Insert protect default:\n%s", fn(t, src, "Insert"))
	}
	// adminLevel 이 없으면 관리자 예외도 없다
	src = genRest(t, "user_tb", raws, nil, "jwt", "")
	if !strings.Contains(fn(t, src, "Read"), "if id != c.Session.Id {") || strings.Contains(src, "isAdmin") {
		t.Error("no-admin User read")
	}
}

func TestFilterOps(t *testing.T) {
	raws := []util.RawColumn{{Name: "i_id", DataType: "bigint"}, {Name: "i_name", DataType: "varchar"}, {Name: "i_code", DataType: "char"},
		{Name: "i_cnt", DataType: "int"}, {Name: "i_tag", DataType: "varchar", HasDefault: true, Default: "'new'"}, {Name: "i_on", DataType: "tinyint", HasDefault: true, Default: "1"}}
	gpa := &config.Gpa{Name: "item", Compare: []config.GpaCompare{{Name: "code", Type: "="}, {Name: "cnt", Type: ">="}}, Protect: []string{"tag", "on"}}
	src := genRest(t, "item_tb", raws, gpa, "", "")
	filters := fn(t, src, "ParseItemFilters")
	for _, want := range []string{
		`models.Custom{Query: "i_name like ? escape '!'", Params: []interface{}{likeItemPattern(_name)}}`,
		`models.Where{Column: "code", Value: _code, Compare: "="}`,
		`models.Where{Column: "cnt", Value: v, Compare: ">="}`,
	} {
		if !strings.Contains(filters, want) {
			t.Errorf("filters: no %s\n%s", want, filters)
		}
	}
	ins := fn(t, src, "Insert")
	if !strings.Contains(ins, `item.Tag = "new"`) || !strings.Contains(ins, "item.On = true") {
		t.Errorf("Insert protect defaults:\n%s", ins)
	}
}

// 생성 파일은 gofmt 형식이고, 다시 생성해도 같다.
func TestGofmtOutput(t *testing.T) {
	a := genRest(t, "post_tb", postRaws, nil, "jwt", "")
	b := genRest(t, "post_tb", postRaws, nil, "jwt", "")
	if a != b {
		t.Error("not idempotent")
	}
	if strings.Contains(a, "\n\n\n") || strings.Contains(a, "    if") {
		t.Error("not gofmt formatted")
	}
}
