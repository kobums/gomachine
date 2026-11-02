package dartcodegen

import (
	"bytes"
	"database/sql"
	"fmt"
	"gomachine/config"
	"gomachine/util"
	"os"
	"strings"

	"github.com/CloudyKit/jet/v6"
	log "github.com/sirupsen/logrus"
)

func ProcessDart(packageName string, tableName string, prefix string, items []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	path := fmt.Sprintf("%v/bin/buildtool", os.Getenv("HOME"))

	var views = jet.NewSet(jet.NewOSFileSystemLoader(path), jet.InDevelopmentMode())

	views.AddGlobal("title", func(str string) string {
		return strings.Title(str)
	})

	views.AddGlobal("isEnum", func(typeName string, enums []util.EnumData) bool {
		for _, e := range enums {
			if e.Name == typeName {
				return true
			}
		}
		return false
	})

	// 조인 테이블 컬럼 (ordinal_position 순서)
	views.AddGlobal("columns", func(str string) []util.Column {
		columns := make([]util.Column, 0)
		if db == nil {
			return columns
		}
		query := "select column_name as column_name, data_type as data_type from information_schema.columns where table_schema = '" + packageName + "' and table_name = '" + strings.ToLower(str) + "_tb' order by ordinal_position"
		rows, err := db.Query(query)
		if err != nil {
			log.Println(err)
			return columns
		}
		defer rows.Close()

		for rows.Next() {
			var name string
			var typeid string

			if err := rows.Scan(&name, &typeid); err != nil {
				log.Println(err)
				continue
			}

			columns = append(columns, util.NewColumn(str+"_tb", name, typeid, gpa, cnf))
		}

		return columns
	})

	v := make(jet.VarMap)
	v.Set("version", version)
	v.Set("packageName", packageName)
	v.Set("type", util.GetTableType(tableName))
	v.Set("adminLevel", cnf.AdminLevel)
	v.Set("name", util.ModelName(tableName))
	v.Set("tableName", tableName)
	v.Set("prefix", prefix)

	if gpa != nil {
		for i, v := range items {
			for _, v2 := range gpa.Primary {
				if util.MatchColumn(v, v2) {
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
		v.Set("enums", make([]util.EnumData, 0))
	} else {
		joins := make([]config.GpaJoin, 0)
		for _, join := range gpa.Join {
			join.Prefix = util.TablePrefix(join.Name, packageName, db)
			joins = append(joins, join)
		}

		// Process map data into enums for Dart
		enums := make([]util.EnumData, 0)
		for _, mapItem := range gpa.Map {
			enumData := util.EnumData{
				Name: util.CamelCase(mapItem.Name),
				Data: make([]util.EnumValue, 0),
			}
			// Dart enum 값 이름: 식별자로 못 쓰는 key 는 바꾸고, 합성 값 none 과 겹치면 번호를 붙인다.
			used := map[string]bool{"none": true}

			for idx, dataStr := range mapItem.Data {
				if dataStr == "" {
					continue
				}
				parts := strings.Split(dataStr, ":")
				if len(parts) == 2 {
					enumValue := util.EnumValue{
						Key:   util.DedupeIdent(util.SanitizeIdent(parts[0], "v"), used),
						Code:  idx,
						Label: parts[1],
					}
					enumData.Data = append(enumData.Data, enumValue)
				}
			}

			if len(enumData.Data) > 0 {
				log.Printf("Adding enum for table %s: %s with %d values", tableName, enumData.Name, len(enumData.Data))
				enums = append(enums, enumData)
			}
		}
		log.Printf("Total enums for table %s: %d", tableName, len(enums))

		v.Set("consts", gpa.Map)
		v.Set("methods", gpa.Method)
		v.Set("enums", enums)
		primarys := gpa.Primary
		if len(primarys) == 0 {
			primarys = []string{"id"}
		}
		v.Set("primarys", primarys)
		v.Set("joins", joins)
		v.Set("compares", gpa.Compare)
		v.Set("search", gpa.Search)
		v.Set("sessions", gpa.Session)

		funcs := util.ParseFuncs(tableName, gpa.Method, items)

		v.Set("funcs", funcs)
		v.Set("imports", util.Unique(funcs))
	}

	// Only process the model template as it now includes the manager and other necessary parts
	modelFilename := "dart/model.jet"
	t, err := views.GetTemplate(modelFilename)
	if err == nil {
		var b bytes.Buffer
		if err = t.Execute(&b, v, nil); err != nil {
			log.Println("Template execution error:", err)
		} else {
			// Output directly to DartModelFilePath with just the table name
			dartModelPath := cnf.DartModelFilePath
			if dartModelPath == "" {
				dartModelPath = "../gym/app/lib/model/"
			}

			// Ensure directory exists
			if _, err := os.Stat(dartModelPath); os.IsNotExist(err) {
				if err := os.MkdirAll(dartModelPath, 0755); err != nil {
					log.Printf("CRITICAL ERROR: Failed to create dart directory %s: %v", dartModelPath, err)
				}
			}

			dartFile := dartModelPath + strings.ToLower(util.GetTableName(tableName)) + ".dart"

			if err := util.WriteFile(dartFile, b.String()); err != nil {
				log.Printf("ERROR: Failed to write dart file %s: %v", dartFile, err)
			} else {
				log.Printf("Generated Dart: %s", dartFile)
			}
		}
	} else {
		log.Println("error ========================")
		log.Println(err)
		log.Println("error ========================")
	}
}
