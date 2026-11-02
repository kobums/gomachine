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
