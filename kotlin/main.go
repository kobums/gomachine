package kotlin

import (
	"bytes"
	"database/sql"
	"gomachine/config"
	"gomachine/util"
	"os"
	"path/filepath"
	"strings"

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
	IsGenerated  bool // 정수형 기본 키: @GeneratedValue(IDENTITY)
	HasDefault   bool
	DefaultValue string
	IsEnum       bool
	EnumType     string
	IsPassword   bool   // 비밀번호 컬럼: 응답/검색에서 제외하고 저장 시 해시한다
	IsProtected  bool   // 보호 컬럼: Create/Update/Patch 요청으로 바꿀 수 없다
	IsSearchable bool   // 목록 필터 / /search 엔드포인트 / findBy 를 만든다
	DateFormat   string // LocalDateTime / LocalDate 요청 파라미터의 @DateTimeFormat ISO 값 (DATE_TIME / DATE)
}

// KotlinSession 은 세션 값(SessionProvider.currentValue(Key))과 엔티티 컬럼의 매핑이다.
type KotlinSession struct {
	Key          string
	Field        string // 엔티티 프로퍼티 이름
	RequestField string // Create/Update/Patch 요청 프로퍼티 이름
	Convert      string // 세션 값(Long)을 컬럼 타입으로 바꾸는 접미사 (예: ".toInt()")
	IsPrimaryKey bool   // id 컬럼이면 요청 값을 덮어쓰지 않는다 (경로 변수 id 와 소유권 검사로 충분)
}

type KotlinEnumData struct {
	Name        string // enum class 이름 (CamelCase)
	ExtraField  string // ExtraInfo 프로퍼티 이름
	ColumnField string // 이 enum 을 쓰는 엔티티 프로퍼티. 컬럼이 없으면 ""
	Items       []util.EnumItem
	EnumPackage string
}

type KotlinJoin struct {
	Name            string
	Column          string
	ColumnName      string // camelCase column name for Kotlin
	Prefix          string
	Alias           string
	AliasVar        string // 별칭을 매개변수로 쓸 때의 이름
	ModelName       string
	ParentModelName string // The main model name
	Columns         []KotlinColumn
	Index           int // Index in the joins array
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
	ModelName        string
	ModelVar         string // 모델을 매개변수로 쓸 때의 이름
	Columns          []KotlinColumn
	PK               KotlinColumn // 기본 키 컬럼
	HasDateTime      bool         // LocalDateTime 컬럼이 있다
	HasDate          bool         // LocalDate 컬럼이 있다
	HasDateFilter    bool         // 목록 필터에 날짜 범위 컬럼이 있다
	HasDecimal       bool
	IsView           bool // _vw: 읽기 전용 (쓰기 엔드포인트/서비스 메서드 없음)
	Enums            []KotlinEnumData
	Joins            []KotlinJoin
	UniqueJoinNames  []string          // Unique join model names for imports
	EntityGraphPaths []EntityGraphPath // Nested entity graph paths for @EntityGraph with metadata
	HasPassword      bool
	PasswordField    string          // 비밀번호 컬럼의 프로퍼티 이름 (passwd / password)
	Sessions         []KotlinSession // auth=jwt 이고 session 설정이 있을 때만 채워진다
}

// Build a global map of all table joins for recursive entity graph path building
func buildGlobalJoinsMap(cnf config.ModelConfig) map[string][]config.GpaJoin {
	joinsMap := make(map[string][]config.GpaJoin)
	for _, gpa := range cnf.Gpa {
		if gpa.Join != nil && len(gpa.Join) > 0 {
			joinsMap[gpa.Name] = gpa.Join
		}
	}
	return joinsMap
}

