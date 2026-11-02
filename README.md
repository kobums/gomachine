# go machine

이 프로그램은 Go를 이용하여 귀찮고 반복되는 것들을 안하게 하기 위함이다.

go, spring(kotlin), flutter(dart), react, vue(Vue 3 + TypeScript) 코드를 생성할 수 있다.

## 실행

1. config.json 파일을 작성한다.

2. make install

3. 경로를 잘 맞춰야 한다.

## 생성 언어 선택

`model.json` 의 `languages` 에서 언어별로 켜고 끈다. `"all": true` 면 전부 생성한다.

```json
"languages": { "all": false, "go": true, "spring": true, "flutter": false, "react": true, "vue": true },
"kotlinModelFilePath": "../proj/proj_spring",
"reactModelFilePath": "../proj/proj_front",
"vueModelFilePath": "../proj/proj_vue"
```

- 지원 언어: `go`, `spring`(=`kotlin`), `flutter`(=`dart`), `react`, `vue`
- go 외 언어는 `*ModelFilePath` 가 비어 있으면 경고만 찍고 건너뛴다.
- `languages` 가 없으면 예전 `language` 문자열을 쓰고, 그것도 없으면 go 만 생성한다.
- 실행 인자로 덮어쓸 수 있다: `buildtool-model . go,react` 또는 `buildtool-model . all`

## Vue (Vue 3 + TypeScript)

`vueModelFilePath` 아래에 테이블마다 세 파일을 만든다.

- `src/types/{table}.ts` — 모델 인터페이스, 검색 파라미터, 응답 타입
- `src/models/{table}.ts` — API 호출 클래스 (`{Model}Model.find/get/insert/update/remove`, enum 상수, `searchByXxx`)
- `src/composables/use{Model}.ts` — `items`, `item`, `total`, `loading`, `error` ref 와 `fetchPage/fetchAll/fetchOne/create/update/remove`

대상 프로젝트에는 axios 같은 시그니처의 `src/services/api.ts` (`get`, `post`, `put`, `patch`, `del`) 가 있어야 한다.
API 경로는 react 와 같이 spring 백엔드(`/{table}/{id}`, `/{table}/search/{field}`)에 맞춰져 있다.

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

## 인증 / 권한 (model.json `auth`, `table[].session`, `table[].protect`)

### Go

- `"auth": "jwt"` 이면 생성되는 모든 핸들러(Read, Index, Count, Insert, Insertbatch, Update, Delete, Deletebatch,
  커스텀 메서드)가 `c.Session == nil` 일 때 `auth error`/`error` 를 돌려준다. `session` 설정이 없어도 마찬가지다.
  - 예외: User 테이블의 `Insert` 는 회원가입용이라 세션 없이 호출할 수 있다 (Insertbatch 는 세션 필요).
  - User 테이블의 Update/Delete/Deletebatch/UpdateXByY/DeleteBy 는 본인(`c.Session.Id`) 행만 바꿀 수 있다.
- `session` 이 있으면 목록/개수/커스텀 Get·Find·Count 조회에 세션 컬럼 조건을 항상 붙이고, Read/Update/Delete 와
  커스텀 UpdateXByY/DeleteBy 는 다른 사용자 소유 행이 걸리면 `auth error` 로 거부한다. 없는 행을 Read 하면 `error`.
- REST Insert/Insertbatch 는 클라이언트가 보낸 `id` 를 무시한다 (모델의 `Insert` 는 그대로 id 지정 가능).
- `"protect": ["level", ...]` — REST Insert 는 0 값, Update 는 기존 DB 값으로 덮어써서 클라이언트가 못 바꾸게 한다.
  보호 컬럼을 바꾸는 커스텀 `UpdateXByY` 는 거부한다. `protect` 가 없으면 User 테이블의 `level` 을 기본으로 보호한다.
  id 와 session 컬럼은 따로 처리하므로 protect 에 넣어도 무시된다.
  - `auth: "jwt"` 이고 `adminLevel` 이 숫자면, 세션 구조체에 정수형 `Level` 필드가 있고 그 값이 `adminLevel` 이상인
    관리자는 보호 컬럼을 바꿀 수 있고 다른 회원 정보도 수정/삭제할 수 있다 (reflect 로 확인하므로 `Level` 이 없으면 항상 보호).
- 비밀번호 컬럼은 이름이 `Passwd` 또는 `Password` 인 컬럼이다.
  - 출력용 모델 `{Name}` 에서는 `json:"-"` 로 숨기고, 입력용 `{Name}Update` 구조체(json 태그 있음)로 Insert/Update 를 받는다.
    그래서 Read/Index/커스텀 Get·Find 응답에는 비밀번호가 나가지 않고, 검색 파라미터에서도 빠진다.
  - REST Insert/Update 에서 `jwt.GeneratePasswd` 로 해시하고, 해시가 실패하면 저장하지 않고 에러를 돌려준다.
  - Update 에서 비밀번호를 비워 보내면 기존 해시를 유지한다. `createddate` 는 항상 기존 값을 유지한다.
- 대상 프로젝트 요구사항: 비밀번호 컬럼이 있는 테이블은 `{package}/global/jwt.GeneratePasswd(string) (string, error)` 가 있어야 한다
  (예전에는 User 테이블만 필요했다).

### Spring (Kotlin)

- `Passwd`/`Password` 컬럼이 있으면 서비스에 `PasswordEncoder` 를 주입한다 (대상 프로젝트에 `PasswordEncoder` 빈 필요).
  생성/수정 시 인코딩하고, PUT 에서 비워 보내면 기존 해시 유지, PATCH 는 값이 있을 때만 인코딩한다.
  응답(`{Model}Response`), 목록 필터, `/search/{field}`, 리포지토리 `findBy` 에서 비밀번호 컬럼은 빠진다.
- Create/Update/Patch 요청 DTO 에는 `protect` 컬럼(기본: User 의 level)이 없다. id 는 경로 변수로만 정한다.
  Spring 쪽에는 관리자 예외가 없다.
- `"auth": "jwt"` 이고 `session` 이 있는 테이블은 컨트롤러가 `com.gowoobro.gymspring.config.SessionProvider` 를 주입받아
  목록/검색/개수를 본인 소유 행으로 거르고, 단건 조회/수정/삭제는 남의 행이면 404(일괄 삭제는 403), 비로그인이면 401 을 돌려준다.
  생성/수정 요청의 세션 컬럼은 세션 값으로 고정한다. User 의 생성(회원가입)은 세션 없이 호출할 수 있다.
  - `config/SessionProvider.kt` 인터페이스는 파일이 없을 때만 생성한다(덮어쓰지 않음). 대상 프로젝트에서 `@Component` 로
    `currentUserId(): Long?` 를 구현해야 하고, `session` 의 key 가 `id` 가 아니면 `currentValue(key)` 도 구현한다.
  - 소유권 필터는 전체 조회 후 메모리에서 거른다. 큰 테이블은 리포지토리 쿼리로 바꾸는 것이 좋다.
