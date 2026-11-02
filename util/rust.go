package util

import (
	"strings"
	"unicode"
)

// rustKeywords 는 Rust 식별자로 쓸 수 없는 이름이다 (strict / reserved / 2018·2024 edition 예약어, weak keyword union 포함).
// self / super / crate / Self 는 raw identifier(r#) 로도 쓸 수 없으므로 모두 뒤에 _ 를 붙이는 방식을 쓴다.
var rustKeywords = toSet(
	"as", "break", "const", "continue", "crate", "else", "enum", "extern", "false", "fn", "for", "if", "impl", "in",
	"let", "loop", "match", "mod", "move", "mut", "pub", "ref", "return", "self", "Self", "static", "struct", "super",
	"trait", "true", "type", "unsafe", "use", "where", "while", "async", "await", "dyn",
	"abstract", "become", "box", "do", "final", "macro", "override", "priv", "typeof", "unsized", "virtual", "yield",
	"try", "gen", "union",
)

// rustReservedTypes 는 생성 Rust 코드가 이름만으로 쓰는 타입 / trait / derive / prelude 이름이다.
// 모델 struct 나 enum newtype 이 이 이름이면 같은 모듈의 use 나 prelude 를 가려 컴파일되지 않으므로 접미사를 붙인다.
var rustReservedTypes = toSet(
	// std prelude
	"Option", "Some", "None", "Result", "Ok", "Err", "Vec", "String", "Box", "ToString", "ToOwned", "Clone", "Copy",
	"Default", "Debug", "Drop", "Eq", "PartialEq", "Ord", "PartialOrd", "Hash", "From", "Into", "TryFrom", "TryInto",
	"AsRef", "AsMut", "Iterator", "IntoIterator", "Extend", "FromIterator", "Fn", "FnMut", "FnOnce", "Send", "Sync",
	"Sized", "Unpin", "Self",
	// std
	"HashMap", "HashSet", "Arc", "Duration", "Display", "Formatter",
	// axum / http
	"Router", "Json", "Query", "Path", "State", "Extension", "StatusCode", "IntoResponse", "Response", "HeaderMap", "Request",
	// sqlx
	"Pool", "MySql", "MySqlPool", "MySqlRow", "Row", "FromRow", "QueryBuilder", "Type", "Encode", "Decode", "Transaction",
	// serde
	"Serialize", "Deserialize", "Serializer", "Deserializer", "Value",
	// 생성 코드 (support / 인증)
	"Error", "Session", "Claims", "AppState",
)

// rustReservedModules 는 crate 최상위 모듈 / 파일 이름이다. 테이블 모듈 이름이 이것과 같으면 뒤에 _ 를 붙인다.
var rustReservedModules = toSet("mod", "main", "lib", "support", "handlers", "models", "routes", "custom", "std", "core", "alloc")

// RustIdent 는 Rust 식별자(필드, 변수, 모듈)로 쓸 수 있게 한다. 식별자 문자가 아니면 _ 로 바꾸고 숫자로 시작하면 v 를 붙이며,
// 예약어나 _ 하나면 뒤에 _ 를 붙인다 (type → type_, self → self_).
func RustIdent(name string) string {
	id := sanitizeIdent(name, "v")
	if rustKeywords[id] || id == "_" {
		return id + "_"
	}
	return id
}

// RustSnake 는 lowerCamel / CamelCase 이름을 snake_case 로 바꾼다 (createdAt → created_at, userID → user_id).
func RustSnake(name string) string {
	runes := []rune(name)
	var b strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) {
			prev := rune(0)
			if i > 0 {
				prev = runes[i-1]
			}
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if i > 0 && prev != '_' && (unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower)) {
				b.WriteRune('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// RustField 는 컬럼의 Rust struct 필드 이름이다. API 이름(JSONKey)의 snake_case 에 RustIdent 를 적용한다
// (createdAt → created_at, userid → userid, type → type_). DedupeColumns 결과면 테이블 안에서 겹치지 않는다
// (식별자로 쓸 수 없는 문자를 바꾼 경우까지 막으려면 RustFields 를 쓴다).
func RustField(col Column) string {
	return RustIdent(RustSnake(JSONKey(col)))
}

// RustFields 는 컬럼마다 RustField 이고, 그래도 겹치면 뒤에 _2, _3 ... 을 붙인다.
func RustFields(items []Column) []string {
	used := map[string]bool{}
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, dedupe(RustField(item), used))
	}
	return ret
}

// RustTypeName 은 Rust 생성 타입(모델 struct, enum newtype) 이름이다. 예약어, 생성 코드가 쓰는 타입 이름(rustReservedTypes),
// taken 과 겹치면 suffix 를 붙인다 (Option → OptionModel, Type → TypeEnum).
func RustTypeName(name string, suffix string, taken ...string) string {
	name = sanitizeIdent(name, "T")
	clash := rustReservedTypes[name] || rustKeywords[name]
	for _, t := range taken {
		if t == name {
			clash = true
		}
	}
	if !clash {
		return name
	}
	return name + suffix
}

// RustModelName 은 테이블의 Rust 모델 struct 이름이다 (ModelName, 겹치면 Model 접미사: result_tb → ResultModel).
func RustModelName(tableName string) string {
	return RustTypeName(ModelName(tableName), "Model")
}

// RustEnumType 은 gpa.map 의 Rust enum newtype(pub struct Status(pub i32)) 이름이다. CamelCase(map name) 이고,
// 생성 코드의 타입 이름이나 taken(같은 모듈의 모델 struct 이름 등)과 겹치면 Enum 접미사를 붙인다 (value → ValueEnum).
func RustEnumType(mapName string, taken ...string) string {
	return RustTypeName(CamelCase(mapName), "Enum", taken...)
}

// RustModule 은 테이블의 Rust 모듈 / 파일 이름이다 (FileName 과 같은 소문자: user_role_tb → userrole).
// 예약어나 crate 최상위 모듈 이름(mod, support, handlers, models, routes, custom ...)과 겹치면 뒤에 _ 를 붙인다 (type_tb → type_).
func RustModule(tableName string) string {
	name := RustIdent(strings.ToLower(sanitizeIdent(FileName(tableName), "m")))
	if rustReservedModules[name] {
		return name + "_"
	}
	return name
}
