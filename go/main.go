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
	// Value 는 컬럼 타입으로 바꾼 세션 값 식이다 (int 컬럼이면 int(c.Session.Id)). 비교/대입에 쓴다.
	Value string
	// SQL 은 where 절에 쓰는 전체 컬럼 이름이다 (자기 조인이면 테이블 이름으로 한정).
	SQL string
}

// GoJoin 은 조인 하나의 생성 정보다.
type GoJoin struct {
	Name  string // 조인 테이블 기본 이름 (user)
	Var   string // Go 변수 / Extra key (alias 또는 name 소문자, 겹치면 번호)
	Model string // 조인 모델 타입 이름
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

	// 컬럼의 API 이름 (json 태그, 목록 필터 파라미터). 모든 언어가 util.JSONKey 를 쓴다.
	views.AddGlobal("jsonKey", func(col util.Column) string { return util.JSONKey(col) })
	views.AddGlobal("isPassword", util.IsPasswordColumn)
	views.AddGlobal("startParam", func(col util.Column) string { s, _ := util.RangeParams(col); return s })
	views.AddGlobal("endParam", func(col util.Column) string { _, e := util.RangeParams(col); return e })
	views.AddGlobal("isRange", func(col util.Column) bool { return util.IsRangeFilter(col.OriginalType) })

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
	joinColumns := func(joinTable string) []util.Column {
		if cols, ok := joinColumnCache[joinTable]; ok {
			return cols
		}
		columns := make([]util.Column, 0)
		raws, err := util.QueryColumns(db, cnf.Database, joinTable)
		if err != nil {
			log.Println(err)
		}
		// 조인 모델의 필드 타입(enum 등)은 조인 테이블 자신의 gpa 로 정해진다.
		joinGpa := findGpa(cnf, joinTable)
		for _, raw := range raws {
			columns = append(columns, util.NewColumnFromRaw(joinTable, raw, joinGpa, cnf))
		}
		columns = util.DedupeColumns(joinTable, columns)
		joinColumnCache[joinTable] = columns
		return columns
	}

	// _ / 대소문자만 다른 컬럼은 뒤 컬럼 이름에 번호를 붙인다 (TypeScript / Dart 와 같은 규칙).
	items = util.DedupeColumns(tableName, items)

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
			if msg := util.NonIntegerIdWarning("go", tableName, item); msg != "" {
				log.Print(msg)
			}
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
	// 비밀번호 컬럼(Passwd, Password 모두): 응답에서 숨기고 저장할 때 해시한다.
	v.Set("passwordColumns", util.PasswordColumns(items))
	v.Set("createdColumn", createdColumn)
	v.Set("createdDefault", createdDefault)
	v.Set("protects", protects)
	v.Set("adminCheck", adminCheck)
	v.Set("adminLevelNum", adminLevelNum)

	consts := make([]GoEnum, 0)
	joins := make([]config.GpaJoin, 0)
	joinTables := make([]string, 0) // 조인마다 테이블 이름
	joinLocals := make([]string, 0) // 조인마다 이 테이블의 조인 컬럼 (DB 이름)
	joinVars := make([]string, 0)   // 조인마다 변수 / Extra key (util.JoinVars)
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

		// 조인 테이블과 이 테이블의 조인 컬럼(접두사 있는/없는 이름 모두)을 찾는다. 없으면 경고를 찍고 건너뛴다.
		for _, rj := range util.ResolveJoins(tableName, items, gpa.Join) {
			join := rj.GpaJoin
			join.Prefix = util.TablePrefixOf(rj.Table, cnf.Database, db)
			joins = append(joins, join)
			joinTables = append(joinTables, rj.Table)
			joinLocals = append(joinLocals, rj.Local.Column)
			joinVars = append(joinVars, rj.Var)
		}
		compares = append(compares, gpa.Compare...)
		search = gpa.Search

		for _, pair := range gpa.Session {
			// 쓸 수 없는 session 컬럼: jwt 면 소유권 검사 없는 API 가 되므로 멈추고, 아니면 (session 을 쓰지 않으니) 경고만 찍는다.
			if msg := util.SessionColumnError(items, pair.Column, false); msg != "" {
				if auth == "jwt" {
					log.Fatalf("ERROR: %s: %s (generation stopped)", tableName, msg)
				}
				log.Printf("WARNING: %s: %s", tableName, msg)
				continue
			}
			col := util.FindColumn(items, pair.Column)
			s := GoSession{Key: pair.Key, Field: col.Name, Column: col.Key}
			s.Value = "c.Session." + strings.Title(pair.Key)
			// 세션 필드(보통 int64)와 컬럼 타입(int 등)이 달라도 컴파일되게 컬럼 타입으로 바꾼다.
			if isNumericType(col.Type) {
				s.Value = col.Type + "(" + s.Value + ")"
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

	// 조인 SQL: 조인 테이블은 left join 으로 붙인다 (조인 키가 NULL 이거나 없는 행도 목록/개수/Read 에 나온다).
	// 같은 테이블이 다시 나오면(자기 조인, 같은 테이블 두 번) SQL 별칭(j_{변수})을 붙이고, 이 테이블과 접두사가 같은 테이블을
	// 조인하면 이 테이블의 컬럼을 테이블 이름으로 한정한다 (Not unique table/alias, ambiguous column 방지).
	goJoins := make([]GoJoin, 0)
	qual := ""
	for _, join := range joins {
		if join.Prefix == prefix {
			qual = tableName + "."
		}
	}
	usedTables := map[string]bool{strings.ToLower(tableName): true}
	var joinFrom strings.Builder
	selectCols := make([]string, 0)
	for _, item := range items {
		selectCols = append(selectCols, selectColumn(item, qualify(len(joins) > 0, tableName, item.Column)))
	}
	for i, join := range joins {
		joinTable := joinTables[i]
		ref := joinTable
		alias := ""
		if usedTables[joinTable] {
			alias = "j_" + util.SanitizeIdent(joinVars[i], "j")
			ref = alias
		}
		usedTables[joinTable] = true
		for _, col := range joinColumns(joinTable) {
			selectCols = append(selectCols, selectColumn(col, ref+"."+col.Column))
		}
		local := joinLocals[i]
		joinFrom.WriteString(" left join " + joinTable)
		if alias != "" {
			joinFrom.WriteString(" " + alias)
		}
		joinFrom.WriteString(" on " + qualify(true, tableName, local) + " = " + ref + "." + join.Prefix + "_id")
		goJoins = append(goJoins, GoJoin{Name: join.Name, Var: joinVars[i], Model: util.ModelName(joinTable)})
	}
	mainCols := make([]string, 0)
	for _, item := range items {
		mainCols = append(mainCols, selectColumn(item, item.Column))
	}
	for i := range sessions {
		sessions[i].SQL = qual + prefix + "_" + sessions[i].Column
	}
	v.Set("selectQuery", "select "+strings.Join(selectCols, ", ")+" from "+tableName)
	v.Set("joinFrom", joinFrom.String())
	v.Set("rawSelect", "select "+strings.Join(mainCols, ", ")+" from "+tableName)
	v.Set("qual", qual)
	v.Set("wherePrefix", qual+prefix+"_")
	v.Set("goJoins", goJoins)

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
		if util.IsPasswordColumn(item.Name) {
			continue
		}
		for _, k := range []string{strings.ToLower(item.Name), strings.ToLower(item.Key), strings.ToLower(item.Column)} {
			if k != "" && !orderSeen[k] {
				orderSeen[k] = true
				orderColumns = append(orderColumns, OrderColumn{Key: k, Column: qual + item.Column})
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
	// 조인 테이블 컬럼은 left join 이라 모두 NULL 일 수 있으므로 sql.Null* 로 읽는다.
	scan := newScanCode()
	for _, item := range items {
		scan.add("item", "n_"+item.Name, item, constPkg, false, false)
	}
	var joinExtra strings.Builder
	for i, join := range goJoins {
		idTmp := ""
		for _, col := range joinColumns(joinTables[i]) {
			tmp := "n_" + join.Var + "_" + col.Name
			scan.add("_"+join.Var, tmp, col, "", true, false)
			if col.Key == "id" {
				idTmp = tmp
			}
		}
		// 조인 행이 없으면(left join 의 NULL) Extra 에 nil 을 넣는다.
		if idTmp != "" {
			fmt.Fprintf(&joinExtra, "if %s.Valid {\n_%s.InitExtra()\nitem.AddExtra(%q, _%s)\n} else {\nitem.AddExtra(%q, nil)\n}\n", idTmp, join.Var, join.Var, join.Var, join.Var)
		} else {
			fmt.Fprintf(&joinExtra, "_%s.InitExtra()\nitem.AddExtra(%q, _%s)\n", join.Var, join.Var, join.Var)
		}
	}
	// GetRaw: 조인 없이 이 테이블만 읽고, NULL 이었던 null 허용 컬럼을 nulls 에 기록한다.
	rawScan := newScanCode()
	for _, item := range items {
		rawScan.add("item", "n_"+item.Name, item, constPkg, false, true)
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
	// 부분 수정(PUT/PATCH)에서 NULL 을 유지하거나 지울 수 있는 null 허용 컬럼 (JSON key 소문자 → 필드 이름). 비밀번호는 따로 처리한다.
	nullCols := make([]OrderColumn, 0)
	anyNull := map[string]bool{}
	for _, item := range items {
		if item.Nullable && item.Name != "Id" {
			anyNull[item.Name] = true
			if !util.IsPasswordColumn(item.Name) {
				nullCols = append(nullCols, OrderColumn{Key: strings.ToLower(util.JSONKey(item)), Column: item.Name})
			}
		}
	}
	v.Set("nullCols", nullCols)
	views.AddGlobal("nullableCol", func(name string) bool { return anyNull[name] })
	// writeNull 은 UpdateNulls 에 넘기는 값이다. nulls 에 있는 null 허용 컬럼은 NULL 로 저장한다.
	views.AddGlobal("writeNull", func(name string, expr string) string {
		w := expr
		if nullable[name] {
			w = "p.nullIfZero(" + expr + ")"
		}
		if anyNull[name] {
			return "p.nullIf(nulls, " + strconv.Quote(name) + ", " + w + ")"
		}
		return w
	})

	// 목록 필터 중 숫자/enum/bool 파라미터를 strconv 로 해석하는지 (rest 가 strconv 를 import 해야 한다)
	filterParse := false
	for _, item := range items {
		if isSearchColumn(item) && !util.IsRangeFilter(item.OriginalType) && item.Type != "string" {
			filterParse = true
		}
	}

	// createddate 의 현재 시각 식 (model.json timezone). 비어 있으면 예전과 같이 UTC+9 (Asia/Seoul).
	v.Set("createdNow", createdNowCode(cnf.Timezone))

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
	v.Set("joinExtra", joinExtra.String())
	v.Set("rawDecls", rawScan.decls.String())
	v.Set("rawTargets", strings.Join(rawScan.targets, ", "))
	v.Set("rawAssigns", rawScan.assigns.String())
	v.Set("needsReflect", scan.reflect)
	v.Set("needsTime", needsTime && !isView)
	v.Set("hasNullable", len(nullable) > 0 && !isView)
	v.Set("restNeedsStrconv", filterParse)

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

// hasCompare 는 fn 의 조건 중 compare 가 있는지 확인한다.
func hasCompare(fn util.Func, compare string) bool {
	for _, w := range fn.Wheres {
		if w.Compare == compare {
			return true
		}
	}
	return false
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

// MaxPage 는 목록 page 의 최댓값이다 (넘으면 400). Rust 생성기와 같은 규칙.
const MaxPage = 1000000000

// MaxBatch 는 Insertbatch / Deletebatch 한 번에 받는 최대 행 수다 (넘으면 400).
const MaxBatch = 1000

const badRequest = "return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{\"code\": \"bad request\"})"

func bodyParam(model string, slice bool) string {
	decl := fmt.Sprintf("\t\titem_ := &models.%s{}\n\t\terr := c.BodyParser(item_)\n", model)
	if slice {
		decl = fmt.Sprintf("\t\titem_ := &[]models.%s{}\n\t\terr := c.BodyParser(item_)\n", model)
	}
	return decl + "\t\tif err != nil {\n\t\t    log.Error().Msg(err.Error())\n\t\t    " + badRequest + "\n\t\t}"
}

// batchLimit 는 batch 본문(*[]T 변수)이 MaxBatch 행을 넘으면 400 을 돌려주는 코드다.
func batchLimit(v string) string {
	return fmt.Sprintf("\n\t\tif %[1]s != nil && len(*%[1]s) > %[2]d {\n\t\t\t%[3]s\n\t\t}", v, MaxBatch, badRequest)
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

	idCode := "\t\tid_, err := strconv.ParseInt(c.Params(\"id\"), 10, 64)\n\t\tif err != nil {\n\t\t\t" + badRequest + "\n\t\t}"

	// 목록/개수 필터 파라미터를 해석하지 못하면 400 (컨트롤러와 같은 Parse{Model}Filters).
	filterCode := fmt.Sprintf("\t\tif _, bad := rest.Parse%sFilters(func(key string) string { return c.Query(key) }); bad != \"\" {\n\t\t\t%s\n\t\t}", modelName, badRequest)

	// GET /domain - Index. page/pagesize 가 숫자가 아니거나 음수면 400 (없으면 0: 전체 행).
	// page 가 MaxPage 를 넘어도 400 (offset 계산이 넘치지 않게).
	pageCode := ""
	for _, name := range []string{"page", "pagesize"} {
		limit := ""
		if name == "page" {
			limit = fmt.Sprintf(" || v > %d", MaxPage)
		}
		pageCode += fmt.Sprintf("\t\t%[1]s_ := 0\n\t\tif s := c.Query(%[1]q); s != \"\" {\n\t\t\tv, err := strconv.Atoi(s)\n\t\t\tif err != nil || v < 0%[3]s {\n\t\t\t\t%[2]s\n\t\t\t}\n\t\t\t%[1]s_ = v\n\t\t}\n", name, badRequest, limit)
	}
	route("Get", "/"+domainName, "Index", pageCode+filterCode, "page_, pagesize_")

	// GET /domain/:id - Read
	if hasId {
		route("Get", "/"+domainName+"/:id", "Read", idCode, "id_")
	}

	// view 는 읽기 전용이라 쓰기 경로를 만들지 않는다.
	if !isView {
		route("Post", "/"+domainName, "Insert", bodyParam(modelNameForUpdate, false), "item_")

		batch := fmt.Sprintf("\t\tvar items_ *[]models.%s\n\t\titems__ref := &items_\n\t\terr := c.BodyParser(items__ref)\n\t\tif err != nil {\n\t\t    log.Error().Msg(err.Error())\n\t\t    %s\n\t\t}", modelNameForUpdate, badRequest) + batchLimit("items_")
		route("Post", "/"+domainName+"/batch", "Insertbatch", batch, "items_")
	}

	// POST /domain/count - Count (예전 경로), GET /domain/count - Count (Spring 과 같은 경로)
	route("Post", "/"+domainName+"/count", "Count", filterCode, "")
	route("Get", "/"+domainName+"/count", "Count", filterCode, "")

	// GET /domain/search/{field}?{field}=값 - 컬럼 하나가 같은 행 목록 (Spring 과 같은 경로)
	for _, item := range items {
		if !isSearchColumn(item) {
			continue
		}
		key := util.JSONKey(item)
		code := searchParamCode("value_", key, item.Type)
		route("Get", "/"+domainName+"/search/"+util.SearchSegment(item), "Search", code, fmt.Sprintf("%q, value_", item.Key))
	}

	if !isView && hasId {
		// 본문 원문도 넘겨서 보낸 필드만 기존 행에 덮어쓴다 (부분 수정).
		route("Put", "/"+domainName, "Update", bodyParam(modelNameForUpdate, false), "item_, c.Body()")
		// PUT/PATCH /domain/:id - 경로의 id 가 본문의 id 보다 우선한다.
		byId := bodyParam(modelNameForUpdate, false) + "\n" + idCode + "\n\t\titem_.Id = id_"
		route("Put", "/"+domainName+"/:id", "Update", byId, "item_, c.Body()")
		route("Patch", "/"+domainName+"/:id", "Update", byId, "item_, c.Body()")
		route("Delete", "/"+domainName, "Delete", bodyParam(modelName, false), "item_")
		route("Delete", "/"+domainName+"/batch", "Deletebatch", bodyParam(modelName, true)+batchLimit("item_"), "item_")
		route("Delete", "/"+domainName+"/:id", "Delete", idCode+"\n\t\titem_ := &models."+modelName+"{Id: id_}", "item_")
	}

	// 커스텀 메서드 경로: 컨트롤러와 같은 해석 결과(funcs)로 매개변수를 만든다.
	for _, fn := range funcs {
		if fn.Kind == "Delete" && hasCompare(fn, "like") {
			// like 로 지우는 경로는 만들지 않는다 (와일드카드 하나로 많은 행을 지울 수 있다). 모델/컨트롤러 메서드는 남는다.
			log.Printf("WARNING: %s %s deletes by like, HTTP route skipped", tableName, fn.Name)
			continue
		}
		if r := routeFromFunc(fn, domainName, controllerName); r != nil {
			routes = append(routes, *r)
		}
	}

	// 정적 경로(/count, /batch, /search/x)를 /:id 같은 경로 변수보다 먼저 등록한다 (fiber 는 등록 순서로 찾는다).
	routes = staticFirst(routes)

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

// isSearchColumn 은 목록 필터 / GET /{table}/search/{field} 로 찾을 수 있는 컬럼인지 확인한다.
// Spring 의 IsSearchable 과 같다: id, primary, 비밀번호, []byte 컬럼은 뺀다 (rest.jet Parse{Model}Filters 도 같은 조건).
func isSearchColumn(item util.Column) bool {
	return item.Name != "Id" && !util.IsPasswordColumn(item.Name) && item.Type != "[]byte" && !item.Primary
}

// searchParamCode 는 검색 쿼리 파라미터 key 를 컬럼 타입으로 해석해 v 에 담는 코드다. 없거나 해석하지 못하면 400.
// enum 은 코드(int)로 받는다.
func searchParamCode(v string, key string, typeName string) string {
	if strings.Contains(typeName, ".") {
		typeName = "int"
	}
	src := fmt.Sprintf("c.Query(%q)", key)
	return fmt.Sprintf("\t\tif %s == \"\" {\n\t\t\t%s\n\t\t}\n", src, badRequest) + strings.TrimSuffix(parseCode(v, src, typeName, "\t\t"), "\n")
}

// staticFirst 는 경로 변수(:x)가 없는 경로를 앞으로 옮긴다 (같은 그룹 안의 순서는 유지).
func staticFirst(routes []Route) []Route {
	ret := make([]Route, 0, len(routes))
	for _, dynamic := range []bool{false, true} {
		for _, r := range routes {
			if strings.Contains(r.URL, ":") == dynamic {
				ret = append(ret, r)
			}
		}
	}
	return ret
}

// routeFromFunc 는 커스텀 메서드 하나의 경로를 만든다.
// Get/Find/Count: GET /{domain}/{get|find|count}/{by 뒤 이름}/:{매개변수}..., 값을 해석하지 못하면 400.
// DeleteBy: DELETE /{domain}/delete/{by 뒤 이름}/:{매개변수}... (컨트롤러가 소유권을 검사한다).
// UpdateXByY: PUT /{domain}/{x}/{y}, JSON 본문에서 값을 꺼낸다 (key 는 JSONKey).
func routeFromFunc(fn util.Func, domainName string, controllerName string) *Route {
	vars := make([]string, 0)
	for _, w := range fn.Wheres {
		vars = append(vars, strings.ToLower(w.Column)+"_")
	}

	switch fn.Kind {
	case "Get", "Find", "Count", "Delete":
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
		method := "Get"
		if fn.Kind == "Delete" {
			method = "Delete"
		}
		return &Route{Method: method, URL: url, FuncName: fn.Name, ControllerName: controllerName, ParamCode: strings.Join(code, "\n"), ParamStr: strings.Join(vars, ", ")}
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
			code += bodyParamCode(vars[i], util.LowerCamel(w.Key), w.Type)
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
	return util.FindTableConfig(cnf.Gpa, tableName)
}

// scanCode 는 ReadRow/ReadRows/GetRaw 의 Scan 코드다. null 허용 컬럼은 sql.Null* 임시 변수로 읽고 0 값으로 바꾼다.
type scanCode struct {
	decls   strings.Builder
	targets []string
	assigns strings.Builder
	// reflect 는 조인 테이블 enum 을 reflect 로 넣는 코드가 있는지다 (model 이 reflect 를 import 해야 한다).
	reflect bool
}

func newScanCode() *scanCode {
	return &scanCode{targets: make([]string, 0)}
}

// add 는 owner.{col.Name} 에 읽을 Scan 대상을 추가한다. constPkg 가 있으면 Id 는 int64 로 본다 (모델 구조체와 같게).
// forceNull 이면 NOT NULL 컬럼도 sql.Null* 로 읽는다 (left join 컬럼). recordNulls 면 NULL 이었던 컬럼 이름을 nulls 에 넣는다.
// tinyint(bool) 는 0/1 이 아닌 값(2 이상)도 읽을 수 있게 정수로 읽어 != 0 으로 바꾼다.
func (s *scanCode) add(owner string, tmp string, col util.Column, constPkg string, forceNull bool, recordNulls bool) {
	field := owner + "." + col.Name
	typeName := col.Type
	if col.Name == "Id" {
		typeName = "int64"
	}
	nullable := col.Nullable || forceNull
	// []byte 는 NULL 을 nil 로 읽으므로 그대로 둔다.
	if typeName == "[]byte" {
		s.targets = append(s.targets, "&"+field)
		if recordNulls && col.Nullable {
			fmt.Fprintf(&s.assigns, "if %s == nil {\nnulls[%q] = true\n}\n", field, col.Name)
		}
		return
	}
	if !nullable && typeName != "bool" {
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
		nullType, convert = "sql.NullInt64", "%s.Int64 != 0"
	case strings.Contains(typeName, ".") && constPkg == "":
		// 조인 테이블의 enum 은 그 상수 패키지를 import 하지 않으므로 reflect 로 정수 값을 넣는다.
		nullType = "sql.NullInt64"
		fmt.Fprintf(&s.decls, "var %s %s\n", tmp, nullType)
		s.targets = append(s.targets, "&"+tmp)
		fmt.Fprintf(&s.assigns, "reflect.ValueOf(&%s).Elem().SetInt(%s.Int64)\n", field, tmp)
		s.reflect = true
		return
	case strings.Contains(typeName, "."):
		// enum (상수 패키지의 int 기반 타입)
		nullType, convert = "sql.NullInt64", typeName+"(%s.Int64)"
	}
	fmt.Fprintf(&s.decls, "var %s %s\n", tmp, nullType)
	s.targets = append(s.targets, "&"+tmp)
	fmt.Fprintf(&s.assigns, "%s = %s\n", field, fmt.Sprintf(convert, tmp))
	if recordNulls && col.Nullable {
		fmt.Fprintf(&s.assigns, "if !%s.Valid {\nnulls[%q] = true\n}\n", tmp, col.Name)
	}
}

// createdNowExpr 는 Insert 에서 createddate 를 채울 현재 시각 식이다.
// "" / "Asia/Seoul" 은 예전과 같은 UTC+9, "local" 은 서버 지역 시간, 그 밖은 time.LoadLocation 이름 (실패하면 UTC+9).
func createdNowExpr(tz string) string {
	switch strings.TrimSpace(tz) {
	case "", "Asia/Seoul":
		return "time.Now().UTC().Add(time.Hour * 9)"
	case "local", "Local":
		return "time.Now()"
	case "UTC", "utc":
		return "time.Now().UTC()"
	}
	return fmt.Sprintf("createdTime(%q)", strings.TrimSpace(tz))
}

// createdNowCode 는 현재 시각을 t 에 담는 코드다 (createdNowExpr 참고).
func createdNowCode(tz string) string {
	expr := createdNowExpr(tz)
	if !strings.HasPrefix(expr, "createdTime(") {
		return "t := " + expr
	}
	name := strconv.Quote(strings.TrimSpace(tz))
	return "t := time.Now().UTC().Add(time.Hour * 9)\n        if loc, err := time.LoadLocation(" + name + "); err == nil {\n            t = time.Now().In(loc)\n        }"
}

// selectColumn 은 select 절의 컬럼 식이다. bit 컬럼은 드라이버가 원시 바이트("\x01")로 돌려주므로 +0 으로 숫자로 읽는다 (bool 로 Scan).
func selectColumn(col util.Column, expr string) string {
	if col.OriginalType == "bit" {
		return "(" + expr + "+0)"
	}
	return expr
}

// qualify 는 on 이 true 면 컬럼을 테이블 이름으로 한정한다.
func qualify(on bool, table string, column string) string {
	if on {
		return table + "." + column
	}
	return column
}

// isNumericType 은 Go 숫자 타입인지 확인한다.
func isNumericType(t string) bool {
	switch t {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64":
		return true
	}
	return false
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
