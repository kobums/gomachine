package main

import (
	"encoding/json"
	"gomachine/config"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateInitName(t *testing.T) {
	for _, ok := range []string{"safelog", "a", "my_app2"} {
		if err := validateInitName(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "Safelog", "1app", "my-app", "app_", "a/b", "../x", strings.Repeat("a", 33)} {
		if validateInitName(bad) == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestParseInitArgs(t *testing.T) {
	opt, err := parseInitArgs([]string{"demo"})
	if err != nil || !opt.Defaulted || len(opt.Langs) != 2 || opt.Langs[0].Lang != "go" || opt.Langs[1].Lang != "react" {
		t.Errorf("default: %+v %v", opt, err)
	}
	// 이름은 플래그 뒤에 와도 된다.
	opt, err = parseInitArgs([]string{"--vue", "--spring", "demo", "--db-port", "3307", "--no-git"})
	if err != nil || opt.Name != "demo" || opt.DBPort != 3307 || !opt.NoGit || len(opt.Langs) != 2 ||
		opt.Langs[0].Lang != "spring" || opt.Langs[1].Lang != "vue" {
		t.Errorf("flags: %+v %v", opt, err)
	}
	for _, bad := range [][]string{{}, {"demo", "--react"}, {"Demo"}, {"demo", "extra"}, {"demo", "--package", "Bad"}, {"demo", "--db-port", "0"}} {
		if _, err := parseInitArgs(bad); err == nil {
			t.Errorf("%v: want error", bad)
		}
	}
}

func TestSetEnv(t *testing.T) {
	in := "# c\nA=1\n# B=old\nB=2\n\nexport C=3\n"
	got := setEnv(in, [][2]string{{"B", "x"}, {"C", "y"}, {"D", "z"}})
	want := "# c\nA=1\n# B=old\nB=x\n\nC=y\n\n# gomachine init\nD=z\n"
	if got != want {
		t.Errorf("got %q", got)
	}
	if env := parseEnv(got); env["B"] != "x" || env["D"] != "z" || env["A"] != "1" {
		t.Errorf("parse %v", env)
	}
}

// fixtureTemplates 는 init 이 읽는 부분만 있는 작은 basicproject_* 디렉터리를 만든다.
func fixtureTemplates(t *testing.T) string {
	src := t.TempDir()
	w := func(rel, data string) {
		p := filepath.Join(src, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	backendEnv := "DB_PORT=3306\nDB_NAME=project\nDB_USER=project\nDB_PASSWORD=change-me\nJWT_SECRET=change-me\nPORT=1\n"
	w("basicproject_go/go.mod", "module basicproject_go\n\ngo 1.21\n")
	w("basicproject_go/main.go", "package main\n\nimport _ \"basicproject_go/models\"\n")
	w("basicproject_go/Makefile", "APP := basicproject_go\n")
	w("basicproject_go/.env.example", backendEnv)
	w("basicproject_go/.env.local", "SECRET=1\n")
	w("basicproject_go/model.json", `{"languages": {"go": true}, "goModelFilePath": "./"}`)
	w("basicproject_go/.git/HEAD", "ref\n")
	w("basicproject_go/bin/basicproject_go", "\x00binary")
	w("basicproject_springboot/settings.gradle", "rootProject.name = 'basicproject_springboot'\n")
	w("basicproject_springboot/build.gradle", "group = 'com.basicproject'\n")
	w("basicproject_springboot/.env.example", backendEnv)
	w("basicproject_springboot/model.json", `{"kotlinModelFilePath": "src/main/kotlin/com/basicproject/api", "kotlinPackage": "com.basicproject.api", "springBoot": 4}`)
	w("basicproject_springboot/src/main/kotlin/com/basicproject/api/App.kt", "package com.basicproject.api\n")
	w("basicproject_springboot/src/test/kotlin/com/basicproject/api/AppTests.kt", "package com.basicproject.api\n")
	w("basicproject_rust/Cargo.toml", "[package]\nname = \"basicproject_rust\"\n\n[dependencies]\nserde = \"1\"\n")
	w("basicproject_rust/Cargo.lock", "[[package]]\nname = \"basicproject_rust\"\n")
	w("basicproject_rust/.env.example", backendEnv)
	w("basicproject_rust/model.json", `{"rustModelFilePath": "."}`)
	w("basicproject_web/package.json", `{"name": "basicproject_web"}`)
	w("basicproject_web/index.html", "<title>basicproject_web</title>\n")
	w("basicproject_web/src/lib/authStorage.ts", "const TOKEN_KEY = 'basicproject.token'\n// basicproject_go\n")
	w("basicproject_web/.env.example", "VITE_API_BASE_URL=/api\nVITE_PROXY_TARGET=http://localhost:8007\n")
	w("basicproject_web/model.json", `{"reactModelFilePath": ".", "tsOutDir": "src/gen", "tsSemicolons": false}`)
	w("basicproject_web/node_modules/x/index.js", "basicproject_web")
	// Vue: .env 를 커밋하고 model.json 이 gomachine/ 아래에 있는 모양
	w("basicproject_vue/package.json", `{"name": "basicproject_vue"}`)
	w("basicproject_vue/.env", "VITE_PROXY_TARGET=http://localhost:8007\n")
	w("basicproject_vue/gomachine/model.json", `{"vueModelFilePath": "..", "tsOutDir": "src"}`)
	return src
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCreateWorkspace(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // 내장 bootstrap 을 쓴다
	src := fixtureTemplates(t)
	parent := t.TempDir()
	args := []string{"demo", "--go", "--spring", "--rust", "--react", "--vue", "--template-source", src, "--dir", parent, "--no-git", "--db-port", "3307"}
	if code := runInit(args, io.Discard); code != 0 {
		t.Fatalf("exit %d", code)
	}
	root := filepath.Join(parent, "demo")

	// 이름 바꾸기
	if s := readFile(t, filepath.Join(root, "demo_go/main.go")); !strings.Contains(s, `"demo_go/models"`) {
		t.Errorf("go import: %s", s)
	}
	if s := readFile(t, filepath.Join(root, "demo_go/go.mod")); !strings.HasPrefix(s, "module demo_go\n") {
		t.Errorf("go.mod: %s", s)
	}
	if s := readFile(t, filepath.Join(root, "demo_go/Makefile")); s != "APP := demo_go\n" {
		t.Errorf("Makefile: %s", s)
	}
	if s := readFile(t, filepath.Join(root, "demo_spring/settings.gradle")); !strings.Contains(s, "'demo_spring'") {
		t.Errorf("settings.gradle: %s", s)
	}
	if s := readFile(t, filepath.Join(root, "demo_rust/Cargo.lock")); !strings.Contains(s, `"demo_rust"`) {
		t.Errorf("Cargo.lock: %s", s)
	}
	if s := readFile(t, filepath.Join(root, "demo_web/src/lib/authStorage.ts")); s != "const TOKEN_KEY = 'demo.token'\n// basicproject_go\n" {
		t.Errorf("authStorage: %q", s)
	}
	if s := readFile(t, filepath.Join(root, "demo_web/index.html")); !strings.Contains(s, "demo_web") {
		t.Errorf("index.html: %s", s)
	}
	// 복사하지 않는 것
	for _, rel := range []string{"demo_go/.git", "demo_go/.env.local", "demo_go/bin", "demo_web/node_modules"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			t.Errorf("%s copied", rel)
		}
	}

	// .env: 모든 백엔드와 루트가 같은 DB / JWT 값
	rootEnv := parseEnv(readFile(t, filepath.Join(root, ".env")))
	if rootEnv["MARIADB_DATABASE"] != "demo" || rootEnv["DB_USER"] != "demo" || rootEnv["DB_PORT"] != "3307" ||
		len(rootEnv["JWT_SECRET"]) != 64 || rootEnv["DB_PASSWORD"] != rootEnv["MARIADB_PASSWORD"] || len(rootEnv["DB_PASSWORD"]) != 32 {
		t.Errorf("root env: %v", rootEnv)
	}
	for _, sub := range []string{"demo_go", "demo_spring", "demo_rust"} {
		env := parseEnv(readFile(t, filepath.Join(root, sub, ".env")))
		for _, k := range []string{"JWT_SECRET", "DB_PASSWORD", "DB_NAME", "DB_USER", "DB_PORT"} {
			if env[k] != rootEnv[k] {
				t.Errorf("%s %s = %q, root %q", sub, k, env[k], rootEnv[k])
			}
		}
		if env["PORT"] != "1" {
			t.Errorf("%s: template key lost: %v", sub, env)
		}
	}
	for _, sub := range []string{"demo_web", "demo_vue"} {
		if env := parseEnv(readFile(t, filepath.Join(root, sub, ".env"))); env["VITE_PROXY_TARGET"] != "http://localhost:8007" || env["JWT_SECRET"] != "" {
			t.Errorf("%s env: %v", sub, env)
		}
	}
	// 다른 작업 공간은 다른 비밀값
	if code := runInit([]string{"demo2", "--go", "--template-source", src, "--dir", parent, "--no-git"}, io.Discard); code != 0 {
		t.Fatal("demo2")
	}
	if parseEnv(readFile(t, filepath.Join(parent, "demo2/.env")))["JWT_SECRET"] == rootEnv["JWT_SECRET"] {
		t.Error("same secret across workspaces")
	}

	// 루트 model.json: 모든 언어, 경로는 model.json 기준
	cnf, warnings, err := config.Load(root)
	if err != nil || len(warnings) > 0 {
		t.Fatalf("model.json: %v %v", err, warnings)
	}
	if got := strings.Join(cnf.EnabledLanguages(), ","); got != "go,kotlin,react,vue,rust" {
		t.Errorf("languages %s", got)
	}
	if cnf.GoModelFilePath != "./demo_go" || cnf.KotlinModelFilePath != "./demo_spring/src/main/kotlin/com/basicproject/api" ||
		cnf.RustModelFilePath != "./demo_rust" || cnf.ReactModelFilePath != "./demo_web" || cnf.VueModelFilePath != "./demo_vue" ||
		cnf.KotlinPackage != "com.basicproject.api" || cnf.GetSpringBoot() != 4 || cnf.Auth != "jwt" || cnf.AdminLevel != "3" ||
		cnf.Database != "demo" || cnf.GetPort() != "3307" || len(cnf.Gpa) != 2 {
		t.Errorf("model.json: %+v", cnf)
	}
	cnf.Language = "react"
	if cnf.GetTsOutDir() != "src/gen" || cnf.UseTsSemicolons() {
		t.Errorf("react ts: %q %v", cnf.GetTsOutDir(), cnf.UseTsSemicolons())
	}
	cnf.Language = "vue"
	if cnf.GetTsOutDir() != "src" {
		t.Errorf("vue ts: %q", cnf.GetTsOutDir())
	}

	// 하위 프로젝트 model.json 은 포인터: 루트 설정 + 그 언어만, 경로는 루트 기준
	for sub, lang := range map[string]string{"demo_go": "go", "demo_spring": "kotlin", "demo_vue/gomachine": "vue"} {
		dir := filepath.Join(root, filepath.FromSlash(sub))
		real, base := config.FollowPointer(dir, config.Init(dir))
		real.ResolvePaths(base)
		if got := strings.Join(real.EnabledLanguages(), ","); got != lang {
			t.Errorf("%s languages %s", sub, got)
		}
		if real.GoModelFilePath != filepath.Join(root, "demo_go") || real.VueModelFilePath != filepath.Join(root, "demo_vue") {
			t.Errorf("%s paths %q %q", sub, real.GoModelFilePath, real.VueModelFilePath)
		}
	}

	// 비어 있지 않은 디렉터리는 --force 없이 거부하고 아무것도 바꾸지 않는다.
	marker := filepath.Join(root, "demo_go", "main.go")
	before := readFile(t, marker)
	if code := runInit([]string{"demo", "--go", "--template-source", src, "--dir", parent, "--no-git"}, io.Discard); code == 0 {
		t.Error("non-empty dir accepted")
	}
	// --force 여도 하위 프로젝트 디렉터리는 덮어쓰지 않는다.
	if code := runInit([]string{"demo", "--go", "--template-source", src, "--dir", parent, "--no-git", "--force"}, io.Discard); code == 0 {
		t.Error("existing sub-project overwritten")
	}
	if readFile(t, marker) != before {
		t.Error("existing workspace changed")
	}
}

func TestCreateWorkspaceFailureCleanup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := fixtureTemplates(t)
	os.RemoveAll(filepath.Join(src, "basicproject_vue"))
	parent := t.TempDir()
	if code := runInit([]string{"demo", "--go", "--vue", "--template-source", src, "--dir", parent, "--no-git"}, io.Discard); code == 0 {
		t.Fatal("missing template accepted")
	}
	if _, err := os.Stat(filepath.Join(parent, "demo")); err == nil {
		t.Error("partial workspace left behind")
	}
}

func TestSpringPackageRename(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := fixtureTemplates(t)
	parent := t.TempDir()
	if code := runInit([]string{"demo", "--spring", "--package", "com.acme.demo", "--template-source", src, "--dir", parent, "--no-git"}, io.Discard); code != 0 {
		t.Fatalf("exit %d", code)
	}
	sp := filepath.Join(parent, "demo", "demo_spring")
	if s := readFile(t, filepath.Join(sp, "src/main/kotlin/com/acme/demo/App.kt")); s != "package com.acme.demo\n" {
		t.Errorf("App.kt %q", s)
	}
	if _, err := os.Stat(filepath.Join(sp, "src/main/kotlin/com/basicproject")); err == nil {
		t.Error("old package dir left")
	}
	if _, err := os.Stat(filepath.Join(sp, "src/test/kotlin/com/acme/demo/AppTests.kt")); err != nil {
		t.Error("test package not moved")
	}
	if s := readFile(t, filepath.Join(sp, "build.gradle")); s != "group = 'com.acme'\n" {
		t.Errorf("build.gradle %q", s)
	}
	var m map[string]any
	json.Unmarshal([]byte(readFile(t, filepath.Join(parent, "demo", "model.json"))), &m)
	if m["kotlinPackage"] != "com.acme.demo" || m["kotlinModelFilePath"] != "./demo_spring/src/main/kotlin/com/acme/demo" {
		t.Errorf("model.json %v", m)
	}
}

func TestGitInitCommit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	os.WriteFile(filepath.Join(dir, ".env"), []byte("JWT_SECRET=x"), 0o600)
	if err := gitInitCommit(dir, "init from basicproject_go via gomachine", true); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil || strings.Contains(string(out), ".env") || !strings.Contains(string(out), "a.txt") {
		t.Errorf("committed files: %q %v", out, err)
	}
}
