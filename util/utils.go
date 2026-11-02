package util

import (
	"database/sql"
	"fmt"
	"gomachine/config"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	log "github.com/sirupsen/logrus"
)

type Where struct {
	Column  string // 컬럼의 Name (CamelCase)
	Key     string // SQL 에 쓰는 접두사 뺀 컬럼 이름 (created_at)
	Type    string
	Compare string // = / in / like, Update 는 column(set) / where
	Param   string // 생성 코드의 매개변수 이름 (예약어면 뒤에 _)
	Check   string // Go Get/Find/Count 에서 조건을 붙일지 검사하는 식. 비어 있으면 항상 붙인다.
}

type Func struct {
	Name   string
	Kind   string // Get, Find, Count, Update, Delete
	Wheres []Where
}

type Column struct {
	Name         string // 식별자용 이름 (CamelCase: CreatedAt)
	Column       string // DB 컬럼 전체 이름 (p_created_at)
	Key          string // 접두사 뺀 DB 컬럼 이름 (created_at)
	Type         string
	OriginalType string
	Prefix       string
	Primary      bool
	Nullable     bool   // information_schema.columns.is_nullable = YES
	HasDefault   bool   // column_default 가 있다 (NULL 이 아님)
	Default      string // column_default 원문 (예: CURRENT_TIMESTAMP, 0, abc)
}

// RawColumn 은 information_schema.columns 에서 읽은 컬럼 하나다.
type RawColumn struct {
	Name       string
	DataType   string
	Nullable   bool
	HasDefault bool
	Default    string
}

// StaticColumns 는 DB 연결이 없을 때 QueryColumns 가 돌려줄 테이블별 컬럼이다 (테스트용).
var StaticColumns = map[string][]RawColumn{}

// QueryColumns 는 테이블의 컬럼을 ordinal_position 순서로 읽는다 (이름, 타입, null 허용, 기본값).
func QueryColumns(db *sql.DB, schema string, table string) ([]RawColumn, error) {
	ret := make([]RawColumn, 0)
	if db == nil {
		// DB 없이 생성기를 돌릴 때(테스트) 쓰는 컬럼 목록
		if cols, ok := StaticColumns[table]; ok {
			return cols, nil
		}
		return ret, nil
	}
	query := "select column_name, data_type, is_nullable, column_default from information_schema.columns where table_schema = ? and table_name = ? order by ordinal_position"
	rows, err := db.Query(query, schema, table)
	if err != nil {
		return ret, err
	}
	defer rows.Close()

	for rows.Next() {
		var name, typeid, nullable string
		var def sql.NullString
		if err := rows.Scan(&name, &typeid, &nullable, &def); err != nil {
			log.Println(err)
			continue
		}
		ret = append(ret, RawColumn{Name: name, DataType: typeid, Nullable: strings.EqualFold(nullable, "YES"), HasDefault: def.Valid, Default: def.String})
	}
	return ret, rows.Err()
}

// NewColumnFromRaw 는 RawColumn 으로 Column 을 만든다 (null 허용 / 기본값 포함).
func NewColumnFromRaw(tableName string, raw RawColumn, gpa *config.Gpa, cnf config.ModelConfig) Column {
	col := NewColumn(tableName, raw.Name, raw.DataType, gpa, cnf)
	col.Nullable = raw.Nullable
	col.HasDefault = raw.HasDefault
	col.Default = raw.Default
	return col
}

// IsDateType 은 날짜/시간 컬럼(date, datetime, timestamp, time)인지 확인한다.
func IsDateType(originalType string) bool {
	switch strings.ToLower(originalType) {
	case "date", "datetime", "timestamp", "time":
		return true
	}
	return false
}

// IsNowDefault 는 기본값이 현재 시각(CURRENT_TIMESTAMP, now() ...)인지 확인한다.
func IsNowDefault(def string) bool {
	d := strings.ToLower(strings.TrimSpace(def))
	return strings.HasPrefix(d, "current_timestamp") || strings.HasPrefix(d, "now(") || strings.HasPrefix(d, "localtime") ||
		d == "current_date" || d == "curdate()" || d == "current_time" || d == "curtime()"
}

type EnumValue struct {
	Key   string
	Code  int
	Label string
}

type EnumData struct {
	Name    string
	Data    []EnumValue
	HasNone bool   // data[0] 이 비어 있으면 코드 0 인 none 을 만든다
	Zero    string // 코드 0 인 값 이름 (NOT NULL 필드 기본값, 모르는 코드의 대체값)
}

