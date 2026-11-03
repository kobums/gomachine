package kotlin

import (
	"bytes"
	"database/sql"
	"gomachine/config"
	"gomachine/util"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/CloudyKit/jet/v6"
	log "github.com/sirupsen/logrus"
)

type KotlinColumn struct {
	Name         string
	Field        string // Kotlin 프로퍼티 이름 (예약어면 backtick)
	DBName       string
	KotlinType   string
	DBType       string // Original DB type (Int, String, etc.) for Response
	ModelName    string
	IsNullable   bool
	IsPrimaryKey bool
	IsGenerated  bool // id 정수 기본 키: @GeneratedValue(IDENTITY), 생성 요청에서 받지 않는다
	HasDefault   bool
	DefaultValue string
	IsEnum       bool
	EnumType     string
	IsPassword   bool   // 비밀번호 컬럼: 응답/검색에서 제외하고 저장 시 해시한다
	IsProtected  bool   // 보호 컬럼: Create/Update/Patch 요청으로 바꿀 수 없다
	IsSearchable bool   // 목록 필터 / /search 엔드포인트를 만든다
	DateFormat   string // DATE_TIME (LocalDateTime) / DATE (LocalDate). 요청 파라미터는 문자열로 받아 DateParams 로 해석한다
	IsCreated    bool   // createddate: PUT/PATCH 로 바꿀 수 없다
	CreatedNow   string // createddate 를 서버가 채우는 식 (DateParams.now()). 비어 있으면 요청 값을 쓴다
	InCreate     bool   // CreateRequest 에 있는 컬럼
	IsTinyint    bool   // tinyint → Boolean (@Column columnDefinition)
	Prop         string // 엔티티 프로퍼티의 JPA 속성 이름 (backtick 없음, 조인 컬럼은 {join}Id)
	EntityField  string // 엔티티 프로퍼티 이름 (조인 컬럼은 {join}Id)
	RespType     string // 응답 타입 (null 허용이면 ?)
	ParamName    string // 요청 파라미터 / JSON key (util.JSONKey)
	SearchPath   string // GET /search/{SearchPath} (util.SearchSegment)
	StartParam   string // 날짜 범위 필터 파라미터 (util.RangeParams)
	EndParam     string
	StartVar     string // 날짜 범위 매개변수 이름 (컬럼 프로퍼티와 겹치지 않게 _ 포함)
	EndVar       string
	QualType     string // 패키지까지 쓴 타입 (기본 키 타입: repository / service / controller 는 java.time 을 import 하지 않는다)
	QualDefault  string // 패키지까지 쓴 기본값 (service 의 보호 컬럼 값: enum 을 import 하지 않는다)
	// ProtectDefault 는 보호 컬럼을 생성할 때 넣는 DB 기본값 리터럴이다 (util.DefaultLiteral). 없으면 QualDefault / null.
	ProtectDefault string
	// FilterOp 는 목록 / 개수 필터 연산자다 (util.FilterCompare: 문자열은 like, 그 밖은 =, compare 설정으로 바꾼다).
	FilterOp string
}

// KotlinSort 는 정렬 파라미터 이름(소문자)과 엔티티 프로퍼티의 매핑이다.
type KotlinSort struct {
	Key  string
	Prop string
}

// Kotlin 날짜 형식. 응답은 Go 와 같은 "yyyy-MM-dd HH:mm:ss" 다.
// 요청(JSON / 쿼리 파라미터)은 DateParams 가 날짜만, 공백 / ISO('T'), 밀리초, 'Z' / 오프셋을 모두 받는다.
const (
	kotlinDateTimeOut = "yyyy-MM-dd HH:mm:ss"
	kotlinMaxPageSize = 10000
)

// KotlinSession 은 세션 값(SessionProvider.currentValue(Key))과 엔티티 컬럼의 매핑이다.
type KotlinSession struct {
	Key          string
	Field        string // 엔티티 프로퍼티 이름
	RequestField string // Create/Update/Patch 요청 프로퍼티 이름
	Convert      string // 세션 값(Long)을 컬럼 타입으로 바꾸는 접미사 (예: ".toInt()")
	IsPrimaryKey bool   // id 컬럼이면 요청 값을 덮어쓰지 않는다 (경로 변수 id 와 소유권 검사로 충분)
	Prop         string // JPA 속성 이름 (Specification 에 쓴다)
}

type KotlinEnumData struct {
	Name        string // enum class 이름 (CamelCase, Kotlin/Spring 타입과 겹치면 Enum 접미사)
	ExtraField  string // ExtraInfo 프로퍼티 이름
	ColumnField string // 이 enum 을 쓰는 엔티티 프로퍼티. 컬럼이 없으면 ""
	Items       []util.EnumItem
	EnumPackage string
	HasNone     bool   // data[0] 이 비어 있으면 코드 0 인 NONE 을 만든다
	ZeroIdent   string // 코드 0 인 상수 (NOT NULL 컬럼의 기본값). 없으면 ""
}

type KotlinJoin struct {
	Name            string
	Column          string // 이 테이블의 조인 컬럼 (DB 이름)
	ColumnName      string // 조인 컬럼 엔티티 프로퍼티의 앞부분 ({ColumnName}Id)
	Prefix          string
	Alias           string // 엔티티 조인 프로퍼티 / extra key (util.JoinVars, Go / TypeScript 와 같다)
	Var             string // Response.from 의 지역 변수 이름 (_ 접두사: 컬럼 프로퍼티와 겹치지 않는다)
	ModelName       string // 조인 엔티티 이름
	ParentModelName string // The main model name
	Index           int    // Index in the joins array
}

type EntityGraphPath struct {
	Path    string
	IsFirst bool
	IsLast  bool
}

