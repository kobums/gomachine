package util

import (
	"gomachine/config"
	"reflect"
	"strings"
	"testing"
)

func TestNames(t *testing.T) {
	cases := []struct{ in, name, prefix string }{
		{"u_id", "id", "u"},
		{"u_createddate", "createddate", "u"},
		{"p_created_at", "created_at", "p"},
		{"p_created_by", "created_by", "p"},
		{"id", "id", ""},
	}
	for _, c := range cases {
		if got := GetName(c.in); got != c.name {
			t.Errorf("GetName(%q) = %q, want %q", c.in, got, c.name)
		}
		if got := GetPrefix(c.in); got != c.prefix {
			t.Errorf("GetPrefix(%q) = %q, want %q", c.in, got, c.prefix)
		}
	}

	// 밑줄 하나짜리 컬럼은 예전(strings.Title(strs[1]))과 같은 이름이어야 한다.
	camel := map[string]string{"createddate": "Createddate", "id": "Id", "userId": "UserId", "created_at": "CreatedAt", "a__b": "AB"}
	for in, want := range camel {
		if got := CamelCase(in); got != want {
			t.Errorf("CamelCase(%q) = %q, want %q", in, got, want)
		}
	}
	lower := map[string]string{"createddate": "createddate", "userId": "userid", "created_at": "createdAt", "USER_ID": "userId"}
	for in, want := range lower {
		if got := LowerCamel(in); got != want {
			t.Errorf("LowerCamel(%q) = %q, want %q", in, got, want)
		}
	}
}

// JSONKey 는 Go json 태그, Kotlin 프로퍼티, TypeScript key, Dart json key 가 함께 쓰는 이름이다.
func TestJSONKey(t *testing.T) {
	cases := []struct{ raw, key, segment, start, end string }{
		{"u_createddate", "createddate", "createddate", "startcreateddate", "endcreateddate"},
		{"u_loginid", "loginid", "loginid", "startloginid", "endloginid"},
		{"p_created_at", "createdAt", "createdat", "startcreatedat", "endcreatedat"},
		{"a_userId", "userid", "userid", "startuserid", "enduserid"},
		{"ur_user_id", "userId", "userid", "startuserid", "enduserid"},
		{"id", "id", "id", "startid", "endid"},
	}
	for _, c := range cases {
		col := NewColumn("x_tb", c.raw, "int", nil, config.ModelConfig{Language: "go"})
		if got := JSONKey(col); got != c.key {
			t.Errorf("JSONKey(%q) = %q, want %q", c.raw, got, c.key)
		}
		if got := SearchSegment(col); got != c.segment {
			t.Errorf("SearchSegment(%q) = %q, want %q", c.raw, got, c.segment)
		}
		if s, e := RangeParams(col); s != c.start || e != c.end {
			t.Errorf("RangeParams(%q) = %q, %q", c.raw, s, e)
		}
	}
}

func TestTableNames(t *testing.T) {
	tableNameAlias = map[string]string{}
	cases := []struct{ in, base, model, file, typ string }{
		{"user_tb", "user", "User", "user", "table"},
		{"user_role_tb", "user_role", "UserRole", "userrole", "table"},
		{"user_vw", "user", "User", "user", "view"},
		{"orderlist_vw", "orderlist", "Orderlist", "orderlist", "view"},
		{"user", "user", "User", "user", "table"},
		{"_tb", "_tb", "Tb", "tb", "table"},
	}
	for _, c := range cases {
		if got := GetTableName(c.in); got != c.base {
			t.Errorf("GetTableName(%q) = %q, want %q", c.in, got, c.base)
		}
		if got := ModelName(c.in); got != c.model {
			t.Errorf("ModelName(%q) = %q, want %q", c.in, got, c.model)
		}
		if got := FileName(c.in); got != c.file {
			t.Errorf("FileName(%q) = %q, want %q", c.in, got, c.file)
		}
		if got := GetTableType(c.in); got != c.typ {
			t.Errorf("GetTableType(%q) = %q, want %q", c.in, got, c.typ)
		}
	}

	for _, name := range []string{"user_role", "userrole", "UserRole", "user_role_tb"} {
		if !MatchTableName(name, "user_role_tb") {
			t.Errorf("MatchTableName(%q, user_role_tb) = false", name)
		}
	}
	if MatchTableName("user", "user_role_tb") {
		t.Errorf("MatchTableName(user, user_role_tb) = true")
	}
}

