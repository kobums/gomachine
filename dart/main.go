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

	// dartString 은 Dart 작은따옴표 문자열 리터럴이다 (\, ', $ 이스케이프).
	views.AddGlobal("dartString", func(str string) string {
		r := strings.NewReplacer("\\", "\\\\", "'", "\\'", "$", "\\$", "\n", "\\n", "\r", "\\r")
		return "'" + r.Replace(str) + "'"
	})

	// enumZero 는 코드 0 인 값 이름이다 (NOT NULL enum 필드 기본값).
	views.AddGlobal("enumZero", func(typeName string, enums []util.EnumData) string {
		for _, e := range enums {
			if e.Name == typeName {
				return e.Zero
			}
		}
		return ""
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
		raws, err := util.QueryColumns(db, cnf.Database, strings.ToLower(str)+"_tb")
		if err != nil {
			log.Println(err)
			return columns
		}
		for _, raw := range raws {
			columns = append(columns, util.NewColumnFromRaw(str+"_tb", raw, gpa, cnf))
		}
		return columns
	})

	// dartField 는 Dart 필드 / 생성자 매개변수 이름이다 (컬럼 Name 소문자, 예약어나 클래스 멤버와 겹치면 뒤에 _).
	views.AddGlobal("dartField", func(col util.Column) string { return DartIdent(strings.ToLower(col.Name)) })

	// JSON key 는 모든 언어가 util.JSONKey 를 쓴다 (createddate, created_at → createdAt, userId → userid).
	views.AddGlobal("jsonKey", func(col util.Column) string { return util.JSONKey(col) })

	// _ / 대소문자만 다른 컬럼은 Go / TypeScript 와 같은 규칙으로 뒤 컬럼 이름에 번호를 붙인다.
	items = util.DedupeColumns(tableName, items)

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
	// id 컬럼이 있으면 수정/삭제를 /{table}/{id} 로 부른다 (Go, Spring 모두 있는 경로).
	hasId := false
	// 엔티티에 item 필드가 있으면 Go 래퍼({item})와 헷갈리므로 id 가 있는 응답은 엔티티로 본다.
	hasItemKey := false
	for _, item := range items {
		if item.Name == "Id" {
			hasId = true
		}
		if util.JSONKey(item) == "item" {
			hasItemKey = true
		}
	}
	v.Set("hasId", hasId)
	// fromJson 에서 쓰는 너그러운 변환 함수 (쓰는 것만 만든다)
	uses := map[string]bool{}
	for _, item := range items {
		switch item.Type {
		case "bool", "int", "double", "String":
			uses[item.Type] = true
		default:
			uses["int"] = true // enum 코드
		}
	}
	v.Set("usesBool", uses["bool"])
	v.Set("usesInt", uses["int"])
	v.Set("usesDouble", uses["double"])
	v.Set("usesString", uses["String"])
	// view 는 읽기 전용이라 insert/update/delete 를 만들지 않는다 (Go / Spring 에 경로가 없다).
	v.Set("isView", util.GetTableType(tableName) == "view")
	v.Set("hasItemKey", hasItemKey)
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
		// 백엔드와 같이 테이블 / 컬럼을 찾지 못한 조인은 건너뛴다.
		for _, rj := range util.ResolveJoins(tableName, items, gpa.Join) {
			join := rj.GpaJoin
			join.Prefix = util.TablePrefixOf(rj.Table, cnf.Database, db)
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
			// 코드는 data 위치 (Go / Kotlin / TypeScript 와 같다). none(0) 은 data[0] 이 비어 있을 때만 만든다.
			used := map[string]bool{"none": true}
			enumData.HasNone = len(mapItem.Data) > 0 && mapItem.Data[0] == ""
			if enumData.HasNone {
				enumData.Zero = "none"
			}

			for idx, dataStr := range mapItem.Data {
				if dataStr == "" {
					continue
				}
				enumValue := util.EnumValue{
					Key:   util.DedupeIdent(dartEnumIdent(util.SanitizeIdent(util.EnumKey(dataStr), "v")), used),
					Code:  util.EnumCode(idx),
					Label: util.EnumLabel(dataStr),
				}
				if idx == 0 {
					enumData.Zero = enumValue.Key
				}
				enumData.Data = append(enumData.Data, enumValue)
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

// dartReserved 는 Dart 예약어와 생성 클래스의 멤버 이름이다. 필드 / enum 값 이름으로 쓰면 뒤에 _ 를 붙인다.
var dartReserved = map[string]bool{
	"assert": true, "break": true, "case": true, "catch": true, "class": true, "const": true, "continue": true, "default": true,
	"do": true, "else": true, "enum": true, "extends": true, "false": true, "final": true, "finally": true, "for": true,
	"if": true, "in": true, "is": true, "new": true, "null": true, "rethrow": true, "return": true, "super": true,
	"switch": true, "this": true, "throw": true, "true": true, "try": true, "var": true, "void": true, "while": true,
	"with": true, "await": true, "yield": true,
	// 생성 클래스 / Object 멤버
	"extra": true, "clone": true, "toJson": true, "fromJson": true, "hashCode": true, "runtimeType": true,
	"toString": true, "noSuchMethod": true,
}

// dartEnumReserved 는 enum 값 이름으로 쓸 수 없는 enum 멤버 이름이다.
var dartEnumReserved = map[string]bool{"values": true, "index": true, "name": true, "code": true, "label": true, "fromCode": true}

// DartIdent 는 Dart 예약어나 생성 클래스 멤버와 겹치는 이름이면 뒤에 _ 를 붙인다.
func DartIdent(name string) string {
	if dartReserved[name] {
		return name + "_"
	}
	return name
}

// dartEnumIdent 는 enum 값 이름이다. 예약어나 enum 멤버(values, index, code ...)와 겹치면 뒤에 _ 를 붙인다.
func dartEnumIdent(name string) string {
	if dartReserved[name] || dartEnumReserved[name] {
		return name + "_"
	}
	return name
}