func CopyFile(src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		panic(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		panic(err)
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		panic(err)
	}
	err = out.Sync()
	if err != nil {
		panic(err)
	}
}

func WriteFile(filename string, content string) error {
	return os.WriteFile(filename, []byte(content), 0644)
}

// tableNameAlias 는 이름이 겹치는 테이블(예: user_tb 와 user_vw)에 따로 붙인 기본 이름이다.
var tableNameAlias = map[string]string{}

// SetTableNameAlias 는 tableName 의 기본 이름을 base 로 고정한다 (이름 충돌 회피용).
func SetTableNameAlias(tableName string, base string) {
	tableNameAlias[tableName] = base
}

// GetTableName 은 테이블 이름에서 _tb / _vw 접미사만 떼어 낸 기본 이름을 돌려준다.
// user_tb → user, user_role_tb → user_role, user_vw → user.
func GetTableName(name string) string {
	if alias, ok := tableNameAlias[name]; ok {
		return alias
	}

	lower := strings.ToLower(name)
	for _, suffix := range []string{"_tb", "_vw"} {
		if strings.HasSuffix(lower, suffix) && len(name) > len(suffix) {
			return name[:len(name)-len(suffix)]
		}
	}

	return name
}

// GetTableType 은 _vw 로 끝나는 테이블을 view 로 본다.
func GetTableType(name string) string {
	if strings.HasSuffix(strings.ToLower(name), "_vw") {
		return "view"
	}

	return "table"
}

// ModelName 은 테이블의 타입 이름이다. user_tb → User, user_role_tb → UserRole.
func ModelName(tableName string) string {
	return CamelCase(GetTableName(tableName))
}

// FileName 은 생성 파일, Go 패키지 디렉터리, URL 에 쓰는 소문자 이름이다. user_role_tb → userrole.
func FileName(tableName string) string {
	return strings.ToLower(ModelName(tableName))
}

// ResolveTableNames 는 생성 파일 이름이 겹치는 테이블(user_tb 와 user_vw 등)에 다른 이름을 붙인다.
// 테이블(_tb)이 원래 이름을 갖고, 나머지는 view 면 {name}_view, 아니면 테이블 이름 전체를 쓴다.
func ResolveTableNames(names []string) {
	groups := make(map[string][]string)
	order := make([]string, 0)
	for _, name := range names {
		key := FileName(name)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], name)
	}

	used := make(map[string]bool)
	for _, key := range order {
		used[key] = true
	}

	for _, key := range order {
		group := groups[key]
		if len(group) < 2 {
			continue
		}
		keep := group[0]
		for _, name := range group {
			if strings.HasSuffix(strings.ToLower(name), "_tb") {
				keep = name
				break
			}
		}
		for _, name := range group {
			if name == keep {
				continue
			}
			base := name
			if GetTableType(name) == "view" {
				base = GetTableName(name) + "_view"
			}
			for n := 2; used[strings.ToLower(CamelCase(base))]; n++ {
				base = fmt.Sprintf("%s%d", GetTableName(name), n)
			}
			used[strings.ToLower(CamelCase(base))] = true
			SetTableNameAlias(name, base)
			log.Printf("WARNING: %s has the same generated name as %s, generated as %s", name, keep, ModelName(name))
		}
	}
}

// MatchTableName 은 model.json table[].name 이 이 테이블을 가리키는지 확인한다.
// user_role, userrole, UserRole 모두 user_role_tb 와 맞는다.
func MatchTableName(configName string, tableName string) bool {
	if configName == "" {
		return false
	}
	base := GetTableName(tableName)
	if strings.EqualFold(configName, base) || strings.EqualFold(configName, tableName) {
		return true
	}
	return strings.EqualFold(strings.ReplaceAll(configName, "_", ""), FileName(tableName))
}

// GetName 은 컬럼 이름에서 접두사(첫 번째 _ 앞)를 뗀 나머지다. p_created_at → created_at.
func GetName(name string) string {
	idx := strings.Index(name, "_")
	if idx < 0 || idx == len(name)-1 {
		return name
	}

	return name[idx+1:]
}

// GetPrefix 는 컬럼 이름의 접두사(첫 번째 _ 앞)다. 접두사가 없으면 "".
func GetPrefix(name string) string {
	idx := strings.Index(name, "_")
	if idx < 0 {
		return ""
	}

	return name[:idx]
}

