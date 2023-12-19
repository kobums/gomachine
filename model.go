package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"gomachine/config"
	"os"

	"strings"
	"time"

	"github.com/CloudyKit/jet/v6"
	_ "github.com/go-sql-driver/mysql"
	log "github.com/sirupsen/logrus"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)


type Column struct {
	Name         string
	Column       string
	Type         string
	OriginalType string
	Prefix       string
}

func WriteFile(filename string, content string) error {
	return os.WriteFile(filename, []byte(content), 0644)
}

func GetConnection() *sql.DB {
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
	path := fmt.Sprintf("%v/bin/buildtool", os.Getenv("HOME"))

	log.Println(path)
	db := GetConnection()
	defer db.Close()

	query := fmt.Sprintf("select table_name from information_schema.tables where table_schema = '%v'", config.Database)
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
		println(tableName)

		readColumn(name, db)
		tables = append(tables, tableName)
	}
}

func getTableName(name string) string {
	strs := strings.Split(name, "_")

	if len(strs) < 2 {
		return name
	}

	return strs[0]
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


func readColumn(tableName string, db *sql.DB) {
	query := "select column_name as column_name, data_type as data_type from information_schema.columns where table_schema = '" + config.Database + "' and table_name = '" + tableName + "'"
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

		column := Column{Name: getName(name), Column: name, Type: getType(getName(name), typeid), OriginalType: typeid}
		columns = append(columns, column)
	}

	process(tableName, columns, db)
}

func getType(name string, t string) string {
	if config.Language == "dart" || config.Language == "flutter" {
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

func process(tableName string, items []Column, db *sql.DB) {
	path := fmt.Sprintf("%v/bin/buildtool", os.Getenv("HOME"))

	var views = jet.NewSet(jet.NewOSFileSystemLoader(path), jet.InDevelopmentMode())

	views.AddGlobal("items", items)
	
	v := make(jet.VarMap)

	v.Set("packageName", config.Database)
	v.Set("tableName", tableName)
	v.Set("name", cases.Title(language.Und).String(getTableName(tableName)))
	v.Set("partName", getTableName((tableName)))
	v.Set("consts", make([]string, 0))
	v.Set("funcs", make([]string, 0))

	var templateBuffers []bytes.Buffer

	modelFilename := "model.jet"
	if config.Language == "dart" || config.Language == "flutter" {
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
			filename := fmt.Sprintf("../gym/gym/lib/%s_%s.dart", getTableName(tableName), arr[i])
			log.Printf("write file: %s\n", filename)
			WriteFile("../gym/gym/lib/"+arr[i]+"/"+getTableName(tableName)+"_"+arr[i]+".dart", b.String())
		}
	}
}