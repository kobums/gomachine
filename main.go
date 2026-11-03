package main

import (
	"database/sql"
	"fmt"
	"gomachine/config"
	dartcodegen "gomachine/dart"
	gocodegen "gomachine/go"
	kotlincodegen "gomachine/kotlin"
	reactcodegen "gomachine/react"
	rustcodegen "gomachine/rust"
	"gomachine/util"
	vuecodegen "gomachine/vue"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	log "github.com/sirupsen/logrus"
)

func GetConnection(config config.ModelConfig) *sql.DB {
	// 포트는 model.json port / DB_PORT (없으면 3306), DB 이름은 DB_NAME 이 model.json database 보다 우선한다 (config.Load).
	connectionString := fmt.Sprintf("%v:%v@tcp(%v)/%v", config.User, config.Password, net.JoinHostPort(config.Server, config.GetPort()), config.Database)

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
	// buildtool-model init <name> [flags]: basicproject 템플릿으로 새 작업 공간을 만든다 (initcmd.go).
	if len(os.Args) > 1 && os.Args[1] == "init" {
		os.Exit(runInit(os.Args[2:], os.Stdout))
	}

	log.Println("run buildtool model")

	// --force: 머리줄 없는 기존 파일도 덮어쓴다. 나머지는 위치 인자다.
	args := parseArgs(os.Args[1:])

	// 현재 작업 디렉토리 가져오기 (config 파일이 있는 위치)
	configDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current working directory: %v", err)
	}

	var modelConfig config.ModelConfig
	if dir := projectDir(args); dir != "" {
		// buildtool-model <projectDir>: <projectDir>/model.json, 상대 출력 경로는 그 디렉터리 기준이다.
		// model.json 이 포인터(modelJsonPath)면 가리키는 model.json 을 읽고, 상대 경로는 그 디렉터리 기준이다 (init 의 하위 프로젝트).
		var base string
		modelConfig, base = config.FollowPointer(dir, config.Init(dir))
		modelConfig.ResolvePaths(base)
	} else {
		// config/model.json 로드 ("pointer" config to find the real model.json location)
		pointerConfig := config.Init(filepath.Join(configDir, "config"))
		if pointerConfig.ModelJsonPath != "" {
			modelConfig = config.Init(pointerConfig.ModelJsonPath)
		} else {
			modelConfig = pointerConfig
		}
	}

	// 생성할 언어: model.json 의 languages / language, 두 번째 인자가 있으면 그것으로 오버라이드 ("go,react", "all" 가능)
	languages := modelConfig.EnabledLanguages()
	if len(args) > 1 && args[1] != "" {
		languages = config.ResolveLanguages(strings.Split(args[1], ","))
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

	// packageName: 세 번째 인자 > {goModelFilePath}/go.mod 의 module > Database 이름 (경고)
	packageName := ""
	if len(args) > 2 && args[2] != "" {
		packageName = args[2]
	}
	if packageName == "" {
		packageName = config.GoModule(modelConfig.GoModelFilePath)
	}
	if packageName == "" {
		packageName = modelConfig.Database
		if hasLanguage(enabled, "go") {
			log.Printf("WARNING: no go.mod module found in goModelFilePath %q and no package argument, using database name %q as Go package", modelConfig.GoModelFilePath, packageName)
		}
	}

	gpas := modelConfig.Gpa

	db := GetConnection(modelConfig)
	if db == nil {
		log.Fatal("Failed to get database connection")
	}
	defer db.Close()

	rows, err := db.Query("select table_name from information_schema.tables where table_schema = ? order by table_name", modelConfig.Database)

	// DB 에 붙지 못하면 아무것도 만들지 않았으므로 실패 코드로 끝낸다.
	if err != nil {
		log.Fatalf("Failed to read table list: %v", err)
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
	util.CheckTableConfigs(gpas, names)

	// 테이블마다 쓸 설정: 정확히 맞는 이름이 먼저, 그다음 _ / 대소문자 무시 (util.FindTableConfig).
	tableGpa := make(map[string]*config.Gpa)
	for _, name := range names {
		if g := util.FindTableConfig(gpas, name); g != nil {
			c := *g
			tableGpa[name] = &c
		}
	}

	// 읽기 정책(table[].read)이 잘못되면 아무것도 만들기 전에 멈춘다.
	policyErrs := make([]string, 0)
	for _, name := range names {
		if _, err := util.ReadPolicy(util.ModelName(name), tableGpa[name], modelConfig.Auth); err != nil {
			policyErrs = append(policyErrs, err.Error())
		}
	}
	if len(policyErrs) > 0 {
		for _, e := range policyErrs {
			log.Println("ERROR:", e)
		}
		os.Exit(1)
	}

	// auth=jwt 에서 session 컬럼을 쓸 수 없으면 소유권 검사 없는 API 가 만들어지므로 아무것도 만들기 전에 멈춘다.
	if errs := checkSessions(db, modelConfig, names, tableGpa, enabled); len(errs) > 0 {
		for _, e := range errs {
			log.Println("ERROR:", e)
		}
		os.Exit(1)
	}

	// Rust 는 실행마다 한 번 지원 모듈을 만들고(Prepare), 테이블을 모두 만든 뒤 모듈 목록 / 경로를 쓴다(Finish).
	rustCnf := modelConfig
	rustCnf.Language = "rust"
	if hasLanguage(enabled, "rust") {
		rustcodegen.Prepare(rustCnf)
	}

	for _, name := range names {
		readColumn(packageName, name, db, tableGpa[name], modelConfig.Buildtool, modelConfig.Auth, modelConfig, enabled)
	}

	if hasLanguage(enabled, "go") {
		goCnf := modelConfig
		goCnf.Language = "go"
		gocodegen.Finish(goCnf)
	}

	if hasLanguage(enabled, "rust") {
		rustcodegen.Finish(rustCnf)
	}
}

// parseArgs 는 --force / -force 를 처리하고 나머지 위치 인자를 돌려준다.
func parseArgs(argv []string) []string {
	ret := make([]string, 0, len(argv))
	for _, a := range argv {
		if a == "--force" || a == "-force" {
			util.Force = true
			continue
		}
		ret = append(ret, a)
	}
	return ret
}

// projectDir 는 첫 인자가 model.json 이 있는 디렉터리면 그 경로를, 아니면 "" 를 돌려준다 (예전처럼 ./config/model.json).
func projectDir(args []string) string {
	if len(args) == 0 || args[0] == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(args[0], "model.json")); err != nil {
		// "." 은 예전 사용법(첫 인자 무시)이라 경고하지 않는다.
		if args[0] != "." {
			log.Printf("WARNING: %s has no model.json, falling back to ./config/model.json", args[0])
		}
		return ""
	}
	return args[0]
}