// CamelCase 는 _ 로 나눈 각 부분의 첫 글자를 대문자로 바꿔 붙인다.
// created_at → CreatedAt, createddate → Createddate (예전 strings.Title 결과와 같다).
func CamelCase(str string) string {
	var b strings.Builder
	for _, part := range strings.Split(str, "_") {
		if part == "" {
			continue
		}
		b.WriteString(strings.Title(part))
	}
	return b.String()
}

// LowerCamel 은 Kotlin 프로퍼티 이름이다. 첫 부분은 전부 소문자, 나머지 부분은 첫 글자만 대문자.
// created_at → createdAt, createddate → createddate, userId → userid (예전 .Name|lower 와 같다).
func LowerCamel(str string) string {
	var b strings.Builder
	first := true
	for _, part := range strings.Split(str, "_") {
		if part == "" {
			continue
		}
		if first {
			b.WriteString(strings.ToLower(part))
			first = false
		} else {
			b.WriteString(strings.Title(strings.ToLower(part)))
		}
	}
	return b.String()
}

// JSONKey 는 컬럼의 API 이름이다. JSON key, 목록 필터 / 검색 파라미터 이름으로 모든 백엔드(Go, Spring)와
// 프론트엔드(TypeScript, Dart)가 같은 이름을 쓴다. 접두사 뺀 컬럼 이름을 LowerCamel 로 바꾼다.
// createddate → createddate, created_at → createdAt, userId → userid (Go 의 예전 json 태그와 같다).
func JSONKey(col Column) string {
	key := col.Key
	if key == "" {
		key = col.Name
	}
	return LowerCamel(key)
}

// SearchSegment 는 GET /{table}/search/{segment} 의 경로 이름이다 (JSONKey 의 소문자).
func SearchSegment(col Column) string {
	return strings.ToLower(JSONKey(col))
}

// RangeParams 는 날짜 컬럼 목록 필터의 범위 파라미터 이름이다 (start/end + JSONKey 소문자).
// createddate → startcreateddate / endcreateddate, created_at → startcreatedat / endcreatedat.
func RangeParams(col Column) (string, string) {
	lower := strings.ToLower(JSONKey(col))
	return "start" + lower, "end" + lower
}

// PrimaryKeyIndex 는 기본 키 컬럼 위치다. gpa.primary > 이름이 id 인 컬럼 > 첫 번째 컬럼 순서 (Spring / TypeScript).
func PrimaryKeyIndex(columns []Column, gpa *config.Gpa) int {
	if gpa != nil {
		for _, p := range gpa.Primary {
			for i, col := range columns {
				if MatchColumn(col, p) {
					return i
				}
			}
		}
	}
	for i, col := range columns {
		if col.Name == "Id" {
			return i
		}
	}
	return 0
}

// IsRangeFilter 는 목록 필터에서 start/end 범위로 거르는 날짜 컬럼인지 확인한다 (date, datetime, timestamp).
func IsRangeFilter(originalType string) bool {
	switch strings.ToLower(originalType) {
	case "date", "datetime", "timestamp":
		return true
	}
	return false
}

// NewColumn 은 information_schema 의 컬럼 하나를 생성기용 Column 으로 바꾼다.
func NewColumn(tableName string, rawName string, dataType string, gpa *config.Gpa, cnf config.ModelConfig) Column {
	key := GetName(rawName)
	return Column{
		Name:         CamelCase(key),
		Column:       rawName,
		Key:          key,
		Type:         GetType(GetTableName(tableName), key, dataType, gpa, cnf),
		OriginalType: strings.ToLower(dataType),
		Prefix:       GetPrefix(rawName),
	}
}

// MatchColumn 은 설정에 적힌 컬럼 이름(created_by, createdBy, CreatedBy, p_created_by)이 item 인지 확인한다.
func MatchColumn(item Column, name string) bool {
	if name == "" {
		return false
	}
	if strings.EqualFold(item.Name, name) || strings.EqualFold(item.Key, name) || strings.EqualFold(item.Column, name) {
		return true
	}
	return strings.EqualFold(strings.ReplaceAll(name, "_", ""), item.Name)
}

// FindColumn 은 설정에 적힌 이름에 해당하는 컬럼을 찾는다.
func FindColumn(items []Column, name string) *Column {
	for i := range items {
		if MatchColumn(items[i], name) {
			return &items[i]
		}
	}
	return nil
}

