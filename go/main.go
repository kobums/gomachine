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

type RouteParam struct {
	Name      string
	Type      string
	ParamType string // "path", "query", "body"
}

type Route struct {
	Method         string
	URL            string
	FuncName       string
	ControllerName string
	ParamCode      string
	ParamStr       string
	Params         []RouteParam
}

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

	views.AddGlobal("joinVar", func(join config.GpaJoin) string {
		if join.Alias != "" {
			return strings.ToLower(join.Alias)
		}
		return strings.ToLower(join.Name)
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
		modelFile := cnf.GoModelFilePath + "models/" + util.GetTableName(tableName) + ".go"

		if err := util.WriteFile(modelFile, b.String()); err != nil {
			log.Printf("ERROR: Failed to write model file %s: %v", modelFile, err)
		} else {
			log.Printf("Generated Go: %s", modelFile)
		}
	}

	// Generate REST controller
	var b2 bytes.Buffer
	t2, err := views.GetTemplate("go/rest.jet")
	if err == nil {
		if err = t2.Execute(&b2, v, nil); err != nil {
			log.Printf("ERROR: REST template execution failed: %v", err)
		}
	} else {
		log.Printf("ERROR: Failed to load REST template: %v", err)
	}

	restFile := cnf.GoModelFilePath + "controllers/rest/" + util.GetTableName(tableName) + ".go"

	if err := util.WriteFile(restFile, b2.String()); err != nil {
		log.Printf("ERROR: Failed to write rest controller file %s: %v", restFile, err)
	} else {
		log.Printf("Generated Go: %s", restFile)
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
			log.Printf("ERROR: Const template execution failed: %v", err)
		}
	} else {
		log.Printf("ERROR: Failed to load const template: %v", err)
	}

	constDir := cnf.GoModelFilePath + "models/" + util.GetTableName(tableName)
	if err := os.MkdirAll(constDir, 0755); err != nil {
		log.Printf("ERROR: Failed to create const directory %s: %v", constDir, err)
	}

	constFile := constDir + "/" + util.GetTableName(tableName) + ".go"

	if err := util.WriteFile(constFile, b3.String()); err != nil {
		log.Printf("ERROR: Failed to write const file %s: %v", constFile, err)
	} else {
		log.Printf("Generated Go: %s", constFile)
	}

	// Generate domain router file
	generateDomainRouter(views, packageName, tableName, items, gpa, cnf)
}

