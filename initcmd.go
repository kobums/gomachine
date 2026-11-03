package main

// buildtool-model init <name> [flags]: basicproject 템플릿 + 표준 DB 로 새 작업 공간을 만든다.
//
//	<name>/
//	  db/schema.sql, docker-compose.yml, .env, Makefile, README.md, .gitignore
//	  model.json            ← 테이블 설정의 기준 (모든 언어, 하위 프로젝트 경로)
//	  <name>_go | <name>_spring | <name>_rust | <name>_web | <name>_vue
//	    model.json          ← 포인터 {"modelJsonPath": "..", "languages": {"go": true}}

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"gomachine/config"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 표준 자산 (bootstrap/). ~/bin/buildtool/bootstrap 이 없을 때(소스에서 실행 등) 쓴다.
//
//go:embed all:bootstrap
var embeddedBootstrap embed.FS

// templateSpec 은 언어별 basicproject 템플릿이다.
type templateSpec struct {
	Lang    string // 플래그 / model.json languages 이름
	Repo    string // GitHub kobums/<Repo>, 로컬 <source>/<Repo>
	Suffix  string // 하위 프로젝트 이름 <name>_<Suffix>
	Backend bool
	Port    int // 백엔드 기본 포트 (프런트 VITE_PROXY_TARGET)
}

var initTemplates = []templateSpec{
	{"go", "basicproject_go", "go", true, 8007},
	{"spring", "basicproject_springboot", "spring", true, 8081},
	{"rust", "basicproject_rust", "rust", true, 8008},
	{"react", "basicproject_web", "web", false, 0},
	{"vue", "basicproject_vue", "vue", false, 0},
}

const defaultKotlinPackage = "com.basicproject.api"

type initOptions struct {
	Name      string
	Parent    string
	Source    string // "github" 또는 basicproject_* 가 있는 로컬 디렉터리
	Branch    string
	Langs     []templateSpec
	NoGit     bool
	Force     bool
	Gen       bool
	DBUp      bool
	DBPort    int
	Package   string // Spring 패키지 (비면 템플릿 그대로)
	Defaulted bool   // 언어를 고르지 않아 go + react 를 썼다
}

// initSecrets 는 작업 공간 전체가 함께 쓰는 DB / JWT 값이다.
type initSecrets struct {
	DBPassword   string
	RootPassword string
	JWTSecret    string
}

// subProject 는 복사한 하위 프로젝트 하나다.
type subProject struct {
	Spec      templateSpec
	Name      string // <name>_<suffix>
	Dir       string
	ModelJSON string // 하위 프로젝트 안의 model.json 상대 경로 ("model.json" / "gomachine/model.json")
	OutRel    string // *ModelFilePath (하위 프로젝트 루트 기준, slash)
	KotlinPkg string
	TsOutDir  string
	TsSemi    *bool
	Warnings  []string
}

var initNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// validateInitName 은 작업 공간 이름 규칙이다: [a-z][a-z0-9_]*, 32자 이하 (DB / 사용자 이름으로도 쓴다).
func validateInitName(name string) error {
	if !initNameRe.MatchString(name) {
		return fmt.Errorf("이름 %q 은 [a-z][a-z0-9_]* 형식이어야 한다 (예: safelog)", name)
	}
	if len(name) > 32 {
		return fmt.Errorf("이름 %q 이 너무 길다 (32자 이하)", name)
	}
	if strings.HasSuffix(name, "_") {
		return fmt.Errorf("이름 %q 은 _ 로 끝날 수 없다", name)
	}
	return nil
}

var kotlinPackageRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

func parseInitArgs(args []string) (initOptions, error) {
	opt := initOptions{}
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	sel := map[string]*bool{}
	for _, t := range initTemplates {
		sel[t.Lang] = fs.Bool(t.Lang, false, t.Repo)
	}
	fs.StringVar(&opt.Parent, "dir", "", "")
	fs.StringVar(&opt.Source, "template-source", "github", "")
	fs.StringVar(&opt.Branch, "branch", "main", "")
	fs.BoolVar(&opt.NoGit, "no-git", false, "")
	fs.BoolVar(&opt.Force, "force", false, "")
	fs.BoolVar(&opt.Gen, "gen", false, "")
	fs.BoolVar(&opt.DBUp, "db-up", false, "")
	fs.IntVar(&opt.DBPort, "db-port", 3306, "")
	fs.StringVar(&opt.Package, "package", "", "")

	// 이름은 플래그 앞이나 뒤 어디든 둘 수 있다.
	rest := args
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		opt.Name = rest[0]
		rest = rest[1:]
	}
	if err := fs.Parse(rest); err != nil {
		return opt, err
	}
	extra := fs.Args()
	if opt.Name == "" && len(extra) > 0 {
		opt.Name = extra[0]
		extra = extra[1:]
		if err := fs.Parse(extra); err != nil {
			return opt, err
		}
		extra = fs.Args()
	}
	if len(extra) > 0 {
		return opt, fmt.Errorf("알 수 없는 인자: %v", extra)
	}
	if opt.Name == "" {
		return opt, errors.New("이름이 필요하다: buildtool-model init <name> [--go --spring --rust --react --vue]")
	}
	if err := validateInitName(opt.Name); err != nil {
		return opt, err
	}
	for _, t := range initTemplates {
		if *sel[t.Lang] {
			opt.Langs = append(opt.Langs, t)
		}
	}
	if len(opt.Langs) == 0 {
		opt.Defaulted = true
		opt.Langs = []templateSpec{initTemplates[0], initTemplates[3]}
	}
	backend := false
	for _, t := range opt.Langs {
		backend = backend || t.Backend
	}
	if !backend {
		return opt, errors.New("백엔드(--go / --spring / --rust)를 하나 이상 골라야 한다")
	}
	if opt.DBPort < 1 || opt.DBPort > 65535 {
		return opt, fmt.Errorf("--db-port %d 가 잘못되었다", opt.DBPort)
	}
	if opt.Package != "" && !kotlinPackageRe.MatchString(opt.Package) {
		return opt, fmt.Errorf("--package %q 는 com.example.app 형식이어야 한다", opt.Package)
	}
	if opt.Branch == "" {
		opt.Branch = "main"
	}
	return opt, nil
}

