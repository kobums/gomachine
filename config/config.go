package config

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path"
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
	ReactModelFilePath  string `json:"reactModelFilePath"`
	VueModelFilePath    string `json:"vueModelFilePath"`
	ModelJsonPath       string `json:"modelJsonPath"`
	// 생성할 언어별 on/off. 예: {"go": true, "react": true, "spring": false}. "all": true 면 전부 생성.
	// 비어 있으면 예전 방식대로 language 필드를 쓴다.
	Languages map[string]bool `json:"languages"`
	Gpa       []Gpa           `json:"table"`
}

type Pubspec struct {
	Name string `yaml:"name"`
}

func Init(dir string) ModelConfig {
	var modelConfig ModelConfig

	log.Println("config dir", dir)
	file, err := os.Open(path.Join(dir, "model.json"))
	if err != nil {
		log.Println(err)
		return modelConfig
	}

	defer file.Close()
	data, _ := ioutil.ReadAll(file)

	if err := json.Unmarshal(data, &modelConfig); err != nil {
		log.Println("Error parsing model.json:", err)
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

	return modelConfig
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
