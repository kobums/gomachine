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
	IsSearchable   bool // 목록 필터에 쓸 수 있다 (기본 키, 비밀번호, blob 제외: Go / Spring 과 같다)
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

	// Make sure the target directory exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		err := os.MkdirAll(targetPath, 0755)
		if err != nil {
			log.Printf("Failed to create target directory: %v", err)
		}
	}

	templateData := BuildTemplateData(packageName, tableName, items, gpa)

	views := newViews(filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "react"))
	Render(views, "contract.jet", filepath.Join(targetPath, "src", "models"), "contract.ts", templateData)
	Render(views, "model.jet", filepath.Join(targetPath, "src", "models"), strings.ToLower(templateData.TableName)+".ts", templateData)
	Render(views, "type.jet", filepath.Join(targetPath, "src", "types"), strings.ToLower(templateData.TableName)+".ts", templateData)
}

// BuildTemplateData 는 TypeScript 계열 생성기(react, vue)가 함께 쓰는 템플릿 데이터를 만든다.
func BuildTemplateData(packageName string, tableName string, items []util.Column, gpa *config.Gpa) ReactTemplateData {
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
			IsOptional:     i == pkIndex || isPassword,
			IsDateType:     util.IsRangeFilter(originalTypeLower),
			IsSearchable:   i != pkIndex && col.Name != "Id" && !isPassword && !isBinaryType(originalTypeLower),
			IsPassword:     isPassword,
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
		for _, join := range gpa.Join {
			alias := join.Alias
			if alias == "" {
				alias = join.Name
			}
			j := ReactJoin{
				Name:      join.Name,
				Column:    join.Column,
				Alias:     alias,
				ModelName: util.ModelName(join.Name),
				FileName:  util.FileName(join.Name),
			}
			joins = append(joins, j)
			if !seenJoin[j.ModelName] {
				seenJoin[j.ModelName] = true
				uniqueJoins = append(uniqueJoins, j)
			}
		}
	}

	return ReactTemplateData{
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
	case "tinyint":
		// Go 는 bool(true/false), Spring 은 Int(0/1) 로 주고받는다.
		return "boolean | number"
	case "float", "double", "real", "decimal", "numeric":
		return "number"
	case "varchar", "char", "tinytext", "text", "mediumtext", "longtext", "enum", "set":
		return "string"
	case "datetime", "date", "timestamp", "time":
		return "string"
	case "boolean", "bool":
		return "boolean"
	case "bit":
		// Spring 은 Boolean, Go 는 []byte (JSON 에서 base64 문자열) 다.
		return "boolean | string"
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
func Render(views *jet.Set, templateName string, dir string, filename string, data ReactTemplateData) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Failed to create directory %s: %v", dir, err)
		return
	}
	t, err := views.GetTemplate(templateName)
	if err != nil {
		log.Printf("Failed to load TypeScript template %s: %v", templateName, err)
		return
	}
	var b bytes.Buffer
	if err := t.Execute(&b, nil, data); err != nil {
		log.Printf("Failed to execute TypeScript template %s: %v", templateName, err)
		return
	}
	outputPath := filepath.Join(dir, filename)
	if err := util.WriteFile(outputPath, b.String()); err != nil {
		log.Printf("ERROR: Failed to write %s: %v", outputPath, err)
	} else {
		log.Printf("Generated TypeScript: %s", outputPath)
	}
}
