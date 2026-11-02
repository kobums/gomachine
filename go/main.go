package gocodegen

import (
	"bytes"
	"database/sql"
	"fmt"
	"gomachine/config"
	"gomachine/util"
	"os"
	"strconv"
	"strings"

	"github.com/CloudyKit/jet/v6"
	log "github.com/sirupsen/logrus"
)

type Route struct {
	Method         string
	URL            string
	FuncName       string
	ControllerName string
	ParamCode      string
	ParamStr       string
}

// GoEnum 은 gpa.map 하나로 만드는 Go 상수 타입이다.
type GoEnum struct {
	Name   string          // 타입 이름 (Status)
	Key    string          // InitExtra 의 Extra 키 (예전과 같이 map name 소문자)
	Field  string          // 이 enum 을 쓰는 모델 필드 이름. 컬럼이 없으면 ""
	Items  []util.EnumItem // 상수 (빈 항목 제외, iota 순서)
	Labels []string        // 표시 이름 문자열 리터럴 (data 순서 그대로)
}

// GoSession 은 세션 값과 모델 컬럼의 매핑이다.
type GoSession struct {
	Key    string // 세션 구조체 필드 (c.Session.{Key|title})
	Field  string // 모델 필드 이름 (User)
	Column string // SQL 컬럼 이름 (접두사 제외: user)
}

