package rustcodegen

import (
	"bytes"
	"database/sql"
	"fmt"
	"gomachine/config"
	"gomachine/util"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/CloudyKit/jet/v6"
	log "github.com/sirupsen/logrus"
)

// 생성 파일은 cnf.RustModelFilePath(crate 루트) 아래 src/ 에 둔다. 모듈은 crate:: 경로로 참조하므로 crate 이름은 필요 없다.
//
// 실행 순서: Prepare(실행마다 한 번, 지원 모듈) → ProcessRust(테이블마다) → Finish(실행마다 한 번, 모듈 목록 / 경로).
// 생성 파일은 매번 다시 만들되 내용이 같으면 쓰지 않고, 사용자 파일(src/support/auth.rs, src/main.rs)은 없을 때만 만든다.

// supportFile 은 지원 모듈 파일 하나다. UserOwned 면 없을 때만 만든다.
type supportFile struct {
	Template  string
	Path      string // crate 루트 기준
	UserOwned bool
}

var supportFiles = []supportFile{
	{"support/mod.jet", "src/support/mod.rs", false},
	{"support/clock.jet", "src/support/clock.rs", false},
	{"support/error.jet", "src/support/error.rs", false},
	{"support/password.jet", "src/support/password.rs", false},
	{"support/query.jet", "src/support/query.rs", false},
	{"support/response.jet", "src/support/response.rs", false},
	{"support/row.jet", "src/support/row.rs", false},
	{"support/serde_ext.jet", "src/support/serde_ext.rs", false},
	{"support/session.jet", "src/support/session.rs", false},
	{"support/state.jet", "src/support/state.rs", false},
	{"support/auth.jet", "src/support/auth.rs", true},
}

// mainMarker 는 생성한 main.rs 의 첫 줄이다 (main.rs 가 쓰는 의존성 확인에 쓴다).
const mainMarker = "// buildtool 이 한 번만 만드는 사용자 파일이다"

// modules 는 이번 실행에서 생성한 테이블 모듈 이름이다 (Finish 가 models/mod.rs, handlers/mod.rs, routes.rs 에 쓴다).
var modules []string

// Prepare 는 실행마다 한 번 지원 모듈(src/support), 없으면 src/main.rs 를 만들고 Cargo.toml 의존성을 확인한다.
func Prepare(cnf config.ModelConfig) {
	root := cnf.RustModelFilePath
	modules = nil
	views := newViews()
	clock := clockVars(cnf.Timezone)

	for _, f := range supportFiles {
		content, err := render(views, f.Template, clock)
		if err != nil {
			log.Printf("Failed to render %s: %v", f.Template, err)
			continue
		}
		write(filepath.Join(root, f.Path), content, f.UserOwned)
	}

	mainPath := filepath.Join(root, "src", "main.rs")
	libPath := filepath.Join(root, "src", "lib.rs")
	if !exists(mainPath) && !exists(libPath) {
		if content, err := render(views, "main.jet", nil); err != nil {
			log.Printf("Failed to render main.jet: %v", err)
		} else {
			write(mainPath, content, true)
		}
	} else {
		checkCrateRoot(mainPath, libPath)
	}

	// Cargo.toml 은 고치지 않고 빠진 의존성 줄만 알려준다 (README 의 Cargo.toml 블록).
	deps := RequiredDeps(clock.NeedsChronoTz, generatedMain(mainPath))
	missing, err := MissingDeps(filepath.Join(root, "Cargo.toml"), deps)
	if err != nil {
		log.Printf("WARNING: rust: cannot read Cargo.toml (%v), the crate needs every dependency below", err)
		missing = deps
	}
	for _, d := range missing {
		log.Printf("WARNING: rust: add to Cargo.toml [dependencies]: %s", d.Line)
	}
}

// ProcessRust 는 Rust(axum + sqlx) 모델 / 핸들러를 생성한다. 아직 템플릿이 없어서 경고만 찍는다 (다음 단계에서 구현).
// 구현하면 util.RustModule(tableName) 을 modules 에 더한다.
func ProcessRust(packageName string, tableName string, prefix string, items []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	log.Printf("WARNING: rust model/handler generation not implemented yet, %s skipped", tableName)
}

// Finish 는 실행마다 한 번 src/models/mod.rs, src/handlers/mod.rs, src/routes.rs 를 이번에 생성한 모듈 목록으로 다시 쓴다.
func Finish(cnf config.ModelConfig) {
	root := cnf.RustModelFilePath
	views := newViews()
	mods := append([]string(nil), modules...)
	sort.Strings(mods)
	data := map[string]interface{}{"modules": mods}
	for _, f := range []struct{ tmpl, path string }{
		{"models_mod.jet", "src/models/mod.rs"},
		{"handlers_mod.jet", "src/handlers/mod.rs"},
		{"routes.jet", "src/routes.rs"},
	} {
		content, err := render(views, f.tmpl, data)
		if err != nil {
			log.Printf("Failed to render %s: %v", f.tmpl, err)
			continue
		}
		write(filepath.Join(root, f.path), content, false)
	}
}