func TestResolveTableNames(t *testing.T) {
	tableNameAlias = map[string]string{}
	defer func() { tableNameAlias = map[string]string{} }()

	ResolveTableNames([]string{"user_vw", "user_tb", "user_role_tb", "userrole_tb"})
	if got := ModelName("user_tb"); got != "User" {
		t.Errorf("user_tb = %q, want User", got)
	}
	if got := ModelName("user_vw"); got != "UserView" {
		t.Errorf("user_vw = %q, want UserView", got)
	}
	if FileName("user_role_tb") == FileName("userrole_tb") {
		t.Errorf("user_role_tb and userrole_tb still collide: %q", FileName("userrole_tb"))
	}
	if !MatchTableName("user_view", "user_vw") {
		t.Errorf("config name user_view should match renamed user_vw")
	}
}

func TestGetType(t *testing.T) {
	goCnf := config.ModelConfig{Language: "go"}
	ktCnf := config.ModelConfig{Language: "kotlin"}
	cases := []struct{ db, goType, ktType string }{
		{"int", "int", "Int"},
		{"smallint", "int", "Int"},
		{"mediumint", "int", "Int"},
		{"bigint", "int64", "Long"},
		{"tinyint", "bool", "Boolean"},
		{"year", "int", "Int"},
		{"double", "float64", "Double"},
		{"float", "float64", "Double"},
		{"decimal", "float64", "BigDecimal"},
		{"char", "string", "String"},
		{"varchar", "string", "String"},
		{"mediumtext", "string", "String"},
		{"longtext", "string", "String"},
		{"tinytext", "string", "String"},
		{"enum", "string", "String"},
		{"set", "string", "String"},
		{"json", "string", "String"},
		{"datetime", "string", "LocalDateTime"},
		{"timestamp", "string", "LocalDateTime"},
		{"date", "string", "LocalDate"},
		{"time", "string", "String"},
		{"blob", "[]byte", "ByteArray"},
		{"longblob", "[]byte", "ByteArray"},
		{"varbinary", "[]byte", "ByteArray"},
		{"bit", "bool", "Boolean"},
		{"VARCHAR", "string", "String"},
		{"geometry", "string", "String"},
	}
	for _, c := range cases {
		if got := GetType("evt", "col", c.db, nil, goCnf); got != c.goType {
			t.Errorf("go %s = %q, want %q", c.db, got, c.goType)
		}
		if got := GetType("evt", "col", c.db, nil, ktCnf); got != c.ktType {
			t.Errorf("kotlin %s = %q, want %q", c.db, got, c.ktType)
		}
	}

	gpa := &config.Gpa{Map: []config.GpaMap{{Name: "Status"}, {Name: "user_status"}}}
	if got := GetType("evt", "status", "int", gpa, goCnf); got != "evt.Status" {
		t.Errorf("go enum = %q", got)
	}
	if got := GetType("log", "status", "int", gpa, goCnf); got != "logpkg.Status" {
		t.Errorf("go enum in log table = %q", got)
	}
	if got := GetType("evt", "user_status", "int", gpa, ktCnf); got != "UserStatus" {
		t.Errorf("kotlin enum = %q", got)
	}
}

