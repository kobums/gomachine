# go machine

이 프로그램은 Go를 이용하여 귀찮고 반복되는 것들을 안하게 하기 위함이다.

go, spring(kotlin), flutter(dart), react, vue(Vue 3 + TypeScript) 코드를 생성할 수 있다. rust(axum + sqlx) 백엔드는 작업 중이다 ([Rust](#rust-axum--sqlx)).

## 실행

1. config.json 파일을 작성한다.

2. make install

3. 경로를 잘 맞춰야 한다.

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
- 실행 인자로 덮어쓸 수 있다: `buildtool-model . go,react` 또는 `buildtool-model . all`

## model.json 읽기

- 실행한 디렉터리의 `config/model.json` 을 읽는다 (`modelJsonPath` 가 있으면 그 디렉터리의 `model.json`). 첫 번째 실행 인자(경로)는 쓰지 않는다.
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
  (REST `Read` 는 에러면 `code: "error"`).
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
  `pagesize` 는 10000 을 넘지 않는다 (Spring 과 같다).
- 목록/개수 필터의 숫자/enum/bool 값을 해석하지 못하면 400 이다 (`?status=abc`, 예전에는 조건을 빼서 전체 행을 돌려줬다).
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
- `Insertbatch` 는 모든 행을 준비(해시 등)한 뒤 저장하고, `Deletebatch` 는 모든 행의 권한을 검사한 뒤 지운다.
  없는 행은 건너뛰고, 남의 행이 하나라도 있으면 아무것도 지우지 않는다 (`auth error`, Spring 은 403). 예전 Go 는 없는 행이 있으면
  `error` 로 멈췄다. 둘 다 트랜잭션은 아니다: 저장/삭제 중간에 DB 에러가 나면 앞의 행은 이미 반영되어 있다.
- 조인 `column` 은 접두사가 있어도(`"a_member"`) 없어도(`"member"`), API 이름(`"memberId"`)이어도 된다. 이 테이블에 없는 컬럼이면
  경고를 찍고 그 조인을 건너뛴다 (Spring / TypeScript / Dart 도 같다).
- Postgres 는 지원하지 않는다. 생성 Go 모델의 `Find/Count/UpdateWhere/GroupBy` 는 `$n` 자리표시를 쓰지만 `Get/GetRaw/Insert/Update/Delete`,
  커스텀 `UpdateXByY`, 소유권 검사 등은 `?` 를 쓴다. MySQL 만 대상으로 한다.

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

## Vue (Vue 3 + TypeScript)

`vueModelFilePath` 아래에 테이블마다 세 파일을 만든다.

- `src/types/{table}.ts` — 모델 인터페이스, 검색 파라미터, 응답 타입
- `src/models/{table}.ts` — API 호출 클래스 (`{Model}Model.find/get/insert/update/remove`, enum 상수, `searchByXxx`)
- `src/composables/use{Model}.ts` — `items`, `item`, `total`, `loading`, `error` ref 와 `fetchPage/fetchAll/fetchOne/create/update/remove`
  (view 는 `create/update/remove`, id 컬럼이 없는 테이블은 `fetchOne/update/remove` 가 없다)

대상 프로젝트에는 axios 같은 시그니처의 `src/services/api.ts` (`get`, `post`, `put`, `patch`, `del`) 가 있어야 한다.
API 경로와 응답 처리는 [HTTP 계약](#http-계약-go--spring--react--vue--dart)을 따르므로 Go 와 Spring 어느 백엔드에도 쓸 수 있다.
공통 정규화 코드 `src/models/contract.ts` 를 같이 생성한다 (테이블마다 덮어쓴다).

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
  소유자 컬럼이 NULL 인 행도 남의 행으로 본다 (`(owner <> ? or owner is null)`, 예전에는 NULL 소유자 행을 누구나 바꿀 수 있었다).
  세션 값은 컬럼의 Go 타입으로 바꿔 비교/대입한다 (`int` 컬럼이면 `int(c.Session.Id)`).
- REST Insert/Insertbatch 는 클라이언트가 보낸 `id` 를 무시한다 (모델의 `Insert` 는 그대로 id 지정 가능).
- `"protect": ["level", ...]` — REST Insert 는 0 값, Update 는 기존 DB 값으로 덮어써서 클라이언트가 못 바꾸게 한다.
  보호 컬럼을 바꾸는 커스텀 `UpdateXByY` 는 거부한다. User 테이블의 `level` 은 항상 보호한다
  (예전에는 `protect` 를 설정하면 `level` 이 빠졌다).
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
  응답(`{Model}Response`), 목록 필터, `/search/{field}` 에서 비밀번호 컬럼은 빠진다.
- Create/Update/Patch 요청 DTO 에는 `protect` 컬럼(기본: User 의 level, `protect` 를 설정해도 빠지지 않는다)과 `createddate` 가 없다.
  기본 키는 경로 변수로만 정한다. Spring 쪽에는 관리자 예외가 없다.
- `"auth": "jwt"` 면 **모든** 컨트롤러가 `{kotlinPackage}.config.SessionProvider` 를 주입받고, 모든 엔드포인트가
  `currentUserId()` 가 null 이면 401 이다 (Go 와 같다. 예전에는 `session` 이 없는 테이블이 모두 공개였다).
  - 예외: User 의 단건 생성(`POST /api/user`, 회원가입)은 로그인 없이 호출할 수 있다 (`POST /batch` 는 로그인 필요, Go 와 같다).
  - User 테이블에 id `session` 이 없으면 수정/삭제는 본인(`currentUserId()`) 행만 된다 (Go 와 같다, 남의 행은 404 / 일괄 삭제 403).
- `session` 이 있는 테이블은 목록/검색/개수를 본인 소유 행으로 거르고(DB where), 단건 조회/수정/삭제는 남의 행이면 404,
  일괄 삭제는 남의 행이 하나라도 있으면 403 (없는 행은 건너뛴다) 이다. 생성/수정 요청의 세션 컬럼은 세션 값으로 고정한다.
  - `config/SessionProvider.kt` 인터페이스는 파일이 없을 때만 생성한다(덮어쓰지 않음). 대상 프로젝트에서 `@Component` 로
    `currentUserId(): Long?` 를 구현해야 하고, `session` 의 key 가 `id` 가 아니면 `currentValue(key)` 도 구현한다.

### Spring 목록 / 수정 / 삭제

- 리포지토리는 `JpaSpecificationExecutor` 를 상속한다. `GET /api/{table}` 은 필터(같음, 날짜 `start`/`end` 범위, enum 코드)와
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
- 문자열 필터: Go 는 `name`/`title`/`content` 는 `=`(또는 `compare` 설정), 나머지는 `like`(와일드카드는 보낸 값 그대로), Spring 은 모두 `=`.

## Rust (axum + sqlx)

작업 중이다. 지금은 설정과 이름 / 타입 규칙만 있고 `rust` 를 켜면 테이블마다 "not implemented yet" 경고만 찍는다.

- `rustModelFilePath`: crate 루트 (`Cargo.toml` 이 있는 디렉터리). 생성 파일은 그 아래 `src/` 에 둔다. 모듈은 `crate::` 경로로 참조한다.
- Go 와 같은 HTTP 계약(HTTP 200 + `code`, 파라미터 해석 실패만 400)이고, 같은 DB / 계정(bcrypt `$2a$` cost 10, 같은 JWT)을 쓴다.
- `session` 컬럼은 Go 와 같이 정수 컬럼만 된다.
- 타입: `int`/`smallint`/`mediumint`/`year` → `i32`, `bigint` → `i64` (`column_type` 이 unsigned 면 `u32` / `u64`),
  `tinyint`/`bit` → `bool`, `float`/`double`/`decimal` → `f64`, 문자열 / 날짜 → `String`, blob / binary → `Vec<u8>`.
  enum(`map`) 컬럼은 newtype `pub struct Status(pub i32)` 이다 (코드는 다른 언어와 같다).
- 이름: 필드는 JSON key 의 snake_case (`createdAt` → `created_at`), Rust 예약어는 뒤에 `_` (`type_`, `self_`).
  struct / enum 이 생성 코드가 쓰는 타입(`Result`, `Option`, `Path`, `Json`, `Value`, `Error`, `Session` ...)과 같으면
  `Model` / `Enum` 접미사 (`result_tb` → `ResultModel`, enum `type` → `TypeEnum`). 모듈 / 파일은 Go 파일 이름과 같은 소문자이고
  예약어나 `mod`, `support`, `handlers`, `models`, `routes`, `custom` 이면 뒤에 `_` 다.
