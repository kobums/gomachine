package config

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
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
}

type ModelConfig struct {
	Buildtool string `json:"buildtool"`
	Server    string `json:"server"`
	Database  string `json:"database"`
	User      string `json:"user"`
	Password  string `json:"password"`
	Auth      string `json:"auth"`
	Language  string `json:"language"`
	GoModelFilePath string `json:"goModelFilePath"`
	DartModelFilePath string `json:"dartModelFilePath"`
	Gpa       []Gpa  `json:"table"`
}


func Init(dir string) ModelConfig {
	file, _ := os.Open(path.Join(dir, "config/config.json"))
	defer file.Close()
	data, _ := ioutil.ReadAll(file)

	var modelConfig ModelConfig

	json.Unmarshal(data, &modelConfig)

	return modelConfig
}

// func Init() {
// 	viper.SetConfigType("json")
// 	viper.SetConfigName("config")
// 	viper.AddConfigPath("./config")
// 	viper.AddConfigPath("../config")
// 	viper.AddConfigPath(".")
// 	err := viper.ReadInConfig()

// 	if err != nil {
// 		panic(fmt.Errorf("Fatal error config file: %s \n", err))
// 	}

// 	if value := viper.Get("Database"); value != nil {
// 		Database = value.(string)
// 	}

// 	if value := viper.Get("User"); value != nil {
// 		User = value.(string)
// 	}

// 	if value := viper.Get("Server"); value != nil {
// 		Server = value.(string)
// 	}

// 	if value := viper.Get("Password"); value != nil {
// 		Password = value.(string)
// 	}

// 	if value := viper.Get("Language"); value != nil {
// 		Language = value.(string)
// 	}

// 	if value := viper.Get("port"); value != nil {
// 		Port = value.(string)
// 	}

// 	if value := viper.Get("uploadPath"); value != nil {
// 		UploadPath = value.(string)
// 	}
// }