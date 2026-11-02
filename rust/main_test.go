package rustcodegen

import (
	"gomachine/config"
	"gomachine/util"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// setupHome 은 HOME/bin/buildtool 이 저장소 views/ 를 가리키는 임시 HOME 을 만든다 (템플릿을 실행 시점에 읽으므로).
func setupHome(t *testing.T) {
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
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func generate(root string, tz string) {
	cnf := config.ModelConfig{RustModelFilePath: root, Language: "rust", Timezone: tz}
	Prepare(cnf)
	Finish(cnf)
}

func TestPrepareWritesSupportAndKeepsUserFiles(t *testing.T) {
	setupHome(t)
	root := t.TempDir()
	generate(root, "")

	for _, f := range supportFiles {
		if s := readFile(t, filepath.Join(root, f.Path)); !strings.Contains(s, "buildtool") {
			t.Errorf("%s: missing header", f.Path)
		}
	}
	for _, p := range []string{"src/main.rs", "src/routes.rs", "src/models/mod.rs", "src/handlers/mod.rs"} {
		readFile(t, filepath.Join(root, p))
	}
	if s := readFile(t, filepath.Join(root, "src/main.rs")); !strings.HasPrefix(s, mainMarker) || !strings.Contains(s, "mod support;") {
		t.Errorf("main.rs: unexpected content:\n%s", s)
	}
	if s := readFile(t, filepath.Join(root, "src/support/clock.rs")); !strings.Contains(s, seoulNow) {
		t.Errorf("clock.rs: default timezone should be UTC+9:\n%s", s)
	}

	// 사용자 파일은 덮어쓰지 않고, 고친 생성 파일은 되돌리고, 바뀌지 않은 생성 파일은 쓰지 않는다 (수정 시각 유지).
	auth := filepath.Join(root, "src/support/auth.rs")
	mainRs := filepath.Join(root, "src/main.rs")
	query := filepath.Join(root, "src/support/query.rs")
	response := filepath.Join(root, "src/support/response.rs")
	writeFile(t, auth, "// my auth\n")
	writeFile(t, mainRs, "// my main\n")
	original := readFile(t, query)
	writeFile(t, query, "// edited\n")
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(response, old, old); err != nil {
		t.Fatal(err)
	}

	generate(root, "")

	if s := readFile(t, auth); s != "// my auth\n" {
		t.Errorf("auth.rs overwritten: %q", s)
	}
	if s := readFile(t, mainRs); s != "// my main\n" {
		t.Errorf("main.rs overwritten: %q", s)
	}
	if s := readFile(t, query); s != original {
		t.Errorf("query.rs not regenerated")
	}
	if st, err := os.Stat(response); err != nil || !st.ModTime().Equal(old) {
		t.Errorf("response.rs rewritten although unchanged (mtime %v, want %v)", st.ModTime(), old)
	}

	// timezone 이 바뀌면 clock.rs 만 다시 쓴다.
	generate(root, "UTC")
	if s := readFile(t, filepath.Join(root, "src/support/clock.rs")); !strings.Contains(s, "chrono::Utc::now().format(fmt)") {
		t.Errorf("clock.rs not regenerated for UTC:\n%s", s)
	}
}

func TestPrepareSkipsMainWhenLibExists(t *testing.T) {
	setupHome(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src/lib.rs"), "pub mod support;\n")
	generate(root, "")
	if _, err := os.Stat(filepath.Join(root, "src/main.rs")); !os.IsNotExist(err) {
		t.Errorf("main.rs generated although lib.rs exists (err %v)", err)
	}
}

func TestClockVars(t *testing.T) {
	cases := []struct {
		tz      string
		body    string
		chronTz bool
	}{
		{"", seoulNow, false},
		{"Asia/Seoul", seoulNow, false},
		{" Asia/Seoul ", seoulNow, false},
		{"local", "chrono::Local::now().format(fmt).to_string()", false},
		{"Local", "chrono::Local::now().format(fmt).to_string()", false},
		{"UTC", "chrono::Utc::now().format(fmt).to_string()", false},
		{"utc", "chrono::Utc::now().format(fmt).to_string()", false},
		{"America/New_York", `match "America/New_York".parse::<chrono_tz::Tz>() {`, true},
		{"Etc/GMT+9", `match "Etc/GMT+9".parse::<chrono_tz::Tz>() {`, true},
		{"Bad Zone!", seoulNow, false},
		{`x"); panic!("`, seoulNow, false},
	}
	for _, c := range cases {
		v := clockVars(c.tz)
		if !strings.HasPrefix(v.NowBody, c.body) {
			t.Errorf("clockVars(%q).NowBody = %q, want prefix %q", c.tz, v.NowBody, c.body)
		}
		if v.NeedsChronoTz != c.chronTz {
			t.Errorf("clockVars(%q).NeedsChronoTz = %v", c.tz, v.NeedsChronoTz)
		}
		if c.chronTz && !strings.Contains(v.NowBody, "Err(_) => "+seoulNow) {
			t.Errorf("clockVars(%q): IANA name must fall back to UTC+9: %s", c.tz, v.NowBody)
		}
	}
}

func TestCargoDependencies(t *testing.T) {
	toml := `[package]
name = "app" # dependencies = no
tokio = "1"

[dependencies]
axum = "0.8.9"
"serde" = { version = "1", features = ["derive"] }
serde_json.workspace = true
tracing_subscriber = "0.3"
# bcrypt = "0.19"

[dependencies.sqlx]
version = "0.9.0"

[dev-dependencies]
chrono = "0.4"
`
	got := CargoDependencyNames(toml)
	want := map[string]bool{"axum": true, "serde": true, "serde-json": true, "tracing-subscriber": true, "sqlx": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CargoDependencyNames = %v, want %v", got, want)
	}

	path := filepath.Join(t.TempDir(), "Cargo.toml")
	writeFile(t, path, toml)
	missing, err := MissingDeps(path, RequiredDeps(true, true))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0)
	for _, d := range missing {
		names = append(names, d.Name)
	}
	wantMissing := []string{"tokio", "serde_with", "jsonwebtoken", "bcrypt", "base64", "chrono", "thiserror", "tracing", "chrono-tz", "tower-http"}
	if !reflect.DeepEqual(names, wantMissing) {
		t.Errorf("MissingDeps = %v, want %v", names, wantMissing)
	}
	if len(RequiredDeps(false, false)) != len(supportDeps) {
		t.Errorf("RequiredDeps(false, false) should be the support deps only")
	}
}

func TestMissingModDecls(t *testing.T) {
	if got := MissingModDecls("mod support;\npub(crate) mod models;\n  pub mod handlers ;\nmod routes;\n"); len(got) != 0 {
		t.Errorf("MissingModDecls = %v, want none", got)
	}
	got := MissingModDecls("mod support;\n// mod models;\nmod handlers_x;\nfn main() {}\n")
	want := []string{"mod models;", "mod handlers;", "mod routes;"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MissingModDecls = %v, want %v", got, want)
	}
}

// ProcessRust 는 테이블마다 모델 / 핸들러를 만들고, Finish 가 모듈 목록과 경로를 쓴다.
func TestProcessRustModulesAndRoutes(t *testing.T) {
	setupHome(t)
	root := t.TempDir()
	cnf := config.ModelConfig{RustModelFilePath: root, Language: "rust", Auth: "jwt", AdminLevel: "3"}
	Prepare(cnf)
	gen := func(table, prefix string, raws []util.RawColumn, gpa *config.Gpa) {
		cols := make([]util.Column, 0)
		for _, r := range raws {
			cols = append(cols, util.NewColumnFromRaw(table, r, gpa, cnf))
		}
		ProcessRust("testproj", table, prefix, cols, nil, gpa, "1", "jwt", cnf)
	}
	gen("type_tb", "t", []util.RawColumn{{Name: "t_id", DataType: "bigint"}, {Name: "t_user", DataType: "bigint"}, {Name: "t_name", DataType: "varchar"}},
		&config.Gpa{Name: "type", Method: []string{"FindByName", "FindByName", "DeleteByNamelike"}, Session: []config.SessionPair{{Key: "id", Column: "user"}}})
	gen("stat_vw", "s", []util.RawColumn{{Name: "s_id", DataType: "bigint"}, {Name: "s_cnt", DataType: "int"}}, nil)
	Finish(cnf)

	if s := readFile(t, filepath.Join(root, "src/models/mod.rs")); !strings.Contains(s, "pub mod stat;\npub mod type_;") {
		t.Errorf("models/mod.rs:\n%s", s)
	}
	if s := readFile(t, filepath.Join(root, "src/routes.rs")); !strings.Contains(s, ".merge(crate::handlers::stat::routes())") || !strings.Contains(s, ".merge(crate::handlers::type_::routes())") {
		t.Errorf("routes.rs:\n%s", s)
	}
	h := readFile(t, filepath.Join(root, "src/handlers/type_.rs"))
	for _, want := range []string{
		`.route("/type", get(index).post(insert).put(update_body).delete(delete_body))`,
		`.route("/type/{id}", get(read).put(update_path).patch(update_path).delete(delete_path))`,
		`.route("/type/batch", post(insert_batch).delete(delete_batch))`,
		`.route("/type/count", post(count).get(count))`,
		`.route("/type/find/name/{name}", get(find_by_name))`,
	} {
		if !strings.Contains(h, want) {
			t.Errorf("type_ handler missing %s:\n%s", want, h)
		}
	}
	// 같은 메서드는 한 번만, like 로 지우는 경로는 없다
	if strings.Count(h, `.route("/type/find/name/`) != 1 || strings.Contains(h, `.route("/type/delete/`) {
		t.Errorf("duplicate / like delete routes:\n%s", h)
	}
	v := readFile(t, filepath.Join(root, "src/handlers/stat.rs"))
	if strings.Contains(v, "fn insert") || strings.Contains(v, "put(") || strings.Contains(v, "delete(") || !strings.Contains(v, `.route("/stat/count", post(count).get(count))`) {
		t.Errorf("view must be read-only:\n%s", v)
	}
	m := readFile(t, filepath.Join(root, "src/models/type_.rs"))
	if !strings.Contains(m, "pub struct TypeModel {") || !strings.Contains(m, "fn merge") {
		t.Errorf("type_ model:\n%s", m)
	}
}

func TestGroupRoutes(t *testing.T) {
	got := groupRoutes("t", []route{
		{"get", "/t", "index"}, {"post", "/t", "insert"}, {"get", "/t/{id}", "read"}, {"put", "/t/{id}", "update"},
		{"get", "/t/find/ab/{a}/{b}", "f1"}, {"get", "/t/find/ab/{x}/{y}", "f2"}, {"get", "/t", "again"},
	})
	want := []string{"/t get(index).post(insert)", "/t/{id} get(read).put(update)", "/t/find/ab/{a}/{b} get(f1)"}
	if len(got) != len(want) {
		t.Fatalf("groupRoutes = %v", got)
	}
	for i, g := range got {
		if g.Path+" "+g.Chain != want[i] {
			t.Errorf("group %d = %s %s, want %s", i, g.Path, g.Chain, want[i])
		}
	}
}

// 같은 모듈 이름(zeta_tb, zeta_vw → zeta)이 다시 나오면 먼저 만든 파일을 덮어쓰지 않고 건너뛴다.
func TestModuleCollisionKeepsFirst(t *testing.T) {
	setupHome(t)
	root := t.TempDir()
	cnf := config.ModelConfig{RustModelFilePath: root, Language: "rust"}
	Prepare(cnf)
	gen := func(table string, raws []util.RawColumn) {
		cols := make([]util.Column, 0)
		for _, r := range raws {
			cols = append(cols, util.NewColumnFromRaw(table, r, nil, cnf))
		}
		ProcessRust("testproj", table, "z", cols, nil, nil, "1", "", cnf)
	}
	gen("zeta_tb", []util.RawColumn{{Name: "z_id", DataType: "bigint"}, {Name: "z_first", DataType: "varchar"}})
	gen("zeta_vw", []util.RawColumn{{Name: "z_id", DataType: "bigint"}, {Name: "z_second", DataType: "varchar"}})
	Finish(cnf)

	m := readFile(t, filepath.Join(root, "src/models/zeta.rs"))
	if !strings.Contains(m, "zeta_tb") || strings.Contains(m, "zeta_vw") || strings.Contains(m, "z_second") {
		t.Errorf("first module must be kept:\n%s", m)
	}
	h := readFile(t, filepath.Join(root, "src/handlers/zeta.rs"))
	if !strings.Contains(h, "fn insert") {
		t.Errorf("handler of the table (not the view) must be kept:\n%s", h)
	}
	if s := readFile(t, filepath.Join(root, "src/models/mod.rs")); strings.Count(s, "pub mod zeta;") != 1 {
		t.Errorf("models/mod.rs:\n%s", s)
	}
}

// 다시 생성하는 파일은 GeneratedHeader 로 시작하고, 사용자 파일(main.rs, auth.rs)은 아니다.
func TestGeneratedHeader(t *testing.T) {
	setupHome(t)
	root := t.TempDir()
	cnf := config.ModelConfig{RustModelFilePath: root, Language: "rust"}
	Prepare(cnf)
	cols := []util.Column{util.NewColumnFromRaw("item_tb", util.RawColumn{Name: "i_id", DataType: "bigint"}, nil, cnf)}
	ProcessRust("testproj", "item_tb", "i", cols, nil, nil, "1", "", cnf)
	Finish(cnf)
	generated := []string{"src/models/item.rs", "src/handlers/item.rs", "src/models/mod.rs", "src/handlers/mod.rs", "src/routes.rs"}
	for _, f := range supportFiles {
		if !f.UserOwned {
			generated = append(generated, f.Path)
		}
	}
	for _, p := range generated {
		if !isGenerated(filepath.Join(root, p)) {
			t.Errorf("%s does not start with %q", p, GeneratedHeader)
		}
	}
	for _, p := range []string{"src/main.rs", "src/support/auth.rs"} {
		if isGenerated(filepath.Join(root, p)) {
			t.Errorf("user file %s must not carry the generated header", p)
		}
	}
}

// Finish 는 이번에 만들지 않은 예전 생성 파일(테이블 삭제 / 이름 변경)만 지운다. 머리줄이 없는 파일, mod.rs, 하위 디렉터리는 남긴다.
func TestRemoveStaleGeneratedFiles(t *testing.T) {
	setupHome(t)
	root := t.TempDir()
	cnf := config.ModelConfig{RustModelFilePath: root, Language: "rust"}
	gen := func(tables ...string) {
		Prepare(cnf)
		for _, table := range tables {
			cols := []util.Column{util.NewColumnFromRaw(table, util.RawColumn{Name: "x_id", DataType: "bigint"}, nil, cnf)}
			ProcessRust("testproj", table, "x", cols, nil, nil, "1", "", cnf)
		}
		Finish(cnf)
	}
	gen("alpha_tb", "beta_tb")
	for _, p := range []string{"src/models/alpha.rs", "src/models/beta.rs", "src/handlers/alpha.rs", "src/handlers/beta.rs"} {
		readFile(t, filepath.Join(root, p))
	}
	// 사용자 파일 (머리줄 없음 / 머리줄이 첫 줄이 아님), 하위 디렉터리, .rs 가 아닌 파일
	user := filepath.Join(root, "src/models/custom.rs")
	writeFile(t, user, "// my code\npub fn f() {}\n")
	notFirst := filepath.Join(root, "src/handlers/extra.rs")
	writeFile(t, notFirst, "// mine\n"+GeneratedHeader+"\n")
	nested := filepath.Join(root, "src/models/sub/old.rs")
	writeFile(t, nested, GeneratedHeader+"\n")
	other := filepath.Join(root, "src/models/notes.txt")
	writeFile(t, other, GeneratedHeader+"\n")
	crlf := filepath.Join(root, "src/models/gamma.rs")
	writeFile(t, crlf, GeneratedHeader+"\r\n// old\r\n")

	gen("alpha_tb") // beta_tb 가 없어졌다
	for _, p := range []string{"src/models/beta.rs", "src/handlers/beta.rs", "src/models/gamma.rs"} {
		if _, err := os.Stat(filepath.Join(root, p)); !os.IsNotExist(err) {
			t.Errorf("stale %s must be removed (err=%v)", p, err)
		}
	}
	for _, p := range []string{"src/models/alpha.rs", "src/handlers/alpha.rs", "src/models/mod.rs", "src/handlers/mod.rs", user, notFirst, nested, other} {
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s must be kept: %v", p, err)
		}
	}
	if s := readFile(t, filepath.Join(root, "src/models/mod.rs")); strings.Contains(s, "beta") {
		t.Errorf("models/mod.rs still lists beta:\n%s", s)
	}
	if got := RemoveStale(root, []string{"alpha"}); len(got) != 0 {
		t.Errorf("second cleanup removed %v", got)
	}
	// 디렉터리가 없어도 된다
	if got := RemoveStale(t.TempDir(), nil); len(got) != 0 {
		t.Errorf("empty root removed %v", got)
	}
}

// select 식: bit 는 <>0, float / double / real 은 텍스트(CAST AS CHAR → f64), 부호 있는 정수가 아닌 enum 은 CAST AS SIGNED.
func TestSelectExpr(t *testing.T) {
	col := func(typ string, unsigned bool) util.Column {
		return util.Column{OriginalType: typ, Unsigned: unsigned}
	}
	for _, tc := range []struct {
		col        util.Column
		base       string
		enum       bool
		want, kind string
	}{
		{col("bit", false), "bool", false, "(x<>0)", ""},
		{col("float", false), "f64", false, "CAST(x AS CHAR)", "decimal"},
		{col("double", false), "f64", false, "CAST(x AS CHAR)", "decimal"},
		{col("real", false), "f64", false, "CAST(x AS CHAR)", "decimal"},
		{col("decimal", false), "f64", false, "CAST(x AS CHAR)", "decimal"},
		{col("year", false), "i32", false, "CAST(x AS SIGNED)", ""},
		{col("year", true), "u32", false, "CAST(x AS UNSIGNED)", ""},
		{col("int", false), "Status", true, "x", ""},
		{col("tinyint", false), "Status", true, "x", ""},
		{col("bigint", false), "Status", true, "x", ""},
		{col("int", true), "Status", true, "CAST(x AS SIGNED)", ""},
		{col("varchar", false), "Status", true, "CAST(x AS SIGNED)", ""},
		{col("bit", false), "Status", true, "CAST(x AS SIGNED)", ""},
		{col("year", false), "Status", true, "CAST(x AS SIGNED)", ""},
		{col("int", true), "u32", false, "x", ""},
		{col("varchar", false), "String", false, "x", ""},
	} {
		got, kind := selectExpr(tc.col, tc.base, tc.enum, "x")
		if got != tc.want || kind != tc.kind {
			t.Errorf("%s unsigned=%v enum=%v: (%s, %s), want (%s, %s)", tc.col.OriginalType, tc.col.Unsigned, tc.enum, got, kind, tc.want, tc.kind)
		}
	}
}
