package util

import (
	"fmt"
	"gomachine/config"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// 읽기 정책 (model.json table[].read). 모든 백엔드(Go, Spring, Rust)가 ReadPolicy 하나로 정한다.
const (
	ReadOwner  = "owner"  // 세션 컬럼이 현재 사용자인 행만 (session 필요)
	ReadLogin  = "login"  // 로그인 사용자는 모든 행
	ReadPublic = "public" // 비로그인도 모든 행
	// ReadSelf 는 auth=jwt 의 User 테이블이다: 본인 행(id = 세션 id)만, 관리자(adminLevel)는 모든 행. read 설정은 무시한다.
	ReadSelf = "self"
)

// ReadPolicy 는 테이블의 읽기(목록 / 개수 / 검색 / 단건 / 커스텀 Get·Find·Count) 정책이다.
// auth 가 jwt 가 아니면 항상 public (세션을 쓰지 않는다). jwt 면 User 는 self, 그 밖은 read 설정,
// 없으면 session 이 있으면 owner, 없으면 login 이다. 쓰기(수정 / 삭제 / batch)는 정책과 상관없이 로그인 + 소유권 검사다.
// 모르는 값이나 session 없는 owner 는 에러다 (생성을 멈춘다).
func ReadPolicy(modelName string, gpa *config.Gpa, auth string) (string, error) {
	read := ""
	hasSession := false
	if gpa != nil {
		read = strings.ToLower(strings.TrimSpace(gpa.Read))
		hasSession = len(gpa.Session) > 0
	}
	switch read {
	case "", ReadOwner, ReadLogin, ReadPublic:
	default:
		return "", fmt.Errorf("table %q: unknown read %q (owner, login, public)", gpaName(gpa, modelName), gpa.Read)
	}
	if auth != "jwt" {
		if read != "" && read != ReadPublic {
			warnOnce("WARNING: table %q: read %q has no effect without auth \"jwt\" (every read is public)", gpaName(gpa, modelName), read)
		}
		return ReadPublic, nil
	}
	if modelName == "User" {
		if read != "" {
			warnOnce("WARNING: table %q: read %q is ignored, User reads are always the user's own row (admins: all rows)", gpaName(gpa, modelName), read)
		}
		return ReadSelf, nil
	}
	switch {
	case read == ReadOwner && !hasSession:
		return "", fmt.Errorf("table %q: read \"owner\" needs a session column (table[].session)", gpaName(gpa, modelName))
	case read != "":
		return read, nil
	case hasSession:
		return ReadOwner, nil
	}
	return ReadLogin, nil
}

func gpaName(gpa *config.Gpa, modelName string) string {
	if gpa != nil && gpa.Name != "" {
		return gpa.Name
	}
	return modelName
}

// MustReadPolicy 는 ReadPolicy 의 에러를 로그로 찍고 생성을 멈춘다 (main.go 가 미리 검사하므로 테스트 / 하네스용 안전장치).
func MustReadPolicy(modelName string, gpa *config.Gpa, auth string) string {
	p, err := ReadPolicy(modelName, gpa, auth)
	if err != nil {
		log.Fatalf("ERROR: %v (generation stopped)", err)
	}
	return p
}

// 목록 필터의 문자열 부분 일치(like)에 쓰는 escape 문자. SQL 에 `like ? escape '!'` 로 쓴다.
// 백슬래시가 아닌 문자를 써서 sql_mode(NO_BACKSLASH_ESCAPES)와 상관없이 같다.
const LikeEscape = "!"

// LikePattern 은 목록 필터 값 v 의 부분 일치 패턴이다: %, _, ! 를 ! 로 escape 하고 양쪽에 % 를 붙인다.
// 생성 코드(Go / Rust / Kotlin)가 같은 규칙을 런타임에 적용한다 (테스트 기준).
func LikePattern(v string) string {
	r := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return "%" + r.Replace(v) + "%"
}

// FilterKind 는 목록 필터의 컬럼 분류다: "range"(날짜 start/end), "enum", "bool", "text", "number", ""(필터 없음: []byte).
// 언어마다 타입이 달라서 MySQL 타입(OriginalType)과 gpa.map 으로 정한다.
func FilterKind(col Column, gpa *config.Gpa) string {
	if IsRangeFilter(col.OriginalType) {
		return "range"
	}
	if FindEnumMap(gpa, col.Key) != nil {
		return "enum"
	}
	if k := FilterKindOf(col.OriginalType); k != "bytes" {
		return k
	}
	return ""
}

// filterOps 는 compare 설정으로 쓸 수 있는 목록 필터 연산자다 (분류마다).
var filterOps = map[string][]string{
	"text":   {"like", "=", "<>", "<", "<=", ">", ">=", "not like"},
	"number": {"=", "<>", "<", "<=", ">", ">="},
	"enum":   {"=", "<>"},
	"bool":   {"=", "<>"},
}

// FilterCompare 는 목록 / 개수 필터에서 컬럼 값과 비교하는 연산자다. 모든 백엔드가 같은 값을 쓴다.
// 기본: 문자열 컬럼은 like (부분 일치, LikePattern), 그 밖은 =. 날짜 범위 컬럼은 "" (start/end 로 >= / <=).
// table[].compare 로 컬럼마다 바꿀 수 있다 ("=", "<>"(= "!="), "<", "<=", ">", ">=", 문자열은 "like", "not like").
// 그 컬럼에 쓸 수 없는 연산자는 경고하고 기본값을 쓴다.
func FilterCompare(col Column, gpa *config.Gpa) string {
	kind := FilterKind(col, gpa)
	if kind == "" || kind == "range" {
		return ""
	}
	def := "="
	if kind == "text" {
		def = "like"
	}
	if gpa == nil {
		return def
	}
	for _, c := range gpa.Compare {
		if !strings.EqualFold(c.Name, col.Name) && !strings.EqualFold(strings.ReplaceAll(c.Name, "_", ""), col.Name) && !MatchColumn(col, c.Name) {
			continue
		}
		op := strings.Join(strings.Fields(strings.ToLower(c.Type)), " ")
		if op == "!=" {
			op = "<>"
		}
		for _, ok := range filterOps[kind] {
			if op == ok {
				return op
			}
		}
		warnOnce("WARNING: table %q compare %q: %q cannot be used for a %s filter, using %q", gpa.Name, c.Name, c.Type, kind, def)
		return def
	}
	return def
}

// DefaultLiteral 은 컬럼 기본값(information_schema column_default)의 상수 값이다. 상수가 아니면 ok=false.
// MariaDB 는 문자열을 '...' 로 감싸고 기본값이 NULL 이면 "NULL" 을, MySQL 은 감싸지 않은 값을 준다.
// 함수 / 식(current_timestamp(), uuid() ...), NULL 은 상수가 아니다. bit 기본값 b'1' 은 "1" 이다.
// 숫자 / bool 컬럼은 숫자로 읽을 수 있을 때만 상수다 (bool 은 "0" / "1" ... 그대로 돌려준다).
func DefaultLiteral(col Column) (string, bool) {
	if !col.HasDefault {
		return "", false
	}
	d := strings.TrimSpace(col.Default)
	if strings.EqualFold(d, "null") || IsNowDefault(d) {
		return "", false
	}
	quoted := false
	switch {
	case len(d) >= 3 && (d[0] == 'b' || d[0] == 'B') && d[1] == '\'' && d[len(d)-1] == '\'':
		// bit 기본값 b'101'
		v, err := strconv.ParseInt(d[2:len(d)-1], 2, 64)
		if err != nil {
			return "", false
		}
		d = strconv.FormatInt(v, 10)
	case len(d) >= 2 && d[0] == '\'' && d[len(d)-1] == '\'':
		d = strings.ReplaceAll(d[1:len(d)-1], "''", "'")
		quoted = true
	}
	switch FilterKindOf(col.OriginalType) {
	case "number", "bool":
		if _, err := strconv.ParseFloat(d, 64); err != nil {
			return "", false
		}
		return d, true
	case "bytes":
		return "", false
	}
	// 문자열 / 날짜: MySQL 은 감싸지 않으므로 식(괄호)은 상수로 보지 않는다.
	if !quoted && strings.ContainsAny(d, "()") {
		return "", false
	}
	return d, true
}

// FilterKindOf 는 MySQL 타입만으로 본 값 분류다: "number", "bool", "bytes", "text" (날짜, 모르는 타입도 text).
func FilterKindOf(originalType string) string {
	t, ok := goTypes[strings.ToLower(strings.TrimSpace(originalType))]
	if !ok {
		return "text"
	}
	switch t {
	case "string":
		return "text"
	case "bool":
		return "bool"
	case "[]byte":
		return "bytes"
	}
	return "number"
}