func ProcessGo(packageName string, tableName string, prefix string, items []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	path := fmt.Sprintf("%v/bin/buildtool", os.Getenv("HOME"))

	var views = jet.NewSet(jet.NewOSFileSystemLoader(path), jet.InDevelopmentMode())

	views.AddGlobal("title", func(str string) string {
		return strings.Title(str)
	})

	views.AddGlobal("camel", func(str string) string {
		return util.CamelCase(str)
	})

	views.AddGlobal("joinVar", func(join config.GpaJoin) string {
		if join.Alias != "" {
			return strings.ToLower(join.Alias)
		}
		return strings.ToLower(join.Name)
	})

	views.AddGlobal("isProtected", func(str string, cols []string) bool {
		for _, v := range cols {
			if strings.EqualFold(v, str) {
				return true
			}
		}
		return false
	})

	views.AddGlobal("compareColumn", func(str string, cols []config.GpaCompare) string {
		for _, v := range cols {
			if strings.EqualFold(v.Name, str) || strings.EqualFold(strings.ReplaceAll(v.Name, "_", ""), str) {
				return v.Type
			}
		}
		return "="
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

			err := rows.Scan(&name, &typeid)
			if err != nil {
				log.Println(err)
				continue
			}

			columns = append(columns, util.NewColumn(str+"_tb", name, typeid, gpa, cnf))
		}

		return columns
	})

	modelName := util.ModelName(tableName)
	fileName := util.FileName(tableName)
	constPkg := util.GoConstPackage(tableName)
	constClause := util.GoConstPackageClause(tableName)
	isView := util.GetTableType(tableName) == "view"

	// 상수 패키지 import 줄. package 이름과 다른 이름을 쓰면 별칭을 붙인다 (log_tb → logpkg "pkg/models/log").
	constImport := fmt.Sprintf("\"%s/models/%s\"", packageName, fileName)
	if constPkg != constClause {
		constImport = constPkg + " " + constImport
	}

	if gpa != nil {
		for i, v := range items {
			for _, v2 := range gpa.Primary {
				if util.MatchColumn(v, v2) {
					items[i].Primary = true
				}
			}
		}
	}

	// id 컬럼이 없으면 id 기준 메서드(Get/Update/Delete, Read/Update/Delete 핸들러)는 만들지 않는다.
	hasId := false
	orderKey := ""
	for _, item := range items {
		if item.Name == "Id" {
			hasId = true
			orderKey = item.Key
		}
	}
	if orderKey == "" && len(items) > 0 {
		orderKey = items[0].Key
		for _, item := range items {
			if item.Primary {
				orderKey = item.Key
				break
			}
		}
	}
	nonIdItems := make([]util.Column, 0)
	for _, item := range items {
		if item.Name != "Id" {
			nonIdItems = append(nonIdItems, item)
		}
	}

	v := make(jet.VarMap)
	v.Set("version", version)
	v.Set("packageName", packageName)
	v.Set("type", util.GetTableType(tableName))
	v.Set("isView", isView)
	v.Set("adminLevel", cnf.AdminLevel)
	v.Set("name", modelName)
	v.Set("tableName", tableName)
	v.Set("prefix", prefix)
	v.Set("constPkg", constPkg)
	v.Set("constClause", constClause)
	v.Set("constImport", constImport)
	v.Set("hasId", hasId)
	v.Set("orderKey", orderKey)
	v.Set("items", items)
	v.Set("nonIdItems", nonIdItems)
	v.Set("auth", auth)

	passwordColumn := util.PasswordColumn(items)
	protects := util.ProtectedColumns(modelName, items, gpa)
	createdColumn := ""
	createdDefault := false
	for _, item := range items {
		if item.Name == "Createddate" {
			createdColumn = item.Name
			// Insert 에서 현재 시각 문자열을 넣는 것은 문자열 컬럼일 때만 한다.
			createdDefault = item.Type == "string"
		}
	}
	// adminLevel 이 숫자로 설정되어 있고 jwt 인증이면 세션 Level 이 그 이상인 관리자는 보호 컬럼을 바꿀 수 있다.
	adminLevelNum, adminErr := strconv.Atoi(strings.TrimSpace(cnf.AdminLevel))
	adminCheck := auth == "jwt" && adminErr == nil && len(protects) > 0

	v.Set("inputName", inputModelName(modelName, items))
	v.Set("passwordColumn", passwordColumn)
	v.Set("createdColumn", createdColumn)
	v.Set("createdDefault", createdDefault)
	v.Set("protects", protects)
	v.Set("adminCheck", adminCheck)
	v.Set("adminLevelNum", adminLevelNum)

	consts := make([]GoEnum, 0)
	joins := make([]config.GpaJoin, 0)
	compares := make([]config.GpaCompare, 0)
	sessions := make([]GoSession, 0)
	funcs := make([]util.Func, 0)
	search := false

	if gpa != nil {
		for _, m := range gpa.Map {
			e := GoEnum{Name: util.CamelCase(m.Name), Key: strings.ToLower(m.Name)}
			e.Items = util.GoEnumItems(e.Name, m.Data)
			for _, d := range m.Data {
				e.Labels = append(e.Labels, strconv.Quote(util.EnumLabel(d)))
			}
			for _, item := range items {
				if fm := util.FindEnumMap(gpa, item.Key); fm != nil && fm.Name == m.Name {
					e.Field = item.Name
					break
				}
			}
			consts = append(consts, e)
		}

		for _, join := range gpa.Join {
			join.Prefix = util.TablePrefix(join.Name, packageName, db)
			joins = append(joins, join)
		}
		compares = append(compares, gpa.Compare...)
		search = gpa.Search

		for _, pair := range gpa.Session {
			s := GoSession{Key: pair.Key, Field: util.CamelCase(pair.Column), Column: strings.ToLower(pair.Column)}
			if col := util.FindColumn(items, pair.Column); col != nil {
				s.Field = col.Name
				s.Column = col.Key
			} else {
				log.Printf("WARNING: session column %s not found in %s", pair.Column, tableName)
			}
			sessions = append(sessions, s)
		}

		for _, fn := range util.ParseFuncs(tableName, gpa.Method, items) {
			if isView && (fn.Kind == "Update" || fn.Kind == "Delete") {
				log.Printf("WARNING: %s is a view, write method %s skipped", tableName, fn.Name)
				continue
			}
			for i := range fn.Wheres {
				w := &fn.Wheres[i]
				if w.Param == constPkg {
					w.Param += "_"
				}
				w.Check = zeroCheck(w.Type, w.Param)
			}
			funcs = append(funcs, fn)
		}
	}

	// rest 컨트롤러가 상수 패키지를 쓰는 것은 커스텀 메서드 매개변수가 enum 타입일 때뿐이다.
	restUsesConst := false
	for _, fn := range funcs {
		for _, w := range fn.Wheres {
			if strings.HasPrefix(strings.TrimPrefix(w.Type, "[]"), constPkg+".") {
				restUsesConst = true
			}
		}
	}
	// model 은 UpdateWhere(쓰기) 와 enum 필드/InitExtra 에서 상수 패키지를 쓴다.
	modelUsesConst := !isView || len(consts) > 0
	for _, item := range items {
		if strings.HasPrefix(item.Type, constPkg+".") {
			modelUsesConst = true
		}
	}

	v.Set("consts", consts)
	v.Set("joins", joins)
	v.Set("compares", compares)
	v.Set("sessions", sessions)
	v.Set("search", search)
	v.Set("funcs", funcs)
	v.Set("restUsesConst", restUsesConst)
	v.Set("modelUsesConst", modelUsesConst)

	// Generate Go model file
	var b bytes.Buffer
	t, err := views.GetTemplate("go/model.jet")
	if err == nil {
		if err = t.Execute(&b, v, nil); err != nil {
			log.Printf("ERROR: model template execution failed: %v", err)
		}
	} else {
		log.Printf("ERROR: Failed to load model template: %v", err)
	}

	if cnf.Language == "go" {
		modelFile := cnf.GoModelFilePath + "models/" + fileName + ".go"

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

	restFile := cnf.GoModelFilePath + "controllers/rest/" + fileName + ".go"

	if err := util.WriteFile(restFile, b2.String()); err != nil {
		log.Printf("ERROR: Failed to write rest controller file %s: %v", restFile, err)
	} else {
		log.Printf("Generated Go: %s", restFile)
	}

	// Generate const file
	var b3 bytes.Buffer
	t, err = views.GetTemplate("go/const.jet")
	if err == nil {
		if err = t.Execute(&b3, v, nil); err != nil {
			log.Printf("ERROR: Const template execution failed: %v", err)
		}
	} else {
		log.Printf("ERROR: Failed to load const template: %v", err)
	}

	constDir := cnf.GoModelFilePath + "models/" + fileName
	if err := os.MkdirAll(constDir, 0755); err != nil {
		log.Printf("ERROR: Failed to create const directory %s: %v", constDir, err)
	}

	constFile := constDir + "/" + fileName + ".go"

	if err := util.WriteFile(constFile, b3.String()); err != nil {
		log.Printf("ERROR: Failed to write const file %s: %v", constFile, err)
	} else {
		log.Printf("Generated Go: %s", constFile)
	}

	// Generate domain router file
	generateDomainRouter(views, packageName, tableName, items, funcs, hasId, isView, constPkg, constImport, cnf)
}

