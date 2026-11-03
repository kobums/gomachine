package react

import (
	"bytes"
	"database/sql"
	"gomachine/config"
	"gomachine/util"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/CloudyKit/jet/v6"
	log "github.com/sirupsen/logrus"
)

type ReactEnumData struct {
	Name       string
	Data       []ReactEnumItem
	TypeString string // 코드 union (예: "0 | 1 | 2"). data[0] 이 빈 항목이면 0(없음)도 들어간다.
}

type ReactEnumItem struct {
	Index int
	Code  int    // 코드 (util.EnumCode: data 위치, Go/Kotlin/Dart 와 같다)
	Key   string // TypeScript 객체 key 로 쓸 수 있게 바꾼 이름 (대문자). 빈 항목은 ""
	Label string
	Quote string // Label 의 문자열 리터럴 (따옴표 포함)
}

// ReactSearchMethod 는 커스텀 GetBy/FindBy 메서드로 만드는 GET /{table}/search/{field} 호출이다.
// 조건이 컬럼 하나의 같음(=)인 메서드만 만든다 (Go / Spring 모두 같은 경로를 제공한다).
type ReactSearchMethod struct {
	MethodName string
	ParamName  string // 함수 매개변수 이름
	ParamType  string
	Key        string // 쿼리 파라미터 이름 (JSON key)
	Segment    string // 경로 /search/{Segment}
	TableName  string
	ModelName  string
	IsSingle   bool
}

type ReactColumn struct {
	Name           string
	Key            string // JSON key (util.JSONKey): 응답/요청 필드, 목록 필터 이름
	StartParam     string // 날짜 범위 필터 (util.RangeParams)
	EndParam       string
	TypeScriptType string
	OriginalType   string
	IsNullable     bool
	IsPrimaryKey   bool
	IsOptional     bool
	IsDateType     bool
	IsSearchable   bool // 목록 필터에 쓸 수 있다 (id, gpa.primary, 비밀번호, blob 제외: Go / Spring 과 같다)
	IsPassword     bool // 응답에 없다 (Go json:"-", Spring Response 제외). 요청에만 쓴다.
}

type ReactJoin struct {
	Name      string
	Column    string
	Alias     string
	ModelName string
	FileName  string // 조인 모델 파일 이름 (user_role → userrole)
}

type ReactTemplateData struct {
	PackageName   string
	TableName     string
	ModelName     string
	Columns       []ReactColumn
	Enums         []ReactEnumData
	HasStatus     bool
	SearchMethods []ReactSearchMethod
	Joins         []ReactJoin
	UniqueJoins   []ReactJoin
	PK            ReactColumn
	// HasId 는 id 컬럼이 있는지다. 없으면 Go 에 id 경로가 없으므로 get/update/patch/remove 를 만들지 않는다.
	HasId bool
	// IsView 는 view(_vw)인지다. view 는 읽기 전용이라 쓰기 메서드를 만들지 않는다.
	IsView bool
	// CreateOmit 은 생성 요청 타입(Create{Model}Request)에서 빼는 필드(JSON key)다: 서버가 정하는 세션 컬럼
	// (auth=jwt, User 제외), 보호 컬럼(util.ProtectedColumns), createddate. 기본 키는 따로 선택으로 둔다.
	CreateOmit []string
	// StrictShape 는 엔티티에 item 또는 code 필드가 있어 Go 래퍼({item, code})와 헷갈릴 수 있을 때 true 다.
	// 그러면 기본 키 필드가 있는 응답을 엔티티로 본다 (contract.ts 참고).
	StrictShape bool
}