const initUsage = `사용법: buildtool-model init <name> [flags]

  --go --spring --rust     백엔드 (하나 이상, 아무 언어도 없으면 --go --react)
  --react --vue            프런트엔드
  --dir <parent>           작업 공간을 만들 상위 디렉터리 (기본: 현재 디렉터리)
  --template-source <src>  github (기본, kobums/basicproject_*) 또는 basicproject_* 가 있는 로컬 디렉터리
  --branch <name>          템플릿 브랜치 (github, 기본 main)
  --db-port <port>         DB 포트 (기본 3306)
  --package <pkg>          Spring 기본 패키지 바꾸기 (기본 com.basicproject.api 그대로)
  --no-git                 하위 프로젝트에 git init / 첫 커밋을 하지 않는다
  --db-up                  docker compose up -d --wait db
  --gen                    DB 에 붙을 수 있으면 모든 언어 코드를 다시 생성한다
  --force                  비어 있지 않은 <name> 디렉터리도 쓴다 (하위 프로젝트 디렉터리는 없어야 한다)
`

// runInit 은 init 명령이다. 종료 코드를 돌려준다.
func runInit(args []string, out io.Writer) int {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "-help" {
			fmt.Fprint(out, initUsage)
			return 0
		}
	}
	opt, err := parseInitArgs(args)
	if err != nil {
		fmt.Fprintf(out, "ERROR: %v\n\n%s", err, initUsage)
		return 2
	}
	if err := createWorkspace(opt, out); err != nil {
		fmt.Fprintf(out, "ERROR: %v\n", err)
		return 1
	}
	return 0
}

// createWorkspace 는 작업 공간을 만든다. 실패하면 이번에 만든 디렉터리만 지운다.
func createWorkspace(opt initOptions, out io.Writer) (retErr error) {
	parent := opt.Parent
	if parent == "" {
		var err error
		if parent, err = os.Getwd(); err != nil {
			return err
		}
	}
	parent, err := filepath.Abs(parent)
	if err != nil {
		return err
	}
	if st, err := os.Stat(parent); err != nil || !st.IsDir() {
		return fmt.Errorf("상위 디렉터리 %s 가 없다", parent)
	}
	root := filepath.Join(parent, opt.Name)

	createdRoot := false
	if st, err := os.Lstat(root); err == nil {
		if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			return fmt.Errorf("%s 가 이미 있고 디렉터리가 아니다", root)
		}
		empty, err := dirEmpty(root)
		if err != nil {
			return err
		}
		if !empty && !opt.Force {
			return fmt.Errorf("%s 가 비어 있지 않다 (--force 로 쓸 수 있다)", root)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(root, 0o755); err != nil {
			return err
		}
		createdRoot = true
	} else {
		return err
	}

	// 로컬 템플릿이 작업 공간 안에 있으면 자기 자신을 복사하게 된다.
	if opt.Source != "github" {
		src, err := filepath.Abs(opt.Source)
		if err != nil {
			return err
		}
		if within(src, root) {
			return fmt.Errorf("--template-source %s 가 작업 공간 안에 있다", src)
		}
		opt.Source = src
	}

	subs := make([]*subProject, 0, len(opt.Langs))
	for _, t := range opt.Langs {
		dir := filepath.Join(root, opt.Name+"_"+t.Suffix)
		if _, err := os.Lstat(dir); err == nil {
			return fmt.Errorf("%s 가 이미 있다 (하위 프로젝트는 덮어쓰지 않는다)", dir)
		}
	}

	createdSubs := []string{}
	defer func() {
		if retErr == nil {
			return
		}
		if createdRoot {
			os.RemoveAll(root)
			return
		}
		for _, d := range createdSubs {
			os.RemoveAll(d)
		}
	}()

	secrets, err := workspaceSecrets(root, opt.Force)
	if err != nil {
		return err
	}

	assets, assetDesc := bootstrapAssets()
	fmt.Fprintf(out, "작업 공간: %s\n", root)
	if opt.Defaulted {
		fmt.Fprintln(out, "언어를 고르지 않아 기본값 --go --react 를 쓴다.")
	}
	fmt.Fprintf(out, "표준 자산: %s\n", assetDesc)

	if err := writeRootFiles(root, opt, secrets, assets); err != nil {
		return err
	}

	proxy := ""
	for _, t := range opt.Langs {
		if t.Backend {
			proxy = fmt.Sprintf("http://localhost:%d", t.Port)
			break
		}
	}

	for _, t := range opt.Langs {
		sp := &subProject{Spec: t, Name: opt.Name + "_" + t.Suffix}
		sp.Dir = filepath.Join(root, sp.Name)
		fmt.Fprintf(out, "- %s ← %s\n", sp.Name, t.Repo)
		createdSubs = append(createdSubs, sp.Dir)
		if err := fetchTemplate(opt, t, sp.Dir); err != nil {
			return err
		}
		if err := prepareSubProject(sp, opt, secrets, proxy); err != nil {
			return fmt.Errorf("%s: %v", sp.Name, err)
		}
		for _, w := range sp.Warnings {
			fmt.Fprintf(out, "  WARNING: %s\n", w)
		}
		subs = append(subs, sp)
	}

	model, err := rootModelJSON(opt, subs, assets)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "model.json"), model, 0o644); err != nil {
		return err
	}

	if !opt.NoGit {
		for _, sp := range subs {
			if err := gitInitCommit(sp.Dir, "init from "+sp.Spec.Repo+" via gomachine", sp.Spec.Backend); err != nil {
				fmt.Fprintf(out, "WARNING: %s git: %v\n", sp.Name, err)
			}
		}
	}

	if opt.DBUp {
		if err := composeDBUp(root, out); err != nil {
			fmt.Fprintf(out, "WARNING: DB 를 띄우지 못했다: %v\n", err)
		}
	}
	generated := false
	if opt.Gen {
		generated = runInitGen(root, opt, secrets, out)
	}

	printInitSummary(out, root, opt, subs, proxy, generated)
	return nil
}

func dirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

// within 은 p 가 dir 이거나 그 아래인지다.
func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func newSecrets() initSecrets {
	jwt := make([]byte, 48)
	if _, err := rand.Read(jwt); err != nil {
		panic(err)
	}
	return initSecrets{
		DBPassword:   randomHex(16),
		RootPassword: randomHex(16),
		JWTSecret:    base64.StdEncoding.EncodeToString(jwt),
	}
}

// workspaceSecrets 는 새 비밀값을 만든다. --force 로 기존 작업 공간에 쓰면 루트 .env 의 값을 그대로 쓴다 (DB 볼륨과 맞게).
func workspaceSecrets(root string, force bool) (initSecrets, error) {
	s := newSecrets()
	if !force {
		return s, nil
	}
	b, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		return s, nil
	}
	env := parseEnv(string(b))
	if v := env["MARIADB_PASSWORD"]; v != "" {
		s.DBPassword = v
	}
	if v := env["MARIADB_ROOT_PASSWORD"]; v != "" {
		s.RootPassword = v
	}
	if v := env["JWT_SECRET"]; len(v) >= 32 {
		s.JWTSecret = v
	}
	return s, nil
}

// bootstrapAssets 는 ~/bin/buildtool/bootstrap (make install) 이 있으면 그것을, 없으면 바이너리에 넣은 bootstrap/ 을 돌려준다.
func bootstrapAssets() (fs.FS, string) {
	if home := os.Getenv("HOME"); home != "" {
		dir := filepath.Join(home, "bin", "buildtool", "bootstrap")
		if _, err := os.Stat(filepath.Join(dir, "db", "schema.sql")); err == nil {
			return os.DirFS(dir), dir
		}
	}
	sub, err := fs.Sub(embeddedBootstrap, "bootstrap")
	if err != nil {
		panic(err)
	}
	return sub, "내장 bootstrap/"
}

// parseEnv 는 .env 의 KEY=VALUE 를 읽는다 (주석 / 빈 줄 제외, 따옴표 제거).
func parseEnv(content string) map[string]string {
	env := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		s = strings.TrimPrefix(s, "export ")
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		env[strings.TrimSpace(k)] = v
	}
	return env
}

// setEnv 는 .env 내용에서 key 의 (주석이 아닌) 줄 값을 바꾸고, 없는 key 는 끝에 붙인다. 순서 / 주석은 그대로다.
func setEnv(content string, kv [][2]string) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if content == "" {
		lines = nil
	}
	done := map[string]bool{}
	want := map[string]string{}
	for _, p := range kv {
		want[p[0]] = p[1]
	}
	for i, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		k, _, ok := strings.Cut(strings.TrimPrefix(s, "export "), "=")
		k = strings.TrimSpace(k)
		if v, has := want[k]; ok && has {
			lines[i] = k + "=" + v
			done[k] = true
		}
	}
	added := false
	for _, p := range kv {
		if done[p[0]] {
			continue
		}
		if !added {
			lines = append(lines, "", "# gomachine init")
			added = true
		}
		lines = append(lines, p[0]+"="+p[1])
		done[p[0]] = true
	}
	return strings.Join(lines, "\n") + "\n"
}

// backendEnv 는 모든 백엔드와 루트가 같이 쓰는 DB / JWT 값이다.
func backendEnv(name string, port int, s initSecrets) [][2]string {
	return [][2]string{
		{"MARIADB_DATABASE", name},
		{"MARIADB_USER", name},
		{"MARIADB_PASSWORD", s.DBPassword},
		{"MARIADB_ROOT_PASSWORD", s.RootPassword},
		{"DB_PORT", strconv.Itoa(port)},
		{"DB_HOST", "127.0.0.1"},
		{"DB_NAME", name},
		{"DB_USER", name},
		{"DB_PASSWORD", s.DBPassword},
		{"JWT_SECRET", s.JWTSecret},
	}
}

