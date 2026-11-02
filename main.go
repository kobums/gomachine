package main

import (
	"database/sql"
	"fmt"
	"gomachine/config"
	dartcodegen "gomachine/dart"
	gocodegen "gomachine/go"
	kotlincodegen "gomachine/kotlin"
	reactcodegen "gomachine/react"
	"gomachine/util"
	vuecodegen "gomachine/vue"
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

	// config/model.json 로드 ("pointer" config to find the real model.json location)
	configPath := filepath.Join(configDir, "config")
	pointerConfig := config.Init(configPath)

	var modelConfig config.ModelConfig
	modelConfigDir := configDir

	if pointerConfig.ModelJsonPath != "" {
		modelConfigDir = pointerConfig.ModelJsonPath
		modelConfig = config.Init(modelConfigDir)
	} else {
		modelConfig = pointerConfig
	}

	// 생성할 언어: model.json 의 languages / language, os.Args[2] 가 있으면 그것으로 오버라이드 ("go,react", "all" 가능)
	languages := modelConfig.EnabledLanguages()
	if len(os.Args) > 2 && os.Args[2] != "" {
		languages = config.ResolveLanguages(strings.Split(os.Args[2], ","))
	}

	// go 외 언어는 출력 경로가 없으면 엉뚱한 기본 경로에 쓰지 않도록 건너뛴다.
	enabled := make([]string, 0)
	for _, lang := range languages {
		if lang != "go" && modelConfig.OutputPath(lang) == "" {
			log.Printf("WARNING: %s is enabled but %sModelFilePath is not set in model.json, skipped", lang, lang)
			continue
		}
		enabled = append(enabled, lang)
	}
	if len(enabled) == 0 {
		log.Println("No language to generate")
		return
	}
	log.Printf("Generate languages: %v", enabled)

	// packageName은 Database명 사용 (또는 os.Args[3]이 있으면 사용)
	packageName := modelConfig.Database
	if len(os.Args) > 3 && os.Args[3] != "" {
		packageName = os.Args[3]
	}

	gpas := modelConfig.Gpa

	db := GetConnection(modelConfig)
	if db == nil {
		log.Println("Failed to get database connection")
		return
	}
	defer db.Close()

	query := fmt.Sprintf("select table_name from information_schema.tables where table_schema = '%v' order by table_name", modelConfig.Database)
	rows, err := db.Query(query)

	if err != nil {
		log.Println(err)
		return
	}

	names := make([]string, 0)
	for rows.Next() {
		name := ""

		err := rows.Scan(&name)
		if err != nil {
			log.Println(err)
			continue
		}
		names = append(names, name)
	}
	rows.Close()

	util.ResolveTableNames(names)

	for _, name := range names {
		var gpa *config.Gpa = nil
		for i := range gpas {
			if util.MatchTableName(gpas[i].Name, name) {
				g := gpas[i]
				gpa = &g
				break
			}
		}

		readColumn(packageName, name, db, gpa, modelConfig.Buildtool, modelConfig.Auth, modelConfig, enabled)
	}
}

type generator func(packageName string, tableName string, prefix string, items []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig)

var generators = map[string]generator{
	"go":     gocodegen.ProcessGo,
	"kotlin": kotlincodegen.ProcessKotlin,
	"dart":   dartcodegen.ProcessDart,
	"react":  reactcodegen.ProcessReact,
	"vue":    vuecodegen.ProcessVue,
}

func readColumn(packageName string, tableName string, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig, languages []string) {
	// 이름, 타입, null 허용(is_nullable), 기본값(column_default) 을 ordinal_position 순서로 읽는다.
	rawCols, err := util.QueryColumns(db, cnf.Database, tableName)
	if err != nil {
		log.Println(err)
		return
	}

	prefix := ""
	for _, raw := range rawCols {
		prefix = util.GetPrefix(raw.Name)
	}

	// 언어마다 타입 매핑이 달라서 컬럼 목록을 따로 만든다.
	for _, lang := range languages {
		localCnf := cnf
		localCnf.Language = lang
		columns := make([]util.Column, 0)
		for _, raw := range rawCols {
			columns = append(columns, util.NewColumnFromRaw(tableName, raw, gpa, localCnf))
		}
		generators[lang](packageName, tableName, prefix, columns, db, gpa, version, auth, localCnf)
	}
}