func generateDomainRouter(views *jet.Set, packageName string, tableName string, items []util.Column, gpa *config.Gpa, cnf config.ModelConfig) {
	domainName := util.GetTableName(tableName)

	routes := make([]Route, 0)
	controllerName := "rest." + strings.Title(domainName) + "Controller"

	// Add standard CRUD routes
	// GET /domain - Index
	routes = append(routes, Route{
		Method:         "Get",
		URL:            "/" + domainName,
		FuncName:       "Index",
		ControllerName: controllerName,
		ParamCode:      "\t\tpage_, _ := strconv.Atoi(c.Query(\"page\"))\n\t\tpagesize_, _ := strconv.Atoi(c.Query(\"pagesize\"))",
		ParamStr:       "page_, pagesize_",
	})

	// GET /domain/:id - Read
	routes = append(routes, Route{
		Method:         "Get",
		URL:            "/" + domainName + "/:id",
		FuncName:       "Read",
		ControllerName: controllerName,
		ParamCode:      "\t\tid_, _ := strconv.ParseInt(c.Params(\"id\"), 10, 64)",
		ParamStr:       "id_",
	})

	modelNameForUpdate := strings.Title(domainName)
	if domainName == "user" {
		modelNameForUpdate = "UserUpdate"
	}

	// POST /domain - Insert
	routes = append(routes, Route{
		Method:         "Post",
		URL:            "/" + domainName,
		FuncName:       "Insert",
		ControllerName: controllerName,
		ParamCode:      fmt.Sprintf("\t\titem_ := &models.%s{}\n\t\terr := c.BodyParser(item_)\n\t\tif err != nil {\n\t\t    log.Error().Msg(err.Error())\n\t\t}", modelNameForUpdate),
		ParamStr:       "item_",
	})

	// POST /domain/batch - Insertbatch
	routes = append(routes, Route{
		Method:         "Post",
		URL:            "/" + domainName + "/batch",
		FuncName:       "Insertbatch",
		ControllerName: controllerName,
		ParamCode:      fmt.Sprintf("\t\tvar items_ *[]models.%s\n\t\titems__ref := &items_\n\t\terr := c.BodyParser(items__ref)\n\t\tif err != nil {\n\t\t    log.Error().Msg(err.Error())\n\t\t}", modelNameForUpdate),
		ParamStr:       "items_",
	})

	// POST /domain/count - Count
	routes = append(routes, Route{
		Method:         "Post",
		URL:            "/" + domainName + "/count",
		FuncName:       "Count",
		ControllerName: controllerName,
		ParamCode:      "",
		ParamStr:       "",
	})

	// PUT /domain - Update
	routes = append(routes, Route{
		Method:         "Put",
		URL:            "/" + domainName,
		FuncName:       "Update",
		ControllerName: controllerName,
		ParamCode:      fmt.Sprintf("\t\titem_ := &models.%s{}\n\t\terr := c.BodyParser(item_)\n\t\tif err != nil {\n\t\t    log.Error().Msg(err.Error())\n\t\t}", modelNameForUpdate),
		ParamStr:       "item_",
	})

	// DELETE /domain - Delete
	routes = append(routes, Route{
		Method:         "Delete",
		URL:            "/" + domainName,
		FuncName:       "Delete",
		ControllerName: controllerName,
		ParamCode:      fmt.Sprintf("\t\titem_ := &models.%s{}\n\t\terr := c.BodyParser(item_)\n\t\tif err != nil {\n\t\t    log.Error().Msg(err.Error())\n\t\t}", strings.Title(domainName)),
		ParamStr:       "item_",
	})

	// DELETE /domain/batch - Deletebatch
	routes = append(routes, Route{
		Method:         "Delete",
		URL:            "/" + domainName + "/batch",
		FuncName:       "Deletebatch",
		ControllerName: controllerName,
		ParamCode:      fmt.Sprintf("\t\titem_ := &[]models.%s{}\n\t\terr := c.BodyParser(item_)\n\t\tif err != nil {\n\t\t    log.Error().Msg(err.Error())\n\t\t}", strings.Title(domainName)),
		ParamStr:       "item_",
	})

	needsJson := false
	enumImports := make([]string, 0)

	// Add custom method routes from GPA
	if gpa != nil && gpa.Method != nil {
		for _, method := range gpa.Method {
			if strings.HasPrefix(strings.ToLower(method), "update") && strings.Contains(strings.ToLower(method), "by") {
				needsJson = true
			}

			route := generateRouteFromMethod(method, domainName, items, controllerName)
			if route != nil {
				routes = append(routes, *route)
			}
		}
	}

	for _, item := range items {
		if strings.Contains(item.Type, ".") {
			parts := strings.Split(item.Type, ".")
			enumPkg := parts[0]

			found := false
			for _, e := range enumImports {
				if e == enumPkg {
					found = true
					break
				}
			}
			if !found {
				enumImports = append(enumImports, enumPkg)
			}
		}
	}

	v := make(jet.VarMap)
	v.Set("packageName", packageName)
	v.Set("domainName", domainName)
	v.Set("routes", routes)
	v.Set("controllerType", "rest")
	v.Set("needsLog", true)
	v.Set("needsJson", needsJson)
	v.Set("enumImports", enumImports)

	var b bytes.Buffer
	t, err := views.GetTemplate("go/domain_router.jet")
	if err != nil {
		log.Printf("ERROR: Failed to load domain router template: %v", err)
		return
	}

	if err = t.Execute(&b, v, nil); err != nil {
		log.Printf("ERROR: Domain router template execution failed: %v", err)
		return
	}

	routerDir := cnf.GoModelFilePath + "router/routers"
	if err := os.MkdirAll(routerDir, 0755); err != nil {
		log.Printf("ERROR: Failed to create router directory %s: %v", routerDir, err)
		return
	}

	routerFile := routerDir + "/" + domainName + ".go"
	if err := util.WriteFile(routerFile, b.String()); err != nil {
		log.Printf("ERROR: Failed to write router file %s: %v", routerFile, err)
	} else {
		log.Printf("Generated Go: %s", routerFile)
	}
}

