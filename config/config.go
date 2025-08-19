package config

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path"

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
	Buildtool string `json:"buildtool"`
	Store      string `json:"store"`
	Server    string `json:"server"`
	Database  string `json:"database"`
	User      string `json:"user"`
	Password  string `json:"password"`
	Auth      string `json:"auth"`
	AdminLevel string `json:"adminLevel"`
	Language  string `json:"language"`
	GoModelFilePath string `json:"goModelFilePath"`
	DartModelFilePath string `json:"dartModelFilePath"`
	Gpa       []Gpa  `json:"table"`
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

	json.Unmarshal(data, &modelConfig)

	return modelConfig
}

func GetPubspec() string {
	file, _ := os.Open("pubspec.yml")
	defer file.Close()
	data, _ := ioutil.ReadAll(file)

	var pubspec Pubspec

	yaml.Unmarshal(data, &pubspec)

	return pubspec.Name
}