package util

import (
	"gomachine/config"
	"reflect"
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
		{"tinyint", "bool", "Int"},
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
		{"bit", "[]byte", "Boolean"},
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
