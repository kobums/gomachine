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
	Items  []util.EnumItem // 상수 (빈 항목 제외, 값은 data 위치)
	Labels []string        // 표시 이름 문자열 리터럴 (data 순서 그대로, 코드로 찾는다)
	// HasZero 는 코드 0 이 실제 값인지 (data[0] 이 비어 있지 않음). 그러면 0 을 "조건 없음"으로 보지 않는다.
	HasZero bool
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

	// 조인 테이블 컬럼 (ordinal_position 순서). 테이블마다 한 번만 읽는다.
	joinColumnCache := map[string][]util.Column{}
	joinColumns := func(str string) []util.Column {
		if cols, ok := joinColumnCache[str]; ok {
			return cols
		}
		columns := make([]util.Column, 0)
		joinTable := strings.ToLower(str) + "_tb"
		raws, err := util.QueryColumns(db, cnf.Database, joinTable)
		if err != nil {
			log.Println(err)
		}
		// 조인 모델의 필드 타입(enum 등)은 조인 테이블 자신의 gpa 로 정해진다.
		joinGpa := findGpa(cnf, joinTable)
		for _, raw := range raws {
			columns = append(columns, util.NewColumnFromRaw(joinTable, raw, joinGpa, cnf))
		}
		joinColumnCache[str] = columns
		return columns
	}
	views.AddGlobal("columns", joinColumns)

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
			e.HasZero = util.EnumHasZero(m.Data)
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
			join.Prefix = util.TablePrefix(join.Name, cnf.Database, db)
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
				// 코드 0 이 실제 값인 enum 은 0 도 조건으로 쓴다.
				if m := util.FindEnumMap(gpa, w.Key); m != nil && util.EnumHasZero(m.Data) {
					w.Check = ""
				}
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

	// 정렬에 쓸 수 있는 컬럼 (API 이름 / 접두사 뺀 이름 / 전체 이름 → SQL 컬럼). 비밀번호 컬럼은 뺀다.
	orderColumns := make([]OrderColumn, 0)
	orderSeen := map[string]bool{}
	for _, item := range items {
		if item.Name == passwordColumn {
			continue
		}
		for _, k := range []string{strings.ToLower(item.Name), strings.ToLower(item.Key), strings.ToLower(item.Column)} {
			if k != "" && !orderSeen[k] {
				orderSeen[k] = true
				orderColumns = append(orderColumns, OrderColumn{Key: k, Column: item.Column})
			}
		}
	}

	// 코드 0 이 실제 값인 enum 필드 (목록 필터에서 0 도 조건으로 쓴다)
	enumZero := map[string]bool{}
	for _, e := range consts {
		if e.Field != "" && e.HasZero {
			enumZero[e.Field] = true
		}
	}

	// null 허용 컬럼은 sql.Null* 로 Scan 해서 0 값으로 바꾼다 (NULL 이 있는 행이 Scan 실패로 빠지지 않게).
	scan := newScanCode()
	for _, item := range items {
		scan.add("item", "n_"+item.Name, item, constPkg)
	}
	for _, join := range joins {
		jv := joinVarName(join)
		for _, col := range joinColumns(join.Name) {
			scan.add("_"+jv, "n_"+jv+"_"+col.Name, col, "")
		}
	}

	// 비어 있는 날짜 값을 저장할 때 넣을 값 (NOT NULL 컬럼만). null 허용 컬럼은 NULL 을 저장한다.
	dateFill := map[string]string{}
	needsTime := createdDefault
	for _, item := range items {
		if item.Type != "string" || !util.IsDateType(item.OriginalType) || item.Nullable {
			continue
		}
		// createddate 는 Insert 앞부분에서 현재 시각을 넣는다.
		if item.Name == createdColumn && createdDefault {
			continue
		}
		if fill := dateFillExpr(item); fill != "" {
			dateFill[item.Name] = fill
			if strings.Contains(fill, "time.") {
				needsTime = true
			}
		}
	}
	// 빈 값을 NULL 로 바꾸는 건 null 허용 날짜/시간 컬럼만이다.
	// 숫자 0, false, 빈 문자열은 실제 값이므로 그대로 저장한다.
	nullable := map[string]bool{}
	for _, item := range items {
		if item.Nullable && isDateLikeType(item.OriginalType) {
			nullable[item.Name] = true
		}
	}
	views.AddGlobal("dateFill", func(name string) string { return dateFill[name] })
	views.AddGlobal("isNullable", func(name string) bool { return nullable[name] })
	views.AddGlobal("enumZero", func(name string) bool { return enumZero[name] })
	// writeParam 은 Insert/Update 에 넘기는 값이다. null 허용 날짜/시간 컬럼은 빈 값이면 NULL 로 저장한다.
	views.AddGlobal("writeParam", func(name string, expr string) string {
		if nullable[name] {
			return "p.nullIfZero(" + expr + ")"
		}
		return expr
	})

	v.Set("orderColumns", orderColumns)
	v.Set("scanDecls", scan.decls.String())
	v.Set("scanTargets", strings.Join(scan.targets, ", "))
	v.Set("scanAssigns", scan.assigns.String())
	v.Set("needsTime", needsTime && !isView)
	v.Set("hasNullable", len(nullable) > 0 && !isView)
	v.Set("restNeedsStrconv", len(enumZero) > 0)

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
		// 본문 원문도 넘겨서 보낸 필드만 기존 행에 덮어쓴다 (부분 수정).
		route("Put", "/"+domainName, "Update", bodyParam(modelNameForUpdate, false), "item_, c.Body()")
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
		// key 가 없으면 0 값으로 덮어쓰지 않도록 400 을 돌려준다.
		return fmt.Sprintf("\t\tvar %[1]s %[2]s\n\t\tif v, flag := results[%[3]q]; flag {\n\t\t\t%[1]s_, ok := v.(%[4]s)\n\t\t\tif !ok {\n\t\t\t\t%[5]s\n\t\t\t}\n\t\t\t%[1]s = %[6]s\n\t\t} else {\n\t\t\t%[5]s\n\t\t}\n",
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

// OrderColumn 은 정렬 이름(소문자) 과 SQL 컬럼의 매핑이다.
type OrderColumn struct {
	Key    string
	Column string
}

// findGpa 는 model.json table 설정 중 tableName 에 해당하는 것을 찾는다.
func findGpa(cnf config.ModelConfig, tableName string) *config.Gpa {
	for i := range cnf.Gpa {
		if util.MatchTableName(cnf.Gpa[i].Name, tableName) {
			return &cnf.Gpa[i]
		}
	}
	return nil
}

func joinVarName(join config.GpaJoin) string {
	if join.Alias != "" {
		return strings.ToLower(join.Alias)
	}
	return strings.ToLower(join.Name)
}

// scanCode 는 ReadRow/ReadRows 의 Scan 코드다. null 허용 컬럼은 sql.Null* 임시 변수로 읽고 0 값으로 바꾼다.
type scanCode struct {
	decls   strings.Builder
	targets []string
	assigns strings.Builder
}

func newScanCode() *scanCode {
	return &scanCode{targets: make([]string, 0)}
}

// add 는 owner.{col.Name} 에 읽을 Scan 대상을 추가한다. constPkg 가 있으면 Id 는 int64 로 본다 (모델 구조체와 같게).
func (s *scanCode) add(owner string, tmp string, col util.Column, constPkg string) {
	field := owner + "." + col.Name
	typeName := col.Type
	if col.Name == "Id" {
		typeName = "int64"
	}
	// []byte 는 NULL 을 nil 로 읽으므로 그대로 둔다.
	// 조인 테이블의 enum 은 그 상수 패키지를 import 하지 않으므로 그대로 읽는다 (NULL 이면 Scan 에러로 로그가 남는다).
	if !col.Nullable || typeName == "[]byte" || (constPkg == "" && strings.Contains(typeName, ".")) {
		s.targets = append(s.targets, "&"+field)
		return
	}

	nullType, convert := "sql.NullString", "%s.String"
	switch {
	case typeName == "int":
		nullType, convert = "sql.NullInt64", "int(%s.Int64)"
	case typeName == "int64":
		nullType, convert = "sql.NullInt64", "%s.Int64"
	case typeName == "float64":
		nullType, convert = "sql.NullFloat64", "%s.Float64"
	case typeName == "bool":
		nullType, convert = "sql.NullBool", "%s.Bool"
	case strings.Contains(typeName, "."):
		// enum (상수 패키지의 int 기반 타입)
		nullType, convert = "sql.NullInt64", typeName+"(%s.Int64)"
	}
	fmt.Fprintf(&s.decls, "var %s %s\n", tmp, nullType)
	s.targets = append(s.targets, "&"+tmp)
	fmt.Fprintf(&s.assigns, "%s = %s\n", field, fmt.Sprintf(convert, tmp))
}

// dateFillExpr 는 NOT NULL 날짜 컬럼에 빈 값을 저장할 때 넣을 Go 식이다.
// 기본값이 현재 시각이면 현재 시각, 고정 값이면 그 값, 기본값이 없으면 예전과 같은 자리표시 값 (timestamp 는 범위 밖이라 현재 시각).
func dateFillExpr(item util.Column) string {
	layout := "2006-01-02 15:04:05"
	sentinel := "1000-01-01 00:00:00"
	switch strings.ToLower(item.OriginalType) {
	case "date":
		layout, sentinel = "2006-01-02", "1000-01-01"
	case "time":
		layout, sentinel = "15:04:05", "00:00:00"
	case "timestamp":
		sentinel = ""
	}
	now := "time.Now().Format(\"" + layout + "\")"

	if item.HasDefault {
		if util.IsNowDefault(item.Default) {
			return now
		}
		def := strings.Trim(item.Default, "'")
		if def != "" {
			return strconv.Quote(def)
		}
	}
	if sentinel == "" {
		return now
	}
	return strconv.Quote(sentinel)
}

// isDateLikeType 은 빈 문자열을 저장할 수 없는 날짜/시간 타입인지 확인한다.
func isDateLikeType(t string) bool {
	switch strings.ToLower(t) {
	case "date", "datetime", "timestamp", "time", "year":
		return true
	}
	return false
}
