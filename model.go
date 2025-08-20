package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"gomachine/config"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	log "github.com/sirupsen/logrus"

	"github.com/CloudyKit/jet/v6"
	_ "github.com/go-sql-driver/mysql"
)

type Where struct {
	Column  string
	Type    string
	Compare string
}

type Func struct {
	Name   string
	Wheres []Where
}


type Column struct {
	Name         string
	Column       string
	Type         string
	OriginalType string
	Prefix       string
	Primary			 bool
}

func CopyFile(src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		panic(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		panic(err)
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		panic(err)
	}
	err = out.Sync()
	if err != nil {
		panic(err)
	}
}

func WriteFile(filename string, content string) error {
	return os.WriteFile(filename, []byte(content), 0644)
}

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
	log.Println("Arguments:", os.Args)
	
	// 현재 작업 디렉토리 출력
	if cwd, err := os.Getwd(); err == nil {
		log.Printf("Current working directory: %s", cwd)
	} else {
		log.Printf("Failed to get current working directory: %v", err)
	}
	
	targetPath := os.Args[1]
	log.Printf("root path : %v\n", targetPath)
	
	// 타겟 경로의 절대경로 출력
	if absTargetPath, err := filepath.Abs(targetPath); err == nil {
		log.Printf("Absolute target path: %s", absTargetPath)
	} else {
		log.Printf("Failed to get absolute target path: %v", err)
	}

	packageName := ""

	if len(os.Args) > 3 {
		packageName = os.Args[3]
	} else {
		file, err := os.Open(path.Join(targetPath, "go.mod"))
		if err != nil {
			log.Println("go.mode not found")
			return
		}
		defer file.Close()

		reader := bufio.NewReader(file)
		for {
			line, isPrefix, err := reader.ReadLine()
			if isPrefix || err != nil {
				break
			}

			data := strings.Split(string(line), " ")
			packageName = data[1]

			break
		}
	}

	modelConfig := config.Init(targetPath)

	if len(os.Args) > 2 {
		modelConfig.Language = os.Args[2]
	}

	log.Println("language :", modelConfig.Language)

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

		tableName := getTableName(name)

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
	
	// Generate router file
	if modelConfig.Language == "" || modelConfig.Language == "go" || modelConfig.Language == "golang" {
		generateRouter(packageName, modelConfig)
	}
}

func getTableName(name string) string {
	strs := strings.Split(name, "_")

	if len(strs) < 2 {
		return name
	}

	return strs[0]
}

func getTableType(name string) string {
	strs := strings.Split(name, "_")

	if len(strs) < 2 {
		return "table"
	}

	if strs[1] == "vw" {
		return "view"
	}

	return "table"
}

func getName(name string) string {
	strs := strings.Split(name, "_")

	if len(strs) < 2 {
		return name
	}

	return strs[1]
}

func getPrefix(name string) string {
	strs := strings.Split(name, "_")

	if len(strs) < 2 {
		return ""
	}

	return strs[0]
}


func readColumn(packageName string, tableName string, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	query := "select column_name as column_name, data_type as data_type from information_schema.columns where table_schema = '" + cnf.Database + "' and table_name = '" + tableName + "'"
	rows, err := db.Query(query)

	if err != nil {
		log.Println(err)
	}

	prefix := ""

	columns := make([]Column, 0)
	for rows.Next() {
		var name string
		var typeid string

		err := rows.Scan(&name, &typeid)
		if err != nil {
			log.Println(err)
		}

		column := Column{Name: strings.Title(getName(name)), Column: name, Type: getType(getTableName(tableName), getName(name), typeid, gpa, cnf), OriginalType: typeid}
		columns = append(columns, column)

		prefix = getPrefix(name)
	}

	process(packageName, tableName, prefix, columns, db, gpa, version, auth, cnf)
}


func Split(src string) (entries []string) {
	// don't split invalid utf8
	if !utf8.ValidString(src) {
		return []string{src}
	}
	entries = []string{}
	var runes [][]rune
	lastClass := 0
	class := 0
	// split into fields based on class of unicode character
	for _, r := range src {
		switch true {
		case unicode.IsLower(r):
			class = 1
		case unicode.IsUpper(r):
			class = 2
		case unicode.IsDigit(r):
			class = 3
		default:
			class = 4
		}
		if class == lastClass {
			runes[len(runes)-1] = append(runes[len(runes)-1], r)
		} else {
			runes = append(runes, []rune{r})
		}
		lastClass = class
	}
	// handle upper case -> lower case sequences, e.g.
	// "PDFL", "oader" -> "PDF", "Loader"
	for i := 0; i < len(runes)-1; i++ {
		if unicode.IsUpper(runes[i][0]) && unicode.IsLower(runes[i+1][0]) {
			runes[i+1] = append([]rune{runes[i][len(runes[i])-1]}, runes[i+1]...)
			runes[i] = runes[i][:len(runes[i])-1]
		}
	}
	// construct []string from results
	for _, s := range runes {
		if len(s) > 0 {
			entries = append(entries, string(s))
		}
	}
	return
}

