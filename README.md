# go machine

이 프로그램은 Go를 이용하여 귀찮고 반복되는 것들을 안하게 하기 위함이다.

아직 go, flutter만 사용하게 되어있다.

## 실행

1. config.json 파일을 작성한다.

2. make install

3. 경로를 잘 맞춰야 한다.

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
