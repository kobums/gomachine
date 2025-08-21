package util

import (
	"database/sql"
	"gomachine/config"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	log "github.com/sirupsen/logrus"
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
	Primary      bool
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

func GetTableName(name string) string {
	strs := strings.Split(name, "_")

	if len(strs) < 2 {
		return name
	}

	return strs[0]
}

func GetTableType(name string) string {
	strs := strings.Split(name, "_")

	if len(strs) < 2 {
		return "table"
	}

	if strs[1] == "vw" {
		return "view"
	}

	return "table"
}

func GetName(name string) string {
	strs := strings.Split(name, "_")

	if len(strs) < 2 {
		return name
	}

	return strs[1]
}

func GetPrefix(name string) string {
	strs := strings.Split(name, "_")

	if len(strs) < 2 {
		return ""
	}

	return strs[0]
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

func GetType(tableName string, name string, t string, gpa *config.Gpa, cnf config.ModelConfig) string {
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

func TablePrefix(str string, packageName string, db *sql.DB) string {
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

		prefix := GetPrefix(name)
		return prefix
	}

	return ""
}

func Unique(items []Func) []string {
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