func readAsset(assets fs.FS, name string) ([]byte, error) {
	b, err := fs.ReadFile(assets, name)
	if err != nil {
		return nil, fmt.Errorf("표준 자산 %s: %v", name, err)
	}
	return b, nil
}

// writeRootFiles 는 작업 공간 루트의 DB / 설정 파일을 쓴다 (model.json 은 하위 프로젝트를 본 뒤에).
func writeRootFiles(root string, opt initOptions, s initSecrets, assets fs.FS) error {
	schema, err := readAsset(assets, "db/schema.sql")
	if err != nil {
		return err
	}
	compose, err := readAsset(assets, "docker-compose.yml")
	if err != nil {
		return err
	}
	envExample, err := readAsset(assets, ".env.example")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "db"), 0o755); err != nil {
		return err
	}
	files := []struct {
		name string
		data []byte
		mode os.FileMode
	}{
		{"db/schema.sql", schema, 0o644},
		{"docker-compose.yml", compose, 0o644},
		{".env", []byte(setEnv(string(envExample), backendEnv(opt.Name, opt.DBPort, s))), 0o600},
		{".gitignore", []byte(".env\n.env.*\n!.env.example\n"), 0o644},
		{"Makefile", []byte(rootMakefile), 0o644},
		{"README.md", []byte(rootReadme(opt)), 0o644},
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(f.name)), f.data, f.mode); err != nil {
			return err
		}
	}
	return nil
}

const rootMakefile = `# 작업 공간 루트: DB 와 코드 생성 (gomachine init 이 만든 파일).
-include .env
export

BUILDTOOL ?= $(shell command -v buildtool-model 2>/dev/null || echo $(HOME)/bin/buildtool-model)
COMPOSE   ?= docker compose

.PHONY: db-up db-down db-reset gen

# DB 를 띄우고 healthcheck 가 통과할 때까지 기다린다.
db-up:
	$(COMPOSE) up -d --wait db

db-down:
	$(COMPOSE) down

# 볼륨까지 지우고 db/schema.sql 로 다시 만든다.
db-reset:
	$(COMPOSE) down -v
	$(COMPOSE) up -d --wait db

# model.json 의 모든 언어 코드를 다시 만든다 (DB 가 떠 있어야 한다).
gen:
	$(BUILDTOOL) .
`

func rootReadme(opt initOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", opt.Name)
	b.WriteString("gomachine `buildtool-model init` 으로 만든 작업 공간이다.\n\n")
	b.WriteString("| 디렉터리 | 템플릿 |\n|---|---|\n")
	for _, t := range opt.Langs {
		port := ""
		if t.Backend {
			port = fmt.Sprintf(" (포트 %d)", t.Port)
		}
		fmt.Fprintf(&b, "| `%s_%s` | %s%s |\n", opt.Name, t.Suffix, t.Repo, port)
	}
	b.WriteString("\n## 시작\n\n```bash\nmake db-up          # 또는 docker compose up -d db\n")
	for _, t := range opt.Langs {
		fmt.Fprintf(&b, "%s\n", runHint(opt.Name, t))
	}
	b.WriteString("```\n\n")
	b.WriteString("- DB 는 이 디렉터리의 `docker-compose.yml` 하나로 띄운다. 하위 프로젝트의 `docker-compose.yml` 은 따로 쓸 때용이다 (같은 포트라 함께 띄우지 않는다).\n")
	b.WriteString("- `.env` (커밋하지 않는다): DB 계정 / 비밀번호와 `JWT_SECRET` 은 모든 백엔드 `.env` 와 같다. 바꾸면 모두 같이 바꾼다.\n")
	b.WriteString("- `model.json` 이 테이블 설정의 기준이다. 하위 프로젝트의 `model.json` 은 이 파일을 가리키는 포인터라서 하위 프로젝트에서 `make gen` 등을 해도 이 설정으로 그 언어만 만든다.\n")
	b.WriteString("- 테이블을 바꾸면 `db/schema.sql` 을 고치고 `make db-reset` 후 `make gen` (모든 언어).\n")
	return b.String()
}

func runHint(name string, t templateSpec) string {
	dir := name + "_" + t.Suffix
	switch t.Lang {
	case "go":
		return fmt.Sprintf("(cd %s && go run .)                 # :8007", dir)
	case "spring":
		return fmt.Sprintf("(cd %s && ./gradlew bootRun)        # :8081", dir)
	case "rust":
		return fmt.Sprintf("(cd %s && cargo run)                # :8008", dir)
	default:
		return fmt.Sprintf("(cd %s && npm install && npm run dev)", dir)
	}
}