func ProcessReact(packageName string, tableName string, prefix string, items []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	// 출력 경로는 reactModelFilePath 만 쓴다 (main.go 가 경로 없는 언어는 건너뛴다).
	targetPath := cnf.ReactModelFilePath
	if targetPath == "" {
		log.Println("reactModelFilePath is not set, skip react")
		return
	}

	templateData := BuildTemplateDataAuth(packageName, tableName, items, gpa, auth)
	name := strings.ToLower(templateData.TableName)

	views := newViews(filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "react"))
	out := OutDir(targetPath, cnf)
	semi := cnf.UseTsSemicolons()
	Render(views, "contract.jet", filepath.Join(out, "models"), "contract.ts", templateData, semi)
	Render(views, "model.jet", filepath.Join(out, "models"), name+".ts", templateData, semi)
	Render(views, "type.jet", filepath.Join(out, "types"), name+".ts", templateData, semi)
	Render(views, "hook.jet", filepath.Join(out, "hooks"), "use"+templateData.ModelName+".ts", templateData, semi)
	RenderAPI(views, out, semi)
}

// OutDir 는 TypeScript 생성 파일을 둘 디렉터리다 (출력 경로 / tsOutDir, 기본 src).
func OutDir(targetPath string, cnf config.ModelConfig) string {
	return filepath.Join(targetPath, filepath.FromSlash(cnf.GetTsOutDir()))
}

// BuildTemplateData 는 인증 없는(auth "") BuildTemplateDataAuth 다.
func BuildTemplateData(packageName string, tableName string, items []util.Column, gpa *config.Gpa) ReactTemplateData {
	return BuildTemplateDataAuth(packageName, tableName, items, gpa, "")
}