func Split(src string) (entries []string) {
	// don't split invalid utf8
	if !utf8.ValidString(src) {
		return []string{src}
	}
	entries = []string{}
	var runes [][]rune
	lastClass := 0
	class := 0
	// split into fields based on class of unicode character
	for _, r := range src {
		switch true {
		case unicode.IsLower(r):
			class = 1
		case unicode.IsUpper(r):
			class = 2
		case unicode.IsDigit(r):
			class = 3
		default:
			class = 4
		}
		if class == lastClass {
			runes[len(runes)-1] = append(runes[len(runes)-1], r)
		} else {
			runes = append(runes, []rune{r})
		}
		lastClass = class
	}
	// handle upper case -> lower case sequences, e.g.
	// "PDFL", "oader" -> "PDF", "Loader"
	for i := 0; i < len(runes)-1; i++ {
		if unicode.IsUpper(runes[i][0]) && unicode.IsLower(runes[i+1][0]) {
			runes[i+1] = append([]rune{runes[i][len(runes[i])-1]}, runes[i+1]...)
			runes[i] = runes[i][:len(runes[i])-1]
		}
	}
	// construct []string from results
	for _, s := range runes {
		if len(s) > 0 {
			entries = append(entries, string(s))
		}
	}
	return
}

// FindEnumMap 은 컬럼(접두사 뺀 이름)에 해당하는 gpa.map 항목을 찾는다. 대소문자와 _ 는 무시한다.
func FindEnumMap(gpa *config.Gpa, name string) *config.GpaMap {
	if gpa == nil {
		return nil
	}
	norm := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", "")) }
	for i := range gpa.Map {
		if norm(gpa.Map[i].Name) == norm(name) {
			return &gpa.Map[i]
		}
	}
	return nil
}

func isLang(cnf config.ModelConfig, names ...string) bool {
	for _, name := range names {
		if cnf.Language == name {
			return true
		}
	}
	return false
}

// GetType 은 MySQL data_type 을 언어별 타입으로 바꾼다. 모르는 타입은 경고를 찍고 문자열로 처리한다.
// Go: decimal 은 float64 (예전 int 는 소수점이 잘리고 "12.50" Scan 이 실패했다), blob/binary/bit 는 []byte.
func GetType(tableName string, name string, t string, gpa *config.Gpa, cnf config.ModelConfig) string {
	if m := FindEnumMap(gpa, name); m != nil {
		enumType := CamelCase(m.Name)
		if isLang(cnf, "dart", "flutter", "kotlin", "spring") {
			return enumType
		}
		return GoConstPackage(tableName) + "." + enumType
	}

	t = strings.ToLower(strings.TrimSpace(t))
	var types map[string]string
	fallback := "string"
	if isLang(cnf, "dart", "flutter") {
		types = dartTypes
		fallback = "String"
	} else if isLang(cnf, "kotlin", "spring") {
		types = kotlinTypes
		fallback = "String"
	} else {
		types = goTypes
	}

	if ret, ok := types[t]; ok {
		return ret
	}

	log.Printf("WARNING: unknown column type %q (%s.%s), mapped to %s", t, tableName, name, fallback)
	return fallback
}

var goTypes = map[string]string{
	"int": "int", "integer": "int", "mediumint": "int", "smallint": "int", "year": "int",
	"bigint":  "int64",
	"tinyint": "bool",
	"float":   "float64", "double": "float64", "real": "float64", "decimal": "float64", "numeric": "float64",
	"char": "string", "varchar": "string", "tinytext": "string", "text": "string", "mediumtext": "string", "longtext": "string",
	"enum": "string", "set": "string", "json": "string",
	"datetime": "string", "date": "string", "timestamp": "string", "time": "string",
	"binary": "[]byte", "varbinary": "[]byte", "tinyblob": "[]byte", "blob": "[]byte", "mediumblob": "[]byte", "longblob": "[]byte",
	// bit 은 드라이버가 원시 바이트("\x01")로 돌려줘서 bool 로 Scan 할 수 없다.
	"bit": "[]byte",
}

var kotlinTypes = map[string]string{
	"int": "Int", "integer": "Int", "mediumint": "Int", "smallint": "Int", "tinyint": "Int", "year": "Int",
	"bigint": "Long",
	"float":  "Double", "double": "Double", "real": "Double",
	"decimal": "BigDecimal", "numeric": "BigDecimal",
	"char": "String", "varchar": "String", "tinytext": "String", "text": "String", "mediumtext": "String", "longtext": "String",
	"enum": "String", "set": "String", "json": "String", "time": "String",
	"datetime": "LocalDateTime", "timestamp": "LocalDateTime",
	"date":   "LocalDate",
	"binary": "ByteArray", "varbinary": "ByteArray", "tinyblob": "ByteArray", "blob": "ByteArray", "mediumblob": "ByteArray", "longblob": "ByteArray",
	"bit": "Boolean",
}