// fetchTemplate 은 템플릿을 dest 에 받는다 (.git 은 지운다).
func fetchTemplate(opt initOptions, t templateSpec, dest string) error {
	if opt.Source == "github" {
		url := "https://github.com/kobums/" + t.Repo + ".git"
		cmd := exec.Command("git", "clone", "--quiet", "--depth", "1", "--branch", opt.Branch, url, dest)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git clone %s (branch %s): %v %s", url, opt.Branch, err, strings.TrimSpace(stderr.String()))
		}
		return os.RemoveAll(filepath.Join(dest, ".git"))
	}
	src := filepath.Join(opt.Source, t.Repo)
	if within(dest, src) {
		return fmt.Errorf("작업 공간 %s 가 템플릿 %s 안에 있다", dest, src)
	}
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return fmt.Errorf("템플릿 %s 이 없다", src)
	}
	return copyTemplate(src, dest)
}

// 로컬 템플릿에서 복사하지 않는 것: 빌드 산출물 / 의존성 / 로컬 비밀값.
var copySkipAnywhere = map[string]bool{".git": true, "node_modules": true, ".DS_Store": true, ".idea": true}
var copySkipTop = map[string]bool{"dist": true, "build": true, "target": true, "bin": true, ".gradle": true, ".kotlin": true,
	"webdata": true, "uploads": true, ".env.yml": true}

func skipCopy(rel string, name string) bool {
	if copySkipAnywhere[name] {
		return true
	}
	if !strings.Contains(rel, "/") && copySkipTop[name] {
		return true
	}
	if name == ".env.local" || strings.HasPrefix(name, ".env.") && strings.HasSuffix(name, ".local") {
		return true
	}
	return rel == ".claude/settings.local.json"
}

// copyTemplate 은 src 를 dest 로 복사한다. 심볼릭 링크는 링크로 둔다.
func copyTemplate(src, dest string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if rel == "." {
			return os.MkdirAll(target, 0o755)
		}
		if skipCopy(filepath.ToSlash(rel), d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode().IsRegular():
			return copyFile(p, target, info.Mode().Perm())
		}
		return nil
	})
}

func copyFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// prepareSubProject 는 복사한 템플릿의 이름을 바꾸고 .env / 포인터 model.json 을 쓴다.
func prepareSubProject(sp *subProject, opt initOptions, s initSecrets, proxy string) error {
	if err := readTemplateModel(sp); err != nil {
		return err
	}
	pairs, err := renamePairs(sp)
	if err != nil {
		return err
	}
	if sp.Spec.Lang == "spring" && opt.Package != "" && opt.Package != sp.KotlinPkg {
		if err := renameKotlinPackage(sp, opt.Package); err != nil {
			return err
		}
	}
	if err := replaceInTree(sp.Dir, pairs); err != nil {
		return err
	}
	// module 이름이 바뀌면 import 정렬 순서가 바뀔 수 있다 (make gen 의 gofmt 결과와 같게).
	if sp.Spec.Lang == "go" {
		if gofmt, err := exec.LookPath("gofmt"); err == nil {
			cmd := exec.Command(gofmt, "-w", ".")
			cmd.Dir = sp.Dir
			cmd.Run()
		}
	}
	if !sp.Spec.Backend {
		if err := renameFrontendBrand(sp.Dir, opt.Name); err != nil {
			return err
		}
	}
	if err := writeSubEnv(sp, opt, s, proxy); err != nil {
		return err
	}
	return writePointer(sp)
}

// readTemplateModel 은 템플릿 model.json (루트 또는 gomachine/) 에서 출력 경로 / Kotlin 패키지 / TS 설정을 읽는다.
func readTemplateModel(sp *subProject) error {
	sp.ModelJSON = "model.json"
	for _, c := range []string{"model.json", "gomachine/model.json"} {
		if _, err := os.Stat(filepath.Join(sp.Dir, filepath.FromSlash(c))); err == nil {
			sp.ModelJSON = c
			break
		}
	}
	sp.OutRel = "."
	sp.KotlinPkg = defaultKotlinPackage
	b, err := os.ReadFile(filepath.Join(sp.Dir, filepath.FromSlash(sp.ModelJSON)))
	if err != nil {
		if sp.Spec.Lang == "spring" {
			sp.OutRel = "src/main/kotlin/" + strings.ReplaceAll(sp.KotlinPkg, ".", "/")
		}
		return nil
	}
	cnf, _, err := config.Parse(b)
	if err != nil {
		return fmt.Errorf("템플릿 %s: %v", sp.ModelJSON, err)
	}
	lang := sp.Spec.Lang
	if lang == "spring" {
		lang = "kotlin"
		if p := strings.TrimSpace(cnf.KotlinPackage); p != "" {
			sp.KotlinPkg = p
		}
	}
	cnf.Language = lang
	mjDir := path.Dir(sp.ModelJSON)
	if p := strings.TrimSpace(cnf.OutputPath(lang)); p != "" && !filepath.IsAbs(p) {
		sp.OutRel = path.Clean(path.Join(mjDir, filepath.ToSlash(p)))
	} else if lang == "kotlin" {
		sp.OutRel = "src/main/kotlin/" + strings.ReplaceAll(sp.KotlinPkg, ".", "/")
	}
	// 예전 템플릿처럼 프로젝트 밖을 가리키면 기본 경로를 쓴다.
	if sp.OutRel == ".." || strings.HasPrefix(sp.OutRel, "../") {
		def := "."
		if lang == "kotlin" {
			def = "src/main/kotlin/" + strings.ReplaceAll(sp.KotlinPkg, ".", "/")
		}
		sp.Warnings = append(sp.Warnings, fmt.Sprintf("템플릿 %s 의 출력 경로 %q 가 프로젝트 밖이라 %q 를 쓴다", sp.ModelJSON, sp.OutRel, def))
		sp.OutRel = def
	}
	if lang == "react" || lang == "vue" {
		sp.TsOutDir = cnf.GetTsOutDir()
		semi := cnf.UseTsSemicolons()
		sp.TsSemi = &semi
	}
	return nil
}

