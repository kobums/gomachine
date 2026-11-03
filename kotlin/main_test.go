package kotlin

import (
	"gomachine/config"
	"gomachine/util"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupHome 은 HOME/bin/buildtool 이 저장소 views 를 가리키게 한다.
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

// generate 는 user_tb (비밀번호, createddate 포함) 를 springBoot 버전 boot 로 out 에 만든다.
func generate(t *testing.T, out string, boot int) {
	t.Helper()
	cnf := config.ModelConfig{Language: "kotlin", KotlinModelFilePath: out, Auth: "jwt", AdminLevel: "3", KotlinPackage: "com.example.app", SpringBoot: boot}
	raws := []util.RawColumn{
		{Name: "u_id", DataType: "bigint"},
		{Name: "u_email", DataType: "varchar"},
		{Name: "u_passwd", DataType: "varchar"},
		{Name: "u_birth", DataType: "date", Nullable: true},
		{Name: "u_createddate", DataType: "datetime", HasDefault: true, Default: "CURRENT_TIMESTAMP"},
	}
	items := make([]util.Column, 0)
	for _, r := range raws {
		items = append(items, util.NewColumnFromRaw("user_tb", r, nil, cnf))
	}
	ProcessKotlin("app", "user_tb", "u", items, nil, nil, "1", "jwt", cnf)
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSpringBootVersion(t *testing.T) {
	setupHome(t)
	for _, boot := range []int{3, 4, 0} {
		out := t.TempDir()
		generate(t, out, boot)
		boot4 := boot != 3
		entity := read(t, filepath.Join(out, "entity", "User.kt"))
		repo := read(t, filepath.Join(out, "repository", "UserRepository.kt"))
		service := read(t, filepath.Join(out, "service", "UserService.kt"))
		support := read(t, filepath.Join(out, "support", "DateParams.kt"))
		controller := read(t, filepath.Join(out, "controller", "UserController.kt"))
		for name, src := range map[string]string{"entity": entity, "repo": repo, "service": service, "support": support, "controller": controller} {
			if !strings.HasPrefix(src, util.GeneratedHeader+"\n") {
				t.Errorf("boot %d %s: missing generated header", boot, name)
			}
		}
		checks := []struct {
			src  string
			text string
			want bool
		}{
			{entity, "import tools.jackson.databind.annotation.JsonDeserialize", boot4},
			{entity, "import com.fasterxml.jackson.databind.annotation.JsonDeserialize", !boot4},
			{support, "import tools.jackson.databind.ValueDeserializer", boot4},
			{support, ": ValueDeserializer<LocalDateTime?>()", boot4},
			{support, "com.fasterxml", !boot4},
			{repo, "spec: Specification<User>?", !boot4},
			{repo, "spec: Specification<User>,", boot4},
			{service, "passwordEncoder.encode(it)!!", boot4},
			{controller, "HttpStatus.CONFLICT).body(mapOf(\"success\" to false))", true},
		}
		for _, c := range checks {
			if got := strings.Contains(c.src, c.text); got != c.want {
				t.Errorf("boot %d: contains %q = %v, want %v", boot, c.text, got, c.want)
			}
		}
	}
}

// 머리줄 없는 기존 파일은 덮어쓰지 않고, 예전 DateParams.kt 는 머리줄이 없어도 생성 파일로 보고 덮어쓴다.
func TestHandWrittenNotOverwritten(t *testing.T) {
	setupHome(t)
	out := t.TempDir()
	mine := "package com.example.app.entity\n\nclass User\n"
	legacy := "package com.example.app.support\n\n// " + legacySupportMarker + "\n"
	for p, c := range map[string]string{"entity/User.kt": mine, "support/DateParams.kt": legacy} {
		os.MkdirAll(filepath.Dir(filepath.Join(out, p)), 0755)
		if err := os.WriteFile(filepath.Join(out, p), []byte(c), 0644); err != nil {
			t.Fatal(err)
		}
	}
	generate(t, out, 4)
	if got := read(t, filepath.Join(out, "entity", "User.kt")); got != mine {
		t.Errorf("hand-written entity overwritten:\n%s", got)
	}
	if got := read(t, filepath.Join(out, "support", "DateParams.kt")); !strings.HasPrefix(got, util.GeneratedHeader) {
		t.Errorf("legacy DateParams.kt not upgraded:\n%s", got)
	}
	// 생성 파일(머리줄 있음)은 다시 쓴다.
	if got := read(t, filepath.Join(out, "service", "UserService.kt")); !strings.HasPrefix(got, util.GeneratedHeader) {
		t.Errorf("service not generated")
	}
}

func TestFindHandWritten(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"java/a/User.java":          "@Entity(name = \"AuthUser\")\n@Table(name = \"user_tb\")\npublic class User {\n}\n",
		"java/a/UserService.java":   "@Service\npublic class UserService {}\n",
		"kotlin/b/Board.kt":         "@Entity\n@Table(name = \"board_tb\")\ndata class Board(val id: Long) {}\n",
		"kotlin/b/BoardRepo.kt":     "interface BoardRepository : JpaRepository<Board, Long>\n",
		"kotlin/b/Generated.kt":     util.GeneratedHeader + "\nclass UserController\n",
		"kotlin/build/Ignored.kt":   "class Ignored\n",
		"kotlin/b/notes.txt":        "class Notes\n",
		"kotlin/b/BoardService.kt":  "open class BoardService\n",
		"kotlin/b/Nested.kt":        "class Outer {\n    inner class Inner\n}\n",
		"kotlin/b/BoardControl.kt":  "// class BoardController in a comment line is not at line start\n",
		"kotlin/b/AuthEntity.kt":    "@Entity(\"Member\")\nclass AuthMember\n",
		"kotlin/b/ObjectHolder.kt":  "internal object Holder\n",
		"java/a/RecordThing.java":   "public record Point(int x) {}\n",
		"kotlin/b/EnumThing.kt":     "enum class Color { RED }\n",
		"kotlin/b/Annotation.kt":    "@RestController\n@RequestMapping(\"/x\")\nclass XController\n",
		"kotlin/b/Sealed.kt":        "sealed interface Shape\n",
		"kotlin/b/AbstractThing.kt": "abstract class Base\n",
	}
	for p, c := range files {
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0755)
		if err := os.WriteFile(full, []byte(c), 0644); err != nil {
			t.Fatal(err)
		}
	}
	hw := FindHandWritten(root)
	for _, name := range []string{"User", "UserService", "Board", "BoardRepository", "BoardService", "Outer", "AuthMember", "Holder", "Point", "Color", "XController", "Shape", "Base"} {
		if len(hw.Classes[name]) == 0 {
			t.Errorf("class %s not found", name)
		}
	}
	for _, name := range []string{"UserController", "Ignored", "Notes", "BoardController"} {
		if len(hw.Classes[name]) != 0 {
			t.Errorf("class %s should not be found: %v", name, hw.Classes[name])
		}
	}
	for _, name := range []string{"AuthUser", "Board", "Member"} {
		if len(hw.Entities[name]) == 0 {
			t.Errorf("entity %s not found", name)
		}
	}
	for _, name := range []string{"User", "AuthMember"} {
		if len(hw.Entities[name]) != 0 {
			t.Errorf("entity %s should not be found (explicit @Entity name)", name)
		}
	}
}

func TestSourceRoot(t *testing.T) {
	sep := string(filepath.Separator)
	got := sourceRoot(sep + filepath.Join("p", "src", "main", "kotlin", "com", "x"))
	if want := sep + filepath.Join("p", "src", "main"); got != want {
		t.Errorf("sourceRoot = %s, want %s", got, want)
	}
	if got := sourceRoot(sep + filepath.Join("p", "gen")); got != sep+filepath.Join("p", "gen") {
		t.Errorf("sourceRoot without src/main = %s", got)
	}
}