var dartTypes = map[string]string{
	"int": "int", "integer": "int", "mediumint": "int", "smallint": "int", "bigint": "int", "year": "int",
	"tinyint": "bool", "bit": "bool",
	"float": "double", "double": "double", "real": "double", "decimal": "double", "numeric": "double",
	"char": "String", "varchar": "String", "tinytext": "String", "text": "String", "mediumtext": "String", "longtext": "String",
	"enum": "String", "set": "String", "json": "String",
	"datetime": "String", "date": "String", "timestamp": "String", "time": "String",
	"binary": "String", "varbinary": "String", "tinyblob": "String", "blob": "String", "mediumblob": "String", "longblob": "String",
}

// TablePrefix 는 {str}_tb 테이블의 컬럼 접두사를 돌려준다 (첫 번째 컬럼 기준).
func TablePrefix(str string, schema string, db *sql.DB) string {
	cols, err := QueryColumns(db, schema, str+"_tb")
	if err != nil {
		log.Println(err)
		return ""
	}
	if len(cols) > 0 {
		return GetPrefix(cols[0].Name)
	}
	return ""
}

func Unique(items []Func) []string {
	ret := make([]string, 0)

	for _, item := range items {
		for _, where := range item.Wheres {
			str := where.Type
			if strings.Index(str, ".") > 0 {
				strs := strings.Split(str, ".")

				flag := false

				for i := 0; i < len(ret); i++ {
					if ret[i] == strs[0] {
						flag = true
						break
					}
				}

				if flag == false {
					ret = append(ret, strs[0])
				}
			}
		}
	}

	return ret
}

// IsPasswordColumn 은 비밀번호 컬럼(Passwd 또는 Password)인지 확인한다.
func IsPasswordColumn(name string) bool {
	return name == "Passwd" || name == "Password"
}

// PasswordColumn 은 비밀번호 컬럼의 Name 을 돌려준다. 없으면 "".
func PasswordColumn(items []Column) string {
	for _, item := range items {
		if IsPasswordColumn(item.Name) {
			return item.Name
		}
	}
	return ""
}

// ProtectedColumns 는 REST 입력으로 받지 않을 컬럼의 Name 목록을 돌려준다.
// gpa.protect 가 있으면 그것을 쓰고, 없으면 User 테이블의 Level 컬럼을 기본으로 보호한다.
// id 와 session 컬럼은 따로 처리(자동 증가 / 세션 값으로 고정)하므로 제외한다.
func ProtectedColumns(modelName string, items []Column, gpa *config.Gpa) []string {
	ret := make([]string, 0)
	if gpa != nil && len(gpa.Protect) > 0 {
		for _, p := range gpa.Protect {
			for _, item := range items {
				if MatchColumn(item, p) && item.Name != "Id" && !isSessionColumn(item, gpa) {
					ret = append(ret, item.Name)
					break
				}
			}
		}
		return ret
	}

	if modelName == "User" {
		for _, item := range items {
			if item.Name == "Level" {
				ret = append(ret, item.Name)
			}
		}
	}
	return ret
}

func isSessionColumn(item Column, gpa *config.Gpa) bool {
	for _, pair := range gpa.Session {
		if MatchColumn(item, pair.Column) {
			return true
		}
	}
	return false
}

// goReserved 는 Go 예약어다.
var goReserved = toSet(
	"break", "case", "chan", "const", "continue", "default", "defer", "else", "fallthrough", "for", "func", "go", "goto",
	"if", "import", "interface", "map", "package", "range", "return", "select", "struct", "switch", "type", "var",
)

// goKeywords 는 Go 예약어와 미리 선언된 식별자다.
var goKeywords = toSet(append(keys(goReserved),
	"any", "append", "bool", "byte", "cap", "clear", "close", "comparable", "complex", "complex64", "complex128", "copy",
	"delete", "error", "false", "float32", "float64", "imag", "int", "int8", "int16", "int32", "int64", "iota", "len",
	"make", "max", "min", "new", "nil", "panic", "print", "println", "real", "recover", "rune", "string", "true",
	"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
)...)

