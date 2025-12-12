package kotlin

import (
	"database/sql"
	"gomachine/config"
	"gomachine/util"
	"os"
	"path/filepath"
	"strings"

	"github.com/CloudyKit/jet/v6"
	log "github.com/sirupsen/logrus"
)

type KotlinColumn struct {
	Name         string
	DBName       string
	KotlinType   string
	DBType       string // Original DB type (Int, String, etc.) for Response
	ModelName    string
	IsNullable   bool
	IsPrimaryKey bool
	HasDefault   bool
	DefaultValue string
	IsEnum       bool
	EnumType     string
}

type KotlinEnumData struct {
	Name        string
	Data        []string
	EnumPackage string
}

type KotlinJoin struct {
	Name            string
	Column          string
	ColumnName      string // camelCase column name for Kotlin
	Prefix          string
	Alias           string
	ModelName       string
	ParentModelName string // The main model name
	Columns         []KotlinColumn
	Index           int    // Index in the joins array
}

type EntityGraphPath struct {
	Path    string
	IsFirst bool
	IsLast  bool
}

type KotlinTemplateData struct {
	PackageName     string
	TableName       string
	ModelName       string
	Columns         []KotlinColumn
	HasDateTime     bool
	HasDecimal      bool
	Enums           []KotlinEnumData
	Joins           []KotlinJoin
	UniqueJoinNames []string // Unique join model names for imports
	EntityGraphPaths []EntityGraphPath // Nested entity graph paths for @EntityGraph with metadata
}

// Build a global map of all table joins for recursive entity graph path building
func buildGlobalJoinsMap(cnf config.ModelConfig) map[string][]config.GpaJoin {
	joinsMap := make(map[string][]config.GpaJoin)
	for _, gpa := range cnf.Gpa {
		if gpa.Join != nil && len(gpa.Join) > 0 {
			joinsMap[gpa.Name] = gpa.Join
		}
	}
	return joinsMap
}

// Recursively build entity graph paths
// Example: if current table has join to "gym", and "gym" has join to "owner",
// this will return ["gym", "gym.owner"]
func buildEntityGraphPaths(joins []KotlinJoin, globalJoinsMap map[string][]config.GpaJoin, visited map[string]bool) []EntityGraphPath {
	pathStrings := make([]string, 0)

	for _, join := range joins {
		alias := strings.ToLower(join.Alias)

		// Add the direct join path
		pathStrings = append(pathStrings, alias)

		// Check if this joined table has its own joins
		if nestedJoins, exists := globalJoinsMap[join.Name]; exists && len(nestedJoins) > 0 {
			// Prevent circular references
			if visited[join.Name] {
				continue
			}
			visited[join.Name] = true

			// Add nested join paths
			for _, nestedJoin := range nestedJoins {
				nestedAlias := nestedJoin.Alias
				if nestedAlias == "" {
					nestedAlias = nestedJoin.Name
				}
				nestedPath := alias + "." + strings.ToLower(nestedAlias)
				pathStrings = append(pathStrings, nestedPath)
			}

			delete(visited, join.Name)
		}
	}

	// Convert to EntityGraphPath with metadata
	paths := make([]EntityGraphPath, len(pathStrings))
	for i, pathStr := range pathStrings {
		paths[i] = EntityGraphPath{
			Path:    pathStr,
			IsFirst: i == 0,
			IsLast:  i == len(pathStrings)-1,
		}
	}

	return paths
}

