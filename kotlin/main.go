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

type KotlinTemplateData struct {
	PackageName string
	TableName   string
	ModelName   string
	Columns     []KotlinColumn
	HasDateTime bool
	HasDecimal  bool
	Enums       []KotlinEnumData
}

func ProcessKotlin(packageName string, tableName string, prefix string, columns []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	modelName := strings.Title(util.GetTableName(tableName))
	
	// Determine target path
	var targetPath string
	if len(os.Args) > 1 && os.Args[1] != "" {
		targetPath = os.Args[1]
	} else {
		targetPath = "../gym/gymspring"
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

		kotlinCol := KotlinColumn{
			Name:         col.Name,
			DBName:       col.Column,
			KotlinType:   col.Type,
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

	templateData := KotlinTemplateData{
		PackageName: packageName,
		TableName:   tableName,
		ModelName:   modelName,
		Columns:     kotlinColumns,
		HasDateTime: hasDateTime,
		HasDecimal:  hasDecimal,
		Enums:       enums,
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
	entityDir := filepath.Join(targetPath, "src", "main", "kotlin", "com", "gowoobro", "gymspring", "entity")
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

	log.Printf("Generated Kotlin entity: %s", outputPath)
}

func generateKotlinRepository(targetPath string, data KotlinTemplateData) {
	// Create repository directory
	repositoryDir := filepath.Join(targetPath, "src", "main", "kotlin", "com", "gowoobro", "gymspring", "repository")
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

	log.Printf("Generated Kotlin repository: %s", outputPath)
}

func generateKotlinService(targetPath string, data KotlinTemplateData) {
	// Create service directory
	serviceDir := filepath.Join(targetPath, "src", "main", "kotlin", "com", "gowoobro", "gymspring", "service")
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

	log.Printf("Generated Kotlin service: %s", outputPath)
}

func generateKotlinController(targetPath string, data KotlinTemplateData) {
	// Create controller directory
	controllerDir := filepath.Join(targetPath, "src", "main", "kotlin", "com", "gowoobro", "gymspring", "controller")
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

	log.Printf("Generated Kotlin controller: %s", outputPath)
}

func generateKotlinEnums(targetPath string, data KotlinTemplateData) {
	// Create enums directory
	enumsDir := filepath.Join(targetPath, "src", "main", "kotlin", "com", "gowoobro", "gymspring", "enums", strings.ToLower(data.ModelName))
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

	log.Printf("Generated Kotlin enums: %s", outputPath)
}