// BuildTemplateDataAuth 는 TypeScript 계열 생성기(react, vue)가 함께 쓰는 템플릿 데이터를 만든다.
// auth 는 생성 요청 타입에서 세션 컬럼을 뺄지 정한다 (jwt 면 백엔드가 세션 값으로 채운다).
func BuildTemplateDataAuth(packageName string, tableName string, items []util.Column, gpa *config.Gpa, auth string) ReactTemplateData {
	// _ / 대소문자만 다른 컬럼은 Go 와 같은 규칙으로 뒤 컬럼 이름에 번호를 붙인다.
	items = util.DedupeColumns(tableName, items)
	modelName := util.ModelName(tableName)
	// URL 과 파일 이름은 Go / Spring 과 같은 이름을 쓴다 (user_role_tb → userrole).
	fileName := util.FileName(tableName)

	enums := make([]ReactEnumData, 0)
	hasStatus := false
	if gpa != nil {
		for _, gpaMap := range gpa.Map {
			enumItems := make([]ReactEnumItem, 0)
			codes := make([]string, 0)
			used := map[string]bool{}
			for idx, data := range gpaMap.Data {
				var key, label string
				if data != "" {
					key = util.DedupeIdent(util.SanitizeIdent(strings.ToUpper(util.EnumKey(data)), "V"), used)
					label = util.EnumLabel(data)
				}
				enumItems = append(enumItems, ReactEnumItem{
					Index: idx,
					Code:  util.EnumCode(idx),
					Key:   key,
					Label: label,
					Quote: strconv.Quote(label),
				})
				if key != "" || idx == 0 {
					codes = append(codes, strconv.Itoa(util.EnumCode(idx)))
				}
			}

			enums = append(enums, ReactEnumData{
				Name:       gpaMap.Name,
				Data:       enumItems,
				TypeString: strings.Join(codes, " | "),
			})
			if strings.ToLower(gpaMap.Name) == "status" {
				hasStatus = true
			}
		}
	}

	pkIndex := util.PrimaryKeyIndex(items, gpa)
	reactColumns := make([]ReactColumn, 0)
	strict := false
	for i, col := range items {
		originalTypeLower := strings.ToLower(col.OriginalType)
		isPassword := util.IsPasswordColumn(col.Name)
		tsType := mapMySQLTypeToTypeScript(col.OriginalType)
		if m := util.FindEnumMap(gpa, col.Key); m != nil {
			for _, e := range enums {
				if e.Name == m.Name && e.TypeString != "" {
					tsType = e.TypeString
				}
			}
		}
		start, end := util.RangeParams(col)
		key := util.JSONKey(col)
		if key == "item" || key == "code" {
			strict = true
		}

		reactColumns = append(reactColumns, ReactColumn{
			Name:           col.Name,
			Key:            key,
			StartParam:     start,
			EndParam:       end,
			TypeScriptType: tsType,
			OriginalType:   originalTypeLower,
			IsNullable:     col.Nullable && i != pkIndex,
			IsPrimaryKey:   i == pkIndex,
			// 기본 키는 응답에 항상 있으므로 엔티티에서 필수다 (생성 요청에서만 선택, type.jet).
			IsOptional:   isPassword,
			IsDateType:   util.IsRangeFilter(originalTypeLower),
			IsSearchable: col.Name != "Id" && !util.IsConfigPrimary(col, gpa) && !isPassword && !isBinaryType(originalTypeLower),
			IsPassword:   isPassword,
		})
	}
	var pk ReactColumn
	if len(reactColumns) > 0 {
		pk = reactColumns[pkIndex]
	}

	// 검색 메서드: 생성기 공통 해석기(util.ParseFuncs)로 Go 와 같게 해석한다.
	searchMethods := make([]ReactSearchMethod, 0)
	if gpa != nil {
		usedNames := map[string]bool{}
		for _, fn := range util.ParseFuncs(tableName, gpa.Method, items) {
			if fn.Kind != "Get" && fn.Kind != "Find" {
				continue
			}
			if len(fn.Wheres) != 1 || fn.Wheres[0].Compare != "=" {
				log.Printf("INFO: %s %s has no /search route (multi-column, in or like), skipped in TypeScript", tableName, fn.Name)
				continue
			}
			var col *ReactColumn
			for i := range reactColumns {
				if reactColumns[i].Name == fn.Wheres[0].Column {
					col = &reactColumns[i]
				}
			}
			if col == nil || !col.IsSearchable {
				log.Printf("INFO: %s %s searches a column without /search route, skipped in TypeScript", tableName, fn.Name)
				continue
			}
			name := "searchBy" + fn.Name[len(fn.Kind)+2:]
			if usedNames[name] {
				log.Printf("WARNING: %s %s duplicates %s in TypeScript, skipped", tableName, fn.Name, name)
				continue
			}
			usedNames[name] = true
			param := util.SanitizeIdent(col.Key, "v")
			if tsReserved[param] {
				param += "Value"
			}
			searchMethods = append(searchMethods, ReactSearchMethod{
				MethodName: name,
				ParamName:  param,
				ParamType:  col.TypeScriptType,
				Key:        col.Key,
				Segment:    strings.ToLower(col.Key),
				TableName:  fileName,
				ModelName:  modelName,
				IsSingle:   fn.Kind == "Get",
			})
		}
	}

	joins := make([]ReactJoin, 0)
	uniqueJoins := make([]ReactJoin, 0)
	seenJoin := map[string]bool{}
	if gpa != nil {
		// extra key 는 Go 와 같다 (alias 또는 name 소문자, 겹치면 번호).
		// 백엔드와 같이 테이블 / 컬럼을 찾지 못한 조인은 건너뛴다.
		for _, rj := range util.ResolveJoins(tableName, items, gpa.Join) {
			join := rj.GpaJoin
			j := ReactJoin{
				Name:      join.Name,
				Column:    join.Column,
				Alias:     rj.Var,
				ModelName: util.ModelName(rj.Table),
				FileName:  util.FileName(rj.Table),
			}
			joins = append(joins, j)
			if !seenJoin[j.ModelName] {
				seenJoin[j.ModelName] = true
				uniqueJoins = append(uniqueJoins, j)
			}
		}
	}

	hasId := false
	for _, col := range items {
		if col.Name == "Id" {
			hasId = true
		}
	}

	// 생성 요청에서 뺄 필드: 백엔드가 Insert 때 덮어쓰는 컬럼 (Go / Rust / Spring 과 같다).
	omit := map[string]bool{}
	if auth == "jwt" && modelName != "User" && gpa != nil {
		for _, pair := range gpa.Session {
			if c := util.FindColumn(items, pair.Column); c != nil {
				omit[c.Name] = true
			}
		}
	}
	for _, name := range util.ProtectedColumns(modelName, items, gpa) {
		omit[name] = true
	}
	createOmit := make([]string, 0)
	for _, c := range reactColumns {
		if !c.IsPrimaryKey && (omit[c.Name] || c.Name == "Createddate") {
			createOmit = append(createOmit, c.Key)
		}
	}

	return ReactTemplateData{
		CreateOmit:    createOmit,
		HasId:         hasId,
		IsView:        util.GetTableType(tableName) == "view",
		PackageName:   packageName,
		TableName:     fileName,
		ModelName:     modelName,
		Columns:       reactColumns,
		Enums:         enums,
		HasStatus:     hasStatus,
		SearchMethods: searchMethods,
		Joins:         joins,
		UniqueJoins:   uniqueJoins,
		PK:            pk,
		StrictShape:   strict,
	}
}