func keys(m map[string]bool) []string {
	ret := make([]string, 0, len(m))
	for k := range m {
		ret = append(ret, k)
	}
	return ret
}

// goImportNames 는 생성 코드가 import 하는 패키지 이름이다.
var goImportNames = toSet(
	"config", "sql", "errors", "fmt", "strings", "time", "log", "mysql", "pq", "controllers", "jwt", "models",
	"reflect", "regexp", "json", "strconv", "rest", "api", "fiber", "routers", "os", "global", "main", "init",
)

// goParamLocals 는 커스텀 메서드 본문의 지역 변수 이름이다. 매개변수 이름과 겹치면 안 된다.
var goParamLocals = toSet("p", "c", "args", "rets", "items", "item", "conn", "manager", "err", "query", "params", "res")

// goConstLocals 는 상수 패키지를 참조하는 함수(UpdateWhere, InitExtra, 라우터 클로저)의 지역 변수/수신자 이름이다.
// 상수 패키지 이름이 이것과 같으면 가려지므로 별칭을 쓴다.
var goConstLocals = toSet("p", "c", "v", "i", "s", "e", "err", "ok", "flag", "results", "controller", "group", "columns", "args", "query", "params")

// kotlinKeywords 는 Kotlin hard keyword 다. 식별자로 쓰려면 backtick 으로 감싸야 한다.
var kotlinKeywords = toSet(
	"as", "break", "class", "continue", "do", "else", "false", "for", "fun", "if", "in", "interface", "is", "null",
	"object", "package", "return", "super", "this", "throw", "true", "try", "typealias", "typeof", "val", "var",
	"when", "while",
)

func toSet(names ...string) map[string]bool {
	ret := make(map[string]bool, len(names))
	for _, name := range names {
		ret[name] = true
	}
	return ret
}

// GoIdent 는 Go 매개변수/변수 이름으로 쓸 수 있게 예약어나 생성 코드의 이름과 겹치면 뒤에 _ 를 붙인다.
func GoIdent(name string) string {
	if name == "" {
		return "_v"
	}
	if goKeywords[name] || goImportNames[name] || goParamLocals[name] {
		return name + "_"
	}
	return name
}

// GoConstPackage 는 테이블 상수 패키지(models/{name})를 import 할 때 쓰는 이름이다.
// log, config, time 처럼 생성 코드가 이미 import 하는 이름이나 예약어와 겹치면 뒤에 pkg 를 붙인다 (log → logpkg).
func GoConstPackage(tableName string) string {
	name := FileName(tableName)
	if name == "" || goKeywords[name] || goImportNames[name] || goConstLocals[name] || !isIdent(name) {
		return sanitizeIdent(name, "p") + "pkg"
	}
	return name
}

// GoConstPackageClause 는 상수 패키지 파일의 package 이름이다. 디렉터리 이름을 그대로 쓰고 (손으로 쓴 코드의 import 가 깨지지 않게),
// 패키지 이름으로 쓸 수 없을 때(예약어, main, init)만 GoConstPackage 이름을 쓴다.
func GoConstPackageClause(tableName string) string {
	name := FileName(tableName)
	if name == "" || goReserved[name] || name == "main" || name == "init" || !isIdent(name) {
		return GoConstPackage(tableName)
	}
	return name
}

// KotlinIdent 는 Kotlin 예약어면 backtick 으로 감싼다.
func KotlinIdent(name string) string {
	if kotlinKeywords[name] {
		return "`" + name + "`"
	}
	return name
}

func isIdent(name string) bool {
	for i, r := range name {
		if !(r == '_' || unicode.IsLetter(r) || (i > 0 && unicode.IsDigit(r))) {
			return false
		}
	}
	return name != ""
}