type KotlinTemplateData struct {
	BasePackage      string // 생성 코드의 기본 패키지 (kotlinPackage)
	PackageName      string
	TableName        string
	ModelName        string // Kotlin 엔티티 이름 (Kotlin/Spring 타입과 겹치면 Entity 접미사)
	UrlName          string // /api/{UrlName} (Go 와 같은 util.FileName)
	ModelVar         string // 모델을 매개변수로 쓸 때의 이름
	EnumPackage      string // enums 하위 패키지 이름
	Columns          []KotlinColumn
	PK               KotlinColumn // 기본 키 컬럼
	HasDateTime      bool         // LocalDateTime 컬럼이 있다
	HasDate          bool         // LocalDate 컬럼이 있다
	HasDateFilter    bool         // 목록 필터 / 검색에 날짜 컬럼이 있다
	HasDecimal       bool
	IsView           bool // _vw: 읽기 전용 (쓰기 엔드포인트/서비스 메서드 없음, @Immutable)
	HasIdRoutes      bool // id 컬럼이나 gpa.primary 가 있다: GET/PUT/PATCH/DELETE /{id}, DELETE /batch
	Enums            []KotlinEnumData
	Joins            []KotlinJoin
	EntityGraphPaths []EntityGraphPath // Nested entity graph paths for @EntityGraph with metadata
	HasPassword      bool
	PasswordField    string          // 비밀번호 컬럼의 프로퍼티 이름 (passwd / password)
	Sessions         []KotlinSession // auth=jwt 이고 session 설정이 있을 때만 채워진다
	RequireLogin     bool            // auth=jwt: 모든 엔드포인트가 로그인을 요구한다 (User 생성 제외)
	IsUser           bool            // user 테이블 (회원 가입: 로그인 없이 생성)
	SelfOnly         bool            // auth=jwt 의 User 에 id session 이 없으면 수정/삭제는 본인 행만 (Go 와 같다)
	SelfConvert      string          // SelfOnly 비교에서 currentUserId()(Long) 를 PK 타입으로 바꾸는 접미사
	Sorts            []KotlinSort    // 목록 정렬에 쓸 수 있는 이름
	HasJsonDate      bool            // 요청 DTO 에 날짜가 있다 (@JsonDeserialize)
	HasCreatedNow    bool            // createddate 를 서버 시각으로 채운다 (DateParams)
	CamelNames       bool            // 대문자가 있는 테이블/컬럼 이름 (Spring Boot 기본 이름 전략이 바꾼다)
	DateTimeOut      string
	MaxPageSize      int
	// ReadPolicy 는 읽기 정책이다 (util.ReadPolicy: owner / login / public / self).
	ReadPolicy string
	ReadOwner  bool   // 읽기도 ownerSpec / isOwned (owner 이고 session 이 있다)
	ReadGuard  string // 읽기를 401 로 거절하는 조건 (비로그인). public 이면 ""
	ReadSelf   bool   // User 읽기는 본인 행만 (관리자 제외)
	SelfRead   bool   // 본인 행을 기본 키(id 정수)로 거를 수 있다. false 면 관리자만 읽는다
	AdminLevel string // 관리자 level (숫자 adminLevel 일 때만, 아니면 "")
	HasLike    bool   // like / not like 필터가 있다 (likePattern)
	// Boot4 는 Spring Boot 4 용 코드다 (model.json springBoot, 기본 4): Jackson 3(tools.jackson), Spring Data 4 Specification 시그니처,
	// PasswordEncoder.encode 의 String? 반환. false 면 Boot 3 용 예전 코드.
	Boot4 bool
}

// buildEntityGraphPaths 는 @EntityGraph 경로다. 응답은 조인을 한 단계만 매핑하므로 직접 조인만 가져온다
// (중첩 경로는 쓰지 않는 조인을 더 하고, 서로 참조하는 조인은 끝없이 이어질 수 있다).
func buildEntityGraphPaths(joins []KotlinJoin) []EntityGraphPath {
	pathStrings := make([]string, 0)
	seen := map[string]bool{}
	for _, join := range joins {
		if !seen[join.Alias] {
			seen[join.Alias] = true
			pathStrings = append(pathStrings, strings.Trim(join.Alias, "`"))
		}
	}

	paths := make([]EntityGraphPath, len(pathStrings))
	for i, pathStr := range pathStrings {
		paths[i] = EntityGraphPath{
			Path:    pathStr,
			IsFirst: i == 0,
			IsLast:  i == len(pathStrings)-1,
		}
	}
	return paths
}

// kotlinField 는 컬럼의 Kotlin 프로퍼티 이름이다. JSON key 와 요청 파라미터 이름이 되므로 util.JSONKey 를 쓴다
// (createddate, created_at → createdAt, userId → userid). 예약어는 backtick 으로 감싼다.
// JSONKey 는 _ 가 없으므로 생성 코드의 지역 변수(_page, _sort ...)나 날짜 범위 매개변수({field}_start)와 겹치지 않는다.
func kotlinField(col util.Column) string {
	return util.KotlinIdent(util.JSONKey(col))
}

// KotlinModelName 은 테이블의 Kotlin 엔티티 이름이다. Kotlin/Spring/JPA 타입과 겹치면 Entity 를 붙인다 (page_tb → PageEntity).
func KotlinModelName(tableName string) string {
	return util.KotlinTypeName(util.ModelName(tableName), "Entity")
}

// findPrimaryKey 는 기본 키 컬럼 위치다. gpa.primary > 이름이 id 인 컬럼 > 첫 번째 컬럼 순서.
func findPrimaryKey(columns []util.Column, gpa *config.Gpa) int {
	return util.PrimaryKeyIndex(columns, gpa)
}

func defaultValue(kotlinType string) string {
	switch kotlinType {
	case "LocalDateTime":
		return "java.time.LocalDateTime.now()"
	case "LocalDate":
		return "java.time.LocalDate.now()"
	case "BigDecimal":
		return "java.math.BigDecimal.ZERO"
	case "String":
		return `""`
	case "Int":
		return "0"
	case "Long":
		return "0L"
	case "Double":
		return "0.0"
	case "Boolean":
		return "false"
	case "ByteArray":
		return "ByteArray(0)"
	}
	return ""
}

// qualifiedType 은 java.time / java.math 타입을 패키지까지 쓴 이름이다 (import 없이 쓸 수 있게).
func qualifiedType(kotlinType string) string {
	switch kotlinType {
	case "LocalDateTime", "LocalDate":
		return "java.time." + kotlinType
	case "BigDecimal":
		return "java.math.BigDecimal"
	}
	return kotlinType
}