func ProcessKotlin(packageName string, tableName string, prefix string, columns []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	modelName := strings.Title(util.GetTableName(tableName))

	// Build global joins map for recursive entity graph paths
	globalJoinsMap := buildGlobalJoinsMap(cnf)
	
	// Determine target path
	var targetPath string
	if cnf.KotlinModelFilePath != "" {
		targetPath = cnf.KotlinModelFilePath
	} else if len(os.Args) > 1 && os.Args[1] != "" {
		targetPath = os.Args[1]
	} else {
		targetPath = "../gym/gymspring"
	}

	// Make sure the target directory exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		err := os.MkdirAll(targetPath, 0755)
		if err != nil {
			log.Printf("Failed to create target directory: %v", err)
		}
	}

	// Build enum map for easy lookup
	enumMap := make(map[string][]string)
	if gpa != nil && gpa.Map != nil {
		for _, gpaMap := range gpa.Map {
			enumMap[strings.ToLower(gpaMap.Name)] = gpaMap.Data
		}
	}

	kotlinColumns := make([]KotlinColumn, 0)
	hasDateTime := false
	hasDecimal := false

	for i, col := range columns {
		// Check if this is primary key (usually first column with "id" in name)
		isPrimaryKey := i == 0 && strings.Contains(strings.ToLower(col.Column), "id")

		// Check for nullable and default values based on type and position
		isNullable := !isPrimaryKey && (col.Type == "LocalDateTime" || strings.Contains(col.Column, "_date"))
		hasDefault := false
		defaultValue := ""

		// Check if this column is an enum type
		colNameLower := strings.ToLower(col.Name)
		isEnumType := false
		enumTypeName := ""
		if enumData, isEnum := enumMap[colNameLower]; isEnum {
			isEnumType = true
			enumTypeName = strings.Title(col.Name)
			// Find first non-empty value in enum data
			for _, enumValue := range enumData {
				if enumValue != "" {
					parts := strings.Split(enumValue, ":")
					if len(parts) > 0 && parts[0] != "" {
						// Use the enum constant name with type prefix (e.g., "Type.NOTICE" from "notice:공지")
						hasDefault = true
						defaultValue = enumTypeName + "." + strings.ToUpper(parts[0])
						break
					}
				}
			}
		} else if col.Type == "LocalDateTime" {
			hasDateTime = true
			if !isPrimaryKey {
				hasDefault = true
				if isNullable {
					defaultValue = "LocalDateTime.now()"
				} else {
					defaultValue = "LocalDateTime.now()"
				}
			}
		} else if col.Type == "BigDecimal" {
			hasDecimal = true
			if !isPrimaryKey {
				hasDefault = true
				defaultValue = "BigDecimal.ZERO"
			}
		} else if col.Type == "String" && !isPrimaryKey {
			hasDefault = true
			defaultValue = `""`
		} else if col.Type == "Int" && !isPrimaryKey {
			hasDefault = true
			defaultValue = "0"
		} else if col.Type == "Long" && !isPrimaryKey {
			hasDefault = true
			defaultValue = "0L"
		} else if col.Type == "Double" && !isPrimaryKey {
			hasDefault = true
			defaultValue = "0.0"
		} else if col.Type == "Boolean" && !isPrimaryKey {
			hasDefault = true
			defaultValue = "false"
		}

		// Special handling for primary key
		if isPrimaryKey {
			hasDefault = true
			defaultValue = "0"
			isNullable = false
		}

		// Determine DBType for Response
		dbType := col.Type
		if isEnumType {
			dbType = "Int" // Enums are stored as Int (ordinal) in Response
		} else if col.Type == "LocalDateTime" {
			dbType = "String" // LocalDateTime is converted to String in Response
		}

		kotlinCol := KotlinColumn{
			Name:         col.Name,
			DBName:       col.Column,
			KotlinType:   col.Type,
			DBType:       dbType,
			ModelName:    modelName,
			IsNullable:   isNullable,
			IsPrimaryKey: isPrimaryKey,
			HasDefault:   hasDefault,
			DefaultValue: defaultValue,
			IsEnum:       isEnumType,
			EnumType:     enumTypeName,
		}
		kotlinColumns = append(kotlinColumns, kotlinCol)
	}

	// Get enum data from GPA and convert to KotlinEnumData
	var enums []KotlinEnumData
	if gpa != nil && gpa.Map != nil {
		enumPackage := strings.ToLower(modelName)
		for _, gpaMap := range gpa.Map {
			enums = append(enums, KotlinEnumData{
				Name:        gpaMap.Name,
				Data:        gpaMap.Data,
				EnumPackage: enumPackage,
			})
		}
	} else {
		enums = make([]KotlinEnumData, 0)
	}

	// Process joins
	var joins []KotlinJoin
	if gpa != nil && gpa.Join != nil {
		for joinIndex, join := range gpa.Join {
			joinTableName := strings.ToLower(join.Name) + "_tb"
			joinModelName := strings.Title(join.Name)

			// Get columns for joined table
			query := "select column_name as column_name, data_type as data_type from information_schema.columns where table_schema = '" + packageName + "' and table_name = '" + joinTableName + "'"
			rows, err := db.Query(query)
			if err != nil {
				log.Printf("Failed to query join table columns: %v", err)
				continue
			}

			joinColumns := make([]KotlinColumn, 0)
			for rows.Next() {
				var colName, colType string
				err := rows.Scan(&colName, &colType)
				if err != nil {
					continue
				}

				name := strings.Title(util.GetName(colName))
				kotlinType := util.GetType(joinModelName, name, colType, nil, cnf)

				// Simple processing for joined columns
				isPrimaryKey := strings.Contains(strings.ToLower(colName), "_id")
				isNullable := !isPrimaryKey

				dbType := kotlinType
				if kotlinType == "LocalDateTime" {
					dbType = "String"
				}

				joinColumns = append(joinColumns, KotlinColumn{
					Name:         name,
					DBName:       colName,
					KotlinType:   kotlinType,
					DBType:       dbType,
					ModelName:    joinModelName,
					IsNullable:   isNullable,
					IsPrimaryKey: isPrimaryKey,
				})
			}
			rows.Close()

			alias := join.Alias
			if alias == "" {
				alias = join.Name
			}

			// Convert column name to camelCase for Kotlin (e.g., author_id -> authorId)
			columnName := strings.Title(util.GetName(join.Column))
			columnName = strings.ToLower(columnName[0:1]) + columnName[1:] // make first letter lowercase

			// Build full column name with prefix (e.g., "gym" -> "m_gym")
			fullColumnName := join.Column
			if join.Prefix != "" {
				fullColumnName = join.Prefix + "_" + join.Column
			} else {
				// Use table prefix if join prefix not specified
				fullColumnName = prefix + "_" + join.Column
			}

			kotlinJoin := KotlinJoin{
				Name:            join.Name,
				Column:          fullColumnName,
				ColumnName:      columnName,
				Prefix:          join.Prefix,
				Alias:           alias,
				ModelName:       joinModelName,
				ParentModelName: modelName,
				Columns:         joinColumns,
				Index:           joinIndex,
			}
			joins = append(joins, kotlinJoin)
		}
	} else {
		joins = make([]KotlinJoin, 0)
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

	// Build entity graph paths with nested joins
	visited := make(map[string]bool)
	entityGraphPaths := buildEntityGraphPaths(joins, globalJoinsMap, visited)

	templateData := KotlinTemplateData{
		PackageName:      packageName,
		TableName:        tableName,
		ModelName:        modelName,
		Columns:          kotlinColumns,
		HasDateTime:      hasDateTime,
		HasDecimal:       hasDecimal,
		Enums:            enums,
		Joins:            joins,
		UniqueJoinNames:  uniqueJoinNames,
		EntityGraphPaths: entityGraphPaths,
	}

	// Generate entity file
	generateKotlinEntity(targetPath, templateData)

	// Generate repository file
	generateKotlinRepository(targetPath, templateData)

	// Generate service file
	generateKotlinService(targetPath, templateData)

	// Generate controller file
	generateKotlinController(targetPath, templateData)

	// Generate enums file if there are enums
	if len(enums) > 0 {
		generateKotlinEnums(targetPath, templateData)
	}
}

