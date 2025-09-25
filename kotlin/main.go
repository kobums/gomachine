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
	IsNullable   bool
	IsPrimaryKey bool
	HasDefault   bool
	DefaultValue string
}

type KotlinTemplateData struct {
	PackageName string
	TableName   string
	ModelName   string
	Columns     []KotlinColumn
	HasDateTime bool
	HasDecimal  bool
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

		if col.Type == "LocalDateTime" {
			hasDateTime = true
			if !isPrimaryKey {
				hasDefault = true
				if isNullable {
					defaultValue = "null"
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
			IsNullable:   isNullable,
			IsPrimaryKey: isPrimaryKey,
			HasDefault:   hasDefault,
			DefaultValue: defaultValue,
		}
		kotlinColumns = append(kotlinColumns, kotlinCol)
	}

	templateData := KotlinTemplateData{
		PackageName: packageName,
		TableName:   tableName,
		ModelName:   modelName,
		Columns:     kotlinColumns,
		HasDateTime: hasDateTime,
		HasDecimal:  hasDecimal,
	}

	// Generate entity file
	generateKotlinEntity(targetPath, templateData)
	
	// Generate repository file
	generateKotlinRepository(targetPath, templateData)
	
	// Generate service file
	generateKotlinService(targetPath, templateData)
	
	// Generate controller file
	generateKotlinController(targetPath, templateData)
}

func generateKotlinEntity(targetPath string, data KotlinTemplateData) {
	// Create entity directory
	entityDir := filepath.Join(targetPath, "src", "main", "kotlin", "com", "gowoobro", "gymspring", "entity")
	err := os.MkdirAll(entityDir, 0755)
	if err != nil {
		log.Printf("Failed to create entity directory: %v", err)
		return
	}

	// Load template
	templatePath := filepath.Join(".", "views", "kotlin", "entity.jet")
	views := jet.NewSet(
		jet.NewOSFileSystemLoader(filepath.Dir(templatePath)),
		jet.InDevelopmentMode(),
	)

	template, err := views.GetTemplate(filepath.Base(templatePath))
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

	// Load template
	templatePath := filepath.Join(".", "views", "kotlin", "repository.jet")
	views := jet.NewSet(
		jet.NewOSFileSystemLoader(filepath.Dir(templatePath)),
		jet.InDevelopmentMode(),
	)

	template, err := views.GetTemplate(filepath.Base(templatePath))
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

	// Load template
	templatePath := filepath.Join(".", "views", "kotlin", "service.jet")
	views := jet.NewSet(
		jet.NewOSFileSystemLoader(filepath.Dir(templatePath)),
		jet.InDevelopmentMode(),
	)

	template, err := views.GetTemplate(filepath.Base(templatePath))
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

	// Load template
	templatePath := filepath.Join(".", "views", "kotlin", "controller.jet")
	views := jet.NewSet(
		jet.NewOSFileSystemLoader(filepath.Dir(templatePath)),
		jet.InDevelopmentMode(),
	)

	template, err := views.GetTemplate(filepath.Base(templatePath))
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