func TestIdents(t *testing.T) {
	goCases := map[string]string{"type": "type_", "status": "status", "item": "item_", "string": "string_", "id": "id", "log": "log_"}
	for in, want := range goCases {
		if got := GoIdent(in); got != want {
			t.Errorf("GoIdent(%q) = %q, want %q", in, got, want)
		}
	}
	ktCases := map[string]string{"class": "`class`", "in": "`in`", "status": "status", "type": "type"}
	for in, want := range ktCases {
		if got := KotlinIdent(in); got != want {
			t.Errorf("KotlinIdent(%q) = %q, want %q", in, got, want)
		}
	}
	pkgCases := []struct{ table, alias, clause string }{
		{"user_tb", "user", "user"},
		{"item_tb", "item", "item"},
		{"log_tb", "logpkg", "log"},
		{"flag_tb", "flagpkg", "flag"},
		{"type_tb", "typepkg", "typepkg"},
		{"main_tb", "mainpkg", "mainpkg"},
	}
	for _, c := range pkgCases {
		if got := GoConstPackage(c.table); got != c.alias {
			t.Errorf("GoConstPackage(%q) = %q, want %q", c.table, got, c.alias)
		}
		if got := GoConstPackageClause(c.table); got != c.clause {
			t.Errorf("GoConstPackageClause(%q) = %q, want %q", c.table, got, c.clause)
		}
	}
	if got := KotlinString(`a"b$c\d`); got != `"a\"b\$c\\d"` {
		t.Errorf("KotlinString = %s", got)
	}
}

func TestEnumItems(t *testing.T) {
	data := []string{"wait-time:대기", "1st:First", "none:None", "", "done", "done:Again"}

	goItems := GoEnumItems("Status", data)
	goIdents := []string{}
	for _, item := range goItems {
		goIdents = append(goIdents, item.Ident)
	}
	if want := []string{"StatusWait_Time", "Status1st", "StatusNone", "StatusDone", "StatusDone_2"}; !reflect.DeepEqual(goIdents, want) {
		t.Errorf("GoEnumItems = %v, want %v", goIdents, want)
	}

	ktItems := KotlinEnumItems(data)
	ktIdents := []string{}
	for _, item := range ktItems {
		ktIdents = append(ktIdents, item.Ident)
	}
	if want := []string{"WAIT_TIME", "V1ST", "NONE_2", "DONE", "DONE_2"}; !reflect.DeepEqual(ktIdents, want) {
		t.Errorf("KotlinEnumItems = %v, want %v", ktIdents, want)
	}
	if ktItems[3].Label != "done" || ktItems[3].Index != 4 {
		t.Errorf("label without colon = %+v", ktItems[3])
	}
}

