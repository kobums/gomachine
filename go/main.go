package gocodegen

import (
	"bytes"
	"database/sql"
	"fmt"
	"gomachine/config"
	"gomachine/util"
	"os"
	"strings"
	"unicode"

	"github.com/CloudyKit/jet/v6"
	log "github.com/sirupsen/logrus"
)

func ProcessGo(packageName string, tableName string, prefix string, items []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
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
		tokens := util.Split(str)
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
			if util.GetName(strings.ToLower(str)) == v.Name {
				return false
			}
		}
		return true
	})

	views.AddGlobal("compareColumn", func(str string, cols []config.GpaCompare) string {
		for _, v := range cols {
			if util.GetName(strings.ToLower(str)) == v.Name {
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

	views.AddGlobal("columns", func(str string) []util.Column {
		query := "select column_name as column_name, data_type as data_type from information_schema.columns where table_schema = '" + packageName + "' and table_name = '" + strings.ToLower(str) + "_tb'"
		rows, err := db.Query(query)

		if err != nil {
			log.Println(err)
		}

		columns := make([]util.Column, 0)
		for rows.Next() {
			var name string
			var typeid string

			err := rows.Scan(&name, &typeid)
			if err != nil {
				log.Println(err)
			}

			prefix := util.GetPrefix(name)
			column := util.Column{Name: strings.Title(util.GetName(name)), Column: name, Type: util.GetType(util.GetTableName(str), util.GetName(name), typeid, gpa, cnf), Prefix: prefix}
			columns = append(columns, column)
		}

		return columns
	})

	v := make(jet.VarMap)
	v.Set("version", version)
	v.Set("packageName", packageName)
	v.Set("type", util.GetTableType(tableName))
	v.Set("adminLevel", cnf.AdminLevel)
	v.Set("name", strings.Title(util.GetTableName(tableName)))
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
			gpa.Join[i].Prefix = util.TablePrefix(gpa.Join[i].Name, packageName, db)
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

		funcs := make([]util.Func, 0)

		for _, item := range gpa.Method {
			tokens := util.Split(item)

			wheres := make([]util.Where, 0)
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

					where := util.Where{Column: column, Type: typename, Compare: compare}
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

					where := util.Where{Column: column, Type: typename, Compare: compare}
					wheres = append(wheres, where)
				}
			}

			fn := util.Func{Name: item, Wheres: wheres}
			funcs = append(funcs, fn)
		}

		v.Set("funcs", funcs)
		v.Set("imports", util.Unique(funcs))
	}

	// Generate Go model file
	var b bytes.Buffer
	modelFilename := "go/model.jet"
	t, err := views.GetTemplate(modelFilename)
	if err == nil {
		if err = t.Execute(&b, v, nil); err != nil {
			log.Println(err)
		}
	} else {
		log.Println("error ========================")
		log.Println(err)
		log.Println("error ========================")
	}

	if cnf.Language == "go" {
		modelFile := cnf.GoModelFilePath + "/models/" + util.GetTableName(tableName) + ".go"
		log.Printf("=== PROCESSING GO MODEL FILE ===")
		log.Printf("Table name: %s", tableName)
		log.Printf("Model file path: %s", modelFile)
		log.Printf("Template content length: %d", b.Len())

		if err := util.WriteFile(modelFile, b.String()); err != nil {
			log.Printf("CRITICAL ERROR: Failed to write model file %s: %v", modelFile, err)
		} else {
			log.Printf("SUCCESS: Model file written successfully: %s", modelFile)
		}
	}

	// Generate REST controller
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

	restFile := cnf.GoModelFilePath + "/controllers/rest/" + util.GetTableName(tableName) + ".go"
	log.Printf("=== PROCESSING GO REST CONTROLLER FILE ===")
	log.Printf("REST controller file path: %s", restFile)
	log.Printf("Template content length: %d", b2.Len())

	if err := util.WriteFile(restFile, b2.String()); err != nil {
		log.Printf("CRITICAL ERROR: Failed to write rest controller file %s: %v", restFile, err)
	} else {
		log.Printf("SUCCESS: REST controller file written successfully: %s", restFile)
	}

	// Generate const file
	v2 := make(jet.VarMap)
	v2.Set("version", version)
	v2.Set("name", strings.Title(util.GetTableName(tableName)))
	v2.Set("auth", auth)
	v2.Set("items", items)
	if gpa == nil {
		v2.Set("consts", make([]string, 0))
		v2.Set("methods", make([]string, 0))
		v2.Set("funcs", make([]string, 0))
	} else {
		v2.Set("consts", gpa.Map)
	}

	var b3 bytes.Buffer
	t, err = views.GetTemplate("go/const.jet")
	if err == nil {
		if err = t.Execute(&b3, v2, nil); err != nil {
			log.Printf("CRITICAL ERROR: Const template execution failed: %v", err)
		}
	} else {
		log.Printf("CRITICAL ERROR: Failed to load const template: %v", err)
	}

	constDir := cnf.GoModelFilePath + "/models/" + util.GetTableName(tableName)
	log.Printf("Creating const directory: %s", constDir)
	if err := os.MkdirAll(constDir, 0755); err != nil {
		log.Printf("Failed to create const directory %s: %v", constDir, err)
	}

	constFile := constDir + "/" + util.GetTableName(tableName) + ".go"
	log.Printf("=== PROCESSING GO CONST FILE ===")
	log.Printf("Const file path: %s", constFile)
	log.Printf("Template content length: %d", b3.Len())

	if err := util.WriteFile(constFile, b3.String()); err != nil {
		log.Printf("CRITICAL ERROR: Failed to write const file %s: %v", constFile, err)
	} else {
		log.Printf("SUCCESS: Const file written successfully: %s", constFile)
	}
}