func generateKotlinEntity(targetPath string, data KotlinTemplateData) {
	// Create entity directory
	entityDir := filepath.Join(targetPath, "entity")
	err := os.MkdirAll(entityDir, 0755)
	if err != nil {
		log.Printf("Failed to create entity directory: %v", err)
		return
	}

	// Load template from ~/bin/buildtool
	templateDir := filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "kotlin")
	views := jet.NewSet(
		jet.NewOSFileSystemLoader(templateDir),
		jet.InDevelopmentMode(),
	)

	// Add custom functions for the template
	views.AddGlobal("title", func(str string) string {
		if str == "" {
			return ""
		}
		return strings.Title(str)
	})

	views.AddGlobal("lower", func(str string) string {
		return strings.ToLower(str)
	})

	// Add helper function to find join by column name
	views.AddGlobal("findJoinByColumn", func(columnDBName string, joins []KotlinJoin) *KotlinJoin {
		// log.Printf("Finding join for column: %s", columnDBName)
		for _, join := range joins {
			// log.Printf("  Checking join[%d]: Column=%s, Name=%s, Alias=%s", i, join.Column, join.Name, join.Alias)
			if join.Column == columnDBName {
				// log.Printf("  MATCH FOUND!")
				return &join
			}
		}
		// log.Printf("  No match found")
		return nil
	})

	template, err := views.GetTemplate("entity.jet")
	if err != nil {
		log.Printf("Failed to load template: %v", err)
		return
	}

	// Generate file
	outputPath := filepath.Join(entityDir, data.ModelName+".kt")
	file, err := os.Create(outputPath)
	if err != nil {
		log.Printf("Failed to create entity file: %v", err)
		return
	}
	defer file.Close()

	err = template.Execute(file, nil, data)
	if err != nil {
		log.Printf("Failed to execute template: %v", err)
		return
	}

	log.Printf("Generated Kotlin: %s", outputPath)
}