func TestParseFunc(t *testing.T) {
	cnf := config.ModelConfig{Language: "go"}
	gpa := &config.Gpa{Map: []config.GpaMap{{Name: "status"}}}
	items := []Column{}
	for _, c := range []struct{ name, typ string }{{"a_id", "bigint"}, {"a_user", "bigint"}, {"a_userid", "bigint"}, {"a_status", "int"}, {"a_name", "varchar"}, {"a_created_at", "datetime"}, {"a_type", "int"}} {
		items = append(items, NewColumn("act_tb", c.name, c.typ, gpa, cnf))
	}

	type w struct{ Column, Key, Type, Compare, Param string }
	ok := map[string][]w{
		"GetByName":             {{"Name", "name", "string", "=", "name"}},
		"FindByUserStatus":      {{"User", "user", "int64", "=", "user"}, {"Status", "status", "act.Status", "=", "status"}},
		"FindByUseridAndStatus": {{"Userid", "userid", "int64", "=", "userid"}, {"Status", "status", "act.Status", "=", "status"}},
		// 대소문자까지 같은 컬럼 조합(User + Id)이 먼저다 (예전 해석과 같음).
		"GetByUserId":          {{"User", "user", "int64", "=", "user"}, {"Id", "id", "int64", "=", "id"}},
		"FindByIds":            {{"Id", "id", "[]int64", "in", "id"}},
		"FindByNamelike":       {{"Name", "name", "string", "like", "name"}},
		"CountByCreatedAt":     {{"CreatedAt", "created_at", "string", "=", "createdat"}},
		"GetByType":            {{"Type", "type", "int", "=", "type_"}},
		"UpdateStatusNameById": {{"Status", "status", "act.Status", "column", "status"}, {"Name", "name", "string", "column", "name"}, {"Id", "id", "int64", "where", "id"}},
		"DeleteByUser":         {{"User", "user", "int64", "=", "user"}},
	}
	for method, want := range ok {
		fn, err := ParseFunc(method, items)
		if err != nil {
			t.Errorf("%s: %v", method, err)
			continue
		}
		got := []w{}
		for _, x := range fn.Wheres {
			got = append(got, w{x.Column, x.Key, x.Type, x.Compare, x.Param})
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %+v, want %+v", method, got, want)
		}
	}

	for _, method := range []string{"", "Get", "FindBy", "Fetch", "FindByNope", "UpdateName", "UpdateByName", "UpdateNameBy", "GetByStatusAndStatus", "CountByUserNope", "DeleteBy"} {
		if _, err := ParseFunc(method, items); err == nil {
			t.Errorf("%q should be rejected", method)
		}
	}
	// User 컬럼이 없으면 대소문자를 무시하고 Userid 컬럼과 맞춘다.
	fn, err := ParseFunc("GetByUserId", []Column{items[2], items[3]})
	if err != nil || len(fn.Wheres) != 1 || fn.Wheres[0].Column != "Userid" {
		t.Errorf("GetByUserId without User column = %+v, %v", fn, err)
	}

	if got := ParseFuncs("act_tb", []string{"GetByName", "FindByNope", ""}, items); len(got) != 1 {
		t.Errorf("ParseFuncs kept %d funcs, want 1", len(got))
	}
}

func TestProtectedColumnsSnake(t *testing.T) {
	cnf := config.ModelConfig{Language: "go"}
	items := []Column{NewColumn("post_tb", "p_id", "bigint", nil, cnf), NewColumn("post_tb", "p_view_count", "int", nil, cnf), NewColumn("post_tb", "p_created_by", "bigint", nil, cnf)}
	gpa := &config.Gpa{Protect: []string{"view_count", "created_by"}, Session: []config.SessionPair{{Key: "id", Column: "created_by"}}}
	if got := ProtectedColumns("Post", items, gpa); !reflect.DeepEqual(got, []string{"ViewCount"}) {
		t.Errorf("ProtectedColumns = %v", got)
	}
}

func TestEnumCodeHelpers(t *testing.T) {
	if !EnumHasZero([]string{"a:A"}) || EnumHasZero([]string{"", "a:A"}) || EnumHasZero(nil) {
		t.Fatal("EnumHasZero")
	}
	items := GoEnumItems("Status", []string{"", "wait:Wait", "", "done:Done"})
	if len(items) != 2 || items[0].Code != 1 || items[1].Code != 3 {
		t.Fatalf("GoEnumItems codes: %+v", items)
	}
	kitems := KotlinEnumItems([]string{"", "wait:Wait", "", "done:Done"})
	if len(kitems) != 2 || kitems[0].Code != 1 || kitems[1].Code != 3 {
		t.Fatalf("KotlinEnumItems codes: %+v", kitems)
	}
}

func TestIsNowDefault(t *testing.T) {
	for _, d := range []string{"CURRENT_TIMESTAMP", "current_timestamp(3)", "now()", "CURRENT_DATE", "curdate()"} {
		if !IsNowDefault(d) {
			t.Errorf("%s should be now", d)
		}
	}
	for _, d := range []string{"", "2000-01-01", "0"} {
		if IsNowDefault(d) {
			t.Errorf("%s should not be now", d)
		}
	}
}