// sanitizeIdent 는 글자/숫자/_ 가 아닌 문자를 _ 로 바꾸고, 숫자로 시작하면 lead 를 앞에 붙인다.
func sanitizeIdent(name string, lead string) string {
	var b strings.Builder
	for _, r := range name {
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	ret := b.String()
	if ret == "" {
		return lead
	}
	if unicode.IsDigit([]rune(ret)[0]) {
		return lead + ret
	}
	return ret
}

// SanitizeIdent 는 식별자로 쓸 수 없는 문자를 _ 로 바꾸고, 숫자로 시작하면 lead 를 앞에 붙인다.
func SanitizeIdent(name string, lead string) string {
	return sanitizeIdent(name, lead)
}

// DedupeIdent 는 used 에 이미 있는 이름이면 뒤에 _2, _3 ... 을 붙이고 used 에 기록한다.
func DedupeIdent(ident string, used map[string]bool) string {
	return dedupe(ident, used)
}

// EnumItem 은 gpa.map 의 data 항목 하나("key:label")다.
type EnumItem struct {
	Index int    // data 배열 안의 위치
	Code  int    // 저장/전송하는 코드 (EnumCode, data 위치와 같다)
	Raw   string // 원래 key
	Ident string // 언어별 상수 이름
	Label string // 표시 이름 (":" 가 없으면 key 그대로)
	Quote string // Label 을 문자열 리터럴로 쓴 것 (따옴표 포함)
}

// EnumLabel 은 "key:label" 의 label 이다. ":" 가 없으면 전체를 돌려준다.
func EnumLabel(data string) string {
	if idx := strings.Index(data, ":"); idx >= 0 {
		return data[idx+1:]
	}
	return data
}

// EnumKey 는 "key:label" 의 key 다.
func EnumKey(data string) string {
	if idx := strings.Index(data, ":"); idx >= 0 {
		return data[:idx]
	}
	return data
}

// EnumCode 는 gpa.map data 항목의 코드다. 모든 언어(Go, Kotlin, TypeScript, Dart)에서 data 배열 안의 위치를 코드로 쓴다.
// 빈 항목("")은 자리만 차지하고 상수를 만들지 않는다 (예: ["", "use:Use"] → Use = 1).
func EnumCode(index int) int {
	return index
}

// EnumHasZero 는 코드 0 이 실제 값인지(data[0] 이 비어 있지 않은지) 확인한다.
// 코드 0 이 실제 값이면 0 을 "조건 없음"으로 볼 수 없다.
func EnumHasZero(data []string) bool {
	return len(data) > 0 && data[0] != ""
}

// GoEnumItems 는 Go 상수 이름({Type}{Key})을 만든다. 빈 항목은 건너뛴다. 상수 값은 EnumCode (data 위치).
// 식별자로 쓸 수 없는 문자는 _ 로 바꾸고, 겹치면 뒤에 번호를 붙인다.
func GoEnumItems(typeName string, data []string) []EnumItem {
	ret := make([]EnumItem, 0)
	used := map[string]bool{}
	for i, d := range data {
		if d == "" {
			continue
		}
		key := EnumKey(d)
		ident := typeName + sanitizeIdent(strings.Title(key), "")
		ident = dedupe(ident, used)
		ret = append(ret, EnumItem{Index: i, Code: EnumCode(i), Raw: key, Ident: ident, Label: EnumLabel(d), Quote: strconv.Quote(EnumLabel(d))})
	}
	return ret
}

// KotlinEnumItems 는 Kotlin enum 상수 이름(대문자 key)을 만든다. 빈 항목은 건너뛴다. 코드는 EnumCode (data 위치).
// 식별자로 쓸 수 없으면 바꾸고(숫자로 시작하면 V 접두사), 합성 상수 NONE 이나 다른 항목과 겹치면 뒤에 번호를 붙인다.
func KotlinEnumItems(data []string) []EnumItem {
	ret := make([]EnumItem, 0)
	used := map[string]bool{"NONE": true}
	for i, d := range data {
		if d == "" {
			continue
		}
		key := EnumKey(d)
		ident := sanitizeIdent(strings.ToUpper(key), "V")
		if kotlinKeywords[ident] {
			ident = "`" + ident + "`"
		}
		ident = dedupe(ident, used)
		ret = append(ret, EnumItem{Index: i, Code: EnumCode(i), Raw: key, Ident: ident, Label: EnumLabel(d), Quote: KotlinString(EnumLabel(d))})
	}
	return ret
}

func dedupe(ident string, used map[string]bool) string {
	ret := ident
	for n := 2; used[ret]; n++ {
		ret = fmt.Sprintf("%s_%d", ident, n)
	}
	used[ret] = true
	return ret
}

// KotlinString 은 Kotlin 문자열 리터럴이다 (\, ", $ 이스케이프).
func KotlinString(str string) string {
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "$", "\\$", "\n", "\\n", "\r", "\\r")
	return "\"" + r.Replace(str) + "\""
}

