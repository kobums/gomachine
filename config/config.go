package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type GpaMap struct {
	Name string   `json:"name"`
	Data []string `json:"data"`
}

type GpaJoin struct {
	Name   string `json:"name"`
	Column string `json:"column"`
	Prefix string `json:"prefix"`
	Alias  string `json:"alias"`
}

type GpaCompare struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type SessionPair struct {
	Key    string `json:"key"`
	Column string `json:"column"`
}

type Gpa struct {
	Name    string        `json:"name"`
	Map     []GpaMap      `json:"map"`
	Method  []string      `json:"method"`
	Join    []GpaJoin     `json:"join"`
	Compare []GpaCompare  `json:"compare"`
	Session []SessionPair `json:"session"`
	Search  bool          `json:"search"`
	Primary []string      `json:"primary"`
	// REST Insert/Update 에서 클라이언트 값을 받지 않을 컬럼 (예: ["level"]). Insert 는 0 값, Update 는 기존 DB 값 유지.
	Protect []string `json:"protect"`
}

type ModelConfig struct {
	Buildtool           string `json:"buildtool"`
	Store               string `json:"store"`
	Server              string `json:"server"`
	Database            string `json:"database"`
	User                string `json:"user"`
	Password            string `json:"password"`
	Auth                string `json:"auth"`
	AdminLevel          string `json:"adminLevel"`
	Language            string `json:"language"`
	GoModelFilePath     string `json:"goModelFilePath"`
	DartModelFilePath   string `json:"dartModelFilePath"`
	KotlinModelFilePath string `json:"kotlinModelFilePath"`
	// 생성되는 Kotlin 코드의 기본 패키지. 비어 있으면 예전 값 com.gowoobro.gymspring 을 쓴다.
	// kotlinModelFilePath 는 이 패키지의 디렉터리(예: src/main/kotlin/com/example/app)를 가리켜야 한다.
	KotlinPackage      string `json:"kotlinPackage"`
	ReactModelFilePath string `json:"reactModelFilePath"`
	VueModelFilePath   string `json:"vueModelFilePath"`
	ModelJsonPath      string `json:"modelJsonPath"`
	// 생성할 언어별 on/off. 예: {"go": true, "react": true, "spring": false}. "all": true 면 전부 생성.
	// 비어 있으면 예전 방식대로 language 필드를 쓴다.
	Languages map[string]bool `json:"languages"`
	// Go Insert 가 createddate 를 채울 때 쓰는 시간대. 비어 있으면 예전과 같은 UTC+9 (Asia/Seoul),
	// "local" 은 서버 지역 시간, 그 밖은 IANA 이름 (예: "UTC", "America/New_York").
	Timezone string `json:"timezone"`
	Gpa      []Gpa  `json:"table"`
}

type Pubspec struct {
	Name string `yaml:"name"`
}

// Init 은 dir/model.json 을 읽는다. 읽거나 해석하지 못하면 에러를 찍고 생성을 멈춘다 (exit 1).
// 반쯤 읽은 설정으로 계속하면 session / protect 같은 보안 설정이 조용히 빠질 수 있다.
func Init(dir string) ModelConfig {
	modelConfig, _, err := Load(dir)
	if err != nil {
		log.Println("ERROR:", err)
		os.Exit(1)
	}
	return modelConfig
}

// Load 는 dir/model.json 을 읽는다. JSON 문법 / 타입 오류는 에러다. 모르는 key 는 에러가 아니고 경고 목록으로 돌려준다
// (예전 model.json 에 남은 key 가 있을 수 있다). 경고는 로그에도 찍는다.
func Load(dir string) (ModelConfig, []string, error) {
	var modelConfig ModelConfig

	file := path.Join(dir, "model.json")
	log.Println("config", file)
	data, err := os.ReadFile(file)
	if err != nil {
		return modelConfig, nil, fmt.Errorf("cannot read %s: %v", file, err)
	}

	modelConfig, warnings, err := Parse(data)
	if err != nil {
		return modelConfig, warnings, fmt.Errorf("invalid %s: %v", file, err)
	}
	for _, w := range warnings {
		log.Printf("WARNING: %s: %s", file, w)
	}

	// DB 접속 정보는 model.json 에 커밋하지 않고 환경변수로 주입할 수 있다 (설정 시 model.json 값보다 우선).
	if v := os.Getenv("DB_HOST"); v != "" {
		modelConfig.Server = v
	}
	if v := os.Getenv("DB_USER"); v != "" {
		modelConfig.User = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		modelConfig.Password = v
	}

	return modelConfig, warnings, nil
}

// Parse 는 model.json 내용을 해석한다. 문법 / 타입 오류는 에러, 모르는 key 는 경고("unknown key table[0].sesion") 다.
func Parse(data []byte) (ModelConfig, []string, error) {
	var modelConfig ModelConfig
	if err := json.Unmarshal(data, &modelConfig); err != nil {
		return modelConfig, nil, err
	}
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return modelConfig, nil, err
	}
	warnings := make([]string, 0)
	unknownKeys(raw, reflect.TypeOf(modelConfig), "", &warnings)
	return modelConfig, warnings, nil
}