// zeroCheck 는 커스텀 Get/Find/Count 에서 매개변수가 "값 없음"이 아닐 때만 조건을 붙이는 검사식이다.
// bool 과 슬라이스(in)는 값 없음을 구분할 수 없어서 항상 조건을 붙인다 (빈 in 목록으로 전체 행이 나오지 않도록).
func zeroCheck(typeName string, param string) string {
	switch {
	case typeName == "string":
		return param + ` != ""`
	case typeName == "[]byte":
		return "len(" + param + ") > 0"
	case typeName == "bool" || strings.HasPrefix(typeName, "[]"):
		return ""
	default:
		// int, int64, float64, enum(int 기반)
		return param + " != 0"
	}
}

// inputModelName 은 REST Insert/Update 입력 구조체 이름이다.
// User 테이블이나 비밀번호 컬럼이 있는 테이블은 비밀번호를 입력받을 수 있는 {Name}Update 를 쓴다 (출력용 구조체는 비밀번호를 json 에서 숨긴다).
func inputModelName(modelName string, items []util.Column) string {
	if modelName == "User" || util.PasswordColumn(items) != "" {
		return modelName + "Update"
	}
	return modelName
}

const badRequest = "return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{\"code\": \"bad request\"})"

func bodyParam(model string, slice bool) string {
	decl := fmt.Sprintf("\t\titem_ := &models.%s{}\n\t\terr := c.BodyParser(item_)\n", model)
	if slice {
		decl = fmt.Sprintf("\t\titem_ := &[]models.%s{}\n\t\terr := c.BodyParser(item_)\n", model)
	}
	return decl + "\t\tif err != nil {\n\t\t    log.Error().Msg(err.Error())\n\t\t    " + badRequest + "\n\t\t}"
}