// ParseFuncs 는 gpa.method (GetByName, FindByUserStatus, FindByIds, UpdateNameById, DeleteByUser ...) 를 해석한다.
// 컬럼과 맞지 않는 메서드는 깨진 코드를 만들지 않도록 경고를 찍고 건너뛴다.
func ParseFuncs(tableName string, methods []string, items []Column) []Func {
	ret := make([]Func, 0)
	for _, method := range methods {
		fn, err := ParseFunc(method, items)
		if err != nil {
			log.Printf("WARNING: %s method %q skipped: %v", tableName, method, err)
			continue
		}
		ret = append(ret, fn)
	}
	return ret
}

// ParseFunc 는 메서드 이름 하나를 해석한다.
// Get/Find/Count/Delete + By + 컬럼들 (And 는 건너뜀, 컬럼+s 는 in, 컬럼+like 는 like),
// Update + 바꿀 컬럼들 + By + 조건 컬럼들.
func ParseFunc(method string, items []Column) (Func, error) {
	tokens := Split(method)
	if len(tokens) < 3 {
		return Func{}, fmt.Errorf("method name must be Get/Find/Count/Delete + By + column, or Update + column + By + column")
	}

	kind := tokens[0]
	fn := Func{Name: method, Kind: kind}
	switch kind {
	case "Get", "Find", "Count", "Delete":
		if tokens[1] != "By" {
			return fn, fmt.Errorf("expected By after %s", kind)
		}
		wheres, err := matchColumns(tokens[2:], items, true)
		if err != nil {
			return fn, err
		}
		fn.Wheres = wheres
	case "Update":
		by := -1
		for i := 1; i < len(tokens); i++ {
			if tokens[i] == "By" {
				by = i
				break
			}
		}
		if by <= 1 || by == len(tokens)-1 {
			return fn, fmt.Errorf("expected Update + column + By + column")
		}
		sets, err := matchColumns(tokens[1:by], items, false)
		if err != nil {
			return fn, err
		}
		wheres, err := matchColumns(tokens[by+1:], items, false)
		if err != nil {
			return fn, err
		}
		for i := range sets {
			sets[i].Compare = "column"
		}
		for i := range wheres {
			wheres[i].Compare = "where"
		}
		fn.Wheres = append(sets, wheres...)
	default:
		return fn, fmt.Errorf("unknown method type %q", kind)
	}

	if !isIdent(method) {
		return fn, fmt.Errorf("invalid method name")
	}
	seen := map[string]bool{}
	for _, w := range fn.Wheres {
		if seen[w.Param] {
			return fn, fmt.Errorf("column %s is used twice", w.Column)
		}
		seen[w.Param] = true
	}
	return fn, nil
}

// matchColumns 는 토큰을 앞에서부터 가장 길게 맞는 컬럼으로 묶는다.
// 대소문자까지 같은 컬럼을 먼저 찾고, 없으면 대소문자를 무시한다 (GetByUserId → Userid 컬럼).
func matchColumns(tokens []string, items []Column, allowCompare bool) ([]Where, error) {
	ret := make([]Where, 0)
	for i := 0; i < len(tokens); {
		found := false
		for _, fold := range []bool{false, true} {
			for j := len(tokens); j > i && !found; j-- {
				if w, ok := matchColumn(strings.Join(tokens[i:j], ""), items, allowCompare, fold); ok {
					ret = append(ret, w)
					i = j
					found = true
				}
			}
			if found {
				break
			}
		}
		if found {
			continue
		}
		if tokens[i] == "And" {
			i++
			continue
		}
		return nil, fmt.Errorf("%q is not a column", strings.Join(tokens[i:], ""))
	}
	if len(ret) == 0 {
		return nil, fmt.Errorf("no column")
	}
	return ret, nil
}

func matchColumn(str string, items []Column, allowCompare bool, fold bool) (Where, bool) {
	eq := func(a, b string) bool {
		if fold {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	for _, item := range items {
		if eq(str, item.Name) {
			return Where{Column: item.Name, Key: item.Key, Type: item.Type, Compare: "=", Param: GoIdent(strings.ToLower(item.Name))}, true
		}
	}
	if !allowCompare {
		return Where{}, false
	}
	for _, item := range items {
		if eq(str, item.Name+"s") {
			return Where{Column: item.Name, Key: item.Key, Type: "[]" + item.Type, Compare: "in", Param: GoIdent(strings.ToLower(item.Name))}, true
		}
	}
	for _, item := range items {
		if eq(str, item.Name+"like") || eq(str, item.Name+"Like") {
			return Where{Column: item.Name, Key: item.Key, Type: item.Type, Compare: "like", Param: GoIdent(strings.ToLower(item.Name))}, true
		}
	}
	return Where{}, false
}