// hasUpper 는 대문자가 있는지 확인한다.
func hasUpper(s string) bool {
	for _, r := range s {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

func ProcessKotlin(packageName string, tableName string, prefix string, columns []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	baseModelName := util.ModelName(tableName)
	modelName := KotlinModelName(tableName)
	if modelName != baseModelName {
		log.Printf("WARNING: %s: Kotlin class %s clashes with a Kotlin/Spring/JPA type, generated as %s", tableName, baseModelName, modelName)
	}

	// 출력 경로는 kotlinModelFilePath 만 쓴다 (main.go 가 경로 없는 언어는 건너뛴다).
	targetPath := cnf.KotlinModelFilePath
	if targetPath == "" {
		log.Println("kotlinModelFilePath is not set, skip kotlin")
		return
	}

	// Make sure the target directory exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		err := os.MkdirAll(targetPath, 0755)
		if err != nil {
			log.Printf("Failed to create target directory: %v", err)
		}
	}

	// _ / 대소문자만 다른 컬럼, extra 컬럼은 Go / TypeScript 와 같은 규칙으로 이름에 번호를 붙인다.
	columns = util.DedupeColumns(tableName, columns)

	isView := util.GetTableType(tableName) == "view"
	basePackage := cnf.GetKotlinPackage()
	enumPackage := util.KotlinIdent(strings.ToLower(baseModelName))

	// 조인 (테이블 / 조인 컬럼을 찾지 못하면 건너뛴다)
	resolved := make([]util.ResolvedJoin, 0)
	if gpa != nil {
		resolved = util.ResolveJoins(tableName, columns, gpa.Join)
	}

	// enum 이름: Kotlin/Spring 타입, 이 엔티티, 조인 엔티티와 겹치면 Enum 을 붙인다.
	taken := []string{modelName}
	for _, rj := range resolved {
		taken = append(taken, KotlinModelName(rj.Table))
	}
	enumNames := map[string]string{}
	if gpa != nil {
		for _, m := range gpa.Map {
			name := util.KotlinTypeName(util.CamelCase(m.Name), "Enum", taken...)
			if name != util.CamelCase(m.Name) {
				log.Printf("WARNING: %s: Kotlin enum %s clashes with another type, generated as %s", tableName, util.CamelCase(m.Name), name)
			}
			enumNames[m.Name] = name
		}
	}

	protects := util.ProtectedColumns(baseModelName, columns, gpa)
	kotlinColumns := make([]KotlinColumn, 0)
	hasDateTime := false
	hasDate := false
	hasDateFilter := false
	hasDecimal := false
	pkIndex := findPrimaryKey(columns, gpa)
	// gpa.primary 가 실제 컬럼을 가리키는지 (그러면 그 컬럼이 기본 키다)
	hasPrimaryConfig := false
	if gpa != nil {
		for _, p := range gpa.Primary {
			if util.FindColumn(columns, p) != nil {
				hasPrimaryConfig = true
			}
		}
	}
	hasIdColumn := false
	for _, col := range columns {
		if col.Name == "Id" {
			hasIdColumn = true
		}
	}
	if isView && !hasPrimaryConfig {
		log.Printf("WARNING: %s is a view and uses column %s as @Id; rows with the same %s collapse into one. Set model.json table.primary to a unique column", tableName, columns[pkIndex].Column, columns[pkIndex].Column)
	}
	createdNow := createdExpr()
	camelNames := hasUpper(tableName)

	for i, col := range columns {
		isPrimaryKey := i == pkIndex
		if hasUpper(col.Column) {
			camelNames = true
		}

		// null 허용은 information_schema.columns.is_nullable 을 따른다 (기본 키는 항상 NOT NULL).
		isDateType := col.Type == "LocalDateTime" || col.Type == "LocalDate"
		isNullable := !isPrimaryKey && col.Nullable
		hasDefault := false
		defValue := ""
		kotlinType := col.Type

		// Check if this column is an enum type
		isEnumType := false
		enumTypeName := ""
		if m := util.FindEnumMap(gpa, col.Key); m != nil {
			isEnumType = true
			enumTypeName = enumNames[m.Name]
			kotlinType = enumTypeName
			// 코드 0 인 상수를 기본값으로 쓴다 (Go 의 0 값과 같다).
			if zero := enumZeroIdent(m.Data); zero != "" && !isNullable {
				hasDefault = true
				defValue = enumTypeName + "." + zero
			}
		} else if !isPrimaryKey {
			defValue = defaultValue(col.Type)
			hasDefault = defValue != ""
		}

		switch col.Type {
		case "LocalDateTime":
			hasDateTime = true
		case "LocalDate":
			hasDate = true
		case "BigDecimal":
			hasDecimal = true
		}

		// id 정수 기본 키만 자동 증가(IDENTITY)로 본다. gpa.primary / 첫 컬럼 / 문자열 키와 view 는 클라이언트가 값을 정한다.
		isGenerated := isPrimaryKey && !isView && col.Name == "Id" && (col.Type == "Long" || col.Type == "Int")
		if isPrimaryKey {
			isNullable = false
			if isGenerated {
				hasDefault = true
				defValue = "0"
			} else {
				// 생성 요청에서 반드시 보내야 한다 (기본값 없음). 엔티티는 JPA 용 기본값을 둔다.
				defValue = defaultValue(col.Type)
				hasDefault = defValue != ""
			}
		}

		if isNullable {
			hasDefault = true
			defValue = "null"
		}

		// Determine DBType for Response
		dbType := kotlinType
		if isEnumType {
			dbType = "Int" // enum 은 응답에서 코드(Int)다
		} else if isDateType {
			dbType = "String" // LocalDateTime / LocalDate is converted to String in Response
		}

		isPassword := util.IsPasswordColumn(col.Name) && col.Type == "String"
		kotlinCol := KotlinColumn{
			Name:         col.Name,
			Field:        kotlinField(col),
			DBName:       col.Column,
			KotlinType:   kotlinType,
			DBType:       dbType,
			ModelName:    modelName,
			IsNullable:   isNullable,
			IsPrimaryKey: isPrimaryKey,
			IsGenerated:  isGenerated,
			HasDefault:   hasDefault,
			DefaultValue: defValue,
			IsEnum:       isEnumType,
			EnumType:     enumTypeName,
			IsPassword:   isPassword,
			// Go 의 isSearchColumn 과 같다: id, gpa.primary, 비밀번호, 바이트 컬럼은 빼고, 첫 컬럼을 대신 키로 쓴 경우는 거를 수 있다.
			IsSearchable: col.Name != "Id" && !util.IsConfigPrimary(col, gpa) && !isPassword && col.Type != "ByteArray",
			IsCreated:    col.Name == "Createddate" && !isPrimaryKey,
			IsTinyint:    col.OriginalType == "tinyint" && col.Type == "Boolean",
			Prop:         strings.Trim(kotlinField(col), "`"),
			EntityField:  kotlinField(col),
			RespType:     dbType,
			ParamName:    util.JSONKey(col),
		}
		// createddate 는 서버가 현재 시각으로 넣는다 (Go 와 같다). 클라이언트 값은 받지 않는다.
		if kotlinCol.IsCreated {
			switch col.Type {
			case "LocalDateTime":
				kotlinCol.CreatedNow = createdNow
			case "LocalDate":
				kotlinCol.CreatedNow = createdNow + ".toLocalDate()"
			}
		}
		if isNullable {
			kotlinCol.RespType += "?"
		}
		kotlinCol.SearchPath = util.SearchSegment(col)
		kotlinCol.StartParam, kotlinCol.EndParam = util.RangeParams(col)
		bare := strings.Trim(kotlinCol.Field, "`")
		kotlinCol.StartVar, kotlinCol.EndVar = bare+"_start", bare+"_end"
		if col.Type == "LocalDateTime" {
			kotlinCol.DateFormat = "DATE_TIME"
		} else if col.Type == "LocalDate" {
			kotlinCol.DateFormat = "DATE"
		}
		if kotlinCol.IsSearchable && kotlinCol.DateFormat != "" {
			hasDateFilter = true
		}
		for _, p := range protects {
			if p == col.Name && !isPrimaryKey {
				kotlinCol.IsProtected = true
			}
		}
		kotlinCol.InCreate = !(isPrimaryKey && isGenerated) && !kotlinCol.IsProtected && kotlinCol.CreatedNow == ""
		if kotlinCol.IsSearchable {
			kotlinCol.FilterOp = util.FilterCompare(col, gpa)
		}
		kotlinCol.QualType = qualifiedType(kotlinType)
		kotlinCol.QualDefault = defValue
		if isEnumType {
			kotlinCol.QualType = basePackage + ".enums." + enumPackage + "." + enumTypeName
			if defValue != "null" && defValue != "" {
				kotlinCol.QualDefault = basePackage + ".enums." + enumPackage + "." + defValue
			}
		}
		if kotlinCol.IsProtected {
			kotlinCol.ProtectDefault = protectDefault(col, kotlinCol, gpa)
		}
		kotlinColumns = append(kotlinColumns, kotlinCol)
	}

	if len(kotlinColumns) == 0 {
		log.Printf("WARNING: %s has no column, skip kotlin", tableName)
		return
	}
	if camelNames {
		warnCamelOnce()
	}

	// Get enum data from GPA and convert to KotlinEnumData
	enums := make([]KotlinEnumData, 0)
	if gpa != nil {
		for _, gpaMap := range gpa.Map {
			e := KotlinEnumData{
				Name:        enumNames[gpaMap.Name],
				ExtraField:  util.KotlinIdent(strings.ToLower(gpaMap.Name)),
				Items:       util.KotlinEnumItems(gpaMap.Data),
				EnumPackage: enumPackage,
				HasNone:     len(gpaMap.Data) > 0 && gpaMap.Data[0] == "",
				ZeroIdent:   enumZeroIdent(gpaMap.Data),
			}
			for i, col := range columns {
				if m := util.FindEnumMap(gpa, col.Key); m != nil && m.Name == gpaMap.Name {
					e.ColumnField = kotlinColumns[i].Field
				}
			}
			enums = append(enums, e)
		}
	}

	// 조인: 조인 컬럼 프로퍼티는 {컬럼}Id, 조인 엔티티 프로퍼티는 util.JoinVars 이름이다.
	// 조인 컬럼 자신은 {컬럼}Id 로 바뀌므로 겹침 검사에서 뺀다.
	joinLocal := map[string]bool{}
	for _, rj := range resolved {
		joinLocal[rj.Local.Column] = true
	}
	fields := map[string]bool{}
	for _, col := range kotlinColumns {
		if !joinLocal[col.DBName] {
			fields[strings.Trim(col.Field, "`")] = true
		}
	}
	joins := make([]KotlinJoin, 0)
	for joinIndex, rj := range resolved {
		alias := util.KotlinIdent(rj.Var)
		if fields[rj.Var] {
			log.Printf("WARNING: %s join %q: property %s clashes with a column, join skipped (set another alias)", tableName, rj.Name, rj.Var)
			continue
		}
		joins = append(joins, KotlinJoin{
			Name:            rj.Name,
			Column:          rj.Local.Column,
			ColumnName:      strings.Trim(kotlinField(rj.Local), "`"),
			Prefix:          rj.Prefix,
			Alias:           alias,
			Var:             "_" + rj.Var,
			ModelName:       KotlinModelName(rj.Table),
			ParentModelName: modelName,
			Index:           joinIndex,
		})
	}

	// Build entity graph paths with nested joins
	entityGraphPaths := buildEntityGraphPaths(joins)

	// 조인 컬럼은 엔티티에서 {컬럼}Id 프로퍼티다 (다른 컬럼과 겹치면 {컬럼}JoinId).
	for i := range kotlinColumns {
		for _, join := range joins {
			if join.Column == kotlinColumns[i].DBName {
				name := join.ColumnName + "Id"
				if fields[name] {
					name = join.ColumnName + "JoinId"
				}
				kotlinColumns[i].EntityField = name
				kotlinColumns[i].Prop = name
				break
			}
		}
	}

	// 정렬 이름: 프로퍼티 이름, 요청 필드 이름, 접두사 뺀 컬럼 이름 (소문자). 비밀번호는 뺀다.
	sorts := make([]KotlinSort, 0)
	sortSeen := map[string]bool{}
	for _, col := range kotlinColumns {
		if col.IsPassword || col.KotlinType == "ByteArray" {
			continue
		}
		for _, k := range []string{strings.ToLower(col.Prop), strings.ToLower(strings.Trim(col.Field, "`")), strings.ToLower(util.GetName(col.DBName)), strings.ToLower(col.DBName)} {
			if k != "" && !sortSeen[k] {
				sortSeen[k] = true
				sorts = append(sorts, KotlinSort{Key: k, Prop: col.Prop})
			}
		}
	}

	hasJsonDate := false
	hasCreatedNow := false
	for _, col := range kotlinColumns {
		if col.DateFormat != "" {
			hasJsonDate = true
		}
		if col.CreatedNow != "" {
			hasCreatedNow = true
		}
	}

	passwordField := ""
	for _, col := range kotlinColumns {
		if col.IsPassword {
			passwordField = col.Field
			break
		}
	}

	pk := kotlinColumns[pkIndex]
	sessions := buildKotlinSessions(tableName, auth, gpa, kotlinColumns, joins)
	requireLogin := auth == "jwt"
	if requireLogin {
		generateSessionProvider(targetPath, basePackage)
	}
	// User 테이블은 id session 이 없어도 수정/삭제는 본인 행만 (Go 와 같다).
	selfOnly := false
	selfConvert := ""
	if requireLogin && baseModelName == "User" && !isView {
		selfOnly = true
		for _, s := range sessions {
			if s.IsPrimaryKey {
				selfOnly = false
			}
		}
		switch pk.KotlinType {
		case "Long":
		case "Int":
			selfConvert = ".toInt()"
		case "String":
			selfConvert = ".toString()"
		default:
			selfOnly = false
		}
	}
	// 읽기 정책: owner 는 ownerSpec / isOwned, login 은 로그인만, public 은 확인 없음, self 는 User 본인 행 (관리자 제외).
	readPolicy := util.MustReadPolicy(baseModelName, gpa, auth)
	readGuard := ""
	switch {
	case readPolicy == util.ReadOwner && len(sessions) > 0:
		readGuard = "!isLoggedIn()"
	case readPolicy != util.ReadPublic && requireLogin:
		readGuard = "_sessionProvider.currentUserId() == null"
	}
	adminLevel := ""
	if n, err := strconv.Atoi(strings.TrimSpace(cnf.AdminLevel)); err == nil {
		adminLevel = strconv.Itoa(n)
	}
	hasLike := false
	for _, col := range kotlinColumns {
		if col.FilterOp == "like" || col.FilterOp == "not like" {
			hasLike = true
		}
	}
	boot4 := cnf.GetSpringBoot() == 4
	generateSupport(targetPath, basePackage, cnf.Timezone, boot4)

	templateData := KotlinTemplateData{
		BasePackage:      basePackage,
		PackageName:      packageName,
		TableName:        tableName,
		ModelName:        modelName,
		UrlName:          util.FileName(tableName),
		ModelVar:         util.KotlinIdent(strings.ToLower(modelName)),
		EnumPackage:      enumPackage,
		Columns:          kotlinColumns,
		PK:               pk,
		HasDateTime:      hasDateTime,
		HasDate:          hasDate,
		HasDateFilter:    hasDateFilter,
		HasDecimal:       hasDecimal,
		IsView:           isView,
		HasIdRoutes:      hasIdColumn || hasPrimaryConfig,
		Enums:            enums,
		Joins:            joins,
		EntityGraphPaths: entityGraphPaths,
		HasPassword:      passwordField != "",
		PasswordField:    passwordField,
		Sessions:         sessions,
		RequireLogin:     requireLogin,
		IsUser:           baseModelName == "User",
		SelfOnly:         selfOnly,
		SelfConvert:      selfConvert,
		Sorts:            sorts,
		HasJsonDate:      hasJsonDate,
		HasCreatedNow:    hasCreatedNow,
		CamelNames:       camelNames,
		DateTimeOut:      kotlinDateTimeOut,
		MaxPageSize:      kotlinMaxPageSize,
		Boot4:            boot4,
		ReadPolicy:       readPolicy,
		ReadOwner:        readPolicy == util.ReadOwner && len(sessions) > 0,
		ReadGuard:        readGuard,
		ReadSelf:         readPolicy == util.ReadSelf && requireLogin,
		SelfRead:         pk.Name == "Id" && (pk.KotlinType == "Long" || pk.KotlinType == "Int"),
		AdminLevel:       adminLevel,
		HasLike:          hasLike,
	}

	// 생성할 빈 / 엔티티와 같은 이름의 사용자 클래스가 있으면 Spring 이 시작할 때 이름이 겹친다.
	warnHandWritten(targetPath, modelName, filepath.Join(targetPath, "entity", modelName+".kt"), map[string]string{
		modelName + "Repository": filepath.Join(targetPath, "repository", modelName+"Repository.kt"),
		modelName + "Service":    filepath.Join(targetPath, "service", modelName+"Service.kt"),
		modelName + "Controller": filepath.Join(targetPath, "controller", modelName+"Controller.kt"),
	})

	views := newViews()
	render(views, "entity.jet", filepath.Join(targetPath, "entity"), modelName+".kt", templateData)
	render(views, "repository.jet", filepath.Join(targetPath, "repository"), modelName+"Repository.kt", templateData)
	render(views, "service.jet", filepath.Join(targetPath, "service"), modelName+"Service.kt", templateData)
	render(views, "controller.jet", filepath.Join(targetPath, "controller"), modelName+"Controller.kt", templateData)
	if len(enums) > 0 {
		render(views, "enums.jet", filepath.Join(targetPath, "enums", strings.Trim(enumPackage, "`")), "Enums.kt", templateData)
	}
}

var camelWarned = false

// warnCamelOnce 는 대문자가 있는 테이블/컬럼 이름에 대한 경고를 한 번 찍는다.
func warnCamelOnce() {
	if camelWarned {
		return
	}
	camelWarned = true
	log.Println("WARNING: some table/column names have upper-case letters (e.g. c_userId). Spring Boot's default naming strategy rewrites them " +
		"(c_userId → c_user_id); set spring.jpa.hibernate.naming.physical-strategy=org.hibernate.boot.model.naming.PhysicalNamingStrategyStandardImpl")
}

// createdExpr 는 createddate 를 채우는 현재 시각 식이다 (DateParams.ZONE 은 model.json timezone).
func createdExpr() string {
	return "DateParams.now()"
}

// kotlinZone 은 model.json timezone 의 ZoneId 식이다. Go 와 같이 비어 있으면 Asia/Seoul (UTC+9), local 은 서버 시간대.
// 모르는 이름이면 경고를 찍고 Asia/Seoul 을 쓴다 (Go 도 LoadLocation 이 실패하면 UTC+9 다).
func kotlinZone(tz string) string {
	tz = strings.TrimSpace(tz)
	switch tz {
	case "":
		return `ZoneId.of("Asia/Seoul")`
	case "local", "Local":
		return "ZoneId.systemDefault()"
	case "utc":
		return `ZoneId.of("UTC")`
	}
	if _, err := time.LoadLocation(tz); err != nil {
		log.Printf("WARNING: unknown timezone %q, Spring uses Asia/Seoul", tz)
		return `ZoneId.of("Asia/Seoul")`
	}
	return "ZoneId.of(" + util.KotlinString(tz) + ")"
}

// buildKotlinSessions 는 auth=jwt 이고 session 설정이 있을 때 소유권 검사용 매핑을 만든다.
// 쓸 수 없는 session 컬럼은 jwt 면 생성을 멈추고 (소유권 검사 없는 API 가 된다), 아니면 경고만 찍는다.
func buildKotlinSessions(tableName string, auth string, gpa *config.Gpa, columns []KotlinColumn, joins []KotlinJoin) []KotlinSession {
	ret := make([]KotlinSession, 0)
	if gpa == nil {
		return ret
	}

	items := make([]util.Column, 0, len(columns))
	for _, col := range columns {
		items = append(items, util.Column{Name: col.Name, Column: col.DBName, Key: util.GetName(col.DBName), OriginalType: kotlinOriginalType(col)})
	}
	for _, pair := range gpa.Session {
		if msg := util.SessionColumnError(items, pair.Column, true); msg != "" {
			if auth == "jwt" {
				log.Fatalf("ERROR: %s: %s (generation stopped)", tableName, msg)
			}
			log.Printf("WARNING: %s: %s", tableName, msg)
			continue
		}
		if auth != "jwt" {
			continue
		}
		var col KotlinColumn
		for i := range items {
			if util.MatchColumn(items[i], pair.Column) {
				col = columns[i]
				break
			}
		}
		convert := ""
		switch col.KotlinType {
		case "Int":
			convert = ".toInt()"
		case "String":
			convert = ".toString()"
		}
		ret = append(ret, KotlinSession{Key: strings.ToLower(pair.Key), Field: col.EntityField, RequestField: col.Field, Convert: convert, IsPrimaryKey: col.IsPrimaryKey, Prop: strings.Trim(col.EntityField, "`")})
	}
	return ret
}

// kotlinOriginalType 은 session 검사용으로 Kotlin 타입을 MySQL 타입 계열로 되돌린다 (Long/Int → 정수, String → 문자열).
func kotlinOriginalType(col KotlinColumn) string {
	if col.IsEnum {
		return "enum"
	}
	switch col.KotlinType {
	case "Long":
		return "bigint"
	case "Int":
		return "int"
	case "String":
		return "varchar"
	}
	return strings.ToLower(col.KotlinType)
}

// generateSessionProvider 는 대상 프로젝트가 구현해야 할 SessionProvider 인터페이스를 한 번만 만든다 (이미 있으면 덮어쓰지 않는다).
func generateSessionProvider(targetPath string, basePackage string) {
	configDir := filepath.Join(targetPath, "config")
	outputPath := filepath.Join(configDir, "SessionProvider.kt")
	if _, err := os.Stat(outputPath); err == nil {
		return
	}
	if err := os.MkdirAll(configDir, 0755); err != nil {
		log.Printf("Failed to create config directory: %v", err)
		return
	}

	content := `package ` + basePackage + `.config

// 생성된 컨트롤러가 로그인 확인과 소유권 검사에 쓰는 현재 로그인 사용자 정보 (auth=jwt).
// 대상 프로젝트에서 인증 방식(JWT 필터, Spring Security 등)에 맞게 @Component 로 구현해야 한다.
interface SessionProvider {
    // 현재 로그인 사용자의 id. 비로그인이면 null (생성된 엔드포인트가 401 을 돌려준다).
    fun currentUserId(): Long?

    // model.json session 의 key 에 해당하는 세션 값. 기본은 "id" 만 지원한다.
    fun currentValue(key: String): Long? = if (key == "id") currentUserId() else null
}
`
	if err := util.WriteFile(outputPath, content); err != nil {
		log.Printf("Failed to write SessionProvider: %v", err)
		return
	}
	log.Printf("Generated Kotlin: %s", outputPath)
}

// legacySupportMarker 는 머리줄이 생기기 전에 만든 DateParams.kt 에 있는 문장이다 (그 파일은 생성 파일로 보고 덮어쓴다).
const legacySupportMarker = "buildtool 이 만드는 파일이다 (다시 생성하면 덮어쓴다)"

// generateSupport 는 생성 코드가 함께 쓰는 support/DateParams.kt 를 만든다 (생성 파일이라 매번 다시 쓴다).
// boot4 면 Jackson 3 (tools.jackson, ValueDeserializer), 아니면 Jackson 2 를 쓴다.
func generateSupport(targetPath string, basePackage string, timezone string, boot4 bool) {
	content := util.GeneratedHeader + "\n" + supportContent(basePackage, timezone, boot4)
	outputPath := filepath.Join(targetPath, "support", "DateParams.kt")
	if old, err := os.ReadFile(outputPath); err == nil && !util.IsGeneratedContent(string(old)) && strings.Contains(string(old), legacySupportMarker) {
		// 예전 생성 파일: 머리줄만 없을 뿐이므로 바로 덮어쓴다.
		if err := util.WriteFile(outputPath, content); err != nil {
			log.Printf("Failed to write %s: %v", outputPath, err)
			return
		}
		log.Printf("Generated Kotlin: %s", outputPath)
		return
	}
	written, err := util.WriteGenerated(outputPath, content)
	if err != nil {
		log.Printf("Failed to write %s: %v", outputPath, err)
		return
	}
	if written {
		log.Printf("Generated Kotlin: %s", outputPath)
	}
}

// supportContent 는 DateParams.kt 내용이다 (머리줄 제외).
func supportContent(basePackage string, timezone string, boot4 bool) string {
	src := supportSource
	if boot4 {
		src = strings.NewReplacer(
			"import com.fasterxml.jackson.core.JsonParser", "import tools.jackson.core.JsonParser",
			"import com.fasterxml.jackson.databind.DeserializationContext", "import tools.jackson.databind.DeserializationContext",
			"import com.fasterxml.jackson.databind.JsonDeserializer", "import tools.jackson.databind.ValueDeserializer",
			": JsonDeserializer<", ": ValueDeserializer<",
		).Replace(src)
	}
	return strings.ReplaceAll(strings.ReplaceAll(src, "{{package}}", basePackage), "{{zone}}", kotlinZone(timezone))
}

const supportSource = `package {{package}}.support

import com.fasterxml.jackson.core.JsonParser
import com.fasterxml.jackson.databind.DeserializationContext
import com.fasterxml.jackson.databind.JsonDeserializer
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeParseException

// buildtool 이 만드는 파일이다 (다시 생성하면 덮어쓴다).

// 잘못된 요청 파라미터 (컨트롤러가 400 으로 돌려준다).
class InvalidParamException(val param: String) : RuntimeException("invalid parameter: $param")

// 날짜 요청 값 해석과 서버 시간대.
object DateParams {
    // model.json timezone (비어 있으면 Asia/Seoul, Go 와 같다). createddate 와 'Z' / 오프셋 입력을 이 시간대 지역 시간으로 바꾼다.
    val ZONE: ZoneId = {{zone}}

    fun now(): LocalDateTime = LocalDateTime.now(ZONE)

    // "2024-01-02" (그날 00:00:00, Go 의 MySQL 비교와 같다), "2024-01-02 03:04[:05]", "2024-01-02T03:04:05[.SSS]",
    // "2024-01-02T03:04:05.000Z" / "+09:00" (ZONE 지역 시간으로 바꾼다). 해석하지 못하면 DateTimeParseException.
    fun parseDateTime(value: String): LocalDateTime {
        val s = value.trim()
        if (s.length == 10) return LocalDate.parse(s).atStartOfDay()
        val iso = s.replace(' ', 'T')
        return try {
            LocalDateTime.parse(iso)
        } catch (e: DateTimeParseException) {
            OffsetDateTime.parse(iso).atZoneSameInstant(ZONE).toLocalDateTime()
        }
    }

    // "2024-01-02", 시간이 붙은 값("2024-01-02 00:00:00", ISO)은 날짜 부분만 쓴다.
    fun parseDate(value: String): LocalDate {
        val s = value.trim()
        if (s.length > 10 && (s[10] == ' ' || s[10] == 'T')) return parseDateTime(s).toLocalDate()
        return LocalDate.parse(s)
    }

    // 쿼리 파라미터: 비어 있으면 null, 해석하지 못하면 InvalidParamException (400).
    fun dateTimeParam(value: String?, name: String): LocalDateTime? {
        if (value.isNullOrBlank()) return null
        return try { parseDateTime(value) } catch (e: Exception) { throw InvalidParamException(name) }
    }

    fun dateParam(value: String?, name: String): LocalDate? {
        if (value.isNullOrBlank()) return null
        return try { parseDate(value) } catch (e: Exception) { throw InvalidParamException(name) }
    }
}

// JSON 요청 본문의 LocalDateTime (DateParams.parseDateTime 형식).
class LenientLocalDateTimeDeserializer : JsonDeserializer<LocalDateTime?>() {
    override fun deserialize(p: JsonParser, ctxt: DeserializationContext): LocalDateTime? {
        val text = p.valueAsString ?: return null
        if (text.isBlank()) return null
        return try {
            DateParams.parseDateTime(text)
        } catch (e: Exception) {
            ctxt.handleWeirdStringValue(LocalDateTime::class.java, text, "expected yyyy-MM-dd[ HH:mm:ss] or ISO-8601") as LocalDateTime?
        }
    }
}

// JSON 요청 본문의 LocalDate (DateParams.parseDate 형식).
class LenientLocalDateDeserializer : JsonDeserializer<LocalDate?>() {
    override fun deserialize(p: JsonParser, ctxt: DeserializationContext): LocalDate? {
        val text = p.valueAsString ?: return null
        if (text.isBlank()) return null
        return try {
            DateParams.parseDate(text)
        } catch (e: Exception) {
            ctxt.handleWeirdStringValue(LocalDate::class.java, text, "expected yyyy-MM-dd") as LocalDate?
        }
    }
}
`

// newViews 는 ~/bin/buildtool/kotlin 템플릿과 헬퍼 함수를 준비한다.
func newViews() *jet.Set {
	templateDir := filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "kotlin")
	views := jet.NewSet(
		jet.NewOSFileSystemLoader(templateDir),
		jet.InDevelopmentMode(),
	)

	views.AddGlobal("title", func(str string) string {
		return strings.Title(str)
	})

	views.AddGlobal("lower", func(str string) string {
		return strings.ToLower(str)
	})

	return views
}