// write 는 생성 파일이면 내용이 바뀌었을 때만, 사용자 파일이면 없을 때만 쓴다.
func write(path string, content string, userOwned bool) {
	var written bool
	var err error
	if userOwned {
		written, err = util.WriteIfMissing(path, content)
	} else {
		written, err = util.WriteIfChanged(path, content)
	}
	if err != nil {
		log.Printf("Failed to write %s: %v", path, err)
		return
	}
	if written {
		log.Printf("Generated Rust: %s", path)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// generatedMain 은 main.rs 가 buildtool 이 만든 파일(그래서 tracing-subscriber / tower-http 를 쓰는지)인지 확인한다.
func generatedMain(path string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.HasPrefix(string(b), mainMarker)
}

// checkCrateRoot 는 사용자의 main.rs / lib.rs 가 생성 모듈을 선언하는지 확인하고, 빠졌으면 경고한다.
func checkCrateRoot(mainPath string, libPath string) {
	var src string
	for _, p := range []string{libPath, mainPath} {
		if b, err := os.ReadFile(p); err == nil {
			src += string(b) + "\n"
		}
	}
	if missing := MissingModDecls(src); len(missing) > 0 {
		log.Printf("WARNING: rust: src/main.rs (or src/lib.rs) must declare the generated modules: %s", strings.Join(missing, " "))
	}
}

// MissingModDecls 는 crate 루트 소스에 없는 생성 모듈 선언(mod support; ...)이다.
func MissingModDecls(src string) []string {
	missing := make([]string, 0)
	for _, m := range []string{"support", "models", "handlers", "routes"} {
		if !regexp.MustCompile(`(?m)^\s*(pub(\([^)]*\))?\s+)?mod\s+` + m + `\s*;`).MatchString(src) {
			missing = append(missing, "mod "+m+";")
		}
	}
	return missing
}

// ClockVars 는 support/clock.rs 템플릿 값이다.
type ClockVars struct {
	TimezoneLabel string // 주석에 쓰는 설정 값
	NowBody       string // now_format(fmt) 의 본문 (Rust 식)
	TestNow       string // 테스트가 비교하는 같은 시간대의 현재 시각 (NaiveDateTime 식)
	NeedsChronoTz bool   // IANA 이름이라 chrono-tz 가 필요하다
}

const (
	seoulNow     = "chrono::Utc::now().with_timezone(&seoul()).format(fmt).to_string()"
	seoulTestNow = "(chrono::Utc::now() + chrono::TimeDelta::hours(9)).naive_utc()"
)

var ianaName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+\-]*(/[A-Za-z0-9_+\-]+)*$`)

// clockVars 는 Go createdNowExpr 와 같은 규칙이다: "" / "Asia/Seoul" 은 UTC+9 (FixedOffset), "local" 은 서버 지역 시간,
// "UTC" 는 UTC, 그 밖의 IANA 이름은 chrono-tz 로 찾고 실행 중에 찾지 못하면 UTC+9 (Go time.LoadLocation 실패와 같다).
func clockVars(tz string) ClockVars {
	tz = strings.TrimSpace(tz)
	label := tz
	if label == "" {
		label = "(empty) = Asia/Seoul"
	}
	v := ClockVars{TimezoneLabel: label, NowBody: seoulNow, TestNow: seoulTestNow}
	switch tz {
	case "", "Asia/Seoul":
		return v
	case "local", "Local":
		v.NowBody = "chrono::Local::now().format(fmt).to_string()"
		v.TestNow = "chrono::Local::now().naive_local()"
		return v
	case "UTC", "utc":
		v.NowBody = "chrono::Utc::now().format(fmt).to_string()"
		v.TestNow = "chrono::Utc::now().naive_utc()"
		return v
	}
	if !ianaName.MatchString(tz) {
		log.Printf("WARNING: rust: timezone %q is not an IANA name, using UTC+9", tz)
		return v
	}
	if _, err := time.LoadLocation(tz); err != nil {
		log.Printf("WARNING: rust: timezone %q not found (%v); the generated code falls back to UTC+9 if chrono-tz cannot find it either", tz, err)
	}
	q := fmt.Sprintf("%q", tz)
	v.NowBody = "match " + q + ".parse::<chrono_tz::Tz>() {\n" +
		"        Ok(tz) => chrono::Utc::now().with_timezone(&tz).format(fmt).to_string(),\n" +
		"        // 찾지 못하면 Go 와 같이 UTC+9\n" +
		"        Err(_) => " + seoulNow + ",\n" +
		"    }"
	v.TestNow = "chrono::Utc::now().with_timezone(&" + q + ".parse::<chrono_tz::Tz>().unwrap_or(chrono_tz::Asia::Seoul)).naive_local()"
	v.NeedsChronoTz = true
	return v
}

// CargoDep 는 대상 crate 에 필요한 의존성 한 줄이다 (README 의 Cargo.toml 블록과 같다).
type CargoDep struct {
	Name string
	Line string
}

var supportDeps = []CargoDep{
	{"axum", `axum = "0.8.9"`},
	{"tokio", `tokio = { version = "1.54", features = ["macros", "rt-multi-thread", "net"] }`},
	{"sqlx", `sqlx = { version = "0.9.0", default-features = false, features = ["runtime-tokio", "tls-rustls", "mysql", "mysql-rsa", "derive", "json"] }`},
	{"serde", `serde = { version = "1", features = ["derive"] }`},
	{"serde_json", `serde_json = "1"`},
	{"serde_with", `serde_with = "3"`},
	{"jsonwebtoken", `jsonwebtoken = { version = "11.1", features = ["rust_crypto"] }`},
	{"bcrypt", `bcrypt = "0.19.3"`},
	{"base64", `base64 = "0.22"`},
	{"chrono", `chrono = { version = "0.4", default-features = false, features = ["clock", "std"] }`},
	{"thiserror", `thiserror = "2"`},
	{"tracing", `tracing = "0.1"`},
}

var chronoTzDep = CargoDep{"chrono-tz", `chrono-tz = "0.10"`}

var mainDeps = []CargoDep{
	{"tracing-subscriber", `tracing-subscriber = { version = "0.3", features = ["env-filter"] }`},
	{"tower-http", `tower-http = { version = "0.6", features = ["cors"] }`},
}

// RequiredDeps 는 생성 코드가 쓰는 의존성이다. chronoTz 는 IANA timezone, withMain 은 buildtool 이 만든 main.rs 일 때.
func RequiredDeps(chronoTz bool, withMain bool) []CargoDep {
	deps := append([]CargoDep(nil), supportDeps...)
	if chronoTz {
		deps = append(deps, chronoTzDep)
	}
	if withMain {
		deps = append(deps, mainDeps...)
	}
	return deps
}

// MissingDeps 는 Cargo.toml 의 [dependencies] 에 없는 의존성이다 (이름만 본다. 버전 / feature 는 확인하지 않는다).
func MissingDeps(cargoPath string, deps []CargoDep) ([]CargoDep, error) {
	b, err := os.ReadFile(cargoPath)
	if err != nil {
		return nil, err
	}
	have := CargoDependencyNames(string(b))
	missing := make([]CargoDep, 0)
	for _, d := range deps {
		if !have[normalizeCrate(d.Name)] {
			missing = append(missing, d)
		}
	}
	return missing, nil
}

// CargoDependencyNames 는 Cargo.toml 의 [dependencies] 표 key 와 [dependencies.이름] 표 이름이다 (- 와 _ 는 같게 본다).
func CargoDependencyNames(toml string) map[string]bool {
	names := map[string]bool{}
	inDeps := false
	for _, line := range strings.Split(toml, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			header := strings.TrimSpace(strings.Trim(strings.SplitN(line, "#", 2)[0], " []"))
			inDeps = header == "dependencies"
			if name, ok := strings.CutPrefix(header, "dependencies."); ok {
				names[normalizeCrate(strings.Trim(name, `"' `))] = true
			}
			continue
		}
		if !inDeps {
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if i := strings.Index(key, "."); i > 0 && !strings.HasPrefix(key, `"`) {
			key = key[:i] // serde.workspace = true
		}
		names[normalizeCrate(strings.Trim(key, `"' `))] = true
	}
	return names
}

func normalizeCrate(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "_", "-")
}

// newViews 는 ~/bin/buildtool/rust 템플릿을 준비한다.
func newViews() *jet.Set {
	templateDir := filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "rust")
	return jet.NewSet(jet.NewOSFileSystemLoader(templateDir), jet.InDevelopmentMode())
}

// render 는 템플릿을 실행한다. data 는 ClockVars 나 map (템플릿 변수 이름은 lowerCamel).
func render(views *jet.Set, name string, data interface{}) (string, error) {
	t, err := views.GetTemplate(name)
	if err != nil {
		return "", err
	}
	vars := make(jet.VarMap)
	switch d := data.(type) {
	case ClockVars:
		vars.Set("timezoneLabel", d.TimezoneLabel)
		vars.Set("nowBody", d.NowBody)
		vars.Set("testNow", d.TestNow)
	case map[string]interface{}:
		for k, v := range d {
			vars.Set(k, v)
		}
	}
	var b bytes.Buffer
	if err := t.Execute(&b, vars, nil); err != nil {
		return "", err
	}
	return b.String(), nil
}