var (
	settingsNameRe = regexp.MustCompile(`rootProject\.name\s*=\s*['"]([^'"]+)['"]`)
	cargoNameRe    = regexp.MustCompile(`(?m)^\s*name\s*=\s*"([^"]+)"`)
)

// templateIdentifiers 는 템플릿 프로젝트 이름 (go.mod module, settings.gradle rootProject.name, Cargo.toml / package.json name) 과 저장소 이름이다.
func templateIdentifiers(dir string, t templateSpec) []string {
	ids := []string{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		for _, x := range ids {
			if x == s {
				return
			}
		}
		if s != "" {
			ids = append(ids, s)
		}
	}
	switch t.Lang {
	case "go":
		add(config.GoModule(dir))
	case "spring":
		for _, f := range []string{"settings.gradle", "settings.gradle.kts"} {
			if b, err := os.ReadFile(filepath.Join(dir, f)); err == nil {
				if m := settingsNameRe.FindSubmatch(b); m != nil {
					add(string(m[1]))
				}
			}
		}
	case "rust":
		if b, err := os.ReadFile(filepath.Join(dir, "Cargo.toml")); err == nil {
			if i := bytes.Index(b, []byte("[package]")); i >= 0 {
				if m := cargoNameRe.FindSubmatch(b[i:]); m != nil {
					add(string(m[1]))
					// crate 이름의 - 는 코드에서 _ 다.
					add(strings.ReplaceAll(string(m[1]), "-", "_"))
				}
			}
		}
	default:
		if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
			var pkg struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(b, &pkg) == nil {
				add(pkg.Name)
			}
		}
	}
	add(t.Repo)
	return ids
}

// renamePairs 는 템플릿 이름 → 새 이름 치환 목록이다 (긴 것 먼저).
func renamePairs(sp *subProject) ([][2]string, error) {
	ids := templateIdentifiers(sp.Dir, sp.Spec)
	pairs := make([][2]string, 0, len(ids))
	for _, id := range ids {
		if id != sp.Name {
			pairs = append(pairs, [2]string{id, sp.Name})
		}
	}
	for i := 1; i < len(pairs); i++ {
		for j := i; j > 0 && len(pairs[j][0]) > len(pairs[j-1][0]); j-- {
			pairs[j], pairs[j-1] = pairs[j-1], pairs[j]
		}
	}
	return pairs, nil
}

// 치환하지 않는 디렉터리 (복사하지 않았거나 받은 그대로 둘 것).
var replaceSkipDirs = map[string]bool{".git": true, "node_modules": true, "target": true, ".gradle": true, ".kotlin": true}

const maxReplaceSize = 4 << 20

// replaceInTree 는 dir 아래 텍스트 파일에서 pairs 를 차례로 치환한다 (바이너리 / 큰 파일 / 심볼릭 링크 제외).
func replaceInTree(dir string, pairs [][2]string) error {
	if len(pairs) == 0 {
		return nil
	}
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && replaceSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxReplaceSize {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0 {
			return nil
		}
		nb := b
		for _, pr := range pairs {
			nb = bytes.ReplaceAll(nb, []byte(pr[0]), []byte(pr[1]))
		}
		if bytes.Equal(nb, b) {
			return nil
		}
		return os.WriteFile(p, nb, info.Mode().Perm())
	})
}

var brandRe = regexp.MustCompile(`\bbasicproject(\W|$)`)

// renameFrontendBrand 는 프런트 화면 / 저장소 key 의 "basicproject" (basicproject_xxx 제외)를 이름으로 바꾼다 (index.html, src/).
func renameFrontendBrand(dir, name string) error {
	repl := func(p string) error {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		nb := brandRe.ReplaceAll(b, []byte(name+"$1"))
		if bytes.Equal(b, nb) {
			return nil
		}
		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		return os.WriteFile(p, nb, info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
		if err := repl(filepath.Join(dir, "index.html")); err != nil {
			return err
		}
	}
	src := filepath.Join(dir, "src")
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		switch filepath.Ext(p) {
		case ".ts", ".tsx", ".vue", ".js", ".jsx", ".html", ".css":
			return repl(p)
		}
		return nil
	})
}