// Recursively build entity graph paths
// Example: if current table has join to "gym", and "gym" has join to "owner",
// this will return ["gym", "gym.owner"]
func buildEntityGraphPaths(joins []KotlinJoin, globalJoinsMap map[string][]config.GpaJoin, visited map[string]bool) []EntityGraphPath {
	pathStrings := make([]string, 0)

	for _, join := range joins {
		alias := strings.ToLower(join.Alias)

		// Add the direct join path
		pathStrings = append(pathStrings, alias)

		// Check if this joined table has its own joins
		if nestedJoins, exists := globalJoinsMap[join.Name]; exists && len(nestedJoins) > 0 {
			// Prevent circular references
			if visited[join.Name] {
				continue
			}
			visited[join.Name] = true

			// Add nested join paths
			for _, nestedJoin := range nestedJoins {
				nestedAlias := nestedJoin.Alias
				if nestedAlias == "" {
					nestedAlias = nestedJoin.Name
				}
				nestedPath := alias + "." + strings.ToLower(nestedAlias)
				pathStrings = append(pathStrings, nestedPath)
			}

			delete(visited, join.Name)
		}
	}

	// Convert to EntityGraphPath with metadata
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

// kotlinField 는 컬럼의 Kotlin 프로퍼티 이름이다. 밑줄 하나짜리 컬럼은 예전과 같이 전부 소문자 (createddate),
// 밑줄이 더 있으면 camelCase (created_at → createdAt). 예약어는 backtick 으로 감싼다.
func kotlinField(col util.Column) string {
	key := col.Key
	if key == "" {
		key = col.Name
	}
	return util.KotlinIdent(util.LowerCamel(key))
}

// findPrimaryKey 는 기본 키 컬럼 위치다. gpa.primary > 이름이 id 인 컬럼 > 첫 번째 컬럼 순서.
func findPrimaryKey(columns []util.Column, gpa *config.Gpa) int {
	if gpa != nil {
		for _, p := range gpa.Primary {
			for i, col := range columns {
				if util.MatchColumn(col, p) {
					return i
				}
			}
		}
	}
	for i, col := range columns {
		if col.Name == "Id" {
			return i
		}
	}
	return 0
}

func defaultValue(kotlinType string) string {
	switch kotlinType {
	case "LocalDateTime":
		return "LocalDateTime.now()"
	case "LocalDate":
		return "LocalDate.now()"
	case "BigDecimal":
		return "BigDecimal.ZERO"
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

func ProcessKotlin(packageName string, tableName string, prefix string, columns []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	modelName := util.ModelName(tableName)

	// Build global joins map for recursive entity graph paths
	globalJoinsMap := buildGlobalJoinsMap(cnf)

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

	protects := util.ProtectedColumns(modelName, columns, gpa)
	kotlinColumns := make([]KotlinColumn, 0)
	hasDateTime := false
	hasDate := false
	hasDateFilter := false
	hasDecimal := false
	pkIndex := findPrimaryKey(columns, gpa)

	for i, col := range columns {
		isPrimaryKey := i == pkIndex

		// Check for nullable and default values based on type and position
		isDateType := col.Type == "LocalDateTime" || col.Type == "LocalDate"
		isNullable := !isPrimaryKey && (isDateType || strings.Contains(col.Column, "_date"))
		hasDefault := false
		defValue := ""

		// Check if this column is an enum type
		isEnumType := false
		enumTypeName := ""
		if m := util.FindEnumMap(gpa, col.Key); m != nil {
			isEnumType = true
			enumTypeName = col.Type
			// 첫 번째 항목을 기본값으로 쓴다 (예: "notice:공지" → Type.NOTICE)
			if items := util.KotlinEnumItems(m.Data); len(items) > 0 {
				hasDefault = true
				defValue = enumTypeName + "." + items[0].Ident
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

		isGenerated := false
		// Special handling for primary key
		if isPrimaryKey {
			hasDefault = true
			isNullable = false
			switch col.Type {
			case "Long", "Int":
				defValue = "0"
				isGenerated = true
			default:
				defValue = defaultValue(col.Type)
				hasDefault = defValue != ""
			}
		}

		// Determine DBType for Response
		dbType := col.Type
		if isEnumType {
			dbType = "Int" // Enums are stored as Int (ordinal) in Response
		} else if isDateType {
			dbType = "String" // LocalDateTime / LocalDate is converted to String in Response
		}

		isPassword := util.IsPasswordColumn(col.Name) && col.Type == "String"
		kotlinCol := KotlinColumn{
			Name:         col.Name,
			Field:        kotlinField(col),
			DBName:       col.Column,
			KotlinType:   col.Type,
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
			IsSearchable: !isPrimaryKey && col.Name != "Id" && !isPassword && col.Type != "ByteArray",
		}
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
		kotlinColumns = append(kotlinColumns, kotlinCol)
	}

	if len(kotlinColumns) == 0 {
		log.Printf("WARNING: %s has no column, skip kotlin", tableName)
		return
	}

	// Get enum data from GPA and convert to KotlinEnumData
	enums := make([]KotlinEnumData, 0)
	enumPackage := util.KotlinIdent(strings.ToLower(modelName))
	if gpa != nil {
		for _, gpaMap := range gpa.Map {
			e := KotlinEnumData{
				Name:        util.CamelCase(gpaMap.Name),
				ExtraField:  util.KotlinIdent(strings.ToLower(gpaMap.Name)),
				Items:       util.KotlinEnumItems(gpaMap.Data),
				EnumPackage: enumPackage,
			}
			for i, col := range columns {
				if m := util.FindEnumMap(gpa, col.Key); m != nil && m.Name == gpaMap.Name {
					e.ColumnField = kotlinColumns[i].Field
				}
			}
			enums = append(enums, e)
		}
	}

	// Process joins
	joins := make([]KotlinJoin, 0)
	if gpa != nil {
		for joinIndex, join := range gpa.Join {
			joinTableName := strings.ToLower(join.Name) + "_tb"
			joinModelName := util.ModelName(joinTableName)

			// Get columns for joined table
			joinColumns := make([]KotlinColumn, 0)
			if db != nil {
				query := "select column_name as column_name, data_type as data_type from information_schema.columns where table_schema = '" + packageName + "' and table_name = '" + joinTableName + "' order by ordinal_position"
				rows, err := db.Query(query)
				if err != nil {
					log.Printf("Failed to query join table columns: %v", err)
					continue
				}

				for rows.Next() {
					var colName, colType string
					err := rows.Scan(&colName, &colType)
					if err != nil {
						continue
					}

					col := util.NewColumn(joinTableName, colName, colType, nil, cnf)

					// Simple processing for joined columns
					isPrimaryKey := strings.Contains(strings.ToLower(colName), "_id")
					dbType := col.Type
					if dbType == "LocalDateTime" || dbType == "LocalDate" {
						dbType = "String"
					}

					joinColumns = append(joinColumns, KotlinColumn{
						Name:         col.Name,
						Field:        kotlinField(col),
						DBName:       colName,
						KotlinType:   col.Type,
						DBType:       dbType,
						ModelName:    joinModelName,
						IsNullable:   !isPrimaryKey,
						IsPrimaryKey: isPrimaryKey,
					})
				}
				rows.Close()
			}

			alias := join.Alias
			if alias == "" {
				alias = join.Name
			}

			// Convert column name to camelCase for Kotlin (e.g., author_id -> authorId)
			columnName := util.LowerCamel(join.Column)
			if columnName == "" {
				log.Printf("WARNING: join %s of %s has no column, skipped", join.Name, tableName)
				continue
			}

			// Build full column name with prefix (e.g., "gym" -> "m_gym")
			// Always use current table's prefix for the join column
			fullColumnName := prefix + "_" + join.Column

			joins = append(joins, KotlinJoin{
				Name:            join.Name,
				Column:          fullColumnName,
				ColumnName:      columnName,
				Prefix:          join.Prefix,
				Alias:           alias,
				AliasVar:        util.KotlinIdent(strings.ToLower(alias)),
				ModelName:       joinModelName,
				ParentModelName: modelName,
				Columns:         joinColumns,
				Index:           joinIndex,
			})
		}
	}

	// Extract unique join model names for imports
	uniqueJoinNamesMap := make(map[string]bool)
	var uniqueJoinNames []string
	for _, join := range joins {
		if !uniqueJoinNamesMap[join.Name] {
			uniqueJoinNamesMap[join.Name] = true
			uniqueJoinNames = append(uniqueJoinNames, join.Name)
		}
	}

	// Build entity graph paths with nested joins
	visited := make(map[string]bool)
	entityGraphPaths := buildEntityGraphPaths(joins, globalJoinsMap, visited)

	passwordField := ""
	for _, col := range kotlinColumns {
		if col.IsPassword {
			passwordField = col.Field
			break
		}
	}

	basePackage := cnf.GetKotlinPackage()
	sessions := buildKotlinSessions(auth, gpa, kotlinColumns, joins)
	if len(sessions) > 0 {
		generateSessionProvider(targetPath, basePackage)
	}

	templateData := KotlinTemplateData{
		BasePackage:      basePackage,
		PackageName:      packageName,
		TableName:        tableName,
		ModelName:        modelName,
		ModelVar:         util.KotlinIdent(strings.ToLower(modelName)),
		Columns:          kotlinColumns,
		PK:               kotlinColumns[pkIndex],
		HasDateTime:      hasDateTime,
		HasDate:          hasDate,
		HasDateFilter:    hasDateFilter,
		HasDecimal:       hasDecimal,
		IsView:           util.GetTableType(tableName) == "view",
		Enums:            enums,
		Joins:            joins,
		UniqueJoinNames:  uniqueJoinNames,
		EntityGraphPaths: entityGraphPaths,
		HasPassword:      passwordField != "",
		PasswordField:    passwordField,
		Sessions:         sessions,
	}

	views := newViews()
	render(views, "entity.jet", filepath.Join(targetPath, "entity"), modelName+".kt", templateData)
	render(views, "repository.jet", filepath.Join(targetPath, "repository"), modelName+"Repository.kt", templateData)
	render(views, "service.jet", filepath.Join(targetPath, "service"), modelName+"Service.kt", templateData)
	render(views, "controller.jet", filepath.Join(targetPath, "controller"), modelName+"Controller.kt", templateData)
	if len(enums) > 0 {
		render(views, "enums.jet", filepath.Join(targetPath, "enums", strings.ToLower(modelName)), "Enums.kt", templateData)
	}
}

// buildKotlinSessions 는 auth=jwt 이고 session 설정이 있을 때 소유권 검사용 매핑을 만든다.
func buildKotlinSessions(auth string, gpa *config.Gpa, columns []KotlinColumn, joins []KotlinJoin) []KotlinSession {
	ret := make([]KotlinSession, 0)
	if auth != "jwt" || gpa == nil {
		return ret
	}

	for _, pair := range gpa.Session {
		found := false
		for _, col := range columns {
			if !util.MatchColumn(util.Column{Name: col.Name, Column: col.DBName, Key: util.GetName(col.DBName)}, pair.Column) {
				continue
			}
			found = true

			field := col.Field
			for _, join := range joins {
				if join.Column == col.DBName {
					field = join.ColumnName + "Id"
				}
			}

			convert := ""
			switch col.KotlinType {
			case "Long":
				convert = ""
			case "Int":
				convert = ".toInt()"
			case "String":
				convert = ".toString()"
			default:
				log.Printf("WARNING: session column %s has unsupported type %s, ownership check skipped", col.Name, col.KotlinType)
				found = false
			}
			if found {
				ret = append(ret, KotlinSession{Key: strings.ToLower(pair.Key), Field: field, RequestField: col.Field, Convert: convert, IsPrimaryKey: col.IsPrimaryKey})
			}
			break
		}
		if !found {
			log.Printf("WARNING: session column %s not found in %s", pair.Column, gpa.Name)
		}
	}
	return ret
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

// 생성된 컨트롤러가 소유권 검사에 쓰는 현재 로그인 사용자 정보.
// 대상 프로젝트에서 인증 방식(JWT 필터, Spring Security 등)에 맞게 @Component 로 구현해야 한다.
interface SessionProvider {
    // 현재 로그인 사용자의 id. 비로그인이면 null.
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

	// Add helper function to find join by column name
	views.AddGlobal("findJoinByColumn", func(columnDBName string, joins []KotlinJoin) *KotlinJoin {
		for i := range joins {
			if joins[i].Column == columnDBName {
				return &joins[i]
			}
		}
		return nil
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
	if err := util.WriteFile(outputPath, b.String()); err != nil {
		log.Printf("Failed to write %s: %v", outputPath, err)
		return
	}

	log.Printf("Generated Kotlin: %s", outputPath)
}