// unknownKeys 는 raw(JSON 값)에서 t(구조체)의 json 태그에 없는 key 를 찾아 warnings 에 넣는다. 배열 / 하위 구조체도 본다.
func unknownKeys(raw interface{}, t reflect.Type, at string, warnings *[]string) {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
		if t.Kind() == reflect.Slice {
			if arr, ok := raw.([]interface{}); ok {
				for i, v := range arr {
					unknownKeys(v, t.Elem(), fmt.Sprintf("%s[%d]", at, i), warnings)
				}
			}
			return
		}
		t = t.Elem()
	}
	obj, ok := raw.(map[string]interface{})
	if !ok || t.Kind() != reflect.Struct {
		return
	}
	fields := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			fields[strings.ToLower(name)] = t.Field(i).Type
		}
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		name := k
		if at != "" {
			name = at + "." + k
		}
		// encoding/json 처럼 key 는 대소문자를 무시한다.
		ft, ok := fields[strings.ToLower(k)]
		if !ok {
			*warnings = append(*warnings, "unknown key "+name+" (ignored)")
			continue
		}
		unknownKeys(obj[k], ft, name, warnings)
	}
}

func GetGoModelFilePath(dir string) string {
	log.Println("config dir", path.Join(dir, "model.json"))
	file, err := os.Open(path.Join(dir, "model.json"))
	if err != nil {
		log.Println("Error opening model.json:", err)
		return ""
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println("Error reading model.json:", err)
		return ""
	}

	var config struct {
		GoModelFilePath string `json:"goModelFilePath"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		log.Println("Error parsing model.json:", err)
		return ""
	}

	return config.GoModelFilePath
}

func GetPubspec() string {
	file, _ := os.Open("pubspec.yml")
	defer file.Close()
	data, _ := ioutil.ReadAll(file)

	var pubspec Pubspec

	yaml.Unmarshal(data, &pubspec)

	return pubspec.Name
}

// SupportedLanguages 는 생성 순서이기도 하다.
var SupportedLanguages = []string{"go", "kotlin", "dart", "react", "vue"}

var languageAliases = map[string]string{
	"spring":  "kotlin",
	"flutter": "dart",
}

// NormalizeLanguage 는 별칭(spring, flutter)을 실제 생성기 이름으로 바꾼다. 지원하지 않으면 "" 를 반환한다.
func NormalizeLanguage(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if alias, ok := languageAliases[name]; ok {
		name = alias
	}
	for _, lang := range SupportedLanguages {
		if lang == name {
			return name
		}
	}
	return ""
}

// ResolveLanguages 는 언어 이름 목록("all" 포함 가능)을 중복 없이 SupportedLanguages 순서로 정리한다.
func ResolveLanguages(names []string) []string {
	enabled := make(map[string]bool)
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		if name == "all" {
			return append([]string(nil), SupportedLanguages...)
		}
		lang := NormalizeLanguage(name)
		if lang == "" {
			log.Printf("WARNING: unsupported language %q is skipped (supported: go, kotlin/spring, dart/flutter, react, vue)", name)
			continue
		}
		enabled[lang] = true
	}

	ret := make([]string, 0)
	for _, lang := range SupportedLanguages {
		if enabled[lang] {
			ret = append(ret, lang)
		}
	}
	return ret
}

// EnabledLanguages 는 model.json 기준으로 생성할 언어 목록을 돌려준다.
// languages 맵 > language 문자열(쉼표 구분 가능) > 기본값 go 순서로 결정한다.
func (c ModelConfig) EnabledLanguages() []string {
	if len(c.Languages) > 0 {
		names := make([]string, 0)
		for name, on := range c.Languages {
			if on {
				names = append(names, name)
			}
		}
		return ResolveLanguages(names)
	}
	if c.Language != "" {
		return ResolveLanguages(strings.Split(c.Language, ","))
	}
	return []string{"go"}
}

// DefaultKotlinPackage 는 kotlinPackage 가 없을 때 쓰는 예전 패키지 이름이다.
const DefaultKotlinPackage = "com.gowoobro.gymspring"

// GetKotlinPackage 는 생성할 Kotlin 코드의 기본 패키지다.
func (c ModelConfig) GetKotlinPackage() string {
	if p := strings.TrimSpace(c.KotlinPackage); p != "" {
		return p
	}
	return DefaultKotlinPackage
}

// OutputPath 는 언어별 생성 경로 설정값을 돌려준다.
func (c ModelConfig) OutputPath(lang string) string {
	switch lang {
	case "go":
		return c.GoModelFilePath
	case "kotlin":
		return c.KotlinModelFilePath
	case "dart":
		return c.DartModelFilePath
	case "react":
		return c.ReactModelFilePath
	case "vue":
		return c.VueModelFilePath
	}
	return ""
}