// renameKotlinPackage 는 Spring 기본 패키지를 바꾼다: src/*/kotlin|java 아래 디렉터리 이동 + 텍스트 치환 + build.gradle group.
func renameKotlinPackage(sp *subProject, pkg string) error {
	oldPkg := sp.KotlinPkg
	oldPath := strings.ReplaceAll(oldPkg, ".", "/")
	newPath := strings.ReplaceAll(pkg, ".", "/")
	sets, _ := filepath.Glob(filepath.Join(sp.Dir, "src", "*"))
	for _, set := range sets {
		for _, lang := range []string{"kotlin", "java"} {
			from := filepath.Join(set, lang, filepath.FromSlash(oldPath))
			if st, err := os.Stat(from); err != nil || !st.IsDir() {
				continue
			}
			to := filepath.Join(set, lang, filepath.FromSlash(newPath))
			if _, err := os.Stat(to); err == nil {
				return fmt.Errorf("패키지 디렉터리 %s 가 이미 있다", to)
			}
			if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
				return err
			}
			if err := os.Rename(from, to); err != nil {
				return err
			}
			// 비게 된 예전 상위 디렉터리를 지운다 (com/basicproject).
			base := filepath.Join(set, lang)
			for d := filepath.Dir(from); d != base && within(d, base); d = filepath.Dir(d) {
				if empty, _ := dirEmpty(d); !empty || os.Remove(d) != nil {
					break
				}
			}
		}
	}
	oldGroup := oldPkg[:strings.LastIndex(oldPkg, ".")]
	newGroup := pkg[:strings.LastIndex(pkg, ".")]
	groupRe := regexp.MustCompile(`(?m)^(group\s*=\s*['"])` + regexp.QuoteMeta(oldGroup) + `(['"])`)
	for _, f := range []string{"build.gradle", "build.gradle.kts"} {
		p := filepath.Join(sp.Dir, f)
		if b, err := os.ReadFile(p); err == nil {
			if err := os.WriteFile(p, groupRe.ReplaceAll(b, []byte("${1}"+newGroup+"${2}")), 0o644); err != nil {
				return err
			}
		}
	}
	if err := replaceInTree(sp.Dir, [][2]string{{oldPkg, pkg}, {oldPath, newPath}}); err != nil {
		return err
	}
	sp.KotlinPkg = pkg
	sp.OutRel = strings.Replace(sp.OutRel, oldPath, newPath, 1)
	return nil
}

// writeSubEnv 는 하위 프로젝트 .env 를 .env.example (없으면 템플릿 .env) 에서 만든다.
// 백엔드: 작업 공간과 같은 DB 값 / JWT_SECRET. 프런트: VITE_PROXY_TARGET = 첫 백엔드.
func writeSubEnv(sp *subProject, opt initOptions, s initSecrets, proxy string) error {
	base := ""
	for _, f := range []string{".env.example", ".env"} {
		if b, err := os.ReadFile(filepath.Join(sp.Dir, f)); err == nil {
			base = string(b)
			break
		}
	}
	var kv [][2]string
	mode := os.FileMode(0o600)
	if sp.Spec.Backend {
		kv = backendEnv(opt.Name, opt.DBPort, s)
	} else {
		kv = [][2]string{{"VITE_PROXY_TARGET", proxy}}
		mode = 0o644
	}
	return os.WriteFile(filepath.Join(sp.Dir, ".env"), []byte(setEnv(base, kv)), mode)
}

// pointerModel 은 하위 프로젝트 model.json 이다: 작업 공간 model.json 을 가리키고 그 언어만 만든다.
type pointerModel struct {
	ModelJsonPath string          `json:"modelJsonPath"`
	Languages     map[string]bool `json:"languages"`
}

func writePointer(sp *subProject) error {
	depth := strings.Count(sp.ModelJSON, "/") + 1
	rel := strings.TrimSuffix(strings.Repeat("../", depth), "/")
	b, err := json.MarshalIndent(pointerModel{rel, map[string]bool{sp.Spec.Lang: true}}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sp.Dir, filepath.FromSlash(sp.ModelJSON)), append(b, '\n'), 0o644)
}

// rootModel 은 작업 공간 model.json (키 순서 고정) 이다.
type rootModel struct {
	Buildtool           string          `json:"buildtool"`
	Server              string          `json:"server"`
	Port                int             `json:"port"`
	Database            string          `json:"database"`
	User                string          `json:"user"`
	Password            string          `json:"password"`
	Auth                string          `json:"auth"`
	AdminLevel          string          `json:"adminLevel"`
	Languages           map[string]bool `json:"languages"`
	GoModelFilePath     string          `json:"goModelFilePath,omitempty"`
	KotlinModelFilePath string          `json:"kotlinModelFilePath,omitempty"`
	KotlinPackage       string          `json:"kotlinPackage,omitempty"`
	SpringBoot          int             `json:"springBoot,omitempty"`
	RustModelFilePath   string          `json:"rustModelFilePath,omitempty"`
	ReactModelFilePath  string          `json:"reactModelFilePath,omitempty"`
	ReactTsOutDir       string          `json:"reactTsOutDir,omitempty"`
	ReactTsSemicolons   *bool           `json:"reactTsSemicolons,omitempty"`
	VueModelFilePath    string          `json:"vueModelFilePath,omitempty"`
	VueTsOutDir         string          `json:"vueTsOutDir,omitempty"`
	VueTsSemicolons     *bool           `json:"vueTsSemicolons,omitempty"`
	Table               json.RawMessage `json:"table"`
}

