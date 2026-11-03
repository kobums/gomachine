package vue

import (
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

	views := jet.NewSet(jet.NewOSFileSystemLoader(filepath.Join(os.Getenv("HOME"), "bin", "buildtool", "vue")), jet.InDevelopmentMode())
	react.SetupJetGlobals(views)

	out := react.OutDir(targetPath, cnf)
	semi := cnf.UseTsSemicolons()
	react.Render(views, "contract.jet", filepath.Join(out, "models"), "contract.ts", data, semi)
	react.Render(views, "type.jet", filepath.Join(out, "types"), name+".ts", data, semi)
	react.Render(views, "model.jet", filepath.Join(out, "models"), name+".ts", data, semi)
	react.Render(views, "composable.jet", filepath.Join(out, "composables"), "use"+data.ModelName+".ts", data, semi)
	react.RenderAPI(views, out, semi)
}
