package react

import (
	"bytes"
	"database/sql"
	"fmt"
	"gomachine/config"
	"gomachine/util"
	"os"
	"path/filepath"
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
	Key   string
	Label string
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
	IsNullable     bool
	IsPrimaryKey   bool
	IsOptional     bool
}

type ReactJoinType struct {
	Name      string
	FieldName string
	Columns   []ReactColumn
}

type ReactTemplateData struct {
	PackageName   string
	TableName     string
	ModelName     string
	Columns       []ReactColumn
	Enums         []ReactEnumData
	HasStatus     bool
	SearchMethods []ReactSearchMethod
	HasJoins      bool
	JoinTypes     []ReactJoinType
}

func ProcessReact(packageName string, tableName string, prefix string, items []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	modelName := strings.Title(util.GetTableName(tableName))

	// Determine target path
	var targetPath string
	if len(os.Args) > 1 && os.Args[1] != "" {
		targetPath = os.Args[1]
	} else {
		targetPath = "../gym/front"
	}

	// Get enum data from GPA
	var enums []ReactEnumData
	hasStatus := false
	if gpa != nil && gpa.Map != nil {
		for _, gpaMap := range gpa.Map {
			// Convert string data to ReactEnumItem with index
			enumItems := make([]ReactEnumItem, 0)
			indices := make([]string, 0)
			for idx, data := range gpaMap.Data {
				var key, label string
				if data == "" {
					key = ""
					label = ""
				} else {
					parts := strings.Split(data, ":")
					if len(parts) >= 2 {
						key = parts[0]
						label = parts[1]
					} else {
						key = data
						label = data
					}
				}
				enumItems = append(enumItems, ReactEnumItem{
					Index: idx,
					Key:   key,
					Label: label,
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
			if strings.HasPrefix(methodLower, "getby") {
				fieldName := method[5:] // Remove "GetBy"
				paramName := strings.ToLower(fieldName[:1]) + fieldName[1:]
				searchMethods = append(searchMethods, ReactSearchMethod{
					MethodName:    "searchBy" + fieldName,
					ParamName:     paramName,
					ParamType:     getTypeScriptType(fieldName, items, enums),
					FieldName:     fieldName,
					TableName:     util.GetTableName(tableName),
					ModelName:     modelName,
					TemplateParam: fmt.Sprintf("${%s}", paramName),
					IsSingle:      true,
				})
			}

			// Handle FindBy methods (array result)
			if strings.HasPrefix(methodLower, "findby") {
				fieldName := method[6:] // Remove "FindBy"
				paramName := strings.ToLower(fieldName[:1]) + fieldName[1:]
				searchMethods = append(searchMethods, ReactSearchMethod{
					MethodName:    "searchBy" + fieldName,
					ParamName:     paramName,
					ParamType:     getTypeScriptType(fieldName, items, enums),
					FieldName:     fieldName,
					TableName:     util.GetTableName(tableName),
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
		reactColumns = append(reactColumns, ReactColumn{
			Name:           col.Name,
			TypeScriptType: mapMySQLTypeToTypeScript(col.OriginalType),
			IsNullable:     false, // 나중에 확장 가능
			IsPrimaryKey:   col.Primary,
			IsOptional:     col.Primary, // ID는 생성 시 선택적
		})
	}

	templateData := ReactTemplateData{
		PackageName:   packageName,
		TableName:     util.GetTableName(tableName),
		ModelName:     modelName,
		Columns:       reactColumns,
		Enums:         enums,
		HasStatus:     hasStatus,
		SearchMethods: searchMethods,
		HasJoins:      false,
		JoinTypes:     make([]ReactJoinType, 0),
	}

	generateReactModel(targetPath, templateData)
	generateReactTypes(targetPath, templateData)
}

func mapMySQLTypeToTypeScript(mysqlType string) string {
	// Normalize to lowercase for comparison
	mysqlType = strings.ToLower(mysqlType)

	switch mysqlType {
	case "int", "bigint", "tinyint", "smallint", "mediumint":
		return "number"
	case "float", "double", "decimal":
		return "number"
	case "varchar", "text", "longtext", "mediumtext", "char":
		return "string"
	case "datetime", "date", "timestamp", "time":
		return "string"
	case "boolean", "bool":
		return "boolean"
	case "json":
		return "any"
	default:
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

func setupJetGlobals(views *jet.Set) {
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

	setupJetGlobals(views)

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
		log.Printf("CRITICAL ERROR: Failed to write React model file %s: %v", outputPath, err)
	} else {
		log.Printf("SUCCESS: React model file written successfully: %s", outputPath)
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

	setupJetGlobals(views)

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
		log.Printf("CRITICAL ERROR: Failed to write React types file %s: %v", outputPath, err)
	} else {
		log.Printf("SUCCESS: React types file written successfully: %s", outputPath)
	}
}