func generateRouteFromMethod(method string, domainName string, items []util.Column, controllerName string) *Route {
	methodLower := strings.ToLower(method)

	// GetBy methods
	if strings.HasPrefix(methodLower, "getby") {
		fieldName := method[5:]
		fieldLower := strings.ToLower(fieldName)
		paramType := getParamType(fieldName, items)

		return &Route{
			Method:         "Get",
			URL:            fmt.Sprintf("/%s/get/%s/:%s", domainName, fieldLower, fieldLower),
			FuncName:       method,
			ControllerName: controllerName,
			ParamCode:      generateParamCode(fieldName, fieldLower, paramType),
			ParamStr:       fieldLower + "_",
		}
	}

	// FindBy methods
	if strings.HasPrefix(methodLower, "findby") {
		fieldName := method[6:]
		fieldLower := strings.ToLower(fieldName)
		paramType := getParamType(fieldName, items)

		return &Route{
			Method:         "Get",
			URL:            fmt.Sprintf("/%s/find/%s/:%s", domainName, fieldLower, fieldLower),
			FuncName:       method,
			ControllerName: controllerName,
			ParamCode:      generateParamCode(fieldName, fieldLower, paramType),
			ParamStr:       fieldLower + "_",
		}
	}

	// CountBy methods
	if strings.HasPrefix(methodLower, "countby") {
		fieldName := method[7:]
		fieldLower := strings.ToLower(fieldName)
		paramType := getParamType(fieldName, items)

		return &Route{
			Method:         "Get",
			URL:            fmt.Sprintf("/%s/count/%s/:%s", domainName, fieldLower, fieldLower),
			FuncName:       method,
			ControllerName: controllerName,
			ParamCode:      generateParamCode(fieldName, fieldLower, paramType),
			ParamStr:       fieldLower + "_",
		}
	}

	// UpdateXxxByYyy methods
	if strings.HasPrefix(methodLower, "update") && strings.Contains(methodLower, "by") {
		parts := strings.Split(method[6:], "By")
		if len(parts) == 2 {
			updateField := parts[0]
			whereField := parts[1]
			updateFieldLower := strings.ToLower(updateField)
			whereFieldLower := strings.ToLower(whereField)

			updateType := getParamType(updateField, items)
			whereType := getParamType(whereField, items)

			paramCode := fmt.Sprintf("\t\tvar results map[string]interface{}\n\t\tjsonData := c.Body()\n\t\tjsonErr := json.Unmarshal(jsonData, &results)\n\t\tif jsonErr != nil {\n\t\t    log.Error().Msg(jsonErr.Error())\n\t\t}\n")
			paramCode += generateUpdateParamExtract(updateFieldLower, updateType)
			paramCode += generateUpdateParamExtract(whereFieldLower, whereType)

			return &Route{
				Method:         "Put",
				URL:            fmt.Sprintf("/%s/%s/%s", domainName, updateFieldLower, whereFieldLower),
				FuncName:       method,
				ControllerName: controllerName,
				ParamCode:      paramCode,
				ParamStr:       fmt.Sprintf("%s_, %s_", updateFieldLower, whereFieldLower),
			}
		}
	}

	return nil
}

func getParamType(fieldName string, items []util.Column) string {
	for _, item := range items {
		if strings.EqualFold(item.Name, fieldName) {
			return item.Type
		}
	}
	return "string"
}

func generateParamCode(fieldName string, fieldLower string, paramType string) string {
	// Check if it's an enum type (contains ".")
	if strings.Contains(paramType, ".") {
		// Enum type from models package
		parts := strings.Split(paramType, ".")
		enumPkg := parts[0]
		enumType := parts[1]
		return fmt.Sprintf("\t\tvar %s_ %s.%s\n\t\t%s__, _ := strconv.Atoi(c.Params(\"%s\"))\n\t\t%s_ = %s.%s(%s__)",
			fieldLower, enumPkg, enumType, fieldLower, fieldLower, fieldLower, enumPkg, enumType, fieldLower)
	}

	switch paramType {
	case "int", "int32":
		return fmt.Sprintf("\t\t%s_, _ := strconv.Atoi(c.Params(\"%s\"))", fieldLower, fieldLower)
	case "int64":
		return fmt.Sprintf("\t\t%s_, _ := strconv.ParseInt(c.Params(\"%s\"), 10, 64)", fieldLower, fieldLower)
	case "bool":
		return fmt.Sprintf("\t\t%s_, _ := strconv.ParseBool(c.Params(\"%s\"))", fieldLower, fieldLower)
	case "float64":
		return fmt.Sprintf("\t\t%s_, _ := strconv.ParseFloat(c.Params(\"%s\"), 64)", fieldLower, fieldLower)
	default:
		return fmt.Sprintf("\t\t%s_ := c.Params(\"%s\")", fieldLower, fieldLower)
	}
}

func generateUpdateParamExtract(fieldLower string, paramType string) string {
	switch paramType {
	case "int", "int32":
		return fmt.Sprintf("\t\tvar %s_ int\n\t\tif v, flag := results[\"%s\"]; flag {\n\t\t\t%s_ = int(v.(float64))\n\t\t}\n", fieldLower, fieldLower, fieldLower)
	case "int64":
		return fmt.Sprintf("\t\tvar %s_ int64\n\t\tif v, flag := results[\"%s\"]; flag {\n\t\t\t%s_ = int64(v.(float64))\n\t\t}\n", fieldLower, fieldLower, fieldLower)
	case "bool":
		return fmt.Sprintf("\t\tvar %s_ bool\n\t\tif v, flag := results[\"%s\"]; flag {\n\t\t\t%s_ = v.(bool)\n\t\t}\n", fieldLower, fieldLower, fieldLower)
	case "float64":
		return fmt.Sprintf("\t\tvar %s_ float64\n\t\tif v, flag := results[\"%s\"]; flag {\n\t\t\t%s_ = v.(float64)\n\t\t}\n", fieldLower, fieldLower, fieldLower)
	default:
		return fmt.Sprintf("\t\tvar %s_ string\n\t\tif v, flag := results[\"%s\"]; flag {\n\t\t\t%s_ = v.(string)\n\t\t}\n", fieldLower, fieldLower, fieldLower)
	}
}