func getType(tableName string, name string, t string, gpa *config.Gpa, cnf config.ModelConfig) string {
	if gpa != nil && gpa.Map != nil {
		for _, item := range gpa.Map {
			if item.Name == name {
				if cnf.Language == "dart" || cnf.Language == "flutter" {
					return strings.Title(name)
				} else {
					return tableName + "." + strings.Title(name)
				}
			}
		}
	}

	if cnf.Language == "dart" || cnf.Language == "flutter" {
		if t == "int" {
			return "int"
		} else if t == "bigint" {
			return "int"
		} else if t == "varchar" {
			return "String"
		} else if t == "text" {
			return "String"
		} else if t == "datetime" {
			return "String"
		} else if t == "date" {
			return "String"
		} else if t == "time" {
			return "String"
		} else if t == "double" {
			return "double"
		} else if t == "float" {
			return "double"
		} else if t == "decimal" {
			return "int"
		} else if t == "tinyint" {
			return "bool"
		}
	} else {
		if t == "int" {
			return "int"
		} else if t == "bigint" {
			return "int64"
		} else if t == "varchar" {
			return "string"
		} else if t == "text" {
			return "string"
		} else if t == "longtext" {
			return "string"
		} else if t == "datetime" {
			return "string"
		} else if t == "date" {
			return "string"
		} else if t == "time" {
			return "string"
		} else if t == "double" {
			return "Double"
		} else if t == "float" {
			return "Double"
		} else if t == "decimal" {
			return "int"
		} else if t == "tinyint" {
			return "bool"
		}
	}

	return t
}

func tablePrefix(str string, packageName string, db *sql.DB) string {
	query := "select column_name as column_name, data_type as data_type from information_schema.columns where table_schema = '" + packageName + "' and table_name = '" + str + "_tb'"
	rows, err := db.Query(query)

	if err != nil {
		log.Println(err)
	}

	if rows.Next() {
		var name string
		var typeid string

		err := rows.Scan(&name, &typeid)
		if err != nil {
			log.Println(err)
		}

		prefix := getPrefix(name)
		return prefix
	}

	return ""
}

func unique(items []Func) []string {
	ret := make([]string, 0)

	for _, item := range items {
		for _, where := range item.Wheres {
			str := where.Type
			if strings.Index(str, ".") > 0 {
				strs := strings.Split(str, ".")

				flag := false

				for i := 0; i < len(ret); i++ {
					if ret[i] == strs[0] {
						flag = true
						break
					}
				}

				if flag == false {
					ret = append(ret, strs[0])
				}
			}
		}
	}

	return ret
}

