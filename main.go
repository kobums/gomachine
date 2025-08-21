package main

import (
	"database/sql"
	"fmt"
	"gomachine/config"
	dartcodegen "gomachine/dart"
	gocodegen "gomachine/go"
	"gomachine/util"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	log "github.com/sirupsen/logrus"
)

func GetConnection(config config.ModelConfig) *sql.DB {
	connectionString := fmt.Sprintf("%v:%v@tcp(%v:3306)/%v", config.User, config.Password, config.Server, config.Database)

	r1, err := sql.Open("mysql", connectionString)
	if err != nil {
		log.Println("Database Connect Error")
		return nil
	}

	r1.SetMaxOpenConns(1000)
	r1.SetMaxIdleConns(10)
	r1.SetConnMaxIdleTime(5 * time.Minute)

	return r1
}

func main() {
	log.Println("run buildtool model")
	
	// 현재 작업 디렉토리 가져오기 (config 파일이 있는 위치)
	configDir, err := os.Getwd()
	if err != nil {
		log.Printf("Failed to get current working directory: %v", err)
		return
	}

	// goModelFilePath 가져오기
	goModelPath := config.GetGoModelFilePath(configDir)
	if goModelPath == "" {
		log.Println("goModelFilePath not found in config, using current directory")
		goModelPath = configDir
	}
	
	// 절대 경로로 변환하여 로그 출력
	if targetPath, err := filepath.Abs(goModelPath); err == nil {
		log.Printf("Absolute target path: %s", targetPath)
	}

	// ModelConfig 로드
	modelConfig := config.Init(goModelPath)
	
	// os.Args로 언어가 지정된 경우 오버라이드
	if len(os.Args) > 2 && os.Args[2] != "" {
		modelConfig.Language = os.Args[2]
	}
	
	// packageName은 Database명 사용 (또는 os.Args[3]이 있으면 사용)
	packageName := modelConfig.Database
	if len(os.Args) > 3 && os.Args[3] != "" {
		packageName = os.Args[3]
	}

	log.Printf("Package name: %s", packageName)
	log.Printf("Language: %s", modelConfig.Language)

	gpas := modelConfig.Gpa
	
	db := GetConnection(modelConfig)
	defer db.Close()

	query := fmt.Sprintf("select table_name from information_schema.tables where table_schema = '%v'", modelConfig.Database)
	rows, err := db.Query(query)

	if err != nil {
		log.Println(err)
	}

	tables := make([]string, 0)
	for rows.Next() {
		name := ""

		err := rows.Scan(&name)
		if err != nil {
			log.Println(err)
		}

		tableName := util.GetTableName(name)

		var gpa *config.Gpa = nil
		for _, item := range gpas {
			if item.Name == tableName {
				gpa = &item
				break
			}
		}

		readColumn(packageName, name, db, gpa, modelConfig.Buildtool, modelConfig.Auth, modelConfig)
		tables = append(tables, tableName)
	}
	
	// Generate router file for Go
	if modelConfig.Language == "" || modelConfig.Language == "go" || modelConfig.Language == "golang" {
		gocodegen.GenerateGoRouter(packageName, modelConfig)
	}
}

func readColumn(packageName string, tableName string, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	query := "select column_name as column_name, data_type as data_type from information_schema.columns where table_schema = '" + cnf.Database + "' and table_name = '" + tableName + "'"
	rows, err := db.Query(query)

	if err != nil {
		log.Println(err)
	}

	prefix := ""

	// Process based on language
	if cnf.Language == "dart" || cnf.Language == "flutter" {
		dartColumns := make([]util.Column, 0)
		for rows.Next() {
			var name string
			var typeid string

			err := rows.Scan(&name, &typeid)
			if err != nil {
				log.Println(err)
			}

			column := util.Column{Name: strings.Title(util.GetName(name)), Column: name, Type: util.GetType(util.GetTableName(tableName), util.GetName(name), typeid, gpa, cnf), OriginalType: typeid}
			dartColumns = append(dartColumns, column)

			prefix = util.GetPrefix(name)
		}
		dartcodegen.ProcessDart(packageName, tableName, prefix, dartColumns, db, gpa, version, auth, cnf)
	} else {
		goColumns := make([]util.Column, 0)
		for rows.Next() {
			var name string
			var typeid string

			err := rows.Scan(&name, &typeid)
			if err != nil {
				log.Println(err)
			}

			column := util.Column{Name: strings.Title(util.GetName(name)), Column: name, Type: util.GetType(util.GetTableName(tableName), util.GetName(name), typeid, gpa, cnf), OriginalType: typeid}
			goColumns = append(goColumns, column)

			prefix = util.GetPrefix(name)
		}
		gocodegen.ProcessGo(packageName, tableName, prefix, goColumns, db, gpa, version, auth, cnf)
	}
}