// tsReserved 는 매개변수 이름으로 쓸 수 없는 TypeScript/JavaScript 예약어다.
var tsReserved = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true, "continue": true, "debugger": true, "default": true,
	"delete": true, "do": true, "else": true, "enum": true, "export": true, "extends": true, "false": true, "finally": true,
	"for": true, "function": true, "if": true, "import": true, "in": true, "instanceof": true, "new": true, "null": true,
	"return": true, "super": true, "switch": true, "this": true, "throw": true, "true": true, "try": true, "typeof": true,
	"var": true, "void": true, "while": true, "with": true, "yield": true, "let": true, "static": true, "implements": true,
	"interface": true, "package": true, "private": true, "protected": true, "public": true, "await": true, "params": true,
}

func isBinaryType(t string) bool {
	switch t {
	case "binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob":
		return true
	}
	return false
}

func mapMySQLTypeToTypeScript(mysqlType string) string {
	// Normalize to lowercase for comparison
	mysqlType = strings.ToLower(mysqlType)

	switch mysqlType {
	case "int", "integer", "bigint", "smallint", "mediumint", "year":
		return "number"
	case "tinyint", "bit":
		// Go / Spring 모두 bool(true/false) 로 주고받는다.
		return "boolean"
	case "float", "double", "real", "decimal", "numeric":
		return "number"
	case "varchar", "char", "tinytext", "text", "mediumtext", "longtext", "enum", "set":
		return "string"
	case "datetime", "date", "timestamp", "time":
		return "string"
	case "boolean", "bool":
		return "boolean"
	case "json":
		// Go / Spring 모두 JSON 문자열로 주고받는다.
		return "string"
	default:
		// blob / binary 는 JSON 에서 base64 문자열이다
		return "string"
	}
}

// SetupJetGlobals 는 TypeScript 템플릿에서 쓰는 헬퍼 함수를 등록한다.
func SetupJetGlobals(views *jet.Set) {
	views.AddGlobal("title", func(str string) string {
		if str == "" {
			return ""
		}
		return strings.Title(str)
	})

	views.AddGlobal("lower", func(str string) string {
		return strings.ToLower(str)
	})

	views.AddGlobal("upper", func(str string) string {
		return strings.ToUpper(str)
	})

	views.AddGlobal("untitle", func(str string) string {
		if str == "" {
			return ""
		}
		a := []rune(str)
		a[0] = unicode.ToLower(a[0])
		return string(a)
	})

	views.AddGlobal("first", func(str string) string {
		if str == "" {
			return ""
		}
		ret := strings.Split(str, ":")
		return ret[0]
	})

	views.AddGlobal("last", func(str string) string {
		if str == "" {
			return ""
		}
		ret := strings.Split(str, ":")
		if len(ret) > 1 {
			return ret[1]
		}
		return str
	})

	views.AddGlobal("ne", func(a, b string) bool {
		return a != b
	})

	views.AddGlobal("eq", func(a, b string) bool {
		return a == b
	})

	views.AddGlobal("or", func(a, b bool) bool {
		return a || b
	})
}

// newViews 는 dir 의 템플릿과 헬퍼 함수를 준비한다.
func newViews(dir string) *jet.Set {
	views := jet.NewSet(jet.NewOSFileSystemLoader(dir), jet.InDevelopmentMode())
	SetupJetGlobals(views)
	return views
}

