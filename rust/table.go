package rustcodegen

import (
	"database/sql"
	"fmt"
	"gomachine/config"
	"gomachine/util"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// 테이블 하나의 Rust 생성 정보. Go 생성기(go/main.go)와 같은 규칙으로 SQL / 이름 / 권한 정보를 만든다.

// rustEnum 은 gpa.map 하나로 만드는 enum newtype 이다 (pub struct Status(pub i32)).
type rustEnum struct {
	Type     string          // Rust 타입 이름
	Key      string          // extra key (map name 소문자, Go InitExtra 와 같다)
	Field    string          // 이 enum 을 쓰는 필드 (없으면 "")
	Nullable bool            // 그 필드가 Option 인지
	Items    []util.EnumItem // 연관 상수 (빈 항목 제외)
	Labels   []string        // 표시 이름 Rust 문자열 리터럴 (data 순서)
	HasZero  bool            // 코드 0 이 실제 값
}

// LabelList 는 LABELS 배열 원소다.
func (e rustEnum) LabelList() string {
	return strings.Join(e.Labels, ", ")
}

// rustCol 은 이 테이블 컬럼 하나다.
type rustCol struct {
	Col      util.Column
	Field    string // Rust 필드 이름
	JSON     string // API 이름 (util.JSONKey)
	Base     string // Option 을 뺀 Rust 타입
	Type     string // 필드 타입 (null 허용이면 Option<Base>)
	Nullable bool
	IsId     bool
	Copy     bool   // Copy 타입이라 clone 하지 않는다
	Enum     string // enum 타입 이름 (아니면 "")
	Password bool   // 응답에서 숨기는 비밀번호 컬럼
	Select   string // select 식 (조인이 있으면 테이블 이름으로 한정)
	RawSel   string // GetRaw select 식 (한정하지 않음)
	Serde    string // #[serde(...)] 속성
	Fill     string // NOT NULL 날짜 컬럼이 비어 있을 때 넣을 Rust 식 (Go dateFillExpr)
}

// rustJoinCol 은 조인 테이블 컬럼 하나다 (left join 이라 모두 Option 으로 읽는다).
type rustJoinCol struct {
	JSON     string
	Base     string // 읽는 타입 (enum 은 i32)
	Nullable bool
	Hidden   bool   // 조인 모델의 비밀번호 (json "-")
	Select   string // select 식
	Kind     string // date / date_opt 등 변환 (decodeKind)
	EnumKey  string // enum 이면 extra key
	Labels   string // enum 표시 이름 배열 리터럴
	IsId     bool
}

// rustJoin 은 조인 하나다.
type rustJoin struct {
	Var    string // extra key (util.JoinVars)
	Fn     string // 읽는 함수 이름
	Cols   []rustJoinCol
	Offset int // select 안의 첫 컬럼 위치
}

// rustSession 은 session 설정 하나다.
type rustSession struct {
	Key    string // session key (Session::value)
	Col    rustCol
	Column string // where 컬럼 (접두사 뺀 이름)
	SQL    string // 소유권 검사 SQL 컬럼 (자기 조인이면 테이블 이름으로 한정)
	Idx    int
}

// rustWhere 는 커스텀 메서드의 조건 하나다.
type rustWhere struct {
	util.Where
	Col   rustCol
	Param string // Rust 매개변수 이름
	PType string // 매개변수 타입 (in 이면 Vec<T>)
	Slice bool
	Check string // Get/Find/Count 에서 조건을 붙일지 검사하는 식 (비어 있으면 항상)
}

// rustFunc 는 커스텀 메서드 하나다.
type rustFunc struct {
	util.Func
	Fn     string // Rust 함수 이름 (snake_case)
	Wheres []rustWhere
}

// table 은 테이블 하나의 생성 정보다.
type table struct {
	TableName  string
	Module     string
	Name       string // Go 모델 이름 (User 판단, 경로)
	Model      string // Rust 모델 struct 이름
	Domain     string // URL 이름 (util.FileName)
	Prefix     string
	IsView     bool
	Auth       string
	Jwt        bool
	HasId      bool
	IdType     string
	OrderKey   string
	Cols       []rustCol
	NonId      []rustCol
	Enums      []rustEnum
	Joins      []rustJoin
	Sessions   []rustSession
	Funcs      []rustFunc
	Protects   []rustCol
	Passwords  []rustCol // 비밀번호 컬럼 (Passwd, Password 모두)
	Created    *rustCol
	CreatedNow bool // Insert 가 createddate 를 현재 시각으로 넣는다 (문자열 컬럼)
	AdminCheck bool
	AdminLevel int
	Compares   []config.GpaCompare
	Qual       string
	WherePref  string
	OrderCol   string
	Select     string // select ... from t left join ...
	Count      string // select count(*) from t left join ...
	RawSelect  string // select ... from t
	OrderCols  [][2]string
	EnumZero   map[string]bool
}

func (t *table) isUser() bool { return t.Name == "User" }

// col 은 Go 이름(Name)으로 컬럼을 찾는다.
func (t *table) col(name string) *rustCol {
	for i := range t.Cols {
		if t.Cols[i].Col.Name == name {
			return &t.Cols[i]
		}
	}
	return nil
}

func isCopy(base string) bool {
	return base != "String" && base != "Vec<u8>"
}

var rustIntTypes = map[string]bool{"i32": true, "i64": true, "u32": true, "u64": true}

// plainStrings 는 select 에서 그대로 String 으로 읽는 MySQL 타입이다. 그 밖의 문자열 타입(json, 모르는 타입)은 CAST AS CHAR.
var plainStrings = map[string]bool{"char": true, "varchar": true, "tinytext": true, "text": true, "mediumtext": true, "longtext": true, "enum": true, "set": true}

// signedInts 는 sqlx 가 i32 / i64 로 바로 읽는 (부호 있는) 정수 타입이다.
var signedInts = map[string]bool{"tinyint": true, "smallint": true, "mediumint": true, "int": true, "integer": true, "bigint": true}

// selectExpr 는 컬럼의 select 식과 읽기 변환(decode 의 kind: "" 그대로, date, decimal)이다.
func selectExpr(col util.Column, base string, isEnum bool, expr string) (string, string) {
	t := strings.ToLower(col.OriginalType)
	switch {
	case isEnum && (col.Unsigned || !signedInts[t]):
		// enum 은 i32 로 읽는다. 부호 있는 정수 컬럼이 아니면 (varchar, bit, year, unsigned ...) 정수로 바꾼다.
		return "CAST(" + expr + " AS SIGNED)", ""
	case t == "bit":
		// BIT(n) 은 0 이 아니면 true (NULL 은 NULL). (x+0) 은 128 이상 값이 bool 로 읽히지 않는다.
		return "(" + expr + "<>0)", ""
	case t == "date":
		return "DATE_FORMAT(" + expr + ", '%Y-%m-%d')", "date"
	case t == "datetime" || t == "timestamp":
		return "DATE_FORMAT(" + expr + ", '%Y-%m-%d %H:%i:%s')", "date"
	case t == "decimal" || t == "numeric":
		return "CAST(" + expr + " AS CHAR)", "decimal"
	case t == "float" || t == "double" || t == "real":
		// MySQL 이 텍스트로 보내는 값 (Go 와 같다: FLOAT 0.1 은 0.1, f64 로 바로 읽으면 0.10000000149011612)
		return "CAST(" + expr + " AS CHAR)", "decimal"
	case t == "year":
		if base == "u32" || base == "u64" {
			return "CAST(" + expr + " AS UNSIGNED)", ""
		}
		return "CAST(" + expr + " AS SIGNED)", ""
	case base == "String" && !plainStrings[t]:
		// time, json, 모르는 타입
		return "CAST(" + expr + " AS CHAR)", ""
	}
	return expr, ""
}

// decode 는 행 r 의 idx 번째 값을 읽는 식이다. kind 는 selectExpr 의 변환, enum 이면 i32 로 읽어 감싼다.
func decode(base string, nullable bool, kind string, enum string, idx string) string {
	switch kind {
	case "date":
		if nullable {
			return "row::date_opt(r.try_get::<Option<String>, _>(" + idx + ")?)"
		}
		return "row::date(r.try_get::<Option<String>, _>(" + idx + ")?)"
	case "decimal":
		if nullable {
			return "row::decimal_opt(r.try_get::<Option<String>, _>(" + idx + ")?)"
		}
		return "row::decimal(r.try_get::<Option<String>, _>(" + idx + ")?)"
	}
	if enum != "" {
		if nullable {
			return "r.try_get::<Option<i32>, _>(" + idx + ")?.map(" + enum + ")"
		}
		return enum + "(r.try_get::<i32, _>(" + idx + ")?)"
	}
	if nullable {
		return "r.try_get::<Option<" + base + ">, _>(" + idx + ")?"
	}
	return "r.try_get::<" + base + ", _>(" + idx + ")?"
}

// serdeAttr 는 필드의 serde 속성이다. 구조체에 #[serde(default)] 가 있어서 없는 key 는 기본값이다.
func serdeAttr(c rustCol) string {
	parts := []string{"rename = " + strconv.Quote(c.JSON)}
	if c.Password {
		parts = append(parts, "skip_serializing")
	}
	switch c.Base {
	case "bool":
		if c.Nullable {
			parts = append(parts, `with = "crate::support::serde_ext::opt_bool_lenient"`)
		} else {
			parts = append(parts, `with = "crate::support::serde_ext::bool_lenient"`)
		}
	case "Vec<u8>":
		if c.Nullable {
			parts = append(parts, `with = "crate::support::serde_ext::opt_bytes_b64"`)
		} else {
			parts = append(parts, `with = "crate::support::serde_ext::bytes_b64"`)
		}
	}
	return "#[serde(" + strings.Join(parts, ", ") + ")]"
}

// dateFill 은 NOT NULL 날짜 컬럼에 빈 값을 저장할 때 넣을 Rust 식이다 (Go dateFillExpr 와 같다).
func dateFill(col util.Column) string {
	format, sentinel := "clock::DATETIME_FORMAT", "1000-01-01 00:00:00"
	switch strings.ToLower(col.OriginalType) {
	case "date":
		format, sentinel = "clock::DATE_FORMAT", "1000-01-01"
	case "time":
		format, sentinel = "clock::TIME_FORMAT", "00:00:00"
	case "timestamp":
		sentinel = ""
	}
	now := "clock::local_now(" + format + ")"
	if col.HasDefault {
		if util.IsNowDefault(col.Default) {
			return now
		}
		if def := strings.Trim(col.Default, "'"); def != "" {
			return util.RustString(def) + ".to_string()"
		}
	}
	if sentinel == "" {
		return now
	}
	return util.RustString(sentinel) + ".to_string()"
}

func isDateLike(t string) bool {
	switch strings.ToLower(t) {
	case "date", "datetime", "timestamp", "time", "year":
		return true
	}
	return false
}

// writeExpr 는 Insert / Update 에 바인딩하는 값이다 (item.{field}). null 허용 날짜/시간 컬럼은 빈 값이면 NULL (Go nullIfZero).
func writeExpr(c rustCol, owner string) string {
	v := owner + "." + c.Field
	if c.Nullable && isDateLike(c.Col.OriginalType) {
		if c.Base == "String" {
			return "Bind::from(" + v + ".as_deref().filter(|s| !s.is_empty()))"
		}
		return "Bind::from(" + v + ".filter(|v| *v != 0))"
	}
	if c.Copy {
		return "Bind::from(" + v + ")"
	}
	return "Bind::from(" + v + ".clone())"
}

// paramWriteExpr 는 커스텀 UpdateXByY 의 set 값이다 (Go writeParam: null 허용 날짜/시간 컬럼의 빈 값은 NULL).
func paramWriteExpr(w rustWhere) string {
	c := w.Col
	if c.Nullable && isDateLike(c.Col.OriginalType) {
		if c.Base == "String" {
			return "if " + w.Param + ".is_empty() { Bind::Null } else { Bind::from(" + w.Param + ") }"
		}
		return "if " + w.Param + " == 0 { Bind::Null } else { Bind::from(" + w.Param + ") }"
	}
	return "Bind::from(" + w.Param + ")"
}

// zeroCheck 는 커스텀 Get/Find/Count 에서 매개변수가 "값 없음"이 아닐 때만 조건을 붙이는 검사식이다 (Go zeroCheck).
func zeroCheck(w rustWhere, hasZero bool) string {
	switch {
	case w.Slice || w.Col.Base == "bool":
		return ""
	case w.Col.Enum != "":
		if hasZero {
			return ""
		}
		return w.Param + ".0 != 0"
	case w.Col.Base == "String" || w.Col.Base == "Vec<u8>":
		return "!" + w.Param + ".is_empty()"
	case w.Col.Base == "f64":
		return w.Param + " != 0.0"
	}
	return w.Param + " != 0"
}

// compareColumn 은 Go compareColumn 과 같다 (compare 설정이 없으면 =).
func compareColumn(name string, cols []config.GpaCompare) string {
	for _, v := range cols {
		if strings.EqualFold(v.Name, name) || strings.EqualFold(strings.ReplaceAll(v.Name, "_", ""), name) {
			return v.Type
		}
	}
	return "="
}

func qualify(on bool, table string, column string) string {
	if on {
		return table + "." + column
	}
	return column
}

// buildTable 은 Go 생성기와 같은 규칙으로 테이블 정보를 만든다.
func buildTable(tableName string, prefix string, items []util.Column, db *sql.DB, gpa *config.Gpa, auth string, cnf config.ModelConfig) *table {
	items = util.DedupeColumns(tableName, items)
	if gpa != nil {
		for i, v := range items {
			for _, p := range gpa.Primary {
				if util.MatchColumn(v, p) {
					items[i].Primary = true
				}
			}
		}
	}

	t := &table{
		TableName: tableName,
		Module:    util.RustModule(tableName),
		Name:      util.ModelName(tableName),
		Model:     util.RustModelName(tableName),
		Domain:    util.FileName(tableName),
		Prefix:    prefix,
		IsView:    util.GetTableType(tableName) == "view",
		Auth:      auth,
		Jwt:       auth == "jwt",
		EnumZero:  map[string]bool{},
	}

	for _, item := range items {
		if item.Name == "Id" {
			t.HasId = true
			t.OrderKey = item.Key
		}
	}
	if t.OrderKey == "" && len(items) > 0 {
		t.OrderKey = items[0].Key
		for _, item := range items {
			if item.Primary {
				t.OrderKey = item.Key
				break
			}
		}
	}

	// 컬럼
	fields := util.RustFields(items)
	enumOf := map[string]*config.GpaMap{}
	for i, item := range items {
		c := rustCol{Col: item, Field: fields[i], JSON: util.JSONKey(item), Base: item.Type, Nullable: item.Nullable}
		if item.Name == "Id" {
			c.IsId = true
			c.Nullable = false
			c.Base = "i64"
			if item.Unsigned {
				c.Base = "u64"
			}
			if msg := util.NonIntegerIdWarning("rust", tableName, item); msg != "" {
				log.Print(msg)
			}
		}
		if m := util.FindEnumMap(gpa, item.Key); m != nil {
			c.Enum = c.Base
			enumOf[item.Name] = m
		}
		c.Copy = isCopy(c.Base)
		c.Type = c.Base
		if c.Nullable {
			c.Type = "Option<" + c.Base + ">"
		}
		c.Password = util.IsPasswordColumn(item.Name)
		c.Serde = serdeAttr(c)
		if c.Base == "String" && util.IsDateType(item.OriginalType) && !c.Nullable {
			c.Fill = dateFill(item)
		}
		t.Cols = append(t.Cols, c)
	}
	for i := range t.Cols {
		c := &t.Cols[i]
		if !c.IsId {
			t.NonId = append(t.NonId, *c)
		}
		if c.Password {
			t.Passwords = append(t.Passwords, *c)
		}
		if c.Col.Name == "Createddate" {
			t.Created = c
			t.CreatedNow = c.Base == "String"
		}
	}
	if t.HasId {
		t.IdType = t.col("Id").Base
	}

	for _, name := range util.ProtectedColumns(t.Name, items, gpa) {
		t.Protects = append(t.Protects, *t.col(name))
	}
	adminLevel, adminErr := strconv.Atoi(strings.TrimSpace(cnf.AdminLevel))
	t.AdminLevel = adminLevel
	t.AdminCheck = t.Jwt && adminErr == nil && len(t.Protects) > 0

	// enum
	var joins []util.ResolvedJoin
	if gpa != nil {
		for _, m := range gpa.Map {
			e := rustEnum{Type: util.RustEnumType(m.Name, t.Model), Key: strings.ToLower(m.Name), HasZero: util.EnumHasZero(m.Data)}
			e.Items = util.RustEnumItems(m.Data)
			for _, d := range m.Data {
				e.Labels = append(e.Labels, util.RustString(util.EnumLabel(d)))
			}
			for _, c := range t.Cols {
				if fm := util.FindEnumMap(gpa, c.Col.Key); fm != nil && fm.Name == m.Name {
					e.Field = c.Field
					e.Nullable = c.Nullable
					if e.HasZero {
						t.EnumZero[c.Col.Name] = true
					}
					break
				}
			}
			t.Enums = append(t.Enums, e)
		}
		joins = util.ResolveJoins(tableName, items, gpa.Join)
		t.Compares = gpa.Compare
	}

	// 조인 SQL (Go 와 같다: left join, 같은 테이블이 다시 나오면 별칭 j_{변수}, 접두사가 같은 테이블을 조인하면 이 테이블 컬럼을 한정)
	joinPrefixes := make([]string, len(joins))
	for i, rj := range joins {
		joinPrefixes[i] = util.TablePrefixOf(rj.Table, cnf.Database, db)
		if joinPrefixes[i] == prefix {
			t.Qual = tableName + "."
		}
	}
	t.WherePref = t.Qual + prefix + "_"
	t.OrderCol = t.Qual + prefix + "_" + t.OrderKey

	selects := make([]string, 0)
	raws := make([]string, 0)
	for i := range t.Cols {
		c := &t.Cols[i]
		c.Select, _ = selectExpr(c.Col, c.Base, c.Enum != "", qualify(len(joins) > 0, tableName, c.Col.Column))
		c.RawSel, _ = selectExpr(c.Col, c.Base, c.Enum != "", c.Col.Column)
		selects = append(selects, c.Select)
		raws = append(raws, c.RawSel)
	}
	for i := range t.NonId {
		t.NonId[i] = *t.col(t.NonId[i].Col.Name)
	}

	var joinFrom strings.Builder
	used := map[string]bool{strings.ToLower(tableName): true}
	offset := len(t.Cols)
	for i, rj := range joins {
		ref := rj.Table
		alias := ""
		if used[rj.Table] {
			alias = "j_" + util.SanitizeIdent(rj.Var, "j")
			ref = alias
		}
		used[rj.Table] = true
		j := rustJoin{Var: rj.Var, Fn: "join_" + util.RustIdent(strings.ToLower(util.SanitizeIdent(rj.Var, "j"))), Offset: offset}
		jcols := joinColumns(rj.Table, db, cnf)
		joinGpa := util.FindTableConfig(cnf.Gpa, rj.Table)
		for _, col := range jcols {
			jc := rustJoinCol{JSON: util.JSONKey(col), Base: col.Type, Nullable: col.Nullable, IsId: col.Key == "id"}
			if col.Name == "Id" {
				jc.Base = "i64"
				if col.Unsigned {
					jc.Base = "u64"
				}
			}
			if m := util.FindEnumMap(joinGpa, col.Key); m != nil {
				jc.Base = "i32"
				jc.EnumKey = strings.ToLower(m.Name)
				labels := make([]string, 0)
				for _, d := range m.Data {
					labels = append(labels, util.RustString(util.EnumLabel(d)))
				}
				jc.Labels = "&[" + strings.Join(labels, ", ") + "]"
			}
			jc.Hidden = util.IsPasswordColumn(col.Name)
			jc.Select, jc.Kind = selectExpr(col, jc.Base, jc.EnumKey != "", ref+"."+col.Column)
			selects = append(selects, jc.Select)
			j.Cols = append(j.Cols, jc)
		}
		offset += len(j.Cols)
		joinFrom.WriteString(" left join " + rj.Table)
		if alias != "" {
			joinFrom.WriteString(" " + alias)
		}
		joinFrom.WriteString(" on " + qualify(true, tableName, rj.Local.Column) + " = " + ref + "." + joinPrefixes[i] + "_id")
		t.Joins = append(t.Joins, j)
	}
	t.Select = "select " + strings.Join(selects, ", ") + " from " + tableName + joinFrom.String()
	t.Count = "select count(*) from " + tableName + joinFrom.String()
	t.RawSelect = "select " + strings.Join(raws, ", ") + " from " + tableName

	// 정렬 이름 (API 이름 / 접두사 뺀 이름 / 전체 이름, 소문자) → SQL 컬럼. 비밀번호 제외.
	seen := map[string]bool{}
	for _, c := range t.Cols {
		if c.Password {
			continue
		}
		for _, k := range []string{strings.ToLower(c.Col.Name), strings.ToLower(c.Col.Key), strings.ToLower(c.Col.Column)} {
			if k != "" && !seen[k] {
				seen[k] = true
				t.OrderCols = append(t.OrderCols, [2]string{k, t.Qual + c.Col.Column})
			}
		}
	}

	// session
	if gpa != nil {
		for _, pair := range gpa.Session {
			if msg := util.SessionColumnError(items, pair.Column, false); msg != "" {
				if t.Jwt {
					log.Fatalf("ERROR: %s: %s (generation stopped)", tableName, msg)
				}
				log.Printf("WARNING: %s: %s", tableName, msg)
				continue
			}
			col := util.FindColumn(items, pair.Column)
			t.Sessions = append(t.Sessions, rustSession{Key: pair.Key, Col: *t.col(col.Name), Column: col.Key, SQL: t.Qual + prefix + "_" + col.Key, Idx: len(t.Sessions)})
		}

		for _, fn := range util.ParseFuncs(tableName, gpa.Method, items) {
			if t.IsView && (fn.Kind == "Update" || fn.Kind == "Delete") {
				log.Printf("WARNING: %s is a view, write method %s skipped", tableName, fn.Name)
				continue
			}
			f := rustFunc{Func: fn, Fn: util.RustIdent(util.RustSnake(fn.Name))}
			for _, w := range fn.Wheres {
				c := t.col(w.Column)
				rw := rustWhere{Where: w, Col: *c, Param: "p_" + c.Field, PType: c.Base, Slice: w.Compare == "in"}
				if rw.Slice {
					rw.PType = "Vec<" + c.Base + ">"
				}
				hasZero := false
				if m := util.FindEnumMap(gpa, w.Key); m != nil && util.EnumHasZero(m.Data) {
					hasZero = true
				}
				rw.Check = zeroCheck(rw, hasZero)
				f.Wheres = append(f.Wheres, rw)
			}
			t.Funcs = append(t.Funcs, f)
		}
	}
	return t
}

// joinColumns 는 조인 테이블 컬럼이다 (조인 테이블 자신의 gpa 로 타입을 정한다, Go 와 같다).
func joinColumns(joinTable string, db *sql.DB, cnf config.ModelConfig) []util.Column {
	raws, err := util.QueryColumns(db, cnf.Database, joinTable)
	if err != nil {
		log.Println(err)
	}
	joinGpa := util.FindTableConfig(cnf.Gpa, joinTable)
	cols := make([]util.Column, 0)
	for _, raw := range raws {
		cols = append(cols, util.NewColumnFromRaw(joinTable, raw, joinGpa, cnf))
	}
	return util.DedupeColumns(joinTable, cols)
}

// sessionConv 는 세션 값(i64, 변수 v)을 컬럼 타입으로 바꾸는 식이다 (Result). i64 컬럼이면 "".
func sessionConv(c rustCol, v string) string {
	switch {
	case c.Enum != "":
		return "i32::try_from(" + v + ").map(" + c.Enum + ")"
	case c.Base == "i64":
		return ""
	case rustIntTypes[c.Base]:
		return c.Base + "::try_from(" + v + ")"
	}
	return fmt.Sprintf("Err::<%s, _>(%q)", c.Base, "unsupported session column type")
}