func render(views *jet.Set, templateName string, dir string, fileName string, data KotlinTemplateData) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Failed to create directory %s: %v", dir, err)
		return
	}

	template, err := views.GetTemplate(templateName)
	if err != nil {
		log.Printf("Failed to load template %s: %v", templateName, err)
		return
	}

	// 템플릿 실행이 실패하면 반쯤 만든 파일을 남기지 않는다.
	var b bytes.Buffer
	if err := template.Execute(&b, nil, data); err != nil {
		log.Printf("Failed to execute template %s: %v", templateName, err)
		return
	}

	outputPath := filepath.Join(dir, fileName)
	written, err := util.WriteGenerated(outputPath, util.GeneratedHeader+"\n"+b.String())
	if err != nil {
		log.Printf("Failed to write %s: %v", outputPath, err)
		return
	}
	if written {
		log.Printf("Generated Kotlin: %s", outputPath)
	}
}

// declRe 는 Kotlin / Java 의 클래스 선언 (class / interface / object / record / enum class) 이름이다.
var declRe = regexp.MustCompile(`(?m)^[ \t]*(?:@\w+(?:\([^)\n]*\))?\s+)*(?:(?:public|private|protected|internal|abstract|open|final|data|sealed|static|enum|annotation)\s+)*(?:class|interface|object|record)\s+([A-Za-z_]\w*)`)

