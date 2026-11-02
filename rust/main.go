package rustcodegen

import (
	"database/sql"
	"gomachine/config"
	"gomachine/util"

	log "github.com/sirupsen/logrus"
)

// ProcessRust 는 Rust(axum + sqlx) 백엔드를 생성한다. 아직 템플릿이 없어서 경고만 찍는다 (다음 단계에서 구현).
// 생성 파일은 cnf.RustModelFilePath(crate 루트) 아래 src/ 에 둔다. 모듈은 crate:: 경로로 참조하므로 crate 이름은 필요 없다.
func ProcessRust(packageName string, tableName string, prefix string, items []util.Column, db *sql.DB, gpa *config.Gpa, version string, auth string, cnf config.ModelConfig) {
	log.Printf("WARNING: rust generator not implemented yet, %s skipped", tableName)
}
