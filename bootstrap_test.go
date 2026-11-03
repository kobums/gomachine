package main

import (
	"gomachine/config"
	"gomachine/util"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseArgs(t *testing.T) {
	defer func() { util.Force = false }()
	got := parseArgs([]string{"proj", "--force", "go"})
	if !reflect.DeepEqual(got, []string{"proj", "go"}) || !util.Force {
		t.Errorf("parseArgs: %v force=%v", got, util.Force)
	}
}

// 첫 인자 디렉터리에 model.json 이 있으면 프로젝트 디렉터리 모드, 없으면 예전 ./config/model.json.
func TestProjectDir(t *testing.T) {
	dir := t.TempDir()
	if got := projectDir([]string{dir}); got != "" {
		t.Errorf("no model.json: got %q", got)
	}
	if got := projectDir(nil); got != "" {
		t.Errorf("no args: got %q", got)
	}
	os.WriteFile(filepath.Join(dir, "model.json"), []byte(`{"goModelFilePath": "backend"}`), 0644)
	if got := projectDir([]string{dir, "go"}); got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
	cnf := config.Init(dir)
	cnf.ResolvePaths(dir)
	if cnf.GoModelFilePath != filepath.Join(dir, "backend") {
		t.Errorf("go path %q", cnf.GoModelFilePath)
	}
}

// bootstrap/ 의 표준 자산: model.json 은 모르는 key 없이 읽히고, session key 는 Go Session 필드(Id)와 맞는다.
func TestBootstrapAssets(t *testing.T) {
	cnf, warnings, err := config.Load("bootstrap")
	if err != nil || len(warnings) > 0 {
		t.Fatalf("model.json: %v %v", err, warnings)
	}
	if cnf.Auth != "jwt" || cnf.GetSpringBoot() != 4 {
		t.Errorf("auth=%q boot=%d", cnf.Auth, cnf.GetSpringBoot())
	}
	board := util.FindTableConfig(cnf.Gpa, "board_tb")
	if board == nil || len(board.Session) != 1 || strings.Title(board.Session[0].Key) != "Id" || board.Session[0].Column != "user" {
		t.Errorf("board session: %+v", board)
	}
	// 게시판: 로그인 사용자는 모두 읽고, 작성자(user) 를 조인한다.
	if board == nil || board.Read != "login" || len(board.Join) != 1 || board.Join[0].Name != "user" || board.Join[0].Column != "user" {
		t.Errorf("board read / join: %+v", board)
	}
	if p, err := util.ReadPolicy("Board", board, cnf.Auth); p != util.ReadLogin || err != nil {
		t.Errorf("board read policy = %q, %v", p, err)
	}
	if user := util.FindTableConfig(cnf.Gpa, "user_tb"); user == nil || !reflect.DeepEqual(user.Protect, []string{"level"}) {
		t.Errorf("user protect: %+v", user)
	}

	b, err := os.ReadFile("bootstrap/docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	var compose struct {
		Services map[string]struct {
			Image       string            `yaml:"image"`
			Environment map[string]string `yaml:"environment"`
			Volumes     []string          `yaml:"volumes"`
			Healthcheck map[string]any    `yaml:"healthcheck"`
		} `yaml:"services"`
		Volumes map[string]any `yaml:"volumes"`
	}
	if err := yaml.Unmarshal(b, &compose); err != nil {
		t.Fatal(err)
	}
	db, ok := compose.Services["db"]
	if !ok || !strings.HasPrefix(db.Image, "mariadb:") || db.Healthcheck == nil || len(db.Environment) != 4 {
		t.Errorf("db service: %+v", db)
	}
	if _, ok := compose.Volumes["db-data"]; !ok {
		t.Error("no db-data volume")
	}
	mounted := false
	for _, v := range db.Volumes {
		if strings.HasPrefix(v, "./db/schema.sql:/docker-entrypoint-initdb.d/") {
			mounted = true
		}
	}
	if !mounted {
		t.Errorf("schema not mounted: %v", db.Volumes)
	}

	env, _ := os.ReadFile("bootstrap/.env.example")
	for _, k := range []string{"MARIADB_DATABASE=", "MARIADB_USER=", "MARIADB_PASSWORD=", "MARIADB_ROOT_PASSWORD=", "JWT_SECRET="} {
		if !strings.Contains(string(env), k) {
			t.Errorf(".env.example has no %s", k)
		}
	}
	schema, _ := os.ReadFile("bootstrap/db/schema.sql")
	for _, s := range []string{"`user_tb`", "`board_tb`", "`u_createddate` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP", "utf8mb4"} {
		if !strings.Contains(string(schema), s) {
			t.Errorf("schema.sql has no %s", s)
		}
	}
}