// rootModelJSON 은 표준 model.json (bootstrap) 의 auth / table 에 고른 언어와 하위 프로젝트 경로를 채운다.
func rootModelJSON(opt initOptions, subs []*subProject, assets fs.FS) ([]byte, error) {
	b, err := readAsset(assets, "model.json")
	if err != nil {
		return nil, err
	}
	base, _, err := config.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("표준 model.json: %v", err)
	}
	var raw struct {
		Table json.RawMessage `json:"table"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	if len(raw.Table) == 0 {
		raw.Table = json.RawMessage("[]")
	}
	m := rootModel{
		Buildtool:  base.Buildtool,
		Server:     "127.0.0.1",
		Port:       opt.DBPort,
		Database:   opt.Name,
		User:       opt.Name,
		Auth:       base.Auth,
		AdminLevel: base.AdminLevel,
		Languages:  map[string]bool{},
		Table:      raw.Table,
	}
	for _, t := range initTemplates {
		m.Languages[t.Lang] = false
	}
	for _, sp := range subs {
		m.Languages[sp.Spec.Lang] = true
		p := "./" + path.Join(sp.Name, sp.OutRel)
		switch sp.Spec.Lang {
		case "go":
			m.GoModelFilePath = p
		case "spring":
			m.KotlinModelFilePath = p
			m.KotlinPackage = sp.KotlinPkg
			m.SpringBoot = base.GetSpringBoot()
		case "rust":
			m.RustModelFilePath = p
		case "react":
			m.ReactModelFilePath = p
			m.ReactTsOutDir = sp.TsOutDir
			m.ReactTsSemicolons = sp.TsSemi
		case "vue":
			m.VueModelFilePath = p
			m.VueTsOutDir = sp.TsOutDir
			m.VueTsSemicolons = sp.TsSemi
		}
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// gitInitCommit 은 dir 에 git 저장소를 만들고 첫 커밋을 한다 (사용자 git 설정의 이름 / 이메일).
// ignoreEnv 면 .env (비밀값) 가 .gitignore 에 없을 때 넣는다 (백엔드).
func gitInitCommit(dir, msg string, ignoreEnv bool) error {
	init := exec.Command("git", "init", "-q", "-b", "main")
	init.Dir = dir
	if b, err := init.CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %v %s", err, strings.TrimSpace(string(b)))
	}
	if ignoreEnv {
		check := exec.Command("git", "check-ignore", "-q", ".env")
		check.Dir = dir
		if check.Run() != nil {
			f, err := os.OpenFile(filepath.Join(dir, ".gitignore"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			_, err = f.WriteString("\n# 비밀값 (gomachine init)\n.env\n")
			f.Close()
			if err != nil {
				return err
			}
		}
	}
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-q", "-m", msg},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if b, err := cmd.CombinedOutput(); err != nil {
			hint := ""
			if args[0] == "commit" {
				hint = " (git config user.name / user.email 을 설정하고 git commit 을 직접 한다)"
			}
			return fmt.Errorf("git %s: %v %s%s", args[0], err, strings.TrimSpace(string(b)), hint)
		}
	}
	return nil
}

func composeDBUp(root string, out io.Writer) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("docker 가 없다")
	}
	fmt.Fprintln(out, "docker compose up -d --wait db ...")
	cmd := exec.Command("docker", "compose", "up", "-d", "--wait", "db")
	cmd.Dir = root
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// runInitGen 은 DB 에 붙을 수 있으면 이 바이너리로 작업 공간 model.json 의 모든 언어를 생성한다.
func runInitGen(root string, opt initOptions, s initSecrets, out io.Writer) bool {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(opt.DBPort))
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		fmt.Fprintf(out, "DB(%s)에 붙을 수 없어 생성을 건너뛴다. 템플릿에 이미 생성된 user / board 코드가 있다.\n", addr)
		return false
	}
	conn.Close()
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(out, "WARNING: 실행 파일을 찾지 못했다: %v\n", err)
		return false
	}
	cmd := exec.Command(exe, root)
	cmd.Dir = root
	cmd.Env = os.Environ()
	for _, p := range backendEnv(opt.Name, opt.DBPort, s) {
		cmd.Env = append(cmd.Env, p[0]+"="+p[1])
	}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(out, "WARNING: 생성 실패: %v\n", err)
		return false
	}
	return true
}

func printInitSummary(out io.Writer, root string, opt initOptions, subs []*subProject, proxy string, generated bool) {
	fmt.Fprintf(out, "\n완료: %s\n\n", root)
	for _, sp := range subs {
		extra := ""
		if sp.Spec.Backend {
			extra = fmt.Sprintf("포트 %d", sp.Spec.Port)
		} else {
			extra = "프록시 → " + proxy
		}
		fmt.Fprintf(out, "  %-24s %s (%s)\n", sp.Name, sp.Spec.Repo, extra)
	}
	fmt.Fprintf(out, "\nDB: %s / 사용자 %s / 포트 %d. 비밀번호와 JWT_SECRET 은 .env (모든 백엔드 같은 값, 커밋 금지)\n", opt.Name, opt.Name, opt.DBPort)
	if generated {
		fmt.Fprintln(out, "코드 생성: 완료")
	}
	fmt.Fprintln(out, "\n다음 단계:")
	fmt.Fprintf(out, "  cd %s\n", root)
	if !opt.DBUp {
		fmt.Fprintln(out, "  docker compose up -d db          # 또는 make db-up")
	}
	for _, t := range opt.Langs {
		fmt.Fprintf(out, "  %s\n", runHint(opt.Name, t))
	}
	fmt.Fprintln(out, "\n테이블을 바꾸면 db/schema.sql 수정 → make db-reset → make gen (작업 공간 model.json 이 기준).")
}
