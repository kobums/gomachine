package config

import (
	"fmt"

	"github.com/spf13/viper"
)

var (
	Database			string
	User				string
	Server				string
	Password			string
	Language			string
	Port				string
	UploadPath			string
)

func init() {
	viper.SetConfigType("json")
	viper.SetConfigName("config")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("../config")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()

	if err != nil {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	if value := viper.Get("Database"); value != nil {
		Database = value.(string)
	}

	if value := viper.Get("User"); value != nil {
		User = value.(string)
	}

	if value := viper.Get("Server"); value != nil {
		Server = value.(string)
	}

	if value := viper.Get("Password"); value != nil {
		Password = value.(string)
	}

	if value := viper.Get("Language"); value != nil {
		Language = value.(string)
	}

	if value := viper.Get("port"); value != nil {
		Port = value.(string)
	}

	if value := viper.Get("uploadPath"); value != nil {
		UploadPath = value.(string)
	}
}