func process(packageName string, tableName string, prefix string, items []Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	path := fmt.Sprintf("%v/bin/buildtool", os.Getenv("HOME"))

	var views = jet.NewSet(jet.NewOSFileSystemLoader(path), jet.InDevelopmentMode())

	views.AddGlobal("striparray", func(str string) string {
		return strings.ReplaceAll(str, "[]", "")
	})

	views.AddGlobal("substring", func(str string, start int, end int) string {
		return str[start:end]
	})

	views.AddGlobal("title", func(str string) string {
		return strings.Title(str)
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

	views.AddGlobal("typescriptType", func(str string) string {
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
		return ret[1]
	})

	views.AddGlobal("querytype", func(str string) string {
		tokens := Split(str)

		return tokens[0]
	})

	views.AddGlobal("adjustPackage", func(str string) string {
		if strings.Index(str, ".") > 0 {
			return "models." + str
		} else {
			return str
		}
	})

	views.AddGlobal("isNeedImport", func(str string) bool {
		if strings.Index(str, ".") > 0 {
			return true
		} else {
			return false
		}
	})

	views.AddGlobal("getPrefix", func(str string, prefix string) string {
		strs := strings.Split(str, "_")

		if len(strs) >= 2 {
			return str
		} else {
			return prefix + "_" + str
		}
	})

	views.AddGlobal("joinColumn", func(str string, cols []config.GpaJoin) bool {
		for _, v := range cols {
			if getName(strings.ToLower(str)) == v.Name {
				return false
			}
		}

		return true

	})

	views.AddGlobal("compareColumn", func(str string, cols []config.GpaCompare) string {
		for _, v := range cols {
			if getName(strings.ToLower(str)) == v.Name {
				return v.Type
			}
		}

		return "="

	})

	views.AddGlobal("javascriptfunction", func(str string) string {
		return strings.ToLower(str[0:1]) + str[1:]
	})

	views.AddGlobal("javascriptapiurl", func(str string) string {
		return strings.ReplaceAll(strings.ToLower(str), "delete", "")
	})

	views.AddGlobal("apiurl", func(str string) string {
		funcName := strings.ToLower(str)
		url := ""
		if len(funcName) > 5 && funcName[:5] == "getby" {
			url = fmt.Sprintf("/get/%v", funcName[5:])
		} else if len(funcName) > 7 && funcName[:7] == "countby" {
			url = fmt.Sprintf("/count/%v", funcName[7:])
		} else if len(funcName) > 6 && funcName[:6] == "findby" {
			url = fmt.Sprintf("/find/%v", funcName[6:])
		} else if len(funcName) > 6 && funcName[:6] == "update" {
			strs := strings.Split(str[6:], "By")
			url = fmt.Sprintf("/%v/%v", strings.ToLower(strs[0]), strings.ToLower(strs[1]))
		} else if len(funcName) > 8 && funcName[:8] == "deleteby" {
			url = fmt.Sprintf("/%v", funcName[8:])
		} else {
			url = fmt.Sprintf("/%v", funcName)
		}

		return url
	})

	views.AddGlobal("columns", func(str string) []Column {
		query := "select column_name as column_name, data_type as data_type from information_schema.columns where table_schema = '" + packageName + "' and table_name = '" + strings.ToLower(str) + "_tb'"
		rows, err := db.Query(query)

		if err != nil {
			log.Println(err)
		}

		columns := make([]Column, 0)
		for rows.Next() {
			var name string
			var typeid string

			err := rows.Scan(&name, &typeid)
			if err != nil {
				log.Println(err)
			}

			prefix := getPrefix(name)
			column := Column{Name: strings.Title(getName(name)), Column: name, Type: getType(getTableName(str), getName(name), typeid, gpa, cnf), Prefix: prefix}
			columns = append(columns, column)
		}

		return columns
	})
	
	v := make(jet.VarMap)
	v.Set("version", version)
	v.Set("packageName", packageName)
	v.Set("type", getTableType(tableName))
	v.Set("adminLevel", cnf.AdminLevel)
	v.Set("name", strings.Title(getTableName(tableName)))
	v.Set("tableName", tableName)
	v.Set("prefix", prefix)

	if gpa != nil {
		for i, v := range items {
			for _, v2 := range gpa.Primary {
				if v.Name == strings.Title(v2) {
					items[i].Primary = true
				}
			}
		}
	}

	v.Set("items", items)
	v.Set("auth", auth)
	if gpa == nil {
		v.Set("consts", make([]string, 0))
		v.Set("methods", make([]string, 0))
		v.Set("primarys", []string{"id"})
		v.Set("funcs", make([]string, 0))
		v.Set("joins", make([]config.GpaJoin, 0))
		v.Set("compares", make([]config.GpaCompare, 0))
		v.Set("sessions", make([]config.SessionPair, 0))
		v.Set("search", false)
		v.Set("imports", make([]string, 0))
	} else {
		for i := range gpa.Join {
			gpa.Join[i].Prefix = tablePrefix(gpa.Join[i].Name, packageName, db)
		}
		v.Set("consts", gpa.Map)
		v.Set("methods", gpa.Method)
		if len(gpa.Primary) == 0 {
			gpa.Primary = append(gpa.Primary, "id")
		}
		v.Set("primarys", gpa.Primary)
		v.Set("joins", gpa.Join)
		v.Set("compares", gpa.Compare)
		v.Set("search", gpa.Search)
		v.Set("sessions", gpa.Session)

		funcs := make([]Func, 0)

		for _, item := range gpa.Method {
			tokens := Split(item)

			wheres := make([]Where, 0)
			if tokens[0] == "Update" {
				flag := false
				for i := 1; i < len(tokens); i++ {
					token := tokens[i]
					column := ""
					typename := ""
					compare := ""

					if token == "By" {
						flag = true
						continue
					} else {
						for _, name := range items {
							if token == name.Name {
								column = name.Name
								typename = name.Type
								if flag == true {
									compare = "where"
								} else {
									compare = "column"
								}
								break
							}
						}
					}

					where := Where{Column: column, Type: typename, Compare: compare}
					wheres = append(wheres, where)
				}
			} else {
				for i := 2; i < len(tokens); i++ {
					token := tokens[i]
					column := ""
					typename := ""
					compare := ""
					flag := false
					for _, name := range items {
						if token == name.Name {
							column = name.Name
							typename = name.Type
							compare = "="
							flag = true
							break
						}
					}

					if flag == false {
						for _, name := range items {
							if token == name.Name+"s" {
								column = name.Name
								typename = "[]" + name.Type
								compare = "in"
								flag = true
								break
							}
						}
					}

					if flag == false {
						for _, name := range items {
							if token == name.Name+"like" {
								column = name.Name
								typename = name.Type
								compare = "like"
								flag = true
								break
							}
						}
					}

					where := Where{Column: column, Type: typename, Compare: compare}
					wheres = append(wheres, where)
				}
			}

			fn := Func{Name: item, Wheres: wheres}
			funcs = append(funcs, fn)
		}

		v.Set("funcs", funcs)
		v.Set("imports", unique(funcs))
	}

	// os.Mkdir("./models", 0755)
	// if cnf.Language == "" || cnf.Language == "go" || cnf.Language == "golang" {
	// 	CopyFile(filepath.Join(path, "mysql/cache.go"), "./models/cache.go")

	// 	var b bytes.Buffer
	// 	modelFilename := "mysql/db.go"
	// 	t, err := views.GetTemplate(modelFilename)
	// 	if err == nil {
	// 		if err = t.Execute(&b, v, nil); err != nil {
	// 			log.Println(err)
	// 			// error when executing template
	// 		}

	// 		if err == nil {
	// 			WriteFile("./models/db.go", b.String())
	// 		}
	// 	}
	// }

	var b bytes.Buffer
	modelFilename := "go/model.jet"
	if cnf.Language == "dart" || cnf.Language == "flutter" {
		modelFilename = "dart/model.jet"
	}
	t, err := views.GetTemplate(modelFilename)
	if err == nil {
		if err = t.Execute(&b, v, nil); err != nil {
			log.Println(err)
			// error when executing template
		}
	} else {
		log.Println("error ========================")
		log.Println(err)
		log.Println("error ========================")
	}


	if cnf.Language == "go" {
		modelFile := cnf.GoModelFilePath+"/models/"+getTableName(tableName)+".go"
		log.Printf("=== PROCESSING GO MODEL FILE ===")
		log.Printf("Table name: %s", tableName)
		log.Printf("Model file path: %s", modelFile)
		log.Printf("Template content length: %d", b.Len())
		
		if err := WriteFile(modelFile, b.String()); err != nil {
			log.Printf("CRITICAL ERROR: Failed to write model file %s: %v", modelFile, err)
		} else {
			log.Printf("SUCCESS: Model file written successfully: %s", modelFile)
		}
	}

	var templateBuffers []bytes.Buffer

	if cnf.Language == "dart" || cnf.Language == "flutter" {
		arr := []string{"model", "params", "provider", "repository"};
		for _, a := range arr {
			modelFilename = "dart/"+a+".jet"
			t, err := views.GetTemplate(modelFilename)
			if err == nil {
				var b bytes.Buffer
				if err = t.Execute(&b, v, nil); err != nil {
					log.Println(err)
				}
				templateBuffers = append(templateBuffers, b)
			} else {
				log.Println("error ========================")
				log.Println(err)
				log.Println("error ========================")
			}
		}
		for i, b := range templateBuffers {
			dartFile := cnf.DartModelFilePath+arr[i]+"/"+getTableName(tableName)+"_"+arr[i]+".dart"
			log.Printf("=== PROCESSING DART %s FILE ===", strings.ToUpper(arr[i]))
			log.Printf("Table name: %s", tableName)
			log.Printf("Dart file path: %s", dartFile)
			log.Printf("Template content length: %d", b.Len())
			
			if err := WriteFile(dartFile, b.String()); err != nil {
				log.Printf("CRITICAL ERROR: Failed to write dart file %s: %v", dartFile, err)
			} else {
				log.Printf("SUCCESS: Dart %s file written successfully: %s", arr[i], dartFile)
			}
		}
	} else {
		var b2 bytes.Buffer
		t2, err := views.GetTemplate("go/rest.jet")
		if err == nil {
			log.Printf("REST template loaded successfully")
			if err = t2.Execute(&b2, v, nil); err != nil {
				log.Printf("CRITICAL ERROR: REST template execution failed: %v", err)
			} else {
				log.Printf("REST template executed successfully, result size: %d", b2.Len())
			}
		} else {
			log.Printf("CRITICAL ERROR: Failed to load REST template: %v", err)
		}

		restFile := cnf.GoModelFilePath+"/controllers/rest/"+getTableName(tableName)+".go"
		log.Printf("=== PROCESSING GO REST CONTROLLER FILE ===")
		log.Printf("REST controller file path: %s", restFile)
		log.Printf("Template content length: %d", b2.Len())
		
		if err := WriteFile(restFile, b2.String()); err != nil {
			log.Printf("CRITICAL ERROR: Failed to write rest controller file %s: %v", restFile, err)
		} else {
			log.Printf("SUCCESS: REST controller file written successfully: %s", restFile)
		}

		// Generate API controller
		// var b4 bytes.Buffer
		// t4, err := views.GetTemplate("go/api.jet")
		// if err == nil {
		// 	log.Printf("API template loaded successfully")
		// 	if err = t4.Execute(&b4, v, nil); err != nil {
		// 		log.Printf("CRITICAL ERROR: API template execution failed: %v", err)
		// 	} else {
		// 		log.Printf("API template executed successfully, result size: %d", b4.Len())
		// 	}
		// } else {
		// 	log.Printf("CRITICAL ERROR: Failed to load API template: %v", err)
		// }

		// apiFile := cnf.GoModelFilePath+"/controllers/api/"+getTableName(tableName)+".go"
		// log.Printf("=== PROCESSING GO API CONTROLLER FILE ===")
		// log.Printf("API controller file path: %s", apiFile)
		// log.Printf("Template content length: %d", b4.Len())
		
		// // Create API directory if it doesn't exist
		// apiDir := cnf.GoModelFilePath+"/controllers/api"
		// if err := os.MkdirAll(apiDir, 0755); err != nil {
		// 	log.Printf("Failed to create API directory %s: %v", apiDir, err)
		// }
		
		// if err := WriteFile(apiFile, b4.String()); err != nil {
		// 	log.Printf("CRITICAL ERROR: Failed to write API controller file %s: %v", apiFile, err)
		// } else {
		// 	log.Printf("SUCCESS: API controller file written successfully: %s", apiFile)
		// }

		v2 := make(jet.VarMap)
		v2.Set("version", version)
		v2.Set("name", strings.Title(getTableName(tableName)))
		v2.Set("auth", auth)
		v2.Set("items", items)
		if gpa == nil {
			v2.Set("consts", make([]string, 0))
			v2.Set("methods", make([]string, 0))
			v2.Set("funcs", make([]string, 0))
		} else {
			v2.Set("consts", gpa.Map)
		}

		// Generate const file for all tables
		var b3 bytes.Buffer
		t, err = views.GetTemplate("go/const.jet")
		if err == nil {
			if err = t.Execute(&b3, v2, nil); err != nil {
				log.Printf("CRITICAL ERROR: Const template execution failed: %v", err)
			}
		} else {
			log.Printf("CRITICAL ERROR: Failed to load const template: %v", err)
		}

		constDir := cnf.GoModelFilePath+"/models/"+getTableName(tableName)
		log.Printf("Creating const directory: %s", constDir)
		if err := os.MkdirAll(constDir, 0755); err != nil {
			log.Printf("Failed to create const directory %s: %v", constDir, err)
		}
		
		constFile := constDir+"/"+getTableName(tableName)+".go"
		log.Printf("=== PROCESSING GO CONST FILE ===")
		log.Printf("Const file path: %s", constFile)
		log.Printf("Template content length: %d", b3.Len())
		
		if err := WriteFile(constFile, b3.String()); err != nil {
			log.Printf("CRITICAL ERROR: Failed to write const file %s: %v", constFile, err)
		} else {
			log.Printf("SUCCESS: Const file written successfully: %s", constFile)
		}
	}
}

func generateRouter(packageName string, cnf config.ModelConfig) {
	path := fmt.Sprintf("%v/bin/buildtool", os.Getenv("HOME"))
	var views = jet.NewSet(jet.NewOSFileSystemLoader(path), jet.InDevelopmentMode())

	// Add global functions for templates
	views.AddGlobal("title", func(str string) string {
		return strings.Title(str)
	})

	// Controller analysis data
	routerData := RouterData{
		PackageName: packageName,
		Auth:        cnf.Auth,
	}

	// Analyze controllers and generate routes
	domainRoutes := analyzeControllersByDomain(packageName, cnf, &routerData)

	// Create router directory if it doesn't exist
	routerDir := cnf.GoModelFilePath + "/router"
	if err := os.MkdirAll(routerDir, 0755); err != nil {
		log.Printf("Failed to create router directory %s: %v", routerDir, err)
		return
	}

	if err := os.MkdirAll(routerDir+"/routers", 0755); err != nil {
		log.Printf("Failed to create router directory %s: %v", routerDir+"/routers", err)
		return
	}

	// Generate domain-specific router files
	domains := make([]string, 0)
	for domainName, routes := range domainRoutes {
		domains = append(domains, domainName)
		
		// Determine controller type and if log is needed
		controllerType := "rest"
		needsLog := false
		for _, route := range routes {
			if strings.Contains(route.ControllerName, "api.") {
				controllerType = "api"
			}
			if route.NeedsBodyParser {
				needsLog = true
			}
		}

		domainData := make(jet.VarMap)
		domainData.Set("packageName", packageName)
		domainData.Set("domainName", domainName)
		domainData.Set("controllerType", controllerType)
		domainData.Set("needsLog", needsLog)
		domainData.Set("routes", routes)

		var domainBuffer bytes.Buffer
		domainTemplate, err := views.GetTemplate("go/domain_router.jet")
		if err != nil {
			log.Printf("CRITICAL ERROR: Failed to load domain router template: %v", err)
			continue
		}

		if err = domainTemplate.Execute(&domainBuffer, domainData, nil); err != nil {
			log.Printf("CRITICAL ERROR: Domain router template execution failed for %s: %v", domainName, err)
			continue
		}

		domainFile := routerDir + "/routers/" + strings.ToLower(domainName) + ".go"
		if err := WriteFile(domainFile, domainBuffer.String()); err != nil {
			log.Printf("CRITICAL ERROR: Failed to write domain router file %s: %v", domainFile, err)
		} else {
			log.Printf("SUCCESS: Domain router file written: %s", domainFile)
		}
	}

	// Generate main router file
	mainData := make(jet.VarMap)
	mainData.Set("packageName", packageName)
	mainData.Set("auth", routerData.Auth)
	mainData.Set("jsonFlag", routerData.JsonFlag)
	mainData.Set("urlImport", routerData.UrlImport)
	mainData.Set("apis", routerData.Apis)
	mainData.Set("cassandra", routerData.Cassandra)
	mainData.Set("imports", routerData.Imports)
	mainData.Set("domains", domains)

	var mainBuffer bytes.Buffer
	mainTemplate, err := views.GetTemplate("go/router.jet")
	if err != nil {
		log.Printf("CRITICAL ERROR: Failed to load main router template: %v", err)
		return
	}

	if err = mainTemplate.Execute(&mainBuffer, mainData, nil); err != nil {
		log.Printf("CRITICAL ERROR: Main router template execution failed: %v", err)
		return
	}

	mainRouterFile := routerDir + "/router.go"
	if err := WriteFile(mainRouterFile, mainBuffer.String()); err != nil {
		log.Printf("CRITICAL ERROR: Failed to write main router file %s: %v", mainRouterFile, err)
	} else {
		log.Printf("SUCCESS: Main router file written: %s", mainRouterFile)
	}

	log.Printf("Generated router files for %d domains: %v", len(domains), domains)
}

type RouteGroup struct {
	Name   string
	Path   string
	Routes []Route
}

type Route struct {
	Method         string
	URL            string
	ParamCode      string
	ControllerName string
	ControllerBase string
	FuncName       string
	ParamStr       string
	PreFlag        bool
	PostFlag       bool
	NeedsBodyParser bool
}

type RouterData struct {
	PackageName string
	Auth        string
	JsonFlag    bool
	UrlImport   bool
	Apis        bool
	Cassandra   bool
	Groups      []RouteGroup
	Imports     []string
}

func analyzeControllersByDomain(packageName string, cnf config.ModelConfig, routerData *RouterData) map[string][]Route {
	controllerPath := cnf.GoModelFilePath + "/controllers"
	
	// Check if controllers directory exists
	if _, err := os.Stat(controllerPath); os.IsNotExist(err) {
		log.Printf("Controllers directory not found: %s", controllerPath)
		return make(map[string][]Route)
	}

	// Get list of all controller files and their functions
	apiPath := controllerPath + "/api"
	restPath := controllerPath + "/rest"
	
	controllerFunctions := make(map[string]map[string][]string) // [type][controller] -> functions
	controllerFunctions["api"] = make(map[string][]string)
	controllerFunctions["rest"] = make(map[string][]string)

	// Parse API controllers
	if _, err := os.Stat(apiPath); err == nil {
		parseControllersInDirectory(apiPath, "api", controllerFunctions["api"])
	}

	// Parse REST controllers  
	if _, err := os.Stat(restPath); err == nil {
		parseControllersInDirectory(restPath, "rest", controllerFunctions["rest"])
	}

	// Generate routes with priority: API first, then REST
	allControllers := make(map[string]bool)
	
	// Collect all controller names
	for controller := range controllerFunctions["api"] {
		allControllers[controller] = true
	}
	for controller := range controllerFunctions["rest"] {
		allControllers[controller] = true
	}

	// Group routes by domain (controller name)
	domainRoutes := make(map[string][]Route)
	
	// Generate routes for each controller
	for controllerName := range allControllers {
		apiFunctions := controllerFunctions["api"][controllerName]
		restFunctions := controllerFunctions["rest"][controllerName]
		
		// Determine which controller type to use for each function
		routes := generateSmartRoutes(controllerName, apiFunctions, restFunctions)
		domainRoutes[controllerName] = routes
	}

	if len(controllerFunctions["api"]) > 0 {
		routerData.Apis = true
	}
	routerData.JsonFlag = true

	totalRoutes := 0
	for _, routes := range domainRoutes {
		totalRoutes += len(routes)
	}
	log.Printf("Analyzed controllers, found %d routes across %d domains", totalRoutes, len(domainRoutes))
	
	return domainRoutes
}

func analyzeControllers(packageName string, cnf config.ModelConfig, routerData *RouterData) {
	controllerPath := cnf.GoModelFilePath + "/controllers"
	
	// Check if controllers directory exists
	if _, err := os.Stat(controllerPath); os.IsNotExist(err) {
		log.Printf("Controllers directory not found: %s", controllerPath)
		return
	}

	apiGroup := RouteGroup{
		Name:   "api",
		Path:   "api",
		Routes: []Route{},
	}

	// Get list of all controller files and their functions
	apiPath := controllerPath + "/api"
	restPath := controllerPath + "/rest"
	
	controllerFunctions := make(map[string]map[string][]string) // [type][controller] -> functions
	controllerFunctions["api"] = make(map[string][]string)
	controllerFunctions["rest"] = make(map[string][]string)

	// Parse API controllers
	if _, err := os.Stat(apiPath); err == nil {
		parseControllersInDirectory(apiPath, "api", controllerFunctions["api"])
	}

	// Parse REST controllers  
	if _, err := os.Stat(restPath); err == nil {
		parseControllersInDirectory(restPath, "rest", controllerFunctions["rest"])
	}

	// Generate routes with priority: API first, then REST
	allControllers := make(map[string]bool)
	
	// Collect all controller names
	for controller := range controllerFunctions["api"] {
		allControllers[controller] = true
	}
	for controller := range controllerFunctions["rest"] {
		allControllers[controller] = true
	}

	// Generate routes for each controller
	for controllerName := range allControllers {
		apiFunctions := controllerFunctions["api"][controllerName]
		restFunctions := controllerFunctions["rest"][controllerName]
		
		// Determine which controller type to use for each function
		routes := generateSmartRoutes(controllerName, apiFunctions, restFunctions)
		apiGroup.Routes = append(apiGroup.Routes, routes...)
	}

	if len(controllerFunctions["api"]) > 0 {
		routerData.Apis = true
	}

	routerData.Groups = append(routerData.Groups, apiGroup)
	routerData.JsonFlag = true
	log.Printf("Analyzed controllers, found %d routes", len(apiGroup.Routes))
}

func parseControllersInDirectory(dirPath, controllerType string, result map[string][]string) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") {
			continue
		}

		controllerName := strings.TrimSuffix(file.Name(), ".go")
		filePath := filepath.Join(dirPath, file.Name())
		
		functions, err := parseControllerFunctions(filePath)
		if err != nil {
			log.Printf("Error parsing controller %s: %v", filePath, err)
			continue
		}

		result[controllerName] = functions
	}
}