// Render 는 템플릿 하나를 dir/filename 으로 쓴다 (react, vue 가 함께 쓴다).
// 생성 파일이라 머리줄(util.GeneratedHeader)이 없는 기존 파일은 덮어쓰지 않는다. semi 가 false 면 문장 끝 ; 를 뺀다.
func Render(views *jet.Set, templateName string, dir string, filename string, data ReactTemplateData, semi bool) {
	content, ok := execute(views, templateName, data, semi)
	if !ok {
		return
	}
	outputPath := filepath.Join(dir, filename)
	// 내용이 같아 쓰지 않았거나 사용자 파일이라 건너뛰면 Generated 로그를 찍지 않는다.
	if written, err := util.WriteGenerated(outputPath, content); err != nil {
		log.Printf("ERROR: Failed to write %s: %v", outputPath, err)
	} else if written {
		log.Printf("Generated TypeScript: %s", outputPath)
	}
}

// RenderAPI 는 생성 모델이 쓰는 {out}/services/api.ts 를 없을 때만 만든다 (사용자 파일, 이후 덮어쓰지 않는다).
func RenderAPI(views *jet.Set, out string, semi bool) {
	content, ok := execute(views, "api.jet", ReactTemplateData{}, semi)
	if !ok {
		return
	}
	outputPath := filepath.Join(out, "services", "api.ts")
	written, err := util.WriteIfMissing(outputPath, content)
	if err != nil {
		log.Printf("ERROR: Failed to write %s: %v", outputPath, err)
	} else if written {
		log.Printf("Generated TypeScript: %s", outputPath)
	}
}

// execute 는 템플릿을 실행한 내용을 돌려준다. 실패하면 false (반쯤 만든 파일을 쓰지 않는다).
func execute(views *jet.Set, templateName string, data ReactTemplateData, semi bool) (string, bool) {
	t, err := views.GetTemplate(templateName)
	if err != nil {
		log.Printf("Failed to load TypeScript template %s: %v", templateName, err)
		return "", false
	}
	var b bytes.Buffer
	if err := t.Execute(&b, nil, data); err != nil {
		log.Printf("Failed to execute TypeScript template %s: %v", templateName, err)
		return "", false
	}
	out := Tidy(b.String())
	if !semi {
		return StripSemicolons(out), true
	}
	return out, true
}

// Tidy 는 템플릿 출력을 prettier 모양에 가깝게 다듬는다: 줄 끝 공백을 지우고, 빈 줄은 하나로 줄이고,
// { 바로 뒤 / } 바로 앞의 빈 줄과 파일 끝 빈 줄을 뺀다 (파일은 줄바꿈 하나로 끝난다).
func Tidy(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			if len(out) == 0 || out[len(out)-1] == "" || strings.HasSuffix(out[len(out)-1], "{") {
				continue
			}
		} else if t := strings.TrimSpace(line); (strings.HasPrefix(t, "}") || strings.HasPrefix(t, "]") || strings.HasPrefix(t, ")")) && len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
		out = append(out, line)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n") + "\n"
}

// StripSemicolons 는 줄 끝의 문장 끝 ; 를 뺀다 (prettier semi: false 와 같은 모양).
// 주석 줄은 그대로 두고, 다음 줄이 ( [ ` 로 시작하면 ASI 가 이어 붙이므로 ; 를 남긴다.
func StripSemicolons(src string) string {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasSuffix(trimmed, ";") || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
			continue
		}
		next := ""
		for _, l := range lines[i+1:] {
			if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "//") && !strings.HasPrefix(t, "/*") && !strings.HasPrefix(t, "*") {
				next = t
				break
			}
		}
		if next != "" && strings.ContainsAny(next[:1], "([`+-/") {
			continue
		}
		end := strings.TrimRight(line, " \t\r")
		lines[i] = strings.TrimRight(strings.TrimSuffix(end, ";"), " \t")
	}
	return strings.Join(lines, "\n")
}