// entityRe 는 @Entity 클래스다. 1: @Entity(name = "X") 의 이름, 2: 클래스 이름 (이름을 주지 않으면 JPA 엔티티 이름).
var entityRe = regexp.MustCompile(`@Entity\b(?:\s*\(\s*(?:name\s*=\s*)?"(\w+)"\s*\))?[^{]*?\b(?:class|record)\s+([A-Za-z_]\w*)`)

// HandWritten 은 사용자 파일(생성 머리줄이 없는 .kt / .java)의 클래스 이름 / JPA 엔티티 이름 → 파일 경로 목록이다.
type HandWritten struct {
	Classes  map[string][]string
	Entities map[string][]string
}

// handWrittenCache 는 소스 루트마다 읽은 결과다 (실행마다 한 번만 읽는다).
var handWrittenCache = map[string]HandWritten{}

// handWrittenWarned 는 이미 경고한 이름이다.
var handWrittenWarned = map[string]bool{}

// sourceRoot 는 같은 이름 클래스를 찾을 디렉터리다. targetPath 가 .../src/main/... 아래면 src/main 전체(java + kotlin),
// 아니면 targetPath 다.
func sourceRoot(targetPath string) string {
	abs, err := filepath.Abs(targetPath)
	if err != nil {
		abs = targetPath
	}
	sep := string(filepath.Separator)
	marker := sep + "src" + sep + "main" + sep
	if i := strings.LastIndex(abs+sep, marker); i >= 0 {
		return abs[:i+len(marker)-1]
	}
	return abs
}

