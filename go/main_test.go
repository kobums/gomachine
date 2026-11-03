package gocodegen

import (
	"gomachine/config"
	"gomachine/util"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDateFillExpr(t *testing.T) {
	cases := []struct {
		col  util.Column
		want string
	}{
		{util.Column{OriginalType: "datetime"}, `"1000-01-01 00:00:00"`},
		{util.Column{OriginalType: "date"}, `"1000-01-01"`},
		{util.Column{OriginalType: "time"}, `"00:00:00"`},
		{util.Column{OriginalType: "timestamp"}, `time.Now().Format("2006-01-02 15:04:05")`},
		{util.Column{OriginalType: "datetime", HasDefault: true, Default: "CURRENT_TIMESTAMP"}, `time.Now().Format("2006-01-02 15:04:05")`},
		{util.Column{OriginalType: "date", HasDefault: true, Default: "curdate()"}, `time.Now().Format("2006-01-02")`},
		{util.Column{OriginalType: "date", HasDefault: true, Default: "'2000-01-01'"}, `"2000-01-01"`},
	}
	for _, c := range cases {
		if got := dateFillExpr(c.col); got != c.want {
			t.Errorf("dateFillExpr(%+v) = %s, want %s", c.col, got, c.want)
		}
	}
}

func TestScanCode(t *testing.T) {
	s := newScanCode()
	s.add("item", "n_Id", util.Column{Name: "Id", Type: "int64"}, "x", false, false)
	s.add("item", "n_Memo", util.Column{Name: "Memo", Type: "string", Nullable: true}, "x", false, false)
	s.add("item", "n_Cnt", util.Column{Name: "Cnt", Type: "int", Nullable: true}, "x", false, false)
	s.add("item", "n_Status", util.Column{Name: "Status", Type: "x.Status", Nullable: true}, "x", false, false)
	s.add("item", "n_Data", util.Column{Name: "Data", Type: "[]byte", Nullable: true}, "x", false, false)
	s.add("item", "n_Flag", util.Column{Name: "Flag", Type: "bool"}, "x", false, false)
	s.add("_j", "n_j_Kind", util.Column{Name: "Kind", Type: "y.Kind"}, "", true, false)
	s.add("_j", "n_j_Name", util.Column{Name: "Name", Type: "string"}, "", true, false)

	if got := strings.Join(s.targets, ", "); got != "&item.Id, &n_Memo, &n_Cnt, &n_Status, &item.Data, &n_Flag, &n_j_Kind, &n_j_Name" {
		t.Errorf("targets = %s", got)
	}
	for _, want := range []string{"item.Memo = n_Memo.String", "item.Cnt = int(n_Cnt.Int64)", "item.Status = x.Status(n_Status.Int64)",
		"item.Flag = n_Flag.Int64 != 0", "reflect.ValueOf(&_j.Kind).Elem().SetInt(n_j_Kind.Int64)", "_j.Name = n_j_Name.String"} {
		if !strings.Contains(s.assigns.String(), want) {
			t.Errorf("assigns missing %q:\n%s", want, s.assigns.String())
		}
	}
	if !s.reflect {
		t.Errorf("reflect flag not set")
	}

	// GetRaw 는 NULL 이었던 null 허용 컬럼을 기록한다
	r := newScanCode()
	r.add("item", "n_Memo", util.Column{Name: "Memo", Type: "string", Nullable: true}, "x", false, true)
	r.add("item", "n_Data", util.Column{Name: "Data", Type: "[]byte", Nullable: true}, "x", false, true)
	r.add("item", "n_Name", util.Column{Name: "Name", Type: "string"}, "x", false, true)
	for _, want := range []string{"if !n_Memo.Valid {\nnulls[\"Memo\"] = true", "if item.Data == nil {\nnulls[\"Data\"] = true"} {
		if !strings.Contains(r.assigns.String(), want) {
			t.Errorf("raw assigns missing %q:\n%s", want, r.assigns.String())
		}
	}
	if strings.Contains(r.assigns.String(), "Name") {
		t.Errorf("NOT NULL column recorded:\n%s", r.assigns.String())
	}
}

func TestCreatedNow(t *testing.T) {
	cases := map[string]string{
		"":           "t := time.Now().UTC().Add(time.Hour * 9)",
		"Asia/Seoul": "t := time.Now().UTC().Add(time.Hour * 9)",
		"local":      "t := time.Now()",
		"UTC":        "t := time.Now().UTC()",
	}
	for tz, want := range cases {
		if got := createdNowCode(tz); got != want {
			t.Errorf("createdNowCode(%q) = %s, want %s", tz, got, want)
		}
	}
	if got := createdNowCode("America/New_York"); !strings.Contains(got, `time.LoadLocation("America/New_York")`) {
		t.Errorf("createdNowCode(iana) = %s", got)
	}
}

func TestRoutesSource(t *testing.T) {
	got := RoutesSource([]string{"User", "Board", "User"})
	want := util.GeneratedHeader + "\n\npackage routers\n\nimport \"github.com/gofiber/fiber/v2\"\n\n" +
		"// SetupGeneratedRoutes 는 gomachine 이 만든 테이블 라우트를 모두 등록한다 (router.go 에서 한 번 부른다).\n" +
		"func SetupGeneratedRoutes(group fiber.Router) {\n\tSetupBoardRoutes(group)\n\tSetupUserRoutes(group)\n}\n"
	if got != want {
		t.Errorf("RoutesSource:\n%s\nwant:\n%s", got, want)
	}
}

// Finish 는 이번 실행에서 만든 라우터만 등록하고, 머리줄 없는 사용자 라우터 파일은 건너뛴다.
func TestFinishAndUserFiles(t *testing.T) {
	dir := t.TempDir()
	routers := filepath.Join(dir, "router", "routers")
	os.MkdirAll(routers, 0755)
	userFile := filepath.Join(routers, "user.go")
	os.WriteFile(userFile, []byte("package routers\n// hand-written\n"), 0644)

	if writeGo(userFile, util.GeneratedHeader+"\n\npackage routers\n") {
		t.Error("writeGo overwrote a hand-written file")
	}
	if !writeGo(filepath.Join(routers, "board.go"), util.GeneratedHeader+"\n\npackage routers\n") {
		t.Error("writeGo did not write a new file")
	}
	generatedRoutes = []string{"Board"}
	Finish(config.ModelConfig{GoModelFilePath: dir})
	b, err := os.ReadFile(filepath.Join(routers, RoutesFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "SetupBoardRoutes(group)") || strings.Contains(string(b), "SetupUserRoutes") {
		t.Errorf("routes file:\n%s", b)
	}
	if len(generatedRoutes) != 0 {
		t.Error("Finish did not reset generatedRoutes")
	}
	if b, _ := os.ReadFile(userFile); !strings.Contains(string(b), "hand-written") {
		t.Error("user file changed")
	}
}
