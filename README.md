# go machine

이 프로그램은 Go를 이용하여 귀찮고 반복되는 것들을 안하게 하기 위함이다.

go, spring(kotlin), flutter(dart), react, vue(Vue 3 + TypeScript) 코드를 생성할 수 있다. rust(axum + sqlx) 백엔드도 만든다 ([Rust](#rust-axum--sqlx)).

## 실행

1. `make install` (`~/bin/buildtool-model`, 템플릿 `~/bin/buildtool/{go,kotlin,...}`, 표준 자산 `~/bin/buildtool/bootstrap/`)
2. 프로젝트 디렉터리에 `model.json` 을 둔다 (표준 자산의 `bootstrap/model.json` 참고).
3. 실행한다.

```bash
buildtool-model <projectDir> [languages] [package] [--force]
```

- `projectDir`: `model.json` 이 있는 디렉터리. 상대 `*ModelFilePath` 는 이 디렉터리 기준이고, `goModelFilePath` 가 비어 있으면 이 디렉터리다.
  `projectDir/model.json` 이 없으면(또는 `.`/생략) 예전처럼 현재 디렉터리의 `config/model.json` 을 읽는다 (아래 [model.json 읽기](#modeljson-읽기)).
- `languages`: model.json 의 언어 선택을 덮어쓴다 (`go,react`, `all`).
- `package`: 생성 Go 코드의 import 경로 앞부분. 없으면 `{goModelFilePath}/go.mod` 의 `module`, 그것도 없으면 경고를 찍고 `database` 이름을 쓴다.
- `--force`: 생성 머리줄이 없는 기존 파일도 덮어쓴다 ([생성 파일 머리줄](#생성-파일-머리줄-덮어쓰기-보호)).

## 새 프로젝트 만들기 (`init`)

```bash
buildtool-model init <name> [--go] [--spring] [--rust] [--react] [--vue] [flags]
# 예: buildtool-model init safelog --go --react   → safelog/safelog_go, safelog/safelog_web
```

basicproject 템플릿 + 표준 DB 로 바로 실행할 수 있는 작업 공간을 만든다. 그다음은 `cd <name> && docker compose up -d db` 후 백엔드 / 프런트를 실행하면 된다.

| 플래그 | 뜻 |
|---|---|
| `--go --spring --rust` | 백엔드 (하나 이상). 아무 언어도 고르지 않으면 `--go --react` (출력에 알린다) |
| `--react --vue` | 프런트엔드 |
| `--dir <parent>` | 작업 공간을 만들 상위 디렉터리 (기본: 현재 디렉터리, 있어야 한다) |
| `--template-source github\|<dir>` | `github` (기본): `git clone --depth 1 https://github.com/kobums/<repo>.git`. 디렉터리면 그 아래 `basicproject_*` 를 복사한다 (`.git`, `node_modules`, `dist`, `build`, `target`, `bin`, `.gradle`, `.kotlin`, `.env.local`, `.env.yml` 제외) |
| `--branch <name>` | 템플릿 브랜치 (기본 `main`) |
| `--db-port <port>` | DB 포트 (기본 3306). 모든 `.env` 와 `model.json` 에 같이 쓴다 |
| `--package <pkg>` | Spring 기본 패키지를 바꾼다 (기본 `com.basicproject.api` 그대로). `src/*/kotlin` 디렉터리 이동, 텍스트 치환, `build.gradle` 의 `group` |
| `--no-git` | 하위 프로젝트에 `git init` / 첫 커밋을 하지 않는다 |
| `--db-up` | `docker compose up -d --wait db` (docker 가 있을 때) |
| `--gen` | DB 에 붙을 수 있으면 작업 공간 `model.json` 으로 모든 언어를 다시 생성한다 (없으면 건너뛴다. 템플릿에 user / board 생성 코드가 이미 있다) |
| `--force` | 비어 있지 않은 `<name>` 디렉터리도 쓴다. 루트 `.env` 가 있으면 그 비밀번호 / `JWT_SECRET` 을 그대로 쓴다. 이미 있는 하위 프로젝트 디렉터리는 덮어쓰지 않는다 (에러) |

- 이름은 `[a-z][a-z0-9_]*` (32자 이하, `_` 로 끝나지 않음). DB 이름 / 사용자 이름으로도 쓴다.
- 비어 있지 않은 디렉터리는 `--force` 없이 거부한다. 중간에 실패하면 이번에 만든 디렉터리만 지운다. 작업 공간 밖은 건드리지 않는다.

### 작업 공간

```
<name>/
  db/schema.sql  docker-compose.yml  .env  .gitignore  Makefile  README.md   ← 표준 자산 (bootstrap/)
  model.json                ← 테이블 설정의 기준 (고른 언어 모두 켜짐, 경로는 이 파일 기준)
  <name>_go/                ← basicproject_go        (포트 8007)
  <name>_spring/            ← basicproject_springboot (포트 8081)
  <name>_rust/              ← basicproject_rust      (포트 8008)
  <name>_web/               ← basicproject_web (React)
  <name>_vue/               ← basicproject_vue
```

- DB 는 루트 `docker-compose.yml` 로 띄운다 (`make db-up` = `up -d --wait db`, `make db-reset`, `make gen` = `buildtool-model .`).
  하위 프로젝트의 `docker-compose.yml` 은 따로 쓸 때용이라 그대로 둔다 (같은 3306 포트라 루트 DB 와 함께 띄우지 않는다).
- 이름 바꾸기 (하위 프로젝트마다 자기 템플릿 이름만): Go `go.mod` module → `<name>_go` (import, Makefile `APP`, docker 이미지 이름 등 모든 텍스트),
  Spring `settings.gradle` `rootProject.name` (+ `spring.application.name`) → `<name>_spring`, Rust `Cargo.toml` / `Cargo.lock` 패키지 이름 → `<name>_rust`,
  `package.json` / `package-lock.json` / `index.html` 이름 → `<name>_web` / `<name>_vue`, README 제목. 다른 템플릿을 가리키는 설명(`basicproject_go 와 같다`)은 그대로다.
  프런트는 `index.html` / `src/` 의 단독 단어 `basicproject` (화면 이름, 토큰 저장 key `basicproject.token`)도 `<name>` 으로 바꾼다.
  텍스트 치환은 바이너리 / 4MB 넘는 파일 / 심볼릭 링크를 건너뛴다. Go 는 바꾼 뒤 `gofmt -w .` (module 이름에 따라 import 순서가 바뀐다).
- `.env`: 루트와 모든 백엔드가 같은 값이다 — `MARIADB_DATABASE` / `MARIADB_USER` / `DB_NAME` / `DB_USER` = `<name>`, 무작위
  `MARIADB_PASSWORD` (= `DB_PASSWORD`) / `MARIADB_ROOT_PASSWORD` (16 바이트 hex), `JWT_SECRET` (48 바이트 base64, 백엔드끼리 토큰 호환), `DB_PORT`.
  백엔드 `.env` 는 템플릿 `.env.example` 에서 위 key 만 바꾼다 (`PORT` 등 나머지는 그대로). 프런트 `.env` 는 `.env.example` (없으면 템플릿 `.env`)에서
  `VITE_PROXY_TARGET` 을 첫 백엔드(go > spring > rust)로 바꾼다. 백엔드 `.env` 는 0600 이고, git 첫 커밋 전에 `.gitignore` 에 없으면 넣는다.
- `model.json` (루트): 표준 `bootstrap/model.json` 의 `auth` / `adminLevel` / `table` + 고른 언어, `*ModelFilePath` (`./<name>_go`,
  `./<name>_spring/src/main/kotlin/com/basicproject/api` ...: 템플릿 model.json 의 경로를 옮긴 것), `kotlinPackage`, `springBoot`,
  `reactTsOutDir` / `vueTsOutDir` / `reactTsSemicolons` / `vueTsSemicolons` (템플릿 값). DB 접속 비밀번호는 넣지 않는다 (`.env`).
- 하위 프로젝트 `model.json` (Vue 처럼 `gomachine/model.json` 에 있으면 그 자리)은 **포인터**다:
  `{"modelJsonPath": "..", "languages": {"go": true}}`. 하위 프로젝트에서 `make gen` / `./gradlew gen` (`buildtool-model .`)을 하면
  루트 model.json 을 읽고 그 언어만 만든다 ([model.json 읽기](#modeljson-읽기)). 테이블 설정은 루트 하나만 고친다.
- `git init -b main` + 첫 커밋 `init from basicproject_* via gomachine` (사용자 git 이름 / 이메일). git 이름이 없으면 경고만 하고 커밋은 직접 한다. 루트는 git 저장소로 만들지 않는다.
- 표준 자산은 `~/bin/buildtool/bootstrap` (make install) 이 있으면 그것을, 없으면 바이너리에 넣은(go:embed) `bootstrap/` 을 쓴다 (소스에서 실행해도 된다).

## 표준 프로젝트 자산 (`bootstrap/`)

새 프로젝트를 "basicproject 템플릿 + DB + gomachine" 으로 바로 시작하기 위한 파일이다 (`make install` 이 `~/bin/buildtool/bootstrap/` 에 복사한다).

- `db/schema.sql`: MariaDB (utf8mb4) 표준 스키마. `user_tb` (`u_id` bigint auto_increment, `u_email` unique, `u_passwd`, `u_name`,
  `u_level` int 기본 1, `u_createddate` 기본 현재 시각), `board_tb` (`b_id`, `b_title`, `b_content` text, `b_img` null 허용,
  `b_user` 작성자 = `user_tb.u_id` 외래 키, `b_createddate`). 이름 규칙(`{name}_tb`, `{접두사}_{컬럼}`, `createddate`)을 따른다.
- `docker-compose.yml`: `db` 서비스 (`mariadb:11.4` LTS, 포트 3306, 이름 있는 볼륨 `db-data`, healthcheck `healthcheck.sh --connect --innodb_initialized`).
  `db/schema.sql` 을 `/docker-entrypoint-initdb.d` 에 마운트해서 볼륨이 비어 있을 때 한 번 실행한다 (`docker compose down -v` 로 초기화).
- `.env.example`: `MARIADB_DATABASE/USER/PASSWORD/ROOT_PASSWORD`, `DB_PORT`, gomachine 용 `DB_HOST/DB_NAME/DB_USER/DB_PASSWORD`, 백엔드 공통 `JWT_SECRET`.
  `.env` 로 복사해 쓴다.
- `model.json`: 표준 설정 예. `auth: "jwt"`, `adminLevel: "3"`, `languages` 맵, `{goModelFilePath}` 같은 자리표시 경로(초기화 명령이 바꾼다),
  `user` 는 `protect: ["level"]` (User 의 level 은 원래 항상 보호, 가입 때 DB 기본값 1), `board` 는 `session: [{"key": "id", "column": "user"}]`
  (작성자 = 로그인 사용자. key `id` 는 Go `c.Session.Id`, Spring `currentValue("id")`, Rust `session.id`),
  `join: [{"name": "user", "column": "user"}]` (작성자 정보), `read: "login"` (로그인 사용자는 모든 글을 읽고, 수정 / 삭제는 작성자만).

## DB 접속 (생성기)

- `server`(`DB_HOST`), `port`(`DB_PORT`, 없으면 3306), `database`(`DB_NAME`), `user`(`DB_USER`), `password`(`DB_PASSWORD`).
  환경변수가 있으면 model.json 값보다 우선한다. `port` 는 숫자(`3307`)나 문자열(`"3307"`) 모두 된다.
- **바뀐 점**: 예전에는 포트가 3306 고정이었고 `DB_PORT` / `DB_NAME` 을 읽지 않았다.

## 생성 파일 머리줄 (덮어쓰기 보호)

- Go / Kotlin / TypeScript / Dart 생성 파일의 첫 줄은 `// Code generated by gomachine. DO NOT EDIT.`, Rust 는 `// @generated by gomachine - do not edit` 이다.
- 생성할 경로에 파일이 이미 있는데 첫 줄이 생성 머리줄이 아니면 사용자 파일로 보고 **덮어쓰지 않고** 경고를 찍는다
  (템플릿의 `models/db.go`, `router/routers/auth.go`, 손으로 만든 모델 등 보호). 예전 TypeScript 머리줄(`// Auto-generated ...`)은 생성 파일로 본다.
- 예전 gomachine 이 만든 머리줄 없는 파일을 새로 만들려면 지우거나 `--force` 로 한 번 실행한다.
- 판단은 `util.IsGeneratedFile` / `util.WriteGenerated` 하나로 한다. 내용이 같으면 다시 쓰지 않는다 (수정 시각 유지).
  사용자 소유 파일(Rust `main.rs`/`support/auth.rs`, Kotlin `SessionProvider.kt`, TypeScript `services/api.ts`)은 없을 때만 만든다.

## 생성 언어 선택

`model.json` 의 `languages` 에서 언어별로 켜고 끈다. `"all": true` 면 전부 생성한다.

```json
"languages": { "all": false, "go": true, "spring": true, "flutter": false, "react": true, "vue": true },
"kotlinModelFilePath": "../proj/proj_spring/src/main/kotlin/com/example/proj",
"kotlinPackage": "com.example.proj",
"reactModelFilePath": "../proj/proj_front",
"vueModelFilePath": "../proj/proj_vue",
"rustModelFilePath": "../proj/proj_rust"
```

- 지원 언어: `go`, `spring`(=`kotlin`), `flutter`(=`dart`), `react`, `vue`, `rust`
- go 외 언어는 `*ModelFilePath` 가 비어 있으면 경고만 찍고 건너뛴다.
- `languages` 가 없으면 예전 `language` 문자열을 쓰고, 그것도 없으면 go 만 생성한다.
- 실행 인자로 덮어쓸 수 있다: `buildtool-model <projectDir> go,react` 또는 `buildtool-model <projectDir> all`

## model.json 읽기

- 첫 실행 인자 디렉터리에 `model.json` 이 있으면 그것을 읽고, 상대 출력 경로를 그 디렉터리 기준으로 바꾼다.
- 그 `model.json` 에 `modelJsonPath` 가 있으면 **포인터**다: 그 디렉터리(포인터 기준 상대 경로)의 `model.json` 을 읽고, 상대 출력 경로는 그 디렉터리 기준이다.
  포인터에 `languages` (또는 `language`)가 있으면 그 언어만 생성한다 (실행 인자 언어가 더 우선). 한 단계만 따라간다 (`init` 의 하위 프로젝트).
- 아니면 예전처럼 실행한 디렉터리의 `config/model.json` 을 읽는다 (`modelJsonPath` 가 있으면 그 디렉터리의 `model.json`).
  이때 상대 출력 경로는 예전과 같이 실행한 디렉터리 기준이다.
- JSON 문법 오류나 타입 오류(`"session": {...}` 처럼 배열 자리에 객체 등)가 있으면 에러를 찍고 **아무것도 생성하지 않고** 종료한다 (exit 1).
  예전에는 반쯤 읽은 설정으로 계속해서 `session` / `protect` 가 조용히 빠질 수 있었다.
- 모르는 key(`"sesion"` 같은 오타 포함)는 위치와 함께 경고만 찍는다 (`unknown key table[0].sesion (ignored)`). 생성은 계속한다.
- `auth: "jwt"` 에서 `session` 컬럼이 없거나 쓸 수 없는 타입이면(Go / Rust 는 정수 컬럼, Spring 만 생성하면 문자열 컬럼도 된다)
  에러를 찍고 아무것도 생성하지 않는다 (소유권 검사 없는 API 가 만들어지므로). jwt 가 아니면 경고만 찍는다.

## 이름 규칙

- 테이블: 끝의 `_tb` / `_vw` 만 떼고 나머지를 쓴다. `user_tb` → `User` (파일/패키지/URL `user`),
  `user_role_tb` → `UserRole` (파일/패키지/URL `userrole`). 예전에는 첫 `_` 앞만 써서 `user_role_tb` 가 `user_tb` 를 덮어썼다.
  - `_tb` 가 없는 테이블도 전체 이름을 쓴다 (`member_info` → `MemberInfo`, 예전에는 `Member`).
  - 이름이 겹치면 (`user_tb` 와 `user_vw`) `_tb` 가 원래 이름을 갖고 나머지는 경고와 함께 다른 이름이 된다 (`user_vw` → `UserView`).
  - `model.json` 의 `table[].name` 은 테이블마다 이 순서로 찾는다: ① 전체 이름(`user_role_tb`)이나 접미사 뺀 이름(`user_role`)이
    정확히 같은 설정(대소문자 무시) ② `_` 를 무시하고 같은 설정(`userrole`, `UserRole` → `user_role_tb`). 다른 테이블을 정확히
    가리키는 설정은 ②로 붙지 않는다 (`userrole_tb` 가 있으면 `"userrole"` 은 `userrole_tb` 설정이다. 예전에는 `user_role_tb` 에 붙었다).
    어느 테이블에도 붙지 않거나 여러 테이블에 붙는 설정은 경고를 찍는다.
  - 이름 충돌로 다른 이름이 된 테이블(`user_vw` → `UserView`, `userrole_tb` → `UserroleTb`)은 설정에서 **전체 테이블 이름**
    (`"user_vw"`, `"userrole_tb"`)으로 적는다.
  - 조인(`join[].name`)도 같은 규칙으로 테이블을 찾는다 (`{name}_tb` 가 먼저). 테이블이 없으면 경고를 찍고 그 조인을 건너뛴다.
- view(`_vw`)는 읽기 전용이다. Go 는 Insert/Update/Delete 계열 모델 메서드·핸들러·경로를, Spring 은 POST/PUT/PATCH/DELETE 와
  서비스 쓰기 메서드를 만들지 않는다. view 에 설정한 `UpdateXByY` / `DeleteByX` 는 경고를 찍고 건너뛴다.
- 컬럼: 첫 `_` 앞(접두사)만 뗀다. `p_created_at` → SQL `created_at`, Go 필드 `CreatedAt`, JSON key(Go json 태그, Kotlin/TypeScript/Dart) `createdAt`.
  자세한 규칙은 아래 [HTTP 계약](#http-계약-go--spring--react--vue--dart).
  밑줄이 하나인 컬럼은 예전과 같은 이름이다 (`u_createddate` → `Createddate` / `createddate`).
- 컬럼 이름이 예약어면 Go 매개변수는 뒤에 `_` 를 붙이고 (`type_`), Kotlin 은 backtick 으로 감싼다 (`` `class` ``).
- Kotlin 클래스 이름이 생성 코드가 쓰는 Kotlin / Spring / JPA 타입과 같으면 접미사를 붙인다 (경고): 엔티티 `page_tb` → `PageEntity`
  (`list`, `map`, `column`, `sort`, `string` ... 도 같다), enum `sort` → `SortEnum` (같은 테이블 엔티티 / 조인 엔티티와 같은 이름도).
  URL(`/api/page`), JSON, 파일 이름 외 다른 언어는 그대로다. 생성 코드가 쓰지 않는 이름(`Order` 등)은 바꾸지 않는다.
- Go 상수 패키지(`models/{table}`) 이름이 생성 코드의 import(`log`, `config`, `time` ...)나 지역 변수와 겹치면
  생성 파일에서는 별칭(`logpkg`)으로 import 한다. 디렉터리와 `package` 이름은 그대로다 (예약어인 `type` 등은 `package typepkg`).
- 컬럼 순서는 `information_schema.columns.ordinal_position` 순서다.

## 컬럼 타입

| MySQL | Go | Kotlin |
|---|---|---|
| `int`, `smallint`, `mediumint`, `year` | `int` | `Int` |
| `bigint` | `int64` | `Long` |
| `tinyint` | `bool` | `Boolean` |
| `float`, `double` | `float64` | `Double` |
| `decimal` | `float64` | `BigDecimal` |
| `char`, `varchar`, `*text`, `enum`, `set`, `json` | `string` | `String` |
| `datetime`, `timestamp` | `string` | `LocalDateTime` |
| `date` | `string` | `LocalDate` |
| `time` | `string` | `String` |
| `*blob`, `binary`, `varbinary` | `[]byte` | `ByteArray` |
| `bit` | `bool` | `Boolean` |

- 모르는 타입은 경고를 찍고 문자열로 처리한다.
- Go `decimal` 은 예전 `int` 에서 `float64` 로 바뀌었다 (`int` 는 소수점이 잘리고 `"12.50"` Scan 이 실패했다). 정밀도가 중요하면 직접 바꿔 쓴다.
- Go `double`/`float` 은 예전 `Double`(대상 프로젝트에 정의가 필요했다)에서 `float64` 로 바뀌었다.
- `tinyint` 와 `bit` 는 모든 언어에서 bool 이다 (Go `bool`, Kotlin `Boolean`, TypeScript `boolean`, Dart `bool`).
  - **바뀐 점**: Kotlin `tinyint` 는 예전 `Int`, Go `bit` 는 예전 `[]byte`(JSON base64 문자열)였다. 0/1 이 아닌 값을 쓰는 `tinyint` 컬럼은
    읽을 때 0 이 아니면 `true` 다.
  - Go 는 `bit` 컬럼을 `(컬럼+0)` 으로 select 해서 정수로 읽는다 (드라이버가 원시 바이트를 돌려주므로). `bit(1)` 플래그를 가정한다.
  - Kotlin `tinyint` 프로퍼티에는 `@Column(columnDefinition = "tinyint")` 가 붙는다 (Hibernate 기본 Boolean 타입은 `bit`).
  - JSON 요청은 Spring 이 `true/false` 와 `0/1` 을, 목록 필터는 두 백엔드 모두 `true/false/1/0` 을 받는다.
- Kotlin `double` 은 예전 `BigDecimal` 에서 `Double` 로, `date` 는 `LocalDateTime` 에서 `LocalDate` 로 바뀌었다.

## NULL 허용 컬럼 (`information_schema.columns.is_nullable`)

| | NULL 허용 컬럼 | NOT NULL 컬럼 |
|---|---|---|
| Go | 0 값 타입 그대로. 읽을 때 NULL 은 0 값(`""`, `0`, `false`). 쓸 때는 날짜/시간 컬럼의 빈 값만 NULL 로 저장하고, 숫자·bool·문자열은 값 그대로 저장. REST 부분 수정은 아래 참고 | 그대로 |
| Kotlin | `String?`, `Int?` ... (기본값 `null`), 응답도 `?` | non-null, 기본값 있음 |
| TypeScript | `field: T \| null` | `field: T` |
| Dart | `String?`, `int?` ... | 기본값 있음 |

- Go 는 NULL 허용 컬럼을 `sql.Null*` 로 Scan 한다. 예전에는 NULL 이 있는 행이 Scan 에러로 빠지고, `ReadRows` 가
  첫 에러에서 멈춰 뒤의 행도 조용히 사라졌다. 이제 읽지 못한 행만 로그를 남기고 건너뛴다.
- Go 매니저에 `LastError` 가 생겼다. `Get/Find/Count` 가 nil/빈 결과를 돌려줄 때 DB 에러인지 확인할 수 있다
  (REST `Read` / `Index` / `Count` / `Search` 는 에러면 `code: "error"`. 예전 `Index` / `Count` 는 에러를 무시하고 빈 결과를 돌려줬다).
- Go REST 부분 수정(`PUT /{t}`, `PUT/PATCH /{t}/:id`)은 NULL 을 보존한다: 본문에 없는 null 허용 필드는 DB 의 NULL 을 그대로 두고
  (예전에는 `0`/`""`/enum 코드 0 으로 바뀌었다), 본문에 `null` 로 보낸 null 허용 필드는 NULL 로 지운다. 값을 보내면 그 값을 쓴다.
  NOT NULL 컬럼에 `null` 을 보내면 기존 값을 유지한다. 보호 컬럼 / `createddate` / 비밀번호는 기존 값(NULL 포함)을 유지한다.
  모델: `GetRaw(id) (*X, nulls)` 가 NULL 이었던 컬럼(필드 이름)을 돌려주고, `Merge{X}Nulls(nulls, body)` 가 본문으로 고치고,
  `UpdateNulls(item, nulls)` 가 그 필드를 NULL 로 저장한다 (`Update(item)` 은 `UpdateNulls(item, nil)` 과 같다).
  구조체 필드 타입은 그대로다. Insert 와 커스텀 `UpdateXByY` 는 예전과 같다 (`UpdateXByY` 본문의 `null` 은 400).
- Go `tinyint`(bool) 컬럼은 정수로 읽어 `!= 0` 으로 바꾼다 (예전에는 2 이상 값이 Scan 에러로 행이 빠졌다).
- Kotlin 은 예전 "날짜 컬럼만 nullable" 추측 대신 스키마의 null 허용 여부를 따른다. 기본 키는 항상 non-null.

## 날짜 값

- Go 저장: NULL 허용 날짜 컬럼이 `""` 면 NULL. NOT NULL 컬럼이 `""` 면 기본값이 `CURRENT_TIMESTAMP` 계열이면 현재 시각,
  고정 기본값이면 그 값, 기본값이 없으면 자리표시 값(`datetime` `1000-01-01 00:00:00`, `date` `1000-01-01`,
  `time` `00:00:00`, `timestamp` 는 범위 밖이라 현재 시각). 읽을 때 자리표시 값은 `""` 로 바뀐다.
- Go REST `Insert`/`Insertbatch` 는 문자열 `createddate` 를 서버가 현재 시각으로 넣는다 (클라이언트가 보낸 값은 무시한다).
  현재 시각은 예전과 같이 UTC+9 (Asia/Seoul) 다. `model.json` 의 `"timezone"` 으로 바꿀 수 있다:
  `"local"`(서버 지역 시간), `"UTC"`, 또는 IANA 이름(`"America/New_York"`, 서버에서 찾지 못하면 UTC+9). 비어 있으면 예전과 같다.
- Spring 생성(`POST`)도 `createddate`(datetime / date)를 서버가 현재 시각으로 넣는다. 요청 DTO 에 `createddate` 가 없다
  (보내도 무시된다). 시간대는 Go 와 같은 `timezone` 설정이다 (`{kotlinPackage}.support.DateParams.ZONE`, 비어 있으면 Asia/Seoul).
- 형식: Go 와 Spring 모두 응답은 `yyyy-MM-dd HH:mm:ss` (date 는 `yyyy-MM-dd`) 문자열이다.
  Spring 요청(JSON 본문, 목록 날짜 범위, `/search`)은 `support/DateParams.kt` 가 해석한다:
  - `2024-01-02` (날짜만) → 그날 `00:00:00`. Go 는 문자열을 MySQL 에 그대로 넘기고 MySQL 도 `00:00:00` 으로 비교하므로
    `end{날짜}=2024-01-02` 는 두 백엔드 모두 그날 0시까지다 (그날 전체를 원하면 다음 날을 보낸다).
  - `2024-01-02 03:04:05`, `2024-01-02 03:04`, ISO `2024-01-02T03:04:05`, 밀리초 `2024-01-02T03:04:05.123`.
  - `Z` / 오프셋(`2024-01-02T03:04:05.000Z`, JS `toISOString()`, `+09:00`)은 `timezone` 지역 시간으로 바꾼다 (Z 를 버리지 않는다).
  - date 컬럼은 시간이 붙은 값도 날짜 부분만 쓴다. 해석하지 못하면 400.
  - Go 는 날짜 문자열을 MySQL 에 그대로 넘긴다 (`Z` 붙은 ISO 값은 MySQL 버전에 따라 해석이 다르다). 두 백엔드 모두 응답 형식
    (`yyyy-MM-dd HH:mm:ss`) 그대로 보내는 것이 가장 안전하다.
  React/Vue 는 응답 문자열을 그대로 보내면 된다.

## enum 코드 (`table[].map`)

모든 언어에서 enum 코드는 **data 배열 안의 위치**다. 빈 항목(`""`)은 자리만 차지하고 상수를 만들지 않는다.

```json
"map": [{ "name": "status", "data": ["", "wait:대기", "done:완료"] }]   // Wait = 1, Done = 2 (0 = 없음)
"map": [{ "name": "kind",   "data": ["a:A", "b:B"] }]                  // A = 0, B = 1
```

- Go: `StatusWait Status = 1` 처럼 값을 직접 쓴다. `GetStatus(code)` 는 라벨, `FindStatus(label)` 은 코드(없으면 0, 코드 0 이
  실제 값인 enum 은 -1).
- Kotlin: `enum class Status(val code: Int, val label: String)`. DB 에는 `StatusConverter` 로 코드를 저장하고, JSON 은
  코드(숫자)로 주고받는다 (`@JsonValue` / `@JsonCreator fromCode`). `NONE(0)` 은 data[0] 이 `""` 일 때만 있다.
  목록 필터와 `/search/{enum}` 은 코드(Int)를 받는다.
- TypeScript / Dart 도 같은 코드다.
- **바뀐 점**: 첫 항목이 빈 문자열이 아닌 map 은 예전 Go(iota)·Kotlin(ordinal, NONE 포함)이 1 부터 셌고 TypeScript 는 0 부터 셌다.
  이제 모두 0 부터다 (`["wait:Wait", "done:Done"]` → Wait = 0). 이런 map 으로 저장된 기존 데이터가 있으면
  data 앞에 `""` 를 넣으면 예전 값(1 부터)과 같아진다. 첫 항목만 `""` 이고 중간에 빈 항목이 없는 map 은 예전과 같은 값이다
  (중간 빈 항목은 예전 Go/Kotlin 이 건너뛰고 셌지만 이제 자리를 차지한다).
- 코드 0 이 실제 값인 enum 은 Go 목록 필터와 커스텀 Get/Find/Count 에서 0 도 조건으로 쓴다 (예전에는 0 이면 조건을 뺐다).

## Go 커스텀 메서드 (`table[].method`)

- `GetBy{컬럼}`, `FindBy{컬럼}`, `CountBy{컬럼}`, `DeleteBy{컬럼}`, `Update{컬럼}By{컬럼}`. 컬럼은 여러 개 이어 쓸 수 있다
  (`FindByUserStatus`, `FindByUserAndStatus`). `{컬럼}s` 는 in (`FindByIds`), `{컬럼}like` 는 like 조건이다.
- 컬럼 이름은 대소문자까지 같은 조합을 먼저 찾고, 없으면 대소문자를 무시한다 (`GetByUserId` → `userid` 컬럼).
- 컬럼과 맞지 않는 메서드는 깨진 코드를 만들지 않고 경고를 찍고 건너뛴다.
- Get/Find/Count 는 값이 0 / "" 인 매개변수의 조건을 빼지만, bool 과 in(슬라이스) 조건은 항상 붙인다.
- 경로: `GET /{table}/{get|find|count}/{by 뒤 이름}/:{값}` (컬럼이 여러 개면 `/:{컬럼}/:{컬럼}`, in 은 쉼표로 구분),
  `PUT /{table}/{바꿀 컬럼}/{조건 컬럼}` (JSON 본문). 값을 해석하지 못하면 400 을 돌려준다 (예전에는 0 으로 바꿔 조건이 빠졌다).
  `PUT` 본문에 필요한 key 가 없어도 400 이다 (예전에는 0 값으로 덮어썼다). 본문 key 는 JSON key 다 (`postTitle`).
  `DeleteBy` 는 `DELETE /{table}/delete/{by 뒤 이름}/:{값}` 경로가 생겼다 (소유권 검사는 위와 같다).
  like 조건이 있는 `DeleteBy`(`DeleteByNamelike`)는 와일드카드 하나로 많은 행을 지울 수 있어 HTTP 경로를 만들지 않는다 (경고, 모델/컨트롤러 메서드는 있다).
- `UpdateXByY` 로 id / 기본 키(`primary`)를 바꾸는 메서드(`UpdateIdByName`)는 경고를 찍고 만들지 않는다.
- `UpdatePasswdByX` / `UpdatePasswordByX` 는 REST Update 와 같이 `jwt.GeneratePasswd` 로 해시해서 저장한다 (빈 값은 `bad request`).
  User 테이블이면 본인 행만 바꿀 수 있다 (관리자 제외, 아래 인증 참고).

## Go REST 동작

- `PUT /{table}` 은 부분 수정이다. 기존 행을 읽고 **본문에 있는 JSON key 만** 덮어쓴다 (key 이름은 모델 json 태그, 예: `createdAt`. Go 의 json 은 대소문자를 가리지 않아 `createdat` 도 된다).
  빠진 필드는 DB 값을 유지한다. 그 뒤 id / 세션 컬럼 / `createddate` / 보호 컬럼 / 비밀번호 규칙을 다시 적용한다.
  컨트롤러 시그니처가 `Update(item, body []byte)` 로 바뀌었다 (body 가 비어 있으면 item 전체로 바꾼다).
- 목록 `orderby` 는 이 테이블 컬럼만 받는다. API 이름(`createdAt`, 대소문자 무시), 접두사 뺀 이름(`created_at`), 전체 이름(`p_created_at`)
  + `asc`/`desc` 를 쉼표로 이어 쓴다. 모르는 항목은 버린다. 모델 `Find/Sum` 에 넘기는 정렬도 알려진 컬럼 이름만 SQL 컬럼으로
  바꾸고 나머지(`rand()`, 조인 컬럼 등)는 그대로 둔다 (`p.ParseOrder`, `p.FixOrder`).
- 목록은 `page`/`pagesize` 를 보내면 몇 번째 페이지든 `total` 을 같이 돌려준다 (예전에는 1 페이지만).
  `page` 가 없거나 0 이면 예전과 같이 전체 행이다. `page`/`pagesize` 가 숫자가 아니거나 음수면 400 (예전에는 0 으로 보고 전체 행),
  `pagesize` 는 10000 을 넘지 않는다 (Spring 과 같다). `page` 가 1000000000 을 넘으면 400 이다 (offset 계산이 넘치지 않게, Rust 도 같다).
- 목록/개수 필터 연산자는 [값](#값)의 문자열 필터 / `compare` 규칙이다. 숫자/enum/bool 값을 해석하지 못하면 400 이다 (`?status=abc`, 예전에는 조건을 빼서 전체 행을 돌려줬다).
  bool(`tinyint`) 필터는 `1/0/true/false` 를 받고 보냈을 때만 거른다 (예전에는 `false` 로 거를 수 없었다).
  숫자 필터의 0 은 예전과 같이 조건 없음이다 (코드 0 이 실제 값인 enum 제외). 필터 해석은 `rest.Parse{X}Filters(get)` 순수 함수다.
- 조인(`table[].join`)은 `left join ... on` 이다 (예전에는 쉼표 조인 + where 조건이라 조인 키가 NULL 이거나 조인 행이 없는 행이
  목록/개수/Read 에서 빠졌다). 조인 행이 없으면 `Extra[조인]` 은 `nil` 이다. 조인 컬럼은 모두 NULL 을 받을 수 있게 읽는다.
  - 같은 테이블을 다시 조인하면(자기 조인, 같은 테이블 두 번) SQL 별칭 `j_{이름}` 을 붙이고, Go 변수 / `Extra` key 는
    alias > name 소문자에 겹치면 번호를 붙인다 (`user`, `user2`). 이 테이블과 접두사가 같은 테이블을 조인하면(자기 조인)
    이 테이블의 컬럼(where / order by / group by)을 테이블 이름으로 한정한다 (`comment_tb.c_parent`).
  - `use index(...)` 는 이 테이블 바로 뒤에 붙는다. `JoinQuery` 는 left join 뒤에 `, ` 로 붙는다.
  - `Count` / `GroupBy` 도 같은 from 을 쓴다.
- REST Update/Delete/Deletebatch 의 기존 행 읽기와 소유권 검사는 조인 없이 이 테이블만 읽는다 (`GetRaw`).
- `POST /{table}/batch` (Insertbatch) 와 `DELETE /{table}/batch` (Deletebatch) 는 한 번에 1000 행까지 받는다. 넘으면 라우터가
  본문 해석 실패와 같이 HTTP 400 `{"code":"bad request"}` 를 돌려준다 (Rust 도 같고, Spring 은 400).
- `Insertbatch` 는 모든 행을 준비(해시 등)한 뒤 저장하고, `Deletebatch` 는 모든 행의 권한을 검사한 뒤 지운다.
  없는 행은 건너뛰고, 남의 행이 하나라도 있으면 아무것도 지우지 않는다 (`auth error`, Spring 은 403). 예전 Go 는 없는 행이 있으면
  `error` 로 멈췄다. 둘 다 트랜잭션은 아니다: 저장/삭제 중간에 DB 에러가 나면 앞의 행은 이미 반영되어 있다.
- 조인 `column` 은 접두사가 있어도(`"a_member"`) 없어도(`"member"`), API 이름(`"memberId"`)이어도 된다. 이 테이블에 없는 컬럼이면
  경고를 찍고 그 조인을 건너뛴다 (Spring / TypeScript / Dart 도 같다).
- Postgres 는 지원하지 않는다. 생성 Go 모델의 `Find/Count/UpdateWhere/GroupBy` 는 `$n` 자리표시를 쓰지만 `Get/GetRaw/Insert/Update/Delete`,
  커스텀 `UpdateXByY`, 소유권 검사 등은 `?` 를 쓴다. MySQL 만 대상으로 한다.

## id 컬럼 (Go / Rust)

`id` 컬럼(`{prefix}_id`)은 정수(`int` / `bigint` ...)로 가정한다: Go 는 `int64`, Rust 는 `i64` (unsigned 면 `u64`) 로 읽고,
경로 `/{table}/{id}`, `Get/Update/Delete`, Insert 의 자동 증가 id 가 모두 정수다. `varchar` 같은 정수가 아닌 `id` 컬럼은 생성할 때
경고(`id column ... is varchar, not an integer`)를 찍지만 그대로 만든다 (숫자가 아닌 값은 읽기 / 경로 해석이 실패한다).
문자열 키는 컬럼 이름을 `id` 가 아닌 것으로 하고 `gpa.primary` 와 커스텀 `GetByX` / `UpdateYByX` / `DeleteByX` 를 쓴다.

## id 컬럼이 없는 테이블 (Go)

`id` 컬럼이 없으면 id 기준 메서드와 핸들러(모델 `Get/Update/Delete`, REST `Read/Update/Delete/Deletebatch` 와 그 경로)를 만들지 않는다.
조회/추가/개수와 커스텀 메서드(`GetByKey`, `UpdateValueByKey`, `DeleteByKey` ...)를 쓴다. 기본 정렬은 `gpa.primary` 컬럼, 없으면 첫 컬럼이다.
User 테이블에 id 가 없고 `auth: "jwt"` 면 본인 확인을 할 수 없으므로 커스텀 `UpdateXByY` / `DeleteBy` 는 항상 `auth error` 다.

Spring 은 기본 키를 `gpa.primary` > `id` 컬럼 > 첫 컬럼 순서로 정하고, 그 타입으로 `JpaRepository<X, 타입>` 과 경로 변수를 만든다.
Go 와 같이 `id` 컬럼도 `primary` 도 없으면 id 경로(`GET/PUT/PATCH/DELETE /{id}`, `DELETE /batch`)를 만들지 않는다 (JPA `@Id` 는 첫 컬럼).

### 기본 키 값 (Spring)

- `id` 이름의 정수(`Int`/`Long`) 기본 키만 자동 증가(`@GeneratedValue(IDENTITY)`)로 보고 생성 요청에서 받지 않는다.
- 그 밖의 기본 키(`primary` 로 정한 문자열 키, 첫 컬럼 키 등)는 생성 요청(`{Model}CreateRequest`)에 **반드시** 있어야 한다.
  이미 있는 키면 덮어쓰지 않고 409 다 (예전에는 키가 요청에 없어서 모든 생성이 `""` 키 행으로 merge 되었다). batch 는
  요청 안에서 겹치거나 이미 있는 키가 하나라도 있으면 아무것도 저장하지 않는다. 확인과 저장 사이에 다른 요청이 같은 키를 넣는 경우는 막지 않는다.

## Spring (Kotlin) 패키지와 경로

- `kotlinPackage` 로 생성 코드의 패키지를 정한다. 없으면 예전 값 `com.gowoobro.gymspring` 이다.
- `kotlinModelFilePath` 는 그 패키지의 디렉터리를 가리켜야 한다 (아래에 `entity/`, `repository/`, `service/`, `controller/`, `enums/`, `config/`, `support/` 를 만든다).
- `support/DateParams.kt` (날짜 해석, 시간대, `InvalidParamException`, Jackson 날짜 역직렬화기)는 생성 파일이라 매번 다시 쓴다.
  `config/SessionProvider.kt` 는 없을 때만 만든다.
- 대문자가 있는 테이블/컬럼 이름(`c_userId`): Spring Boot 기본 이름 전략(`CamelCaseToUnderscoresNamingStrategy`)은 `@Column(name = "c_userId")` 를
  `c_user_id` 로 바꿔 없는 컬럼을 찾는다. backtick 으로 감싸도 바뀐다 (Hibernate 6.6 에서 확인). 그런 이름이 있으면 생성기가 경고를 찍고
  엔티티에 주석을 남긴다. 대상 프로젝트 `application.properties` 에
  `spring.jpa.hibernate.naming.physical-strategy=org.hibernate.boot.model.naming.PhysicalNamingStrategyStandardImpl` 이 필요하다.
- view(`_vw`) 엔티티는 `@Immutable` 이고 `@GeneratedValue` 가 없다. `@Id` 는 `primary` 컬럼(없으면 `id`, 그것도 없으면 첫 컬럼)이다.
  JPA 는 같은 `@Id` 값의 행을 하나로 합치므로 view 에는 유일한 컬럼을 `primary` 로 적어야 한다 (없으면 경고). `primary` 가 없을 때
  대신 쓰는 첫 컬럼은 목록 필터 / 검색으로 거를 수 있다.

### Spring Boot 버전 (`springBoot`)

model.json `springBoot`: `3` 또는 `4` (없으면 4).

- 4: Jackson 3 (`tools.jackson.*`, `ValueDeserializer`), Spring Data 4 의 `Specification` 시그니처(`findAll(spec: Specification<X>, …)`, null 불가),
  `PasswordEncoder.encode` 가 `String?` 를 돌려주므로 `!!` 로 받는다.
- 3: 예전 Boot 3 코드 (Jackson 2 `com.fasterxml.jackson.*`, `Specification<X>?`). 예전 출력과 다른 것은 머리줄과 아래 409 뿐이다.
- enum 의 `@JsonCreator` / `@JsonValue` 는 두 버전 모두 `com.fasterxml.jackson.annotation` 이다 (Jackson 3 도 같은 패키지).
- Boot 4 대상 프로젝트: Boot 4.1.x, Kotlin 2.x (`spring`, `jpa` 플러그인), `tools.jackson.module:jackson-module-kotlin`, `-Xjsr305=strict`,
  `PasswordEncoder` 빈(bcrypt), `config.SessionProvider` 를 구현한 `@Component`.

### 삭제 제약 조건 위반 (409)

`DELETE /{id}`, `DELETE /batch` 에서 다른 행이 참조하는 등 `DataIntegrityViolationException` 이 나면 409 `{"success": false}` 를 돌려준다 (Boot 3 / 4 모두).

### 같은 이름의 사용자 클래스 경고

`kotlinModelFilePath` 가 `src/main/...` 아래면 `src/main` 전체(아니면 `kotlinModelFilePath`)의 생성 머리줄 없는 .kt/.java 에서
생성할 엔티티(JPA 엔티티 이름)와 같은 이름, `{Model}Repository` / `{Model}Service` / `{Model}Controller` 와 같은 단순 이름(= Spring 빈 이름)의
클래스를 찾으면 경고한다. `@Entity(name = "AuthUser") class User` 처럼 엔티티 이름을 바꾼 클래스는 겹치지 않는다.

## Vue (Vue 3 + TypeScript)

`vueModelFilePath` 아래 `{tsOutDir}`(기본 `src`)에 테이블마다 세 파일을 만든다.

- `src/types/{table}.ts` — 모델 인터페이스, 검색 파라미터, 응답 타입
- `src/models/{table}.ts` — API 호출 클래스 (`{Model}Model.find/get/insert/update/remove`, enum 상수, `searchByXxx`)
- `src/composables/use{Model}.ts` — `items`, `item`, `total`, `loading`, `error` ref 와 `fetchPage/fetchAll/fetchOne/create/update/remove`
  (view 는 `create/update/remove`, id 컬럼이 없는 테이블은 `fetchOne/update/remove` 가 없다)

생성 모델은 axios 같은 시그니처의 `src/services/api.ts` (`get`, `post`, `put`, `patch`, `del`) 를 쓴다. 없으면 한 번 만든다 (아래).
API 경로와 응답 처리는 [HTTP 계약](#http-계약-go--spring--react--vue--dart)을 따르므로 Go 와 Spring 어느 백엔드에도 쓸 수 있다.
공통 정규화 코드 `src/models/contract.ts` 를 같이 생성한다 (테이블마다 덮어쓴다).

## TypeScript 공통 (react / vue)

- React 는 `{tsOutDir}/models/{table}.ts`, `models/contract.ts`, `types/{table}.ts`, `hooks/use{Model}.ts` 를 만든다.
  React hook 은 Vue composable 과 같은 동작이다 (`items`, `item`, `total`, `loading`, `error`, `fetchPage`, `fetchAll`, `fetchOne`, `create`, `update`, `remove`).
  React 19 의 `useState` / `useCallback` 만 쓴다.
- 엔티티 타입의 기본 키는 필수 (`id: number`), `Create{Model}Request` 에서만 선택이다.
- `reactTsOutDir` / `vueTsOutDir`, `reactTsSemicolons` / `vueTsSemicolons`: 언어별 값 (있으면 `tsOutDir` / `tsSemicolons` 보다 우선). 한 model.json 으로
  React(`src/gen`)와 Vue(`src`)를 함께 만들 때 쓴다 (`init` 작업 공간).
- `tsSemicolons` (기본 true): false 면 문장 끝 `;` 를 뺀다 (prettier `semi: false` 프로젝트용). 다음 줄이 `(` `[` `` ` `` `+` `-` `/` 로 시작하면 ASI 때문에 `;` 를 남긴다.
- `tsOutDir` (기본 `"src"`): 생성 파일을 `{react|vue}ModelFilePath/{tsOutDir}/` 아래에 둔다 (예: `"src/gen"`). models / types / composables / hooks / services 가
  모두 그 아래에 생기고 상대 import 는 그대로다. 손으로 만든 `src/hooks/useUser.ts` 같은 파일과 겹치면 머리줄이 없어 건너뛰므로 `src/gen` 을 권한다.
- 생성 TS 는 공백을 정리한다 (줄 끝 공백, 겹친 빈 줄, `{` 뒤 / `}` 앞 빈 줄 제거, 끝 줄바꿈 하나). 그래도 printWidth 80 에 맞춰
  줄을 나누지는 않아서 prettier `--check` 는 실패한다. 생성 디렉터리(`src/gen/models|types|hooks`, Vue 는 `src/models|types|composables`)를
  `.prettierignore` 에 넣는다 (`services/api.ts` 는 사용자 파일이라 포맷 대상).
- React hook / Vue composable 은 늦게 도착한 이전 요청의 응답을 버린다 (`fetchPage` / `fetchAll` / `fetchOne` 은 마지막 요청 결과만 반영,
  요청 번호 검사). `create` / `update` / `remove` 는 `error` 를 채운 뒤 에러를 **다시 던진다** (**바뀐 점**: 예전에는 `undefined` 를 돌려줬다.
  호출하는 쪽에서 try/catch 한다).
- `Create{Model}Request` 에는 서버가 정하는 필드가 없다: 세션 컬럼(`auth: "jwt"` 이고 User 가 아닐 때), 보호 컬럼(User 의 `level` 포함),
  `createddate`. 기본 키는 예전처럼 선택이다.
- `Generated TypeScript: ...` 로그는 실제로 쓴 파일만 찍는다 (내용이 같은 `contract.ts` 등은 찍지 않는다).

### services/api.ts (사용자 파일)

- `{tsOutDir}/services/api.ts` 는 없을 때만 한 번 만든다 (이후 덮어쓰지 않으므로 고쳐 써도 된다. 같은 export 를 가진 파일을 템플릿이 직접 둬도 된다).
- 생성 모델이 기대하는 형태: `get(url, cfg?)`, `post/put/patch(url, body?, cfg?)`, `del(url, cfg?)` → `Promise<{ data, status }>`,
  `cfg = { params?: object, data?: unknown, headers?: Record<string, string> }` (del 은 `cfg.data` 를 본문으로 보낸다. 빈 params 값은 뺀다).
- 기본 주소는 `import.meta.env.VITE_API_BASE_URL` (없으면 `/api`, `setBaseUrl(url)` 로 변경). 토큰은 `Authorization: Bearer <token>` 으로 보내고
  기본은 `localStorage.getItem('token')`, `setTokenProvider(fn: () => string | null)` 로 바꾼다. 401 이면 `setUnauthorizedHandler(fn)` 로 등록한 함수를 부른다.
  2xx 가 아니면 `HttpError(status, message, fieldErrors, body)` 를 던진다. Go 의 HTTP 200 + `code` 에러는 `models/contract.ts` 가 `ApiError` 로 던진다.

## Go 출력 파일과 라우터 등록

- 생성 Go 파일은 `go/format.Source`(gofmt)로 정리해서 쓴다. 실패하면(템플릿 버그) 경고를 찍고 형식 없이 쓴다.
- 문자열 목록 필터는 `models.Custom{Query: "b_title like ? escape '!'", Params: ...}` 로 보낸다 (템플릿 `WhereClause` 의 like 는
  값 양쪽에 `%` 만 붙이고 escape 를 하지 않아서). 값은 파일마다 있는 `like{Model}Pattern` 이 만든다.

- `goModelFilePath` 아래 `models/{name}.go`, `models/{name}/{name}.go`, `controllers/rest/{name}.go`, `router/routers/{name}.go`
  (디렉터리가 없으면 만든다).
- 테이블을 모두 만든 뒤 `router/routers/generated_routes.go` 를 쓴다. 이번 실행에서 만든 `Setup{Model}Routes` 를 모델 이름 순서로 부르는
  `func SetupGeneratedRoutes(group fiber.Router)` 하나다. 템플릿의 `router.go` 는 인증 / 업로드 라우트 뒤에 한 번만 부른다:

```go
apiGroup := r.Group("/api")
routers.SetupAuthRoutes(apiGroup)
routers.SetupUploadRoutes(apiGroup)
routers.SetupGeneratedRoutes(apiGroup)
```

  머리줄 없는 사용자 라우터 파일이라 건너뛴 테이블은 등록 파일에 넣지 않는다.

## Go Session 계약 (`controllers.Controller`)

생성 컨트롤러는 `controllers.Controller` 를 임베드하고 아래를 쓴다. 템플릿의 `controllers/controllers.go` 가 제공해야 한다.

```go
type Session struct {
	Id    int64  // JWT id (없으면 sub 를 숫자로)
	Email string
	Level int    // JWT level (없으면 0). adminLevel 이상이면 관리자
}

type Controller struct {
	Session *Session // 로그인하지 않았거나 토큰이 무효면 nil
	// ... Context, Result, NewConnection(), Get(), Set(), Init(), Close() 등 기존 필드 / 메서드
}
```

- `Init(*fiber.Ctx)` 가 `Authorization: Bearer <token>` 을 검증해 `Session` 을 채운다 (실패하면 nil 로 두고 생성 코드가 `auth error` 를 돌려준다).
- 생성 코드는 `c.Session == nil`, `c.Session.Id` (User 본인 확인, 소유권), `c.Session.{Key|title}` (model.json `session` 의 key,
  `"id"` → `c.Session.Id`) 를 읽는다. 세션 key 를 더 쓰면(`"email"` 등) 같은 이름의 필드가 있어야 컴파일된다.
- 관리자 판정(`adminLevel`)은 reflect 로 `Session.Level` 정수 필드를 읽는다.
- 토큰 형식은 [통일 JWT](#통일-jwt-go--spring--rust) 를 따른다.

## 생성된 Go 모델이 기대하는 models/db.go

`views/go/model.jet` 로 생성한 모델은 대상 프로젝트의 `models/db.go` 에 아래가 있어야 컴파일된다.
기준 구현은 basicproject_go / go-basic 의 `models/db.go` 를 복사해 쓴다.

- `WhereClause(prefix string, item Where, pos *int) (string, []interface{})`
  — `Where.Column` 은 식별자(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`), `Where.Compare` 는
  허용 연산자(`= != <> < <= > >= like not like in not in between`)만 SQL 에 넣고, 어긋나면 `and 1=0` 으로
  막는다(SQL 인젝션 방지, fail-closed). in / not in 값도 원소마다 자리표시자로 바인딩한다.
- `ValidColumn(name string) bool` — `GroupBy(name, ...)` 의 컬럼명 검증.
- `Custom.Params []interface{}` — `Custom.Query` 안의 `?` 에 바인딩할 값. 원시 SQL 에 외부 입력을
  `fmt.Sprintf` 로 이어붙이지 말고 이 필드를 쓴다.

## 인증 / 권한 (model.json `auth`, `table[].session`, `table[].protect`, `table[].read`)

### 읽기 정책 (`table[].read`, Go / Spring / Rust 공통)

읽기 = 목록, 개수, `/search/{field}`, 단건(`/{id}`), 커스텀 `GetBy` / `FindBy` / `CountBy`. 쓰기(생성 / 수정 / 삭제 / batch, 커스텀
`UpdateXByY` / `DeleteBy`)는 정책과 상관없이 예전과 같다 (로그인 필요, `session` 이 있으면 소유권 검사, 생성 때 세션 컬럼은 세션 값).

| `read` | 읽기 | 기본값이 되는 경우 |
|---|---|---|
| `"owner"` | 로그인 + 세션 컬럼이 현재 사용자인 행만 (예전 `session` 테이블 동작) | `session` 이 있을 때 |
| `"login"` | 로그인 사용자는 모든 행 (세션 조건 없음) | `session` 이 없을 때 (예전 jwt 동작) |
| `"public"` | 비로그인도 모든 행 | - |

- `auth` 가 `"jwt"` 가 아니면 모든 읽기가 공개다 (예전과 같다, `read` 는 경고만).
- 모르는 값(`"everyone"`)이나 `session` 없는 `"owner"` 는 에러를 찍고 아무것도 생성하지 않는다 (`util.ReadPolicy`).
- 예: 게시판은 `session` (작성자) + `read: "login"` 이면 로그인 사용자 누구나 읽고, 수정 / 삭제는 작성자만 한다.
- **User 테이블** (`auth: "jwt"`): 읽기는 항상 본인 행(`id` = 세션 id)만이고, 관리자(`adminLevel` 숫자, 세션 level ≥ adminLevel)는
  모든 행을 읽는다. 목록 / 개수 / 검색 / 커스텀 조회에 `id = 세션 id` 조건을 붙이고, 남의 단건은 Go / Rust `auth error`, Spring 404 다.
  id 컬럼이 없는 User 는 관리자만 읽을 수 있다. User 의 `read` 설정은 경고를 찍고 무시한다.
  **바뀐 점**: 예전에는 로그인한 누구나 모든 회원(이메일 등)을 읽을 수 있었다 (User 에 `session` 이 있으면 관리자도 본인 행만).
  Spring 은 관리자 판정에 `SessionProvider.currentValue("level")` 을 쓴다 (없으면 null = 관리자 아님).

### Go

- `"auth": "jwt"` 이면 쓰기 핸들러(Insert, Insertbatch, Update, Delete, Deletebatch, 커스텀 Update/Delete)는 `c.Session == nil` 일 때
  `error`/`auth error` 를 돌려준다. 읽기 핸들러(Read, Index, Count, Search, 커스텀 Get/Find/Count)는 [읽기 정책](#읽기-정책-tableread-go--spring--rust-공통)을
  따른다: `owner` 는 예전과 같고, `login` 은 로그인만 검사하고 (세션 조건 / 매개변수 덮어쓰기 / Read 소유권 검사 없음), `public` 은 세션을 보지 않는다.
  User 는 `id = c.Session.Id` 조건 (관리자 제외, `isAdmin` 은 보호 컬럼이 없어도 adminLevel 이 숫자면 만든다).
  - 예외: User 테이블의 `Insert` 는 회원가입용이라 세션 없이 호출할 수 있다 (Insertbatch 는 세션 필요).
  - User 테이블의 Update/Delete/Deletebatch/UpdateXByY/DeleteBy 는 본인(`c.Session.Id`) 행만 바꿀 수 있다.
- `session` 이 있고 읽기 정책이 `owner` 면 목록/개수/커스텀 Get·Find·Count 조회에 세션 컬럼 조건을 항상 붙이고, Read/Update/Delete 와
  커스텀 UpdateXByY/DeleteBy 는 다른 사용자 소유 행이 걸리면 `auth error` 로 거부한다. 없는 행을 Read 하면 `error`.
  소유자 컬럼이 NULL 인 행도 남의 행으로 본다 (`(owner <> ? or owner is null)`, 예전에는 NULL 소유자 행을 누구나 바꿀 수 있었다).
  세션 값은 컬럼의 Go 타입으로 바꿔 비교/대입한다 (`int` 컬럼이면 `int(c.Session.Id)`).
- REST Insert/Insertbatch 는 클라이언트가 보낸 `id` 를 무시한다 (모델의 `Insert` 는 그대로 id 지정 가능).
- `"protect": ["level", ...]` — REST Insert 는 컬럼의 DB 기본값(`u_level int DEFAULT 1` → 1, 없으면 0 값), Update 는 기존 DB 값으로 덮어써서
  클라이언트가 못 바꾸게 한다 ([보호 컬럼의 생성 값](#보호-컬럼의-생성-값)).
  보호 컬럼을 바꾸는 커스텀 `UpdateXByY` 는 거부한다. User 테이블의 `level` 은 항상 보호한다
  (예전에는 `protect` 를 설정하면 `level` 이 빠졌다).
  id 와 session 컬럼은 따로 처리하므로 protect 에 넣어도 무시된다.
  - `auth: "jwt"` 이고 `adminLevel` 이 숫자면, 세션 구조체에 정수형 `Level` 필드가 있고 그 값이 `adminLevel` 이상인
    관리자는 보호 컬럼을 바꿀 수 있고 다른 회원 정보도 수정/삭제할 수 있다 (reflect 로 확인하므로 `Level` 이 없으면 항상 보호).
- 비밀번호 컬럼은 이름이 `Passwd` 또는 `Password` 인 컬럼이다. 둘 다 있으면 두 컬럼 모두 같은 규칙(숨김, 필터 / 검색 / 정렬 제외,
  해시)이다 (예전에는 첫 컬럼만 숨겨서 다른 하나가 응답에 나갔다. Spring / Rust 도 같다).
  - 출력용 모델 `{Name}` 에서는 `json:"-"` 로 숨기고, 입력용 `{Name}Update` 구조체(json 태그 있음)로 Insert/Update 를 받는다.
    그래서 Read/Index/커스텀 Get·Find 응답에는 비밀번호가 나가지 않고, 검색 파라미터에서도 빠진다.
  - REST Insert/Update 에서 `jwt.GeneratePasswd` 로 해시하고, 해시가 실패하면 저장하지 않고 에러를 돌려준다.
  - Update 에서 비밀번호를 비워 보내면 기존 해시를 유지한다. `createddate` 는 항상 기존 값을 유지한다.
- 대상 프로젝트 요구사항: 비밀번호 컬럼이 있는 테이블은 `{package}/global/jwt.GeneratePasswd(string) (string, error)` 가 있어야 한다
  (예전에는 User 테이블만 필요했다).

### Spring (Kotlin)

- `Passwd`/`Password` 컬럼이 있으면 서비스에 `PasswordEncoder` 를 주입한다 (대상 프로젝트에 `PasswordEncoder` 빈 필요).
  생성/수정 시 인코딩하고, PUT 에서 비워 보내면 기존 해시 유지, PATCH 는 값이 있을 때만 인코딩한다.
  응답(`{Model}Response`, 조인 객체 포함), 목록 필터, `/search/{field}`, 정렬에서 비밀번호 컬럼(`passwd`, `password` 모두)은 빠진다.
- `POST /batch` 와 `DELETE /batch` 는 한 번에 1000 행(`MAX_BATCH`)까지 받는다. 넘으면 400 (Go / Rust 와 같다).
- 생성 때 보호 컬럼은 DB 기본값(없으면 타입 기본값, null 허용이면 null)이다 ([보호 컬럼의 생성 값](#보호-컬럼의-생성-값)).
- Create/Update/Patch 요청 DTO 에는 `protect` 컬럼(기본: User 의 level, `protect` 를 설정해도 빠지지 않는다)과 `createddate` 가 없다.
  기본 키는 경로 변수로만 정한다. Spring 쪽에는 관리자 예외가 없다.
- `"auth": "jwt"` 면 **모든** 컨트롤러가 `{kotlinPackage}.config.SessionProvider` 를 주입받고, 쓰기 엔드포인트는
  `currentUserId()` 가 null 이면 401 이다 (Go 와 같다. 예전에는 `session` 이 없는 테이블이 모두 공개였다).
  읽기(목록, `/count`, `/{id}`, `/search/*`)는 [읽기 정책](#읽기-정책-tableread-go--spring--rust-공통)을 따른다: `owner` 는 예전과 같고
  (`ownerSpec` / `isOwned`), `login` 은 `currentUserId()` 만 확인하고 세션 값 / 소유권 조건이 없고, `public` 은 401 검사가 없다.
  User 는 `selfSpec()` (id = `currentUserId()`), 남의 `GET /{id}` 는 404, 관리자(`currentValue("level") >= adminLevel`)는 모든 행이다.
  - 예외: User 의 단건 생성(`POST /api/user`, 회원가입)은 로그인 없이 호출할 수 있다 (`POST /batch` 는 로그인 필요, Go 와 같다).
  - User 테이블에 id `session` 이 없으면 수정/삭제는 본인(`currentUserId()`) 행만 된다 (Go 와 같다, 남의 행은 404 / 일괄 삭제 403).
- `session` 이 있고 읽기 정책이 `owner` 인 테이블은 목록/검색/개수를 본인 소유 행으로 거르고(DB where), 단건 조회/수정/삭제는 남의 행이면 404,
  일괄 삭제는 남의 행이 하나라도 있으면 403 (없는 행은 건너뛴다) 이다. 생성/수정 요청의 세션 컬럼은 세션 값으로 고정한다.
  - `config/SessionProvider.kt` 인터페이스는 파일이 없을 때만 생성한다(덮어쓰지 않음). 대상 프로젝트에서 `@Component` 로
    `currentUserId(): Long?` 를 구현해야 하고, `session` 의 key 가 `id` 가 아니면 `currentValue(key)` 도 구현한다.

### Spring 목록 / 수정 / 삭제

- 리포지토리는 `JpaSpecificationExecutor` 를 상속한다. `GET /api/{table}` 은 필터(`util.FilterCompare` 연산자: 문자열 `like` 는
  `_cb.like(..., likePattern(v), '!')`, 날짜 `start`/`end` 범위, enum 코드)와
  소유권을 Specification 으로, 페이지와 정렬을 `PageRequest` 로 DB 에서 처리한다 (예전에는 전체 행을 읽어 메모리에서 걸렀다).
  - `page` / `pagesize` 는 Go 와 같다: 둘 다 1 이상이면 그 페이지 (page 는 1 부터, pagesize 는 10000 까지), 없거나 0 이면 전체 행이다
    (예전 Spring 은 10 행이었다). 전체 행도 10000 행까지만 돌려주고, 더 있으면 `X-Truncated: true` 헤더를 붙이고 경고 로그를 남긴다.
    음수는 400. 응답에 `pagesize`(= `size`)도 있다.
  - 정렬: `sort` (또는 `orderby`) = `createddate desc, name` 또는 `createddate,desc`. 모르는 이름이면 400. 기본은 기본 키 내림차순.
  - 필터 파라미터 이름은 `@RequestParam("{key}")` 로 적고, 생성 코드의 지역 변수 / 매개변수는 `_page`, `_sort`, `{key}_start` 처럼
    컬럼 프로퍼티(밑줄 없음)와 겹치지 않는 이름이다. 그래서 `sort`, `page`, `order`, `result` 같은 컬럼도 컴파일된다.
    단, `page`/`pagesize`/`sort`/`orderby` 이름의 컬럼은 Go 와 같이 같은 쿼리 파라미터를 페이지/정렬과 함께 쓴다.
- `GET /api/{table}/search/{field}?{key}=값` 은 Specification(= 조건 + 소유권)으로 찾는다. 리포지토리에 메서드 이름 쿼리(`findByX`)는
  만들지 않는다 (예전 `findByUserId` / `findByCheckIn` 처럼 컬럼 이름에 따라 Spring Data 가 시작할 때 실패했다).
- `GET /api/{table}/count` 는 목록과 같은 필터 파라미터를 받는다.
- `PUT` 은 `PATCH` 와 같이 보낸(null 이 아닌) 필드만 바꾼다. `createddate` 와 보호 컬럼은 바꾸지 않는다.
  NULL 허용 컬럼을 null 로 지우는 요청은 할 수 없다 (null = 기존 값 유지).
- `DELETE /api/{table}/{id}` 는 없는 행이면 404.
- `DELETE /api/{table}/batch` 본문은 `[{"id": 1}, ...]` 이다 (Go 와 같다. 예전에는 엔티티 배열). 없는 행은 건너뛰고,
  `session` 이 있으면 남의 행이 하나라도 있을 때 아무것도 지우지 않고 403 이다 (예전에는 없는 행도 403 이었다).
- 생성 실패는 로그를 남기고 본문에 에러를 돌려준다 (제약 조건 위반 / 이미 있는 기본 키 409 `{"error": "constraint violation"}`, 그 밖 400).
- 응답의 조인 객체는 한 단계만 채운다 (조인의 조인은 `null`). `@EntityGraph` 도 직접 조인만 가져온다.
  생성/수정 후에는 조인까지 다시 읽어 응답한다 (조인이 있는 서비스는 `EntityManager` 를 주입받는다).
- 대상 프로젝트 요구사항: Spring Boot 기본 의존성(jackson-databind, jackson-module-kotlin, jackson-datatype-jsr310, slf4j)과
  Spring Data JPA 3.x (Hibernate 6: view 의 `@Immutable`). 대문자 컬럼 이름이 있으면 위의 `physical-strategy` 설정.

### 보호 컬럼의 생성 값

REST 생성(Insert / Insertbatch, Spring POST)에서 보호 컬럼(`protect`, User 의 `level`)은 클라이언트 값 대신 아래 값이 된다 (관리자 예외는 예전과 같다: Go / Rust 의 관리자는 보낸 값).

- 컬럼에 상수 기본값이 있으면 그 값 (`util.DefaultLiteral`: `1`, `'new'`, `2.50`, `b'1'`. MariaDB 의 `'...'` 따옴표와 `NULL` 도 해석한다).
- 기본값이 없거나 식(`current_timestamp()`, `uuid()`)이면 예전과 같이 0 값 (Rust / Spring 의 null 허용 컬럼은 NULL, Go 는 날짜 밖의 컬럼에 NULL 을 쓸 수 없어 0 값).
- Spring 의 날짜 컬럼 기본값은 쓰지 않는다 (예전 값).
- **바뀐 점**: 예전에는 항상 0 값이라 `u_level int NOT NULL DEFAULT 1` 인 회원이 level 0 으로 가입했다.

## 통일 JWT (Go / Spring / Rust)

세 백엔드 템플릿이 같은 토큰을 주고받는다 (한 백엔드에서 받은 토큰을 다른 백엔드에서도 쓸 수 있다).

- 알고리즘 HS256 고정, 비밀 키는 환경변수 `JWT_SECRET` (32 바이트 이상, 짧으면 템플릿 / 기본 Rust `auth.rs` 가 거부한다. `bootstrap/.env.example`).
  유효 시간은 `JWT_EXPIRATION_MS` (밀리초, 기본 86400000).
- 헤더 `Authorization: Bearer <token>`.
- claim:

```json
{ "sub": "42", "id": 42, "email": "a@b.c", "level": 1, "iat": 1767225600, "exp": 1767312000 }
```

  - `sub`: id 문자열, `id`: 정수 (읽을 때 `id` 가 없으면 `sub` 를 숫자로), `email`: 문자열, `level`: 정수 (없어도 됨, 없으면 0 / None),
    `iat` / `exp`: 초 단위 정수 (`exp` 는 있으면 검증).
- 세션 매핑: Go `controllers.Session{Id, Email, Level}`, Spring `SessionProvider.currentUserId()` = `id`, Rust `Session{id, email, level}`.
- gomachine 이 직접 만드는 것은 Rust 기본 `support/auth.rs` 뿐이다. Go `global/jwt` 와 Spring 의 발급 / 검증 코드는 템플릿이 이 형식으로 맞춘다.

## 인증 API 계약 (템플릿)

gomachine 은 아래 경로를 만들지 않는다. basicproject 템플릿(Go / Spring / Rust 백엔드, React / Vue 프런트)이 같은 모양으로 구현한다
(prefix `/api`). 프런트는 백엔드를 바꿔도 같은 코드로 로그인한다.

| 요청 | 본문 | 성공 | 실패 |
|---|---|---|---|
| `POST /api/auth/signup` | `{"email", "password", "name"}` | 201 `{token, tokenType: "Bearer", expiresInMs, user}` | 400 (검증), 409 (이미 있는 email) |
| `POST /api/auth/login` | `{"email", "password"}` | 200 `{token, tokenType: "Bearer", expiresInMs, user}` | 401 |
| `GET /api/auth/me` | - (`Authorization: Bearer <token>`) | 200 `user` | 401 |
| `POST /api/upload/index` | multipart, 파일 field `upload` | 200 `{filename, url}` | 400 / 401 |

- `user` = `{id, email, name, level, createddate}`: 생성된 User 응답과 같은 JSON (비밀번호 없음, `createddate` 는 `yyyy-MM-dd HH:mm:ss`).
- `token` 은 [통일 JWT](#통일-jwt-go--spring--rust), `expiresInMs` 는 유효 시간 밀리초 (`JWT_EXPIRATION_MS`, 기본 86400000 = 24시간).
- 에러 본문: `{status, message, fieldErrors?}` (`fieldErrors` 는 `{필드: 메시지}`, 검증 실패일 때). 상태 코드는 HTTP 상태다
  (생성된 Go / Rust 테이블 API 의 HTTP 200 + `code` 와 다르다).
- 회원가입은 생성된 `POST /api/user` 대신 이 경로를 쓴다 (비밀번호 해시, level 은 DB 기본값, 토큰 발급까지 한 번에).

## HTTP 계약 (Go / Spring / React / Vue / Dart)

React/Vue(그리고 Dart)가 두 백엔드 중 어느 것에도 붙을 수 있게 맞춘 규칙이다. Go 는 기존 경로/필드를 바꾸지 않고 **추가만** 했다.

### 이름

- JSON key = 접두사 뺀 컬럼 이름의 lowerCamel (`util.JSONKey`). Go json 태그, Kotlin 프로퍼티, TypeScript 필드, Dart json key 가 모두 같다.
- `_` / 대소문자만 다른 컬럼(`p_created_at` 과 `p_createdat`, `user_id` 와 `userId`)은 이름이 겹치므로 경고를 찍고,
  앞(ordinal_position) 컬럼이 원래 이름을 갖고 뒤 컬럼은 번호를 붙인다 (`util.DedupeColumns`): Go 필드 `Createdat2`,
  JSON key / 필터 `createdat2`, 검색 경로 `/search/createdat2`, 날짜 범위 `startcreatedat2`. Go / Spring / TypeScript / Dart 가 같은 규칙이다.
  SQL 컬럼 이름은 그대로다.
- 생성 코드가 이미 쓰는 이름 `extra`(Go `Extra` 필드, Kotlin / TypeScript `extra`)와 같은 컬럼(`p_extra`)도 같은 방법으로 `extra2`
  (Go 필드 `Extra2`) 가 된다 (예전 Go 는 두 필드의 json 태그가 겹쳐 둘 다 JSON 에서 빠졌다).

  | 컬럼 | JSON key | 검색 경로 | 날짜 범위 파라미터 |
  |---|---|---|---|
  | `u_createddate` | `createddate` | `/search/createddate` | `startcreateddate` / `endcreateddate` |
  | `p_created_at` | `createdAt` | `/search/createdat` | `startcreatedat` / `endcreatedat` |
  | `a_userId` | `userid` | `/search/userid` | - |

  - 밑줄 하나짜리 컬럼은 예전 Go 와 같다. camelCase 컬럼(`userId`)도 예전 Go 처럼 `userid` 다 (예전 TypeScript 는 `userId` 였다).
  - 여러 단어 컬럼의 Go json 태그가 `createdat` → `createdAt` 으로 바뀌었다 (이번 브랜치에서 처음 생긴 이름이라 기존 클라이언트 없음).
- 목록/개수 필터 파라미터 = JSON key, 날짜 컬럼(date/datetime/timestamp)은 `start{key 소문자}` / `end{key 소문자}`.
  `id`, `primary` 컬럼, 비밀번호, blob 컬럼은 필터/검색이 없다 (id 도 primary 도 없어 대신 키로 쓰는 첫 컬럼은 거를 수 있다).
  모든 백엔드(Go, Spring, Rust)와 TypeScript 가 같다. **바뀐 점**: 예전 Go 는 `primary` 컬럼도 목록 필터로 받았다 (검색 경로는 없었다).

### 경로 (prefix 는 대상 프로젝트 설정, 예: `/api`)

| 요청 | Go | Spring | 비고 |
|---|---|---|---|
| `GET /{t}?page&pagesize&{필터}&orderby` | O | O | page 는 1 부터. page / pagesize 가 없으면(0) 전체 행 (Spring 은 10000 행까지) |
| `GET /{t}/count?{필터}` | O (추가) | O | Go 는 예전 `POST /{t}/count` 도 유지 |
| `GET /{t}/:id` | O | O | id 컬럼이나 `primary` 가 있을 때 |
| `GET /{t}/search/{field}?{key}=값` | O (추가, 모든 검색 컬럼) | O | 같은 값(=) 목록. Go 는 세션 조건을 붙인다 |
| `POST /{t}` | O | O | view 없음 |
| `POST /{t}/batch` | O | O | |
| `PUT /{t}/:id`, `PATCH /{t}/:id` | O (추가) | O | 보낸 필드만 수정. 경로 id 가 본문 id 보다 우선 |
| `DELETE /{t}/:id` | O (추가) | O | |
| `DELETE /{t}/batch` 본문 `[{"id":1}]` | O | O (본문 모양 변경) | 없는 행은 건너뛴다, 남의 행이 있으면 아무것도 지우지 않는다 |
| `PUT /{t}`, `DELETE /{t}` (본문) | O (예전 경로) | X | |
| `GET /{t}/{get,find,count}/...`, `PUT /{t}/{x}/{y}` | O (예전 경로) | X | 커스텀 메서드 |
| `DELETE /{t}/delete/{by}/:{값}` | O (추가) | X | 커스텀 `DeleteBy` (like 조건 제외) |

- Go 는 정적 경로(`/count`, `/batch`, `/search/x`)를 `/:id` 보다 먼저 등록한다 (fiber 는 등록 순서로 찾는다).
- 새 Go 경로는 기존 핸들러(Read/Update/Delete/Count/커스텀 메서드)와 새 `Search` 를 부르므로 세션/소유권/보호 컬럼/비밀번호 규칙이 같다.
- view(`_vw`)는 읽기 경로만 있다.
- 대상 프로젝트의 fiber Router 에 `Patch` 가 필요하다 (fiber v2 기본).

### 응답

| | Go | Spring |
|---|---|---|
| 목록 | `{items, total?, code}` (total 은 page 를 보냈을 때) | `{content, page, size, pagesize, totalElements, totalPages, first, last, empty}` |
| 개수 | `{total, count, code}` | `{count}` |
| 단건 | `{item, code}` (없으면 `item: null`) | 엔티티 (없으면 404) |
| 검색 | `{items, code}` | 배열 |
| 생성 | `{id, code}` | 엔티티 (batch 는 배열) |
| 수정 / 삭제 | `{code}` | 엔티티 / `{success}` |
| 에러 | HTTP 200 + `code: "error" \| "auth error"`, 잘못된 요청은 400 + `code: "bad request"` | HTTP 상태 코드 (400/401/403/404/409) |

- TypeScript(`src/models/contract.ts`)가 둘 다 읽는다: 목록 `content ?? items`, 전체 개수 `totalElements ?? total`,
  개수 `count ?? total`, 단건은 `item` key 가 있으면 그 값. 엔티티에 `item`/`code` 필드가 있으면 기본 키가 있는 응답을 엔티티로 본다.
  모델 메서드는 `find/findall → T[]`, `findpage → Page<T> {items, total}`, `get → T | null`, `count → number`,
  `insert/update/patch → Partial<T>` (Go 는 id 만 준다), `remove/removeBatch → void` 를 돌려준다.
  - `findall` 은 `pagesize` 1000 으로 페이지를 차례로 읽어 모은다 (받은 행이 1000 보다 적거나 전체 개수만큼 모이면 멈춘다. 예전에는 `pagesize: 9999` 한 번).
  - view 는 쓰기 메서드(`insert/insertBatch/update/patch/remove/removeBatch`)가 없고, id 컬럼이 없는 테이블은
    `get/update/patch/remove/removeBatch` 가 없다 (Go 에 그 경로가 없다). Vue composable 도 같이 `create/fetchOne/update/remove` 를 뺀다.
- Go 의 `code` 에러(`error`, `auth error`, `bad request`)는 `ApiError` 로 던진다. Spring 에러는 api 래퍼(axios 등)가 던진다.
  Vue composable 은 둘 다 `error` 에 담는다.
- Dart 는 목록 `content ?? items`, 개수 `count ?? total`, 단건 `item` 래퍼를 읽고, 수정/삭제를 `/{t}/{id}` 로 부른다.
  view 는 `insert/update/delete` 가 없고, id 컬럼이 없는 테이블은 `get/update/delete` 가 없다 (예전에는 Go 에 없는 `PUT /{t}` 를 불렀다).
  `fromJson` 은 값을 너그럽게 읽는다: bool 은 `true/false`, 숫자(0 이 아니면 true), `"1"/"true"`, base64(예전 Go `bit`) 를,
  int/double 은 숫자, 숫자 문자열, bool 을 받는다. 없는 NOT NULL 문자열 필드(예: 응답에 없는 비밀번호)는 `''` 이다.
  필드 / enum 값 이름이 Dart 예약어나 생성 멤버(`class`, `in`, `extra`, enum 의 `values`, `index` ...)면 뒤에 `_` 를 붙인다 (JSON key 는 그대로).

### 값

- enum 은 코드(숫자, data 위치)로 주고받는다. TypeScript 필드 타입은 코드 union (`0 | 1 | 2`). 필터와 `/search` 도 코드.
- 날짜는 `yyyy-MM-dd HH:mm:ss` (date 는 `yyyy-MM-dd`). Spring 은 요청에서 날짜만, ISO(밀리초, `Z` / 오프셋 포함)도 받는다 (위 [날짜 값](#날짜-값)).
- `tinyint` / `bit` 는 두 백엔드 모두 bool(`true/false`)이라 TypeScript 는 `boolean` 이다 (예전에는 `boolean | number`, `boolean | string`).
  목록 필터는 두 백엔드 모두 `true/false/1/0` 을 받는다. `json` 은 둘 다 문자열.
- 문자열 필터는 모든 백엔드(Go, Spring, Rust)에서 **부분 일치**다: `?title=ab` → `title like '%ab%' escape '!'`.
  서버가 값의 `%`, `_`, `!` 를 `!` 로 escape 하고 양쪽에 `%` 를 붙인다 (`50%` 는 글자 그대로 `50%` 를 포함하는 행. `util.LikePattern`).
  escape 문자를 `\` 가 아닌 `!` 로 써서 MySQL `NO_BACKSLASH_ESCAPES` 와 상관없이 같다. 대소문자 구분은 컬럼 collation 을 따른다.
- 연산자는 컬럼마다 `table[].compare` 로 바꾼다 (`util.FilterCompare`, 세 백엔드가 같은 값):

  ```json
  "compare": [{ "name": "code", "type": "=" }, { "name": "cnt", "type": ">=" }, { "name": "memo", "type": "not like" }]
  ```

  | 컬럼 | 기본 | 쓸 수 있는 값 |
  |---|---|---|
  | 문자열 (`char`, `varchar`, `*text`, `enum`, `set`, `json`, `time`, 모르는 타입) | `like` | `like`, `not like`, `=`, `<>`(`!=`), `<`, `<=`, `>`, `>=` |
  | 숫자 | `=` | `=`, `<>`, `<`, `<=`, `>`, `>=` |
  | enum(`map`), bool | `=` | `=`, `<>` |
  | 날짜 (`date`, `datetime`, `timestamp`) | `start{key}` / `end{key}` 범위 | - |

  쓸 수 없는 값(숫자의 `like`, `in` 등)은 경고하고 기본값을 쓴다. `/search/{field}` 는 예전과 같이 항상 `=` 다.
- **바뀐 점**: 예전 Go / Rust 는 `name`/`title`/`content` 만 `=`(또는 compare), 나머지 문자열은 보낸 값 그대로 `like`
  (와일드카드를 클라이언트가 넣어야 했다), Spring 은 모든 컬럼 `=` 이고 compare 를 무시했다. 정확히 같은 값만 찾으려면 `"type": "="` 을 설정한다.
  TypeScript 타입 / 필터 이름은 그대로다.

## Rust (axum + sqlx)

Go 백엔드와 같은 REST API (경로, 파라미터, 응답 key, 권한 규칙, SQL)를 axum 0.8 + sqlx 0.9 (MySQL) 로 만든다.

- `rustModelFilePath`: crate 루트 (`Cargo.toml` 이 있는 디렉터리). 생성 파일은 그 아래 `src/` 에 둔다. 모듈은 `crate::` 경로로 참조한다.
- Go 와 같은 HTTP 계약(HTTP 200 + `code`, 파라미터 해석 실패만 400)이고, 같은 DB / 계정(bcrypt `$2a$` cost 10, 같은 JWT)을 쓴다.
- `session` 컬럼은 Go 와 같이 정수 컬럼만 된다.
- 타입: `int`/`smallint`/`mediumint`/`year` → `i32`, `bigint` → `i64` (`column_type` 이 unsigned 면 `u32` / `u64`),
  `tinyint`/`bit` → `bool`, `float`/`double`/`real`/`decimal` → `f64`, 문자열 / 날짜 → `String`, blob / binary → `Vec<u8>`.
  enum(`map`) 컬럼은 newtype `pub struct Status(pub i32)` 이다 (코드는 다른 언어와 같다). `id` 는 정수로 가정한다 ([id 컬럼](#id-컬럼-go--rust)).
- 이름: 필드는 JSON key 의 snake_case (`createdAt` → `created_at`), Rust 예약어는 뒤에 `_` (`type_`, `self_`).
  struct / enum 이 생성 코드가 쓰는 타입(`Result`, `Option`, `Path`, `Json`, `Value`, `Error`, `Session` ...)과 같으면
  `Model` / `Enum` 접미사 (`result_tb` → `ResultModel`, enum `type` → `TypeEnum`). 모듈 / 파일은 Go 파일 이름과 같은 소문자이고
  Rust 예약어(예약만 된 `box`, `try`, `gen`, `typeof` ... 포함)나 `mod`, `main`, `lib`, `support`, `handlers`, `models`, `routes`,
  `custom`, `std`, `core`, `alloc` 이면 뒤에 `_` 다 (`type_tb` → `type_`, `box_tb` → `box_`, `std_tb` → `std_`).
  다른 테이블이 같은 모듈 이름이 되면(`user_tb` 와 `user_vw` 를 `ResolveTableNames` 없이 생성) 먼저 나온 테이블만 만들고 경고한다.

### 생성 파일

| 파일 | 종류 | 내용 |
|---|---|---|
| `src/support/mod.rs` | 생성 | 아래 모듈 선언과 자주 쓰는 타입 re-export (`AppState`, `Session`, `ApiResult`, `bad_request`, `Where`, `Arg` ...) |
| `src/support/query.rs` | 생성 | Go `models/db.go` 의 `Where` / `Custom` / `Paging` / `Ordering` 과 `WhereClause` 규칙 |
| `src/support/response.rs` | 생성 | `ApiResult` (Go `c.Result`), `bad_request()` |
| `src/support/session.rs` | 생성 | `Session`, `Option<Session>` 추출기, `is_admin` |
| `src/support/state.rs` | 생성 | `AppState { pool }`, `FromRef<AppState> for MySqlPool`, `connect` / `connect_lazy` / `connect_options` (세션 time_zone 을 바꾸지 않는 풀) |
| `src/support/password.rs` | 생성 | `generate_passwd` / `verify_passwd` (Go `jwt.GeneratePasswd`) |
| `src/support/clock.rs` | 생성 | `now()` (createddate, `timezone` 설정), `local_now(fmt)` |
| `src/support/row.rs` | 생성 | 날짜 자리표시 값 → `""`, DECIMAL / FLOAT / DOUBLE 문자열 → `f64` |
| `src/support/serde_ext.rs` | 생성 | 너그러운 bool, base64 바이트, `double_option` |
| `src/support/error.rs` | 생성 | `Error` (응답은 HTTP 200 `{"code":"error"}`), `log_sql` |
| `src/support/params.rs` | 생성 | 요청 파라미터 해석 (Go `c.Query` / `strconv` / 본문 map 규칙), 거부(rejection)를 400 으로 바꾸는 인자 타입 |
| `src/models/{module}.rs` | 생성 | 테이블 모델 struct, enum newtype, SQL (Go `models/{x}.go`) |
| `src/handlers/{module}.rs` | 생성 | 핸들러와 `routes()` (Go `controllers/rest/{x}.go` + `router/routers/{x}.go`) |
| `src/models/mod.rs`, `src/handlers/mod.rs`, `src/routes.rs` | 생성 | 테이블 모듈 목록과 `routes::routes()` (모든 테이블 경로를 merge) |
| `src/support/auth.rs` | **사용자** | JWT → `Session` ([통일 JWT](#통일-jwt-go--spring--rust)). 다른 토큰 형식이면 고치는 곳 |
| `src/main.rs` | **사용자** | `src/main.rs` 와 `src/lib.rs` 가 모두 없을 때만 만든다 |

- 생성 파일은 실행마다 다시 만들되 내용이 같으면 쓰지 않는다 (수정 시각 유지, cargo 재빌드 없음). 고치면 다음 실행에 되돌아간다.
  생성 파일의 첫 줄은 `// @generated by gomachine - do not edit` 이다.
- 테이블을 지우거나 이름을 바꾸면 `Finish` 가 `src/models/`, `src/handlers/` 에서 첫 줄이 위 머리줄이고 이번 실행에서 만들지 않은
  `.rs` 파일을 지운다 (`mod.rs`, 하위 디렉터리, 머리줄이 없는 파일은 지우지 않는다). 그 디렉터리에 직접 만든 파일은 머리줄을 넣지 않는다.
  한 번에 일부 테이블만 생성하면 나머지 테이블 파일도 지워진다 (`mod.rs` / `routes.rs` 도 이번 실행의 테이블만 담는다).
- 사용자 파일은 없을 때만 만들고 절대 덮어쓰지 않는다. 다시 받으려면 지우고 실행한다.
- 지원 모듈은 실행마다 한 번(`rustcodegen.Prepare`, 테이블 생성 전), 모듈 목록 / 경로는 모든 테이블 뒤에(`rustcodegen.Finish`) 쓴다.
- crate 루트(`src/main.rs` 또는 `src/lib.rs`)에 `mod support; mod models; mod handlers; mod routes;` 가 있어야 한다. 직접 만든 루트에
  빠져 있으면 경고를 찍는다. 생성한 `main.rs` 는 `DATABASE_URL`(필수)로 `support::state::connect` 풀을 만들고, `RUST_LOG`(기본 `info`)로 tracing 을 켜고,
  `routes::routes()` 를 `/api` 아래에 붙이고, `PORT`(기본 8008, 예전 8080)에서 연다. `JWT_SECRET` 이 없거나 32 바이트보다 짧으면 시작하지 않는다. CORS 는 `CORS_ORIGINS`(쉼표 구분)가 있으면 그 origin 만,
  없으면 모든 origin 을 허용한다 (credentials 없음).
- 지원 파일에는 `#[cfg(test)]` 단위 테스트가 들어 있다 (`cargo test`).
- rustfmt: `rustfmt` 가 PATH 에 있으면 쓰는 모든 `.rs` 를 `rustfmt --edition 2024` 로 정리한다 (파일과 crate 루트 사이의
  `rustfmt.toml` / `.rustfmt.toml` 을 쓴다). 없거나 실패하면 경고 한 번 찍고 형식 없이 쓴다. `GOMACHINE_RUSTFMT=0` 으로 끈다.
  두 번 생성해도 바뀌지 않는다. 지원 모듈 / `auth.rs` / `main.rs` 는 rustfmt 기본 형식으로 만들어 rustfmt 유무와 상관없이 같고,
  테이블 모델 / 핸들러 / `routes.rs` 는 이름 길이에 따라 줄바꿈이 달라서 rustfmt 가 없으면 `cargo fmt --check` 가 실패할 수 있다.
- **DB 풀은 `support::state::connect(url, n)` / `connect_lazy(url, n)` (또는 `connect_options(url)`)로 만든다.** sqlx 는 기본으로 연결마다
  `SET time_zone='+00:00'` 을 보내서 `TIMESTAMP` 컬럼을 UTC 로 읽는다 (서버 시간대가 KST 면 9 시간 차이). 이 함수들은
  `MySqlConnectOptions::timezone(None)` 으로 서버 시간대를 그대로 써서 Go 백엔드와 같은 값을 읽는다 (URL 에 `timezone=` 을 적으면 그 값).
  직접 만든 `main.rs` 에서 `MySqlPoolOptions::new().connect(&url)` 을 쓰면 TIMESTAMP 값이 달라지므로 바꾸거나 `.timezone(None)` 을 넣는다.
### 모델 / 핸들러

Go 생성기(`go/main.go`, `views/go/{model,rest}.jet`, 라우터)가 기준이다. 같은 `util` 규칙(JSON key, 필터 / 검색 이름, enum 코드,
커스텀 메서드 해석, 보호 컬럼, 조인 해석, 세션 컬럼 검사)을 쓴다.

- 모델 (`impl {Model}`): `get` (조인 포함) / `get_raw` (이 테이블만, 소유권 검사 / 수정용) / `find` / `count` / `insert` (LAST_INSERT_ID) /
  `update` (Go `UpdateNulls`, id 를 뺀 모든 컬럼) / `delete`, 커스텀 `get_by_x` / `find_by_x` / `count_by_x` (`_args` 가 조건 목록) /
  `delete_by_x` / `update_x_by_y`, `parse_order`, `from_body` / `list_from_body` / `merge`. SQL 은 `*_query` 함수가 `QueryBuilder` 로
  돌려준다 (테스트에서 `qb.sql()` 로 확인). Sum / GroupBy / UpdateWhere / Truncate / DeleteAll 등 REST 가 쓰지 않는 메서드는 없다.
- 읽기: 날짜(`date` / `datetime` / `timestamp`)는 `DATE_FORMAT(...)` 으로 `yyyy-MM-dd HH:mm:ss` (date 는 `yyyy-MM-dd`) 문자열을 읽고
  Go 의 자리표시 값은 `""` (null 허용이면 `null`), `time` / `json` 은 `CAST(... AS CHAR)`, `decimal` 과 `float` / `double` / `real` 은
  `CAST(... AS CHAR)` → `f64` (MySQL 텍스트 값이라 FLOAT `0.1` 은 Go 와 같은 `0.1`, f64 로 바로 읽으면 `0.10000000149011612`),
  `bit` 는 `(col<>0)` → `bool` (`BIT(n)` 의 128 이상 값도 true, NULL 은 `null`), `year` 는 `CAST(... AS SIGNED)` (unsigned 면 `AS UNSIGNED`),
  enum(`map`) 컬럼은 부호 있는 정수 컬럼이 아니면(`varchar`, `bit`, `year`, unsigned ...) `CAST(... AS SIGNED)` 로 `i32` 코드를 읽는다
  (쓸 때는 `i32` 를 바인딩하고 MySQL 이 컬럼 타입으로 바꾼다). 조인은 Go 와 같은 `left join` / 별칭 `j_{변수}` /
  자기 조인 한정이고, 조인 행은 `extra[변수]` 에 조인 테이블 모델과 같은 JSON (비밀번호 제외, `extra` 는 enum 표시 이름)으로 넣는다 (없으면 `null`).
- 비밀번호: `passwd` / `password` 컬럼은 둘 다 있어도 모두 응답(조인 `extra` 포함)에서 빠지고(`skip_serializing`), 목록 필터 / 검색 /
  정렬에 없고, Insert / Insertbatch / Update / 커스텀 `UpdateXByY` 에서 해시한다 (Go / Spring 과 같다).
- 쓰기: Go 와 같다. `createddate`(문자열)는 Insert 에서 서버 현재 시각(`clock::now()`, `timezone`), NOT NULL 날짜의 빈 값은
  기본값(현재 시각 / 고정 값 / 자리표시 값), null 허용 날짜 / 시간 / year 의 빈 값은 NULL. REST Insert 는 id 를 무시한다.
- enum: `pub struct Status(pub i32)` (JSON 은 숫자, 모르는 코드도 담는다), 연관 상수 `Status::WAIT` (key 대문자, `util.RustEnumItems`),
  `Status::LABELS`, `label()` (Go `GetStatus`), `find(label)` (Go `FindStatus`, 없으면 `None`), `HAS_ZERO`.
- 경로: Go 라우터가 등록하는 모든 경로 (아래 표, `{t}` = Go 파일 이름). 같은 모양의 경로가 또 나오면(같은 커스텀 메서드 두 번 등)
  axum 이 시작할 때 panic 하므로 경고를 찍고 건너뛴다.

  | 테이블 | 경로 |
  |---|---|
  | 모두 | `GET /{t}`, `GET\|POST /{t}/count`, `GET /{t}/search/{field}` (검색 컬럼마다), 커스텀 `GET /{t}/{get,find,count}/{by}/{값}...` |
  | id 있음 | `GET /{t}/{id}` |
  | view 아님 | `POST /{t}`, `POST /{t}/batch`, 커스텀 `PUT /{t}/{x}/{y}` (JSON 본문), `DELETE /{t}/delete/{by}/{값}` (like 조건 제외) |
  | view 아님 + id 있음 | `PUT /{t}` (본문 id), `PUT\|PATCH\|DELETE /{t}/{id}`, `DELETE /{t}` (본문 id), `DELETE /{t}/batch` |

  axum 0.8 경로 문법(`/{id}`)이고 정적 경로(`/count`, `/batch`, `/search/x`)가 경로 변수보다 먼저 맞는다. 응답 / 파라미터 해석은 Go 라우터와 같다
  (page / pagesize 가 숫자가 아니거나 음수, page 가 `params::MAX_PAGE` (1000000000) 초과, 필터 / 경로 / 본문 값을 해석하지 못하면 400).
  `POST /{t}/batch` / `DELETE /{t}/batch` 는 `params::MAX_BATCH` (1000) 행까지 받고 넘으면 400 (Go / Spring 과 같다).
- 응답 / 권한: Go `rest.jet` 과 같다 (읽기 정책: `public` 읽기 핸들러는 세션 인자가 없다. 세션 없음 → `owner` / `login` / User 의 Read / Index / Count / Search / 커스텀 조회는 `auth error`, Insert / Insertbatch / Update /
  Delete / Deletebatch 는 `error`; 세션 조건, 소유권 검사(`GetRaw`, NULL 소유자 거부, 커스텀 Update / Delete 의
  `(owner <> ? or owner is null)` 개수 검사), User 본인 규칙, 보호 컬럼 / 관리자(`adminLevel`), 비밀번호 해시 / 유지, Deletebatch 의
  전체 검사 후 삭제). 세션 값은 `Session::value(key)` (id / level 밖의 key 는 `auth.rs` 가 `values` 에 넣는다, 아래 [인증](#지원-모듈-api-cratesupport)).
- 본문: Go json 과 같이 key 의 대소문자를 가리지 않고, NOT NULL 필드의 `null` 은 보내지 않은 것으로 본다 (Insert 는 0 값, 부분 수정은
  기존 값). 타입이 맞지 않으면 400. 커스텀 `UpdateXByY` 본문 key 는 Go 와 같이 `LowerCamel(컬럼)` 이고 정확히 맞아야 한다.
  정수 컬럼 값은 Go 와 같이 소수점을 버리지만(`1.5` → `1`), 컬럼의 Rust 타입(`i32` / `i64` / `u32` / `u64`, enum 은 `i32`) 범위를 넘거나
  unsigned 에 음수면 400 이다 (Go 는 범위를 넘는 값을 잘라서 저장한다).
- 상태: 핸들러는 `State<MySqlPool>` 만 꺼낸다. `routes::routes::<S>()` 는 `MySqlPool: FromRef<S>` 인 어떤 상태든 된다.

Go 와 일부러 다른 점:

- 빈 목록은 `items: []` 다 (Go 는 `null`).
- null 허용 컬럼은 `Option` 이라 응답에서 `null` 이다 (Go 는 0 값). TypeScript / Dart 타입은 이미 `null` 을 받는다.
  NULL 인 enum 의 `extra` 표시 이름은 `""` 다. 보호 컬럼이 null 허용이고 상수 기본값이 없으면 Insert 는 NULL 을 넣는다 (Go 는 0 값).
- DB 에러는 모든 핸들러에서 `code: "error"` 다 (Go 도 Read / Index / Count / Search 는 `error` 지만, 커스텀 조회는 에러를 무시하고 빈 결과를 `ok` 로 돌려준다).
  Go 처럼 읽지 못한 행만 건너뛰지 않고 요청 전체가 `error` 다. 에러 내용(`error` key)은 응답에 넣지 않고 로그로만 남긴다.
- Update 는 `createddate` 가 비어 있으면(자리표시 값으로 읽힌 경우) 다른 NOT NULL 날짜처럼 기본값을 넣는다 (Go 는 `""` 를 저장해 strict 모드에서 실패).
- 정수 파라미터: 경로 값은 컬럼 타입(`i32` 등) 범위를 넘으면 400, 세션 값이 세션 컬럼 타입(`i32` 등)에 맞지 않으면 그 핸들러의 세션 에러다
  (Go 는 잘라서 비교한다).
- 실수: `f64` 는 serde_json 이 정수 값도 `1.0` 으로 쓴다 (Go `float64` 는 `1`). JSON 숫자로는 같은 값이지만 문자열로 비교하면 다르다.
- 라우터 (Fiber → axum): 경로는 대소문자를 가린다 (`/User` 는 404, Fiber 기본은 가리지 않는다), 끝의 `/` 는 다른 경로다 (`/user/` 는 404,
  Fiber 기본은 같은 경로), 경로 값은 percent-decode 한 값이다 (`/user/find/name/a%20b` → `a b`, Fiber 기본은 디코드하지 않는다).
  본문은 `Content-Type` 을 보지 않고 JSON 으로 해석한다 (Fiber `BodyParser` 는 `application/json` 이 아니면 해석하지 못해 400).

### Cargo.toml

생성기는 `Cargo.toml` 을 고치지 않는다. `[dependencies]` 에 빠진 crate 가 있으면 넣어야 할 줄을 경고로 찍는다.
**이름만 확인한다**: 버전이나 feature 가 모자라면 경고 없이 컴파일 오류다 (예: `tokio = "1"` 은 `#[tokio::main]` / `tokio::net` 이 없어
오류, sqlx 에 `mysql` 이 없으면 `MySqlPool` 이 없다). 아래 feature 는 모두 필요하다:
`tokio` = `macros`, `rt-multi-thread`, `net`, `sqlx` = `runtime-tokio`, `mysql`, `derive`, `json`
(+ TLS 접속이면 `tls-rustls`, TLS 없이 `caching_sha2_password` / `sha256_password` 계정으로 접속하면 `mysql-rsa`), `serde` = `derive`,
`jsonwebtoken` = `rust_crypto` (또는 `aws_lc_rs`), `chrono` = `clock`, `std`, `tracing-subscriber` = `env-filter`, `tower-http` = `cors`.

```toml
[dependencies]
axum = "0.8.9"
tokio = { version = "1.54", features = ["macros", "rt-multi-thread", "net"] }
sqlx = { version = "0.9.0", default-features = false, features = ["runtime-tokio", "tls-rustls", "mysql", "mysql-rsa", "derive", "json"] }
serde = { version = "1", features = ["derive"] }
serde_json = "1"
serde_with = "3"
jsonwebtoken = { version = "11.1", features = ["rust_crypto"] }
bcrypt = "0.19.3"
base64 = "0.22"
chrono = { version = "0.4", default-features = false, features = ["clock", "std"] }
thiserror = "2"
tracing = "0.1"
# 생성한 main.rs 가 쓴다
tracing-subscriber = { version = "0.3", features = ["env-filter"] }
tower-http = { version = "0.6", features = ["cors"] }
# timezone 이 Asia/Seoul / UTC / local 이 아닌 IANA 이름일 때만
chrono-tz = "0.10"
```

### 지원 모듈 API (`crate::support`)

- 조회 조건 (`query`):
  - `Bind` = `I32 | I64 | U32 | U64 | F64 | Bool | Str | Bytes | Null | List(Vec<Bind>) | Pair(..)`. 정수 / 실수 / bool / 문자열 /
    `Vec<u8>` / `Option<T>`(None → NULL)에서 `.into()` 된다. `Bind::list(iter)` 는 in, `Bind::pair(a, b)` 는 between 값.
  - `Where::new(column, value, compare)`, `Where::eq(column, value)`, `Custom::new(sql, params)`,
    `Arg::{Where, Custom, Paging { page, pagesize }, Ordering(String)}`.
  - `push_where(qb, prefix, &where)` 는 Go `WhereClause` 와 같다: `" and {prefix}{column} {op} ?"`. column 은
    `^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`, op 는 `= != <> < <= > >= like, not like, in, not in, between`
    (대소문자 / 공백 수 무시)만 되고 어긋나면 `" and 1=0"`. in / not in 은 원소마다 바인딩하고 빈 목록이면 in 은 `" and 1=0"`,
    not in 은 `" and 1=1"` (단일 값은 원소 하나). between 은 값 두 개가 아니면 `" and 1=0"`. like 는 보낸 값 그대로다 (커스텀 `FindByXlike`).
  - 목록 필터의 부분 일치: `like_pattern(v)` (`!` `%` `_` 를 `!` 로 escape, 양쪽 `%`), `like_filter(column, v, not)` 이
    `{column} like ? escape '!'` Custom 조건을 만든다 (`LIKE_ESCAPE`).
  - `push_custom(qb, &custom)`: `" and " + sql`, 따옴표 밖의 `?` 마다 params 를 바인딩한다. 개수가 다르면 `" and 1=0"`.
  - `apply_args(qb, prefix, &[Arg]) -> Page` 는 Where / Custom 을 붙이고 Paging / Ordering 을 모은다.
    `push_order_page(qb, &page, default_column)` 은 Go Find 와 같다: 페이지면 `order by {order 또는 default desc} limit ? offset ?`
    (offset = (page-1)*pagesize, 넘치면 `i64::MAX` = 빈 페이지), 아니면 `order by {order 또는 default}`. Ordering 은 SQL 에 그대로 들어가므로 검증한 값만 넘긴다.
  - `valid_column(name)` (Go `ValidColumn`).
  - sqlx 0.9 의 `query()` 는 `&'static str` 이나 `AssertSqlSafe(String)` 만 받는다. 생성 코드는 `QueryBuilder<MySql>` 를 쓴다.
- 응답 (`response`): `Reply` (= `Result<Response, Error>`, 핸들러 반환 타입), `ApiResult::ok() / error() / auth_error() / with_code(code)`, `.set(key, value)`, `.code(code)`, `.insert(..)`,
  `IntoResponse` 는 항상 HTTP 200 JSON 객체. `bad_request()` 는 HTTP 400 `{"code":"bad request"}` (라우터 파라미터 해석 실패).
- 세션 (`session`): `Session { id: i64, email: String, level: Option<i64>, values: HashMap<String, i64> }`, `session.value(key)` 는 model.json
  `session` key 의 값 (없으면 0 → Go 와 같이 `auth error`). 핸들러는 `Option<Session>` 으로 받는다 (토큰이 없거나 잘못되면 `None`,
  Go 의 nil Session). `is_admin(Option<&Session>, admin_level: Option<i64>)` 은 level ≥ adminLevel.
- 인증 (`auth.rs`, 사용자 파일): 기본은 `Authorization: Bearer <token>`, [통일 JWT](#통일-jwt-go--spring--rust) (HS256 만, 비밀 키 `JWT_SECRET`
  환경변수, 없으면 모두 비로그인 + 경고). id 는 `id`, 없으면 `sub` 를 숫자로 읽고, 0 이하면 비로그인이다. `exp` 는 있으면 검증한다.
  토큰 발급은 `support::auth::make_token(id, email, level)` (유효 시간 `token_ttl_secs()` = `JWT_EXPIRATION_MS` 밀리초, 기본 86400000 = 1일,
  잘못된 값은 경고 후 기본값. 비밀 키가 없으면 `None`) 또는 `token_with_key(...)`. `JWT_SECRET` 은 32 바이트 이상이어야 한다
  (`MIN_SECRET_LEN`, `check_secret`): 짧으면 에러를 한 번 찍고 비밀 키가 없는 것으로 본다 (모든 요청 비로그인). 이미 있는 auth.rs 는 바뀌지 않는다.
  다른 형식이 필요하면 이 파일의 `Claims` / `session_from_parts` 를 맞춘다. 추출기는 `support::auth::session_from_parts(&Parts) -> Option<Session>` (async) 만 부른다.
  기본 `auth.rs` 는 `id`, `email`, `level` 만 채우고 `Session.values` 는 비워 둔다. 그래서 model.json `session` 에 `id` / `level` 이 아닌 key
  (`{"key": "company", "column": "company"}` 등)가 있으면 `value("company")` 가 0 이라 그 테이블은 모두 `auth error` 다.
  그런 key 는 `Claims` 에 필드를 더하고 `values` 에 소문자 key 로 넣는다:
  ```rust
  pub struct Claims {
      pub id: i64,
      #[serde(default)]
      pub level: Option<i64>,
      #[serde(default)]
      pub company: Option<i64>, // 토큰의 claim 이름
  }
  // session_from_token 의 마지막 줄을 바꾼다
  let mut s = Session { id: data.claims.id, level: data.claims.level, ..Default::default() };
  if let Some(c) = data.claims.company {
      s.values.insert("company".to_string(), c);
  }
  Some(s)
  ```
  `exp` 는 있으면 만료를 검증하고 없어도 된다 (Go 토큰에 exp 가 없을 수 있다. 예전 기본값은 exp 필수였다, 이미 있는 auth.rs 는 바뀌지 않는다).
- 상태 (`state`): `AppState { pool: MySqlPool }`. 생성 경로는 `Router<S>` (where `MySqlPool: FromRef<S>`) 라서 상태를 직접 만들 때는
  `FromRef<내 상태> for MySqlPool` 만 구현하면 된다. `connect(url, max) / connect_lazy(url, max) / connect_options(url)` 은 세션
  `time_zone` 을 바꾸지 않는 풀 / 옵션이다 (위 [생성 파일](#생성-파일)).
- 비밀번호 (`password`): `generate_passwd(&str) -> Result<String, Error>` 는 `$2a$10$...` (Go `bcrypt.GenerateFromPassword(pw, 10)` 과
  같은 형식), 72 바이트를 넘으면 `Error::PasswordTooLong` (Go 도 에러다. 72 바이트는 된다. bcrypt crate 의 `non_truncating_*` 는
  72 바이트도 거부해서 쓰지 않는다). `generate_passwd_blocking(String)` 은 `spawn_blocking` 버전. `verify_passwd(pw, hash) -> bool` 은
  `$2a$` / `$2b$` / `$2y$` 를 받고 Go `CompareHashAndPassword` 와 같이 72 바이트 뒤는 보지 않는다. Go 가 만든 해시로 확인했다.
- 시간 (`clock`): `now()` 는 createddate 값 `yyyy-MM-dd HH:mm:ss`, Go `createdNow` 와 같은 `timezone` 규칙: 비어 있거나 `Asia/Seoul` 은
  UTC+9 (`chrono::FixedOffset`), `local` 은 서버 지역 시간, `UTC` 는 UTC, 그 밖의 IANA 이름은 `chrono-tz` 로 찾고 실행 중에
  찾지 못하면 UTC+9 (Go `time.LoadLocation` 실패와 같다). IANA 이름 모양이 아니면 생성할 때 경고하고 UTC+9.
  `local_now(fmt)` 는 Go 가 NOT NULL 날짜의 빈 값을 채우는 `time.Now().Format` 과 같다. `DATETIME_FORMAT` / `DATE_FORMAT` / `TIME_FORMAT`.
- 행 읽기 (`row`): 날짜는 `DATE_FORMAT(...)` 문자열로 읽는다. `date(Option<String>) -> String` 은 NULL 과 자리표시 값
  (`0000-00-00 00:00:00`, `1000-01-01 00:00:00`, `9999-01-01 00:00:00`, `0000-00-00`, `1000-01-01`)을 `""` 로, `date_opt` 는 `None` 으로.
  DECIMAL / FLOAT / DOUBLE / REAL 은 `CAST(... AS CHAR)` 로 읽어 `decimal` / `decimal_opt`. tinyint 와 `(bit<>0)` 은 sqlx `bool` 로 바로 읽는다 (0 이 아니면 true).
- serde (`serde_ext`): `bool_lenient` / `opt_bool_lenient` / `double_bool_lenient` (true/false, 숫자 0/그 밖, `"true"/"false"/"1"/"0"`),
  `parse_bool` (쿼리 파라미터), `bytes_b64` / `opt_bytes_b64` / `double_bytes_b64` (Go `[]byte` JSON 과 같은 표준 base64, null 은 빈 값),
  `double_option` (부분 수정: 없음 → `None`, null → `Some(None)`). `#[serde(default, with = "crate::support::serde_ext::...")]`.
- 에러 / 로그 (`error`): `Error::{Db, Password, PasswordTooLong, Message}` (`From<sqlx::Error>`), `IntoResponse` 는 로그 + HTTP 200
  `{"code":"error"}`. `log_sql(sql, params)` 는 target `sql` 의 debug 로그 (`RUST_LOG=info,sql=debug` 일 때 찍힌다. Go `config.Log.Database`).
