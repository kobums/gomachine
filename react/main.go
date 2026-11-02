package react

import (
	"bytes"
	"database/sql"
	"fmt"
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
	TypeString string // TypeScript union type string
}

type ReactEnumItem struct {
	Index int
	Code  int    // 코드 (util.EnumCode: data 위치, Go/Kotlin/Dart 와 같다)
	Key   string // TypeScript 객체 key 로 쓸 수 있게 바꾼 이름 (대문자). 빈 항목은 ""
	Label string
	Quote string // Label 의 문자열 리터럴 (따옴표 포함)
}

type ReactSearchMethod struct {
	MethodName    string
	ParamName     string
	ParamType     string
	FieldName     string
	TableName     string
	ModelName     string
	TemplateParam string // ${paramName} for template literal
	IsSingle      bool
}

type ReactColumn struct {
	Name           string
	TypeScriptType string
	OriginalType   string
	IsNullable     bool
	IsPrimaryKey   bool
	IsOptional     bool
	IsDateType     bool
}

type ReactJoin struct {
	Name      string
	Column    string
	Alias     string
	ModelName string
}

type ReactTemplateData struct {
	PackageName     string
	TableName       string
	ModelName       string
	Columns         []ReactColumn
	Enums           []ReactEnumData
	HasStatus       bool
	SearchMethods   []ReactSearchMethod
	Joins           []ReactJoin
	UniqueJoinNames []string
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

	generateReactModel(targetPath, templateData)
	generateReactTypes(targetPath, templateData)
}