// FindHandWritten 은 root 아래 사용자 .kt / .java 파일의 클래스 / 엔티티 이름을 모은다 (build, out, 숨김 디렉터리 제외).
func FindHandWritten(root string) HandWritten {
	hw := HandWritten{Classes: map[string][]string{}, Entities: map[string][]string{}}
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if name := d.Name(); path != root && (name == "build" || name == "out" || name == "node_modules" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".kt") && !strings.HasSuffix(path, ".java") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil || util.IsGeneratedContent(string(b)) {
			return nil
		}
		for _, m := range declRe.FindAllStringSubmatch(string(b), -1) {
			hw.Classes[m[1]] = append(hw.Classes[m[1]], path)
		}
		for _, m := range entityRe.FindAllStringSubmatch(string(b), -1) {
			name := m[1]
			if name == "" {
				name = m[2]
			}
			hw.Entities[name] = append(hw.Entities[name], path)
		}
		return nil
	})
	return hw
}

// warnHandWritten 은 생성할 엔티티(JPA 엔티티 이름) / 빈(repository, service, controller 의 단순 이름 = 빈 이름)과
// 이름이 같은 사용자 클래스가 있으면 경고한다. 그대로 두면 Spring 이 시작할 때 이름이 겹친다.
// outputs 는 이름 → 생성 파일 경로다 (그 파일 자체는 WriteGenerated 가 따로 경고한다).
func warnHandWritten(targetPath string, entity string, entityPath string, beans map[string]string) {
	root := sourceRoot(targetPath)
	hw, ok := handWrittenCache[root]
	if !ok {
		hw = FindHandWritten(root)
		handWrittenCache[root] = hw
	}
	check := func(name string, out string, paths []string, what string) {
		if handWrittenWarned[name] {
			return
		}
		for _, p := range paths {
			if a, err := filepath.Abs(out); err == nil && a == p {
				continue
			}
			if p == out {
				continue
			}
			handWrittenWarned[name] = true
			log.Printf("WARNING: kotlin: hand-written %s %s in %s has the same name as a generated one; Spring bean / JPA entity names will clash. Rename or remove it", what, name, p)
			return
		}
	}
	check(entity, entityPath, hw.Entities[entity], "entity")
	names := make([]string, 0, len(beans))
	for name := range beans {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		check(name, beans[name], hw.Classes[name], "class")
	}
}