func TestProtectedColumnsUserLevel(t *testing.T) {
	cnf := config.ModelConfig{Language: "go"}
	items := []Column{NewColumn("user_tb", "u_id", "bigint", nil, cnf), NewColumn("user_tb", "u_email", "varchar", nil, cnf), NewColumn("user_tb", "u_level", "int", nil, cnf)}
	cases := []struct {
		gpa  *config.Gpa
		want []string
	}{
		{nil, []string{"Level"}},
		{&config.Gpa{}, []string{"Level"}},
		// protect 를 설정해도 User 의 level 은 보호한다 (예전에는 빠졌다)
		{&config.Gpa{Protect: []string{"email"}}, []string{"Level", "Email"}},
		{&config.Gpa{Protect: []string{"level", "email"}}, []string{"Level", "Email"}},
	}
	for _, c := range cases {
		if got := ProtectedColumns("User", items, c.gpa); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ProtectedColumns(%+v) = %v, want %v", c.gpa, got, c.want)
		}
	}
	if got := ProtectedColumns("Member", items, &config.Gpa{Protect: []string{"email"}}); !reflect.DeepEqual(got, []string{"Email"}) {
		t.Errorf("non-User ProtectedColumns = %v", got)
	}
}

func TestDedupeColumns(t *testing.T) {
	cnf := config.ModelConfig{Language: "go"}
	raw := []string{"p_id", "p_created_at", "p_createdat", "p_user_id", "p_userId", "p_userid", "p_name"}
	items := make([]Column, 0)
	for _, r := range raw {
		items = append(items, NewColumn("post_tb", r, "bigint", nil, cnf))
	}
	got := DedupeColumns("post_tb", items)
	names, keys := []string{}, []string{}
	for _, c := range got {
		names = append(names, c.Name)
		keys = append(keys, JSONKey(c))
	}
	if want := []string{"Id", "CreatedAt", "Createdat2", "UserId", "UserId2", "Userid3", "Name"}; !reflect.DeepEqual(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
	if want := []string{"id", "createdAt", "createdat2", "userId", "userid2", "userid3", "name"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("json keys = %v, want %v", keys, want)
	}
	// SQL 컬럼은 그대로, 입력은 바꾸지 않는다
	if got[2].Key != "createdat" || got[2].Column != "p_createdat" || items[2].Name != "Createdat" {
		t.Errorf("sql names changed: %+v / input %+v", got[2], items[2])
	}
	// 다시 불러도 같다 (idempotent)
	if again := DedupeColumns("post_tb", got); !reflect.DeepEqual(again, got) {
		t.Errorf("not idempotent: %+v", again)
	}
	// 이미 있는 이름(createdat2)과 겹치지 않는다
	items2 := []Column{NewColumn("t_tb", "t_createdat2", "int", nil, cnf), NewColumn("t_tb", "t_created_at", "int", nil, cnf), NewColumn("t_tb", "t_createdat", "int", nil, cnf)}
	if got := DedupeColumns("t_tb", items2); JSONKey(got[2]) != "createdat3" {
		t.Errorf("collision with existing suffix: %s", JSONKey(got[2]))
	}
	// 검색 경로 / 범위 파라미터도 번호가 붙는다
	if seg := SearchSegment(got[2]); seg != "createdat2" {
		t.Errorf("search segment = %s", seg)
	}
}

func TestJoinVars(t *testing.T) {
	joins := []config.GpaJoin{{Name: "user"}, {Name: "user"}, {Name: "User", Alias: "agent"}, {Name: "comment", Alias: "User"}}
	if got := JoinVars(joins); !reflect.DeepEqual(got, []string{"user", "user2", "agent", "user3"}) {
		t.Errorf("JoinVars = %v", got)
	}
}

func TestParseFuncPrimaryUpdate(t *testing.T) {
	cnf := config.ModelConfig{Language: "go"}
	items := []Column{NewColumn("m_tb", "m_id", "bigint", nil, cnf), NewColumn("m_tb", "m_code", "varchar", nil, cnf), NewColumn("m_tb", "m_name", "varchar", nil, cnf)}
	if _, err := ParseFunc("UpdateIdByName", items); err == nil {
		t.Errorf("UpdateIdByName accepted")
	}
	items[1].Primary = true
	if _, err := ParseFunc("UpdateCodeByName", items); err == nil {
		t.Errorf("UpdateCodeByName (primary) accepted")
	}
	if _, err := ParseFunc("UpdateNameById", items); err != nil {
		t.Errorf("UpdateNameById rejected: %v", err)
	}
	if _, err := ParseFunc("FindById", items); err != nil {
		t.Errorf("FindById rejected: %v", err)
	}
}

func TestFindTableConfigPriority(t *testing.T) {
	defer func() { tableNameAlias = map[string]string{}; knownTables = nil }()
	tables := []string{"user_role_tb", "user_tb", "user_vw", "userrole_tb"}
	ResolveTableNames(tables)

	gpas := []config.Gpa{{Name: "userrole"}, {Name: "user_role"}, {Name: "user"}, {Name: "user_vw"}, {Name: "ghost"}}
	want := map[string]string{
		"userrole_tb":  "userrole",  // 정확히 맞는 이름이 먼저 (_ 무시 규칙으로 user_role_tb 에 붙지 않는다)
		"user_role_tb": "user_role", // 정확히
		"user_tb":      "user",
		"user_vw":      "user_vw", // view 는 전체 이름으로
	}
	for _, table := range tables {
		g := FindTableConfig(gpas, table)
		if g == nil || g.Name != want[table] {
			t.Errorf("%s: got %v, want %s", table, g, want[table])
		}
	}

	// userrole_tb 가 있으면 userrole 은 정확히 맞는 userrole_tb 에만 붙는다.
	if g := FindTableConfig([]config.Gpa{{Name: "userrole"}}, "user_role_tb"); g != nil {
		t.Error("userrole should not match user_role_tb when userrole_tb exists")
	}

	warnings := CheckTableConfigs(gpas, tables)
	if len(warnings) != 1 || !strings.Contains(warnings[0], `"ghost"`) {
		t.Errorf("warnings = %q", warnings)
	}

	// _ 무시 규칙: 다른 테이블을 정확히 가리키지 않으면 쓴다.
	tableNameAlias = map[string]string{}
	ResolveTableNames([]string{"user_role_tb"})
	if g := FindTableConfig([]config.Gpa{{Name: "UserRole"}}, "user_role_tb"); g == nil {
		t.Error("UserRole should match user_role_tb")
	}

	// 두 테이블에 붙는 설정은 경고한다 (테이블 이름 user 와 user_tb 의 기본 이름 user).
	tableNameAlias = map[string]string{}
	ResolveTableNames([]string{"user", "user_tb"})
	if w := CheckTableConfigs([]config.Gpa{{Name: "user"}}, []string{"user", "user_tb"}); len(w) != 1 || !strings.Contains(w[0], "several") {
		t.Errorf("ambiguous: %q", w)
	}
	if w := CheckTableConfigs([]config.Gpa{{Name: "user_tb"}}, []string{"user", "user_tb"}); len(w) != 0 {
		t.Errorf("full name: %q", w)
	}
}

func TestResolveJoins(t *testing.T) {
	defer func() { knownTables = nil }()
	cnf := config.ModelConfig{Language: "go"}
	items := []Column{
		NewColumn("art_tb", "a_id", "bigint", nil, cnf),
		NewColumn("art_tb", "a_member", "bigint", nil, cnf),
		NewColumn("art_tb", "a_owner_id", "bigint", nil, cnf),
	}
	knownTables = []string{"member_tb", "art_tb", "user_role_tb"}
	joins := []config.GpaJoin{
		{Name: "member", Column: "a_member"},              // 접두사 포함
		{Name: "member", Column: "member", Alias: "m2"},   // 접두사 없음
		{Name: "member", Column: "ownerId", Alias: "own"}, // API 이름
		{Name: "member", Column: "nothing"},               // 없는 컬럼
		{Name: "ghost", Column: "member"},                 // 없는 테이블
		{Name: "userrole", Column: "member", Alias: "r"},  // _ 무시로 찾는 테이블
	}
	got := ResolveJoins("art_tb", items, joins)
	if len(got) != 4 {
		t.Fatalf("got %d joins: %+v", len(got), got)
	}
	wantLocal := []string{"a_member", "a_member", "a_owner_id", "a_member"}
	wantVar := []string{"member", "m2", "own", "r"}
	for i := range got {
		if got[i].Local.Column != wantLocal[i] || got[i].Var != wantVar[i] {
			t.Errorf("join %d: local %s var %s", i, got[i].Local.Column, got[i].Var)
		}
	}
	if got[3].Table != "user_role_tb" || got[0].Table != "member_tb" {
		t.Errorf("tables: %s %s", got[0].Table, got[3].Table)
	}
	// 건너뛴 조인 뒤의 변수 이름은 전체 설정 기준이다 (Go / Spring / TypeScript 가 같다).
	vars := JoinVars([]config.GpaJoin{{Name: "member"}, {Name: "ghost"}, {Name: "member"}})
	if vars[2] != "member2" {
		t.Errorf("vars = %v", vars)
	}
}

func TestSessionColumnError(t *testing.T) {
	cnf := config.ModelConfig{Language: "go"}
	items := []Column{
		NewColumn("doc_tb", "d_user", "int", nil, cnf),
		NewColumn("doc_tb", "d_owner", "varchar", nil, cnf),
		NewColumn("doc_tb", "d_flag", "tinyint", nil, cnf),
	}
	if msg := SessionColumnError(items, "user", false); msg != "" {
		t.Errorf("int: %s", msg)
	}
	if msg := SessionColumnError(items, "owner", false); msg == "" {
		t.Error("varchar for Go: want error")
	}
	if msg := SessionColumnError(items, "owner", true); msg != "" {
		t.Errorf("varchar for Spring: %s", msg)
	}
	if msg := SessionColumnError(items, "flag", true); msg == "" {
		t.Error("tinyint: want error")
	}
	if msg := SessionColumnError(items, "usr", true); !strings.Contains(msg, "not found") {
		t.Errorf("missing: %q", msg)
	}
}

func TestReservedNames(t *testing.T) {
	// extra 컬럼은 생성 코드의 extra 와 겹치므로 번호를 붙인다.
	cnf := config.ModelConfig{Language: "go"}
	items := DedupeColumns("t_tb", []Column{NewColumn("t_tb", "t_id", "bigint", nil, cnf), NewColumn("t_tb", "t_extra", "varchar", nil, cnf)})
	if items[1].Name != "Extra2" || JSONKey(items[1]) != "extra2" || items[1].Column != "t_extra" {
		t.Errorf("extra column: %+v json %s", items[1], JSONKey(items[1]))
	}
	// Kotlin 타입과 겹치는 이름에는 접미사를 붙인다. 생성 코드가 쓰지 않는 이름(Order)은 그대로다.
	cases := map[string]string{"Page": "PageEntity", "List": "ListEntity", "Column": "ColumnEntity", "Order": "Order", "User": "User", "Sort": "SortEntity"}
	for in, want := range cases {
		if got := KotlinTypeName(in, "Entity"); got != want {
			t.Errorf("KotlinTypeName(%s) = %s, want %s", in, got, want)
		}
	}
	if got := KotlinTypeName("Item", "Enum", "Item"); got != "ItemEnum" {
		t.Errorf("taken: %s", got)
	}
}