func generateKotlinRepository(targetPath string, data KotlinTemplateData) {
	// Create repository directory
	repositoryDir := filepath.Join(targetPath, "repository")
	err := os.MkdirAll(repositoryDir, 0755)
	if err != nil {
		log.Printf("Failed to create repository directory: %v", err)
		return
	}

	// Load template from ~/bin/buildtool
	templateDir := filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "kotlin")
	views := jet.NewSet(
		jet.NewOSFileSystemLoader(templateDir),
		jet.InDevelopmentMode(),
	)

	// Add custom functions for the template
	views.AddGlobal("title", func(str string) string {
		if str == "" {
			return ""
		}
		return strings.Title(str)
	})

	views.AddGlobal("lower", func(str string) string {
		return strings.ToLower(str)
	})

	// Add helper function to find join by column name
	views.AddGlobal("findJoinByColumn", func(columnDBName string, joins []KotlinJoin) *KotlinJoin {
		for _, join := range joins {
			if join.Column == columnDBName {
				return &join
			}
		}
		return nil
	})

	// Add helper function to check if this is the first join
	views.AddGlobal("isFirstJoin", func(join KotlinJoin) bool {
		return join.Index == 0
	})

	// Add helper function to check if index is last in array
	views.AddGlobal("isLast", func(index int, arr []string) bool {
		return index == len(arr)-1
	})

	template, err := views.GetTemplate("repository.jet")
	if err != nil {
		log.Printf("Failed to load repository template: %v", err)
		return
	}

	// Generate file
	outputPath := filepath.Join(repositoryDir, data.ModelName+"Repository.kt")
	file, err := os.Create(outputPath)
	if err != nil {
		log.Printf("Failed to create repository file: %v", err)
		return
	}
	defer file.Close()

	err = template.Execute(file, nil, data)
	if err != nil {
		log.Printf("Failed to execute repository template: %v", err)
		return
	}

	log.Printf("Generated Kotlin: %s", outputPath)
}

func generateKotlinService(targetPath string, data KotlinTemplateData) {
	// Create service directory
	serviceDir := filepath.Join(targetPath, "service")
	err := os.MkdirAll(serviceDir, 0755)
	if err != nil {
		log.Printf("Failed to create service directory: %v", err)
		return
	}

	// Load template from ~/bin/buildtool
	templateDir := filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "kotlin")
	views := jet.NewSet(
		jet.NewOSFileSystemLoader(templateDir),
		jet.InDevelopmentMode(),
	)

	// Add custom functions for the template
	views.AddGlobal("title", func(str string) string {
		if str == "" {
			return ""
		}
		return strings.Title(str)
	})

	views.AddGlobal("lower", func(str string) string {
		return strings.ToLower(str)
	})

	// Add helper function to find join by column name
	views.AddGlobal("findJoinByColumn", func(columnDBName string, joins []KotlinJoin) *KotlinJoin {
		for _, join := range joins {
			if join.Column == columnDBName {
				return &join
			}
		}
		return nil
	})

	template, err := views.GetTemplate("service.jet")
	if err != nil {
		log.Printf("Failed to load service template: %v", err)
		return
	}

	// Generate file
	outputPath := filepath.Join(serviceDir, data.ModelName+"Service.kt")
	file, err := os.Create(outputPath)
	if err != nil {
		log.Printf("Failed to create service file: %v", err)
		return
	}
	defer file.Close()

	err = template.Execute(file, nil, data)
	if err != nil {
		log.Printf("Failed to execute service template: %v", err)
		return
	}

	log.Printf("Generated Kotlin: %s", outputPath)
}