func generateSmartRoutes(controllerName string, apiFunctions, restFunctions []string) []Route {
	routes := []Route{}
	
	// Collect all unique functions
	allFunctions := make(map[string]bool)
	for _, fn := range apiFunctions {
		allFunctions[fn] = true
	}
	for _, fn := range restFunctions {
		allFunctions[fn] = true
	}

	// Generate routes for each function
	for funcName := range allFunctions {
		// Check if function exists in API controller first
		var controllerType string
		if contains(apiFunctions, funcName) {
			controllerType = "api"
		} else if contains(restFunctions, funcName) {
			controllerType = "rest"
		} else {
			continue // Should not happen
		}

		route := generateRouteForFunction(controllerName, controllerType, funcName)
		routes = append(routes, route)
	}

	return routes
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func generateRouteForFunction(controllerName, controllerType, funcName string) Route {
	controllerClass := controllerType + "." + strings.Title(controllerName) + "Controller"
	controllerBase := strings.Title(controllerName)
	funcLower := strings.ToLower(funcName)

	// Determine HTTP method and URL based on function name
	if funcName == "Insert" || funcName == "Create" {
		return Route{
			Method:         "Post",
			URL:            "/" + strings.ToLower(controllerName),
			ParamCode:      generateParamCode(controllerName, "Post"),
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       "item_",
			NeedsBodyParser: true,
		}
	} else if funcName == "Update" {
		return Route{
			Method:         "Put",
			URL:            "/" + strings.ToLower(controllerName),
			ParamCode:      generateParamCode(controllerName, "Put"),
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       "item_",
			NeedsBodyParser: true,
		}
	} else if funcName == "Delete" {
		return Route{
			Method:         "Delete",
			URL:            "/" + strings.ToLower(controllerName),
			ParamCode:      generateParamCode(controllerName, "Delete"),
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       "item_",
			NeedsBodyParser: true,
		}
	} else if funcName == "Get" || funcName == "Read" {
		return Route{
			Method:         "Get",
			URL:            "/" + strings.ToLower(controllerName) + "/:id",
			ParamCode:      generateParamCode(controllerName, "Get"),
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       "id_",
		}
	} else if funcName == "Index" || funcName == "List" {
		return Route{
			Method:         "Get",
			URL:            "/" + strings.ToLower(controllerName),
			ParamCode:      generateParamCode(controllerName, "Index"),
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       "page_, pagesize_",
		}
	} else if strings.HasPrefix(funcLower, "getby") {
		param := strings.ToLower(funcName[5:])
		return Route{
			Method:         "Get",
			URL:            "/" + strings.ToLower(controllerName) + "/get/" + param + "/:" + param,
			ParamCode:      generateParamCode(param, "Get"),
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       param + "_",
		}
	} else if strings.HasPrefix(funcLower, "findby") {
		param := strings.ToLower(funcName[6:])
		return Route{
			Method:         "Get",
			URL:            "/" + strings.ToLower(controllerName) + "/find/" + param + "/:" + param,
			ParamCode:      generateParamCode(param, "Get"),
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       param + "_",
		}
	} else {
		// Generic function
		return Route{
			Method:         "Post",
			URL:            "/" + strings.ToLower(controllerName) + "/" + strings.ToLower(funcName),
			ParamCode:      "",
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       "",
		}
	}
}

func analyzeControllerDirectory(dirPath, controllerType string) ([]Route, error) {
	routes := []Route{}
	
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return routes, err
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") {
			continue
		}

		controllerName := strings.TrimSuffix(file.Name(), ".go")
		filePath := filepath.Join(dirPath, file.Name())
		
		// Parse controller file to find functions
		functions, err := parseControllerFunctions(filePath)
		if err != nil {
			log.Printf("Error parsing controller %s: %v", filePath, err)
			continue
		}

		// Generate routes based on found functions
		controllerRoutes := generateRoutesFromFunctions(controllerName, controllerType, functions)
		routes = append(routes, controllerRoutes...)
	}

	return routes, nil
}