func generateDomainRouter(views *jet.Set, packageName string, tableName string, items []util.Column, funcs []util.Func, hasId bool, isView bool, constPkg string, constImport string, cnf config.ModelConfig) {
	domainName := util.FileName(tableName)
	modelName := util.ModelName(tableName)

	routes := make([]Route, 0)
	controllerName := "rest." + modelName + "Controller"
	modelNameForUpdate := inputModelName(modelName, items)
	route := func(method, url, funcName, code, params string) {
		routes = append(routes, Route{Method: method, URL: url, FuncName: funcName, ControllerName: controllerName, ParamCode: code, ParamStr: params})
	}

	// GET /domain - Index
	route("Get", "/"+domainName, "Index", "\t\tpage_, _ := strconv.Atoi(c.Query(\"page\"))\n\t\tpagesize_, _ := strconv.Atoi(c.Query(\"pagesize\"))", "page_, pagesize_")

	// GET /domain/:id - Read
	if hasId {
		route("Get", "/"+domainName+"/:id", "Read", "\t\tid_, err := strconv.ParseInt(c.Params(\"id\"), 10, 64)\n\t\tif err != nil {\n\t\t\t"+badRequest+"\n\t\t}", "id_")
	}

	// view 는 읽기 전용이라 쓰기 경로를 만들지 않는다.
	if !isView {
		route("Post", "/"+domainName, "Insert", bodyParam(modelNameForUpdate, false), "item_")

		batch := fmt.Sprintf("\t\tvar items_ *[]models.%s\n\t\titems__ref := &items_\n\t\terr := c.BodyParser(items__ref)\n\t\tif err != nil {\n\t\t    log.Error().Msg(err.Error())\n\t\t    %s\n\t\t}", modelNameForUpdate, badRequest)
		route("Post", "/"+domainName+"/batch", "Insertbatch", batch, "items_")
	}

	// POST /domain/count - Count
	route("Post", "/"+domainName+"/count", "Count", "", "")

	if !isView && hasId {
		route("Put", "/"+domainName, "Update", bodyParam(modelNameForUpdate, false), "item_")
		route("Delete", "/"+domainName, "Delete", bodyParam(modelName, false), "item_")
		route("Delete", "/"+domainName+"/batch", "Deletebatch", bodyParam(modelName, true), "item_")
	}

	// 커스텀 메서드 경로: 컨트롤러와 같은 해석 결과(funcs)로 매개변수를 만든다.
	for _, fn := range funcs {
		if r := routeFromFunc(fn, domainName, controllerName); r != nil {
			routes = append(routes, *r)
		}
	}

	code := ""
	for _, r := range routes {
		code += r.ParamCode
	}

	v := make(jet.VarMap)
	v.Set("packageName", packageName)
	v.Set("domainName", domainName)
	v.Set("modelName", modelName)
	v.Set("routes", routes)
	v.Set("controllerType", "rest")
	v.Set("needsLog", strings.Contains(code, "log."))
	v.Set("needsJson", strings.Contains(code, "json."))
	v.Set("needsStrings", strings.Contains(code, "strings."))
	v.Set("needsStrconv", strings.Contains(code, "strconv."))
	v.Set("needsModels", strings.Contains(code, "models."))
	v.Set("needsConst", strings.Contains(code, constPkg+"."))
	v.Set("constImport", constImport)

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

// routeFromFunc 는 커스텀 메서드 하나의 경로를 만든다.
// Get/Find/Count: GET /{domain}/{get|find|count}/{by 뒤 이름}/:{매개변수}..., 값을 해석하지 못하면 400.
// UpdateXByY: PUT /{domain}/{x}/{y}, JSON 본문에서 값을 꺼낸다. DeleteBy 는 경로를 만들지 않는다 (예전과 같음).
func routeFromFunc(fn util.Func, domainName string, controllerName string) *Route {
	vars := make([]string, 0)
	for _, w := range fn.Wheres {
		vars = append(vars, strings.ToLower(w.Column)+"_")
	}

	switch fn.Kind {
	case "Get", "Find", "Count":
		segment := strings.ToLower(fn.Name[len(fn.Kind)+2:])
		url := fmt.Sprintf("/%s/%s/%s", domainName, strings.ToLower(fn.Kind), segment)
		code := make([]string, 0)
		for i, w := range fn.Wheres {
			// 조건이 하나면 예전과 같이 경로 변수 이름을 by 뒤 이름으로 쓴다 (/item/find/ids/:ids).
			name := strings.ToLower(w.Column)
			if len(fn.Wheres) == 1 {
				name = segment
			}
			url += "/:" + name
			code = append(code, pathParamCode(vars[i], name, w.Type))
		}
		return &Route{Method: "Get", URL: url, FuncName: fn.Name, ControllerName: controllerName, ParamCode: strings.Join(code, "\n"), ParamStr: strings.Join(vars, ", ")}
	case "Update":
		sets := make([]string, 0)
		wheres := make([]string, 0)
		code := "\t\tvar results map[string]interface{}\n\t\tjsonData := c.Body()\n\t\tjsonErr := json.Unmarshal(jsonData, &results)\n\t\tif jsonErr != nil {\n\t\t    log.Error().Msg(jsonErr.Error())\n\t\t    " + badRequest + "\n\t\t}\n"
		for i, w := range fn.Wheres {
			if w.Compare == "column" {
				sets = append(sets, strings.ToLower(w.Column))
			} else {
				wheres = append(wheres, strings.ToLower(w.Column))
			}
			code += bodyParamCode(vars[i], strings.ToLower(w.Column), w.Type)
		}
		return &Route{Method: "Put", URL: fmt.Sprintf("/%s/%s/%s", domainName, strings.Join(sets, ""), strings.Join(wheres, "")), FuncName: fn.Name, ControllerName: controllerName, ParamCode: code, ParamStr: strings.Join(vars, ", ")}
	}
	return nil
}

// parseCode 는 문자열 식 src 를 typeName 으로 바꿔 변수 v 에 담는 코드다. 해석에 실패하면 400 을 돌려준다.
func parseCode(v string, src string, typeName string, indent string) string {
	fail := indent + "if err != nil {\n" + indent + "\t" + badRequest + "\n" + indent + "}\n"
	switch {
	case typeName == "int":
		return indent + v + ", err := strconv.Atoi(" + src + ")\n" + fail
	case typeName == "int64":
		return indent + v + ", err := strconv.ParseInt(" + src + ", 10, 64)\n" + fail
	case typeName == "bool":
		return indent + v + ", err := strconv.ParseBool(" + src + ")\n" + fail
	case typeName == "float64":
		return indent + v + ", err := strconv.ParseFloat(" + src + ", 64)\n" + fail
	case typeName == "[]byte":
		return indent + v + " := []byte(" + src + ")\n"
	case strings.Contains(typeName, "."):
		// enum (상수 패키지의 int 기반 타입)
		return indent + v + "_, err := strconv.Atoi(" + src + ")\n" + fail + indent + v + " := " + typeName + "(" + v + "_)\n"
	}
	return indent + v + " := " + src + "\n"
}

// pathParamCode 는 경로 변수 name 을 typeName 으로 해석해 변수 v 에 담는 코드다. 슬라이스는 쉼표로 나눈다.
func pathParamCode(v string, name string, typeName string) string {
	src := fmt.Sprintf("c.Params(%q)", name)
	if strings.HasPrefix(typeName, "[]") && typeName != "[]byte" {
		code := parseCode("e", "strings.TrimSpace(s)", typeName[2:], "\t\t\t")
		return fmt.Sprintf("\t\t%s := make(%s, 0)\n\t\tfor _, s := range strings.Split(%s, \",\") {\n%s\t\t\t%s = append(%s, e)\n\t\t}", v, typeName, src, code, v, v)
	}
	return strings.TrimSuffix(parseCode(v, src, typeName, "\t\t"), "\n")
}

// bodyParamCode 는 JSON 본문(results)의 key 값을 typeName 으로 꺼내 변수 v 에 담는 코드다. 타입이 맞지 않으면 400.
func bodyParamCode(v string, key string, typeName string) string {
	extract := func(assertType string, convert string) string {
		return fmt.Sprintf("\t\tvar %[1]s %[2]s\n\t\tif v, flag := results[%[3]q]; flag {\n\t\t\t%[1]s_, ok := v.(%[4]s)\n\t\t\tif !ok {\n\t\t\t\t%[5]s\n\t\t\t}\n\t\t\t%[1]s = %[6]s\n\t\t}\n",
			v, typeName, key, assertType, badRequest, fmt.Sprintf(convert, v+"_"))
	}

	switch {
	case typeName == "int" || typeName == "int64":
		return extract("float64", typeName+"(%s)")
	case typeName == "bool":
		return extract("bool", "%s")
	case typeName == "float64":
		return extract("float64", "%s")
	case typeName == "[]byte":
		return extract("string", "[]byte(%s)")
	case strings.Contains(typeName, ".") && !strings.HasPrefix(typeName, "[]"):
		return extract("float64", typeName+"(int(%s))")
	default:
		return extract("string", "%s")
	}
}