func generateKotlinController(targetPath string, data KotlinTemplateData) {
	// Create controller directory
	controllerDir := filepath.Join(targetPath, "controller")
	err := os.MkdirAll(controllerDir, 0755)
	if err != nil {
		log.Printf("Failed to create controller directory: %v", err)
		return
	}

	// Load template from ~/bin/buildtool
	templateDir := filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "kotlin")
	views := jet.NewSet(
		jet.NewOSFileSystemLoader(templateDir),
		jet.InDevelopmentMode(),
	)

	// Add custom functions for the template
	views.AddGlobal("title", func(str string) string {
		if str == "" {
			return ""
		}
		return strings.Title(str)
	})

	views.AddGlobal("lower", func(str string) string {
		return strings.ToLower(str)
	})

	// Add helper function to find join by column name
	views.AddGlobal("findJoinByColumn", func(columnDBName string, joins []KotlinJoin) *KotlinJoin {
		for _, join := range joins {
			if join.Column == columnDBName {
				return &join
			}
		}
		return nil
	})

	template, err := views.GetTemplate("controller.jet")
	if err != nil {
		log.Printf("Failed to load controller template: %v", err)
		return
	}

	// Generate file
	outputPath := filepath.Join(controllerDir, data.ModelName+"Controller.kt")
	file, err := os.Create(outputPath)
	if err != nil {
		log.Printf("Failed to create controller file: %v", err)
		return
	}
	defer file.Close()

	err = template.Execute(file, nil, data)
	if err != nil {
		log.Printf("Failed to execute controller template: %v", err)
		return
	}

	log.Printf("Generated Kotlin: %s", outputPath)
}

func generateKotlinEnums(targetPath string, data KotlinTemplateData) {
	// Create enums directory
	enumsDir := filepath.Join(targetPath, "enums", strings.ToLower(data.ModelName))
	err := os.MkdirAll(enumsDir, 0755)
	if err != nil {
		log.Printf("Failed to create enums directory: %v", err)
		return
	}

	// Load template from ~/bin/buildtool with custom functions
	templateDir := filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "kotlin")
	views := jet.NewSet(
		jet.NewOSFileSystemLoader(templateDir),
		jet.InDevelopmentMode(),
	)

	// Add custom functions for the template
	views.AddGlobal("first", func(str string) string {
		if str == "" {
			return ""
		}
		parts := strings.Split(str, ":")
		return parts[0]
	})

	views.AddGlobal("last", func(str string) string {
		if str == "" {
			return ""
		}
		parts := strings.Split(str, ":")
		if len(parts) > 1 {
			return parts[1]
		}
		return str
	})

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

	views.AddGlobal("len", func(arr []string) int {
		return len(arr)
	})

	views.AddGlobal("sub", func(a, b int) int {
		return a - b
	})

	views.AddGlobal("ne", func(a, b int) bool {
		return a != b
	})

	template, err := views.GetTemplate("enums.jet")
	if err != nil {
		log.Printf("Failed to load enums template: %v", err)
		return
	}

	// Generate file
	outputPath := filepath.Join(enumsDir, "Enums.kt")
	file, err := os.Create(outputPath)
	if err != nil {
		log.Printf("Failed to create enums file: %v", err)
		return
	}
	defer file.Close()

	err = template.Execute(file, nil, data)
	if err != nil {
		log.Printf("Failed to execute enums template: %v", err)
		return
	}

	log.Printf("Generated Kotlin: %s", outputPath)
}