// enumZeroIdent 는 코드 0 인 enum 상수 이름이다. data[0] 이 비어 있으면 NONE, 아니면 첫 항목.
func enumZeroIdent(data []string) string {
	if len(data) == 0 {
		return ""
	}
	if data[0] == "" {
		return "NONE"
	}
	if items := util.KotlinEnumItems(data); len(items) > 0 && items[0].Index == 0 {
		return items[0].Ident
	}
	return ""
}

// protectDefault 는 보호 컬럼을 생성할 때 넣을 DB 기본값의 Kotlin 리터럴이다 (u_level DEFAULT 1 → 1).
// 상수 기본값이 없거나 날짜 / 바이트 컬럼이면 "" (예전과 같이 타입 기본값 또는 null).
func protectDefault(col util.Column, kc KotlinColumn, gpa *config.Gpa) string {
	lit, ok := util.DefaultLiteral(col)
	if !ok {
		return ""
	}
	if kc.IsEnum {
		m := util.FindEnumMap(gpa, col.Key)
		f, err := strconv.ParseFloat(lit, 64)
		if m == nil || err != nil {
			return ""
		}
		for _, it := range util.KotlinEnumItems(m.Data) {
			if float64(it.Code) == f {
				return kc.QualType + "." + it.Ident
			}
		}
		if len(m.Data) > 0 && m.Data[0] == "" && f == 0 {
			return kc.QualType + ".NONE"
		}
		return ""
	}
	switch kc.KotlinType {
	case "String":
		return util.KotlinString(lit)
	case "Int", "Long", "Double", "Boolean", "BigDecimal":
		f, err := strconv.ParseFloat(lit, 64)
		if err != nil {
			return ""
		}
		switch kc.KotlinType {
		case "Int":
			return strconv.FormatInt(int64(f), 10)
		case "Long":
			return strconv.FormatInt(int64(f), 10) + "L"
		case "Double":
			v := strconv.FormatFloat(f, 'f', -1, 64)
			if !strings.Contains(v, ".") {
				v += ".0"
			}
			return v
		case "Boolean":
			return strconv.FormatBool(f != 0)
		case "BigDecimal":
			return "java.math.BigDecimal(" + util.KotlinString(lit) + ")"
		}
	}
	return ""
}