// BuildTemplateData 는 TypeScript 계열 생성기(react, vue)가 함께 쓰는 템플릿 데이터를 만든다.
func BuildTemplateData(packageName string, tableName string, items []util.Column, gpa *config.Gpa) ReactTemplateData {
	modelName := util.ModelName(tableName)
	// URL 과 파일 이름은 spring 의 /api/{모델 소문자} 와 같은 이름을 쓴다 (user_role_tb → userrole).
	fileName := util.FileName(tableName)

	// Get enum data from GPA
	var enums []ReactEnumData
	hasStatus := false
	if gpa != nil && gpa.Map != nil {
		for _, gpaMap := range gpa.Map {
			// Convert string data to ReactEnumItem with index
			enumItems := make([]ReactEnumItem, 0)
			indices := make([]string, 0)
			used := map[string]bool{}
			for idx, data := range gpaMap.Data {
				var key, label string
				if data == "" {
					key = ""
					label = ""
				} else {
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
				// Collect non-empty indices for type union
				if key != "" {
					indices = append(indices, strings.TrimSpace(strings.Split(fmt.Sprintf("%d", idx), ".")[0]))
				}
			}

			// Build TypeScript union type string
			typeString := strings.Join(indices, " | ")

			enums = append(enums, ReactEnumData{
				Name:       gpaMap.Name,
				Data:       enumItems,
				TypeString: typeString,
			})
			// Check if there's a status enum
			if strings.ToLower(gpaMap.Name) == "status" {
				hasStatus = true
			}
		}
	} else {
		enums = make([]ReactEnumData, 0)
	}

	// Generate search methods from GPA methods
	searchMethods := make([]ReactSearchMethod, 0)
	if gpa != nil && gpa.Method != nil {
		for _, method := range gpa.Method {
			// Parse method names like "GetByLoginid", "FindByName", "FindByRole", etc.
			methodLower := strings.ToLower(method)

			// Handle GetBy methods (single result)
			if strings.HasPrefix(methodLower, "getby") && len(method) > 5 {
				fieldName := method[5:] // Remove "GetBy"
				paramName := strings.ToLower(fieldName[:1]) + fieldName[1:]
				searchMethods = append(searchMethods, ReactSearchMethod{
					MethodName:    "searchBy" + fieldName,
					ParamName:     paramName,
					ParamType:     getTypeScriptType(fieldName, items, enums),
					FieldName:     fieldName,
					TableName:     fileName,
					ModelName:     modelName,
					TemplateParam: fmt.Sprintf("${%s}", paramName),
					IsSingle:      true,
				})
			}

			// Handle FindBy methods (array result)
			if strings.HasPrefix(methodLower, "findby") && len(method) > 6 {
				fieldName := method[6:] // Remove "FindBy"
				paramName := strings.ToLower(fieldName[:1]) + fieldName[1:]
				searchMethods = append(searchMethods, ReactSearchMethod{
					MethodName:    "searchBy" + fieldName,
					ParamName:     paramName,
					ParamType:     getTypeScriptType(fieldName, items, enums),
					FieldName:     fieldName,
					TableName:     fileName,
					ModelName:     modelName,
					TemplateParam: fmt.Sprintf("${%s}", paramName),
					IsSingle:      false,
				})
			}
		}
	}

	// Convert util.Column to ReactColumn with TypeScript types
	reactColumns := make([]ReactColumn, 0)
	for _, col := range items {
		originalTypeLower := strings.ToLower(col.OriginalType)
		isDateType := originalTypeLower == "date" || originalTypeLower == "datetime" || originalTypeLower == "timestamp"

		reactColumns = append(reactColumns, ReactColumn{
			Name:           col.Name,
			TypeScriptType: mapMySQLTypeToTypeScript(col.OriginalType),
			OriginalType:   originalTypeLower,
			IsNullable:     col.Nullable && !col.Primary,
			IsPrimaryKey:   col.Primary,
			IsOptional:     col.Primary, // ID는 생성 시 선택적
			IsDateType:     isDateType,
		})
	}

	// Process joins
	var joins []ReactJoin
	if gpa != nil && gpa.Join != nil {
		for _, join := range gpa.Join {
			alias := join.Alias
			if alias == "" {
				alias = join.Name
			}

			joins = append(joins, ReactJoin{
				Name:      join.Name,
				Column:    join.Column,
				Alias:     alias,
				ModelName: util.CamelCase(join.Name),
			})
		}
	} else {
		joins = make([]ReactJoin, 0)
	}

	// Extract unique join model names for imports
	uniqueJoinNamesMap := make(map[string]bool)
	var uniqueJoinNames []string
	for _, join := range joins {
		if !uniqueJoinNamesMap[join.Name] {
			uniqueJoinNamesMap[join.Name] = true
			uniqueJoinNames = append(uniqueJoinNames, join.Name)
		}
	}

	templateData := ReactTemplateData{
		PackageName:     packageName,
		TableName:       fileName,
		ModelName:       modelName,
		Columns:         reactColumns,
		Enums:           enums,
		HasStatus:       hasStatus,
		SearchMethods:   searchMethods,
		Joins:           joins,
		UniqueJoinNames: uniqueJoinNames,
	}

	return templateData
}

func mapMySQLTypeToTypeScript(mysqlType string) string {
	// Normalize to lowercase for comparison
	mysqlType = strings.ToLower(mysqlType)

	switch mysqlType {
	case "int", "integer", "bigint", "tinyint", "smallint", "mediumint", "year":
		return "number"
	case "float", "double", "real", "decimal", "numeric":
		return "number"
	case "varchar", "char", "tinytext", "text", "mediumtext", "longtext", "enum", "set":
		return "string"
	case "datetime", "date", "timestamp", "time":
		return "string"
	case "boolean", "bool", "bit":
		// spring 은 bit 를 Boolean 으로 매핑한다
		return "boolean"
	case "json":
		return "any"
	default:
		// blob / binary 는 JSON 에서 base64 문자열이다
		return "string"
	}
}

func getTypeScriptType(fieldName string, columns []util.Column, enums []ReactEnumData) string {
	// Check if it's a column first (don't use enum type names)
	fieldNameLower := strings.ToLower(fieldName)
	for _, col := range columns {
		if strings.ToLower(col.Name) == fieldNameLower {
			// Map MySQL original types to TypeScript types
			return mapMySQLTypeToTypeScript(col.OriginalType)
		}
	}

	// Default to string
	return "string"
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

func generateReactModel(targetPath string, data ReactTemplateData) {
	// Create models directory
	modelsDir := filepath.Join(targetPath, "src", "models")
	err := os.MkdirAll(modelsDir, 0755)
	if err != nil {
		log.Printf("Failed to create models directory: %v", err)
		return
	}

	// Load template from ~/bin/buildtool
	templateDir := filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "react")
	views := jet.NewSet(
		jet.NewOSFileSystemLoader(templateDir),
		jet.InDevelopmentMode(),
	)

	SetupJetGlobals(views)

	template, err := views.GetTemplate("model.jet")
	if err != nil {
		log.Printf("Failed to load React model template: %v", err)
		return
	}

	// Generate file
	var b bytes.Buffer
	if err = template.Execute(&b, nil, data); err != nil {
		log.Printf("Failed to execute React model template: %v", err)
		return
	}

	outputPath := filepath.Join(modelsDir, strings.ToLower(data.TableName)+".ts")
	if err := util.WriteFile(outputPath, b.String()); err != nil {
		log.Printf("ERROR: Failed to write React model %s: %v", outputPath, err)
	} else {
		log.Printf("Generated React: %s", outputPath)
	}
}

func generateReactTypes(targetPath string, data ReactTemplateData) {
	// Create types directory
	typesDir := filepath.Join(targetPath, "src", "types")
	err := os.MkdirAll(typesDir, 0755)
	if err != nil {
		log.Printf("Failed to create types directory: %v", err)
		return
	}

	// Load template from ~/bin/buildtool
	templateDir := filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "react")
	views := jet.NewSet(
		jet.NewOSFileSystemLoader(templateDir),
		jet.InDevelopmentMode(),
	)

	SetupJetGlobals(views)

	template, err := views.GetTemplate("type.jet")
	if err != nil {
		log.Printf("Failed to load React types template: %v", err)
		return
	}

	// Generate file
	var b bytes.Buffer
	if err = template.Execute(&b, nil, data); err != nil {
		log.Printf("Failed to execute React types template: %v", err)
		return
	}

	outputPath := filepath.Join(typesDir, strings.ToLower(data.TableName)+".ts")
	if err := util.WriteFile(outputPath, b.String()); err != nil {
		log.Printf("ERROR: Failed to write React types %s: %v", outputPath, err)
	} else {
		log.Printf("Generated React: %s", outputPath)
	}
}
