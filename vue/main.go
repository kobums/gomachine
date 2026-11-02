package vue

import (
	"bytes"
	"database/sql"
	"gomachine/config"
	"gomachine/react"
	"gomachine/util"
	"os"
	"path/filepath"
	"strings"

	"github.com/CloudyKit/jet/v6"
	log "github.com/sirupsen/logrus"
)

// ProcessVue 는 Vue 3 + TypeScript 용 타입, API 모델, composable 을 생성한다.
// 템플릿 데이터는 react 생성기와 같은 것을 쓴다.
func ProcessVue(packageName string, tableName string, prefix string, items []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	targetPath := cnf.VueModelFilePath
	if targetPath == "" {
		log.Println("vueModelFilePath is not set, skip vue")
		return
	}

	data := react.BuildTemplateData(packageName, tableName, items, gpa)
	name := strings.ToLower(data.TableName)

	templateDir := filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "vue")
	views := jet.NewSet(jet.NewOSFileSystemLoader(templateDir), jet.InDevelopmentMode())
	react.SetupJetGlobals(views)

	generate(views, "type.jet", filepath.Join(targetPath, "src", "types"), name+".ts", data)
	generate(views, "model.jet", filepath.Join(targetPath, "src", "models"), name+".ts", data)
	generate(views, "composable.jet", filepath.Join(targetPath, "src", "composables"), "use"+data.ModelName+".ts", data)
}

func generate(views *jet.Set, templateName string, dir string, filename string, data react.ReactTemplateData) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Failed to create directory %s: %v", dir, err)
		return
	}

	t, err := views.GetTemplate(templateName)
	if err != nil {
		log.Printf("Failed to load Vue template %s: %v", templateName, err)
		return
	}

	var b bytes.Buffer
	if err := t.Execute(&b, nil, data); err != nil {
		log.Printf("Failed to execute Vue template %s: %v", templateName, err)
		return
	}

	outputPath := filepath.Join(dir, filename)
	if err := util.WriteFile(outputPath, b.String()); err != nil {
		log.Printf("ERROR: Failed to write Vue file %s: %v", outputPath, err)
	} else {
		log.Printf("Generated Vue: %s", outputPath)
	}
}