func parseControllerFunctions(filePath string) ([]string, error) {
	functions := []string{}
	
	file, err := os.Open(filePath)
	if err != nil {
		return functions, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Look for function definitions like: func (c *Controller) FunctionName(
		if strings.HasPrefix(line, "func (") && strings.Contains(line, ")") {
			// Extract function name
			parts := strings.Split(line, ")")
			if len(parts) >= 2 {
				funcPart := strings.TrimSpace(parts[1])
				if idx := strings.Index(funcPart, "("); idx > 0 {
					funcName := strings.TrimSpace(funcPart[:idx])
					if funcName != "" && !strings.HasPrefix(funcName, "Init") && !strings.HasPrefix(funcName, "Close") {
						functions = append(functions, funcName)
					}
				}
			}
		}
	}

	return functions, scanner.Err()
}

func generateRoutesFromFunctions(controllerName, controllerType string, functions []string) []Route {
	routes := []Route{}
	controllerClass := controllerType + "." + strings.Title(controllerName) + "Controller"
	controllerBase := strings.Title(controllerName)

	for _, funcName := range functions {
		funcLower := strings.ToLower(funcName)
		
		var route Route
		
		// Determine HTTP method and URL based on function name
		if funcName == "Insert" || funcName == "Create" {
			route = Route{
				Method:         "Post",
				URL:            "/" + strings.ToLower(controllerName),
				ParamCode:      generateParamCode(controllerName, "Post"),
				ControllerName: controllerClass,
				ControllerBase: controllerBase,
				FuncName:       funcName,
				ParamStr:       "item_",
				NeedsBodyParser: true,
			}
		} else if funcName == "Update" {
			route = Route{
				Method:         "Put",
				URL:            "/" + strings.ToLower(controllerName),
				ParamCode:      generateParamCode(controllerName, "Put"),
				ControllerName: controllerClass,
				ControllerBase: controllerBase,
				FuncName:       funcName,
				ParamStr:       "item_",
				NeedsBodyParser: true,
			}
		} else if funcName == "Delete" {
			route = Route{
				Method:         "Delete",
				URL:            "/" + strings.ToLower(controllerName),
				ParamCode:      generateParamCode(controllerName, "Delete"),
				ControllerName: controllerClass,
				ControllerBase: controllerBase,
				FuncName:       funcName,
				ParamStr:       "item_",
				NeedsBodyParser: true,
			}
		} else if funcName == "Get" || funcName == "Read" {
			route = Route{
				Method:         "Get",
				URL:            "/" + strings.ToLower(controllerName) + "/:id",
				ParamCode:      generateParamCode(controllerName, "Get"),
				ControllerName: controllerClass,
				ControllerBase: controllerBase,
				FuncName:       funcName,
				ParamStr:       "id_",
			}
		} else if funcName == "Index" || funcName == "List" {
			route = Route{
				Method:         "Get",
				URL:            "/" + strings.ToLower(controllerName),
				ParamCode:      generateParamCode(controllerName, "Index"),
				ControllerName: controllerClass,
				ControllerBase: controllerBase,
				FuncName:       funcName,
				ParamStr:       "page_, pagesize_",
			}
		} else if strings.HasPrefix(funcLower, "getby") {
			param := strings.ToLower(funcName[5:])
			route = Route{
				Method:         "Get",
				URL:            "/" + strings.ToLower(controllerName) + "/get/" + param + "/:" + param,
				ParamCode:      generateParamCode(param, "Get"),
				ControllerName: controllerClass,
				ControllerBase: controllerBase,
				FuncName:       funcName,
				ParamStr:       param + "_",
			}
		} else if strings.HasPrefix(funcLower, "findby") {
			param := strings.ToLower(funcName[6:])
			route = Route{
				Method:         "Get",
				URL:            "/" + strings.ToLower(controllerName) + "/find/" + param + "/:" + param,
				ParamCode:      generateParamCode(param, "Get"),
				ControllerName: controllerClass,
				ControllerBase: controllerBase,
				FuncName:       funcName,
				ParamStr:       param + "_",
			}
		} else {
			// Generic function
			route = Route{
				Method:         "Post",
				URL:            "/" + strings.ToLower(controllerName) + "/" + strings.ToLower(funcName),
				ParamCode:      "",
				ControllerName: controllerClass,
				ControllerBase: controllerBase,
				FuncName:       funcName,
				ParamStr:       "",
			}
		}
		
		routes = append(routes, route)
	}

	return routes
}

func convertRestToApiRoute(route Route) Route {
	// Convert REST route to API route format
	route.ControllerName = strings.Replace(route.ControllerName, "rest.", "api.", 1)
	return route
}

func generateParamCode(controllerName, method string) string {
	if method == "Post" || method == "Put" || method == "Delete" {
		return fmt.Sprintf("\t\t\titem_ := &models.%s{}\n\t\t\terr := c.BodyParser(item_)\n\t\t\tif err != nil {\n\t\t\t    log.Error().Msg(err.Error())\n\t\t\t}", strings.Title(controllerName))
	}
	if method == "Get" {
		return "\t\t\tid_, _ := strconv.ParseInt(c.Params(\"id\"), 10, 64)"
	}
	if method == "Index" {
		return "\t\t\tpage_, _ := strconv.Atoi(c.Query(\"page\"))\n\t\t\tpagesize_, _ := strconv.Atoi(c.Query(\"pagesize\"))"
	}
	return ""
}