func hasLanguage(languages []string, lang string) bool {
	for _, l := range languages {
		if l == lang {
			return true
		}
	}
	return false
}

// checkSessions 는 session 설정 컬럼이 있고 정수(Spring 만 생성하면 문자열도) 컬럼인지 확인한다. Rust 는 Go 와 같이 정수만 된다.
// auth=jwt 면 문제를 에러로 돌려주고, 아니면 경고만 찍는다 (session 은 jwt 일 때만 쓴다).
func checkSessions(db *sql.DB, cnf config.ModelConfig, names []string, tableGpa map[string]*config.Gpa, languages []string) []string {
	allowString := true
	for _, lang := range languages {
		if lang == "go" || lang == "rust" {
			allowString = false
		}
	}
	errs := make([]string, 0)
	for _, name := range names {
		gpa := tableGpa[name]
		if gpa == nil || len(gpa.Session) == 0 {
			continue
		}
		raws, err := util.QueryColumns(db, cnf.Database, name)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		columns := make([]util.Column, 0)
		for _, raw := range raws {
			columns = append(columns, util.NewColumnFromRaw(name, raw, gpa, cnf))
		}
		for _, pair := range gpa.Session {
			msg := util.SessionColumnError(columns, pair.Column, allowString)
			if msg == "" {
				continue
			}
			if cnf.Auth == "jwt" {
				errs = append(errs, fmt.Sprintf("%s: %s; fix model.json table %q session (generation stopped: the API would have no ownership check)", name, msg, gpa.Name))
			} else {
				log.Printf("WARNING: %s: %s", name, msg)
			}
		}
	}
	return errs
}

type generator func(packageName string, tableName string, prefix string, items []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig)

var generators = map[string]generator{
	"go":     gocodegen.ProcessGo,
	"kotlin": kotlincodegen.ProcessKotlin,
	"dart":   dartcodegen.ProcessDart,
	"react":  reactcodegen.ProcessReact,
	"vue":    vuecodegen.ProcessVue,
	"rust":   rustcodegen.ProcessRust,
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
