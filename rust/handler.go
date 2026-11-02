package rustcodegen

import (
	"fmt"
	"gomachine/util"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// 핸들러 코드: Go rest.jet(컨트롤러) + go/main.go generateDomainRouter(라우터)와 같은 경로 / 파라미터 해석 / 권한 / 응답.

// route 는 경로 하나다.
type route struct {
	Method  string // get, post, put, patch, delete
	Path    string
	Handler string
}

// routeGroup 은 같은 경로의 메서드 묶음이다 (axum 은 경로마다 MethodRouter 하나).
type routeGroup struct {
	Path   string
	Chain  string // get(index).post(insert)
	shape  string
	method map[string]bool
}

// hgen 은 핸들러 파일 하나를 만든다.
type hgen struct {
	t      *table
	b      strings.Builder
	routes []route
}

const (
	failAuth  = "auth_error"
	failError = "error"
)

func q(s string) string { return util.RustString(s) }

func (g *hgen) w(format string, args ...interface{}) {
	fmt.Fprintf(&g.b, format, args...)
	g.b.WriteByte('\n')
}

func (g *hgen) route(method, path, handler string) {
	g.routes = append(g.routes, route{Method: method, Path: path, Handler: handler})
}

func ret(fail string) string {
	return "return Ok(ApiResult::" + fail + "().into());"
}

// sessVar / ownVar 는 session i 의 세션 값(i64) / 컬럼 타입 값 변수다.
func sessVar(s rustSession) string { return "sess_" + strconv.Itoa(s.Idx) }

func ownVar(s rustSession) string {
	if sessionConv(s.Col, "") == "" {
		return sessVar(s)
	}
	return "own_" + strconv.Itoa(s.Idx)
}

// ownValue 는 필드에 넣는 값이다 (null 허용이면 Some).
func ownValue(s rustSession) string {
	if s.Col.Nullable {
		return "Some(" + ownVar(s) + ")"
	}
	return ownVar(s)
}

// guard 는 jwt 세션 검사다 (Go: c.Session == nil → fail, session 값이 0 → fail).
// zero: 세션 값 0 검사 (Delete / Deletebatch 는 하지 않는다), sess: 세션 값 변수, own: 컬럼 타입 값 변수, useS: s 를 뒤에서 쓴다.
func (g *hgen) guard(fail string, zero, sess, own, useS bool) {
	t := g.t
	if !t.Jwt {
		return
	}
	needSess := len(t.Sessions) > 0 && (zero || sess || own)
	name := "_"
	if needSess || useS {
		name = "s"
	}
	g.w("    let Some(%s) = session else {", name)
	g.w("        %s", ret(fail))
	g.w("    };")
	if !needSess {
		return
	}
	for _, s := range t.Sessions {
		g.w("    let %s = s.value(%s);", sessVar(s), q(s.Key))
		if zero {
			g.w("    if %s == 0 {", sessVar(s))
			g.w("        %s", ret(fail))
			g.w("    }")
		}
		if own {
			if conv := sessionConv(s.Col, sessVar(s)); conv != "" {
				g.w("    let Ok(%s) = %s else {", ownVar(s), conv)
				g.w("        %s", ret(fail))
				g.w("    };")
			}
		}
	}
}

// sessionWheres 는 세션 조건(Go filters / 커스텀 조회의 models.Where{Column, Session 값, "="})이다.
func (g *hgen) sessionWheres() []string {
	ret := make([]string, 0)
	if !g.t.Jwt {
		return ret
	}
	for _, s := range g.t.Sessions {
		ret = append(ret, fmt.Sprintf("Where::eq(%s, %s).into()", q(s.Column), sessVar(s)))
	}
	return ret
}

// adminExpr 는 관리자인지 식이다 (s 가 있는 곳).
func (g *hgen) adminExpr() string {
	return "is_admin(Some(&s), Some(ADMIN_LEVEL))"
}

// idNeSession 은 id 가 세션 사용자 id 와 다른지 식이다 (User 본인 확인).
func (g *hgen) idNeSession(id string) string {
	if g.t.IdType == "u64" {
		return "u64::try_from(s.id).ok() != Some(" + id + ")"
	}
	return id + " != s.id"
}

// sessionParam 은 jwt 면 세션 인자다.
func (g *hgen) sessionParam() string {
	if g.t.Jwt {
		return ", session: Option<Session>"
	}
	return ""
}

func (g *hgen) sessionArg() string {
	if g.t.Jwt {
		return ", session"
	}
	return ""
}

const (
	stateParam = "State(pool): State<MySqlPool>"
	badReq     = "return Ok(bad_request());"
)

func isSearchColumn(c rustCol) bool {
	return c.Col.Name != "Id" && !util.IsPasswordColumn(c.Col.Name) && c.Base != "Vec<u8>" && !c.Col.Primary
}

// filterKind 는 Go Parse{X}Filters 의 분기와 같은 컬럼 분류다.
func filterKind(c rustCol) string {
	switch {
	case util.IsRangeFilter(c.Col.OriginalType):
		return "range"
	case c.Base == "String":
		return "string"
	case c.Base == "bool":
		return "bool"
	case c.Base == "i64" || c.Base == "u64":
		return "int64"
	case c.Base == "f64":
		return "float"
	}
	return "int"
}

// filters 는 parse_filters (Go Parse{X}Filters) 다.
func (g *hgen) filters() {
	t := g.t
	g.w("/// 목록 / 개수 조건 (Go rest.Parse%sFilters). 파라미터 이름은 JSON key, 날짜는 start / end 범위.", t.Name)
	g.w("/// 숫자 / enum / bool 을 해석하지 못하면 그 파라미터 이름을 Err 로 돌려준다 (라우터 400).")
	cols := make([]rustCol, 0)
	for _, c := range t.Cols {
		if isSearchColumn(c) {
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		g.w("pub fn parse_filters(_q: &QueryParams) -> Result<Vec<Arg>, &'static str> {")
		g.w("    Ok(Vec::new())")
		g.w("}")
		return
	}
	g.w("pub fn parse_filters(q: &QueryParams) -> Result<Vec<Arg>, &'static str> {")
	g.w("    let mut args: Vec<Arg> = Vec::new();")
	for _, c := range cols {
		key := c.Col.Key
		cmp := compareColumn(c.Col.Name, t.Compares)
		switch filterKind(c) {
		case "range":
			start, end := util.RangeParams(c.Col)
			g.w("    let (start, end) = (q.get(%s), q.get(%s));", q(start), q(end))
			g.w("    if !start.is_empty() && !end.is_empty() {")
			g.w("        args.push(Where::new(%s, Bind::pair(start, end), \"between\").into());", q(key))
			g.w("    } else if !start.is_empty() {")
			g.w("        args.push(Where::new(%s, start, \">=\").into());", q(key))
			g.w("    } else if !end.is_empty() {")
			g.w("        args.push(Where::new(%s, end, \"<=\").into());", q(key))
			g.w("    }")
		case "string":
			if c.Col.Name != "Name" && c.Col.Name != "Title" && c.Col.Name != "Content" {
				cmp = "like"
			}
			g.w("    let v = q.get(%s);", q(c.JSON))
			g.w("    if !v.is_empty() {")
			g.w("        args.push(Where::new(%s, v, %s).into());", q(key), q(cmp))
			g.w("    }")
		default:
			parse := map[string]string{"bool": "parse_bool", "float": "parse_float"}[filterKind(c)]
			if parse == "" {
				parse = "parse_int"
			}
			g.w("    let v = q.get(%s);", q(c.JSON))
			g.w("    if !v.is_empty() {")
			g.w("        let Some(v) = params::%s(v) else {", parse)
			g.w("            return Err(%s);", q(c.JSON))
			g.w("        };")
			push := fmt.Sprintf("args.push(Where::new(%s, v, %s).into());", q(key), q(cmp))
			switch {
			case filterKind(c) == "bool" || t.EnumZero[c.Col.Name]:
				// bool 과 코드 0 이 실제 값인 enum 은 보냈으면 항상 조건이다
				g.w("        %s", push)
			case filterKind(c) == "float":
				g.w("        if v != 0.0 {")
				g.w("            %s", push)
				g.w("        }")
			default:
				g.w("        if v != 0 {")
				g.w("            %s", push)
				g.w("        }")
			}
			g.w("    }")
		}
	}
	g.w("    Ok(args)")
	g.w("}")
}

func (g *hgen) index() {
	t := g.t
	g.route("get", "/"+t.Domain, "index")
	sw := g.sessionWheres()
	g.w("")
	g.w("/// GET /%s: 목록 (Go Index). page / pagesize 를 보내면 몇 번째 페이지든 total 도 돌려준다.", t.Domain)
	g.w("async fn index(%s%s, query: QueryArg) -> Reply {", stateParam, g.sessionParam())
	g.w("    let Some(q) = params::query_params(query) else {")
	g.w("        %s", badReq)
	g.w("    };")
	g.w("    let (Some(page), Some(pagesize)) = (params::parse_page(q.get(\"page\")), params::parse_page(q.get(\"pagesize\"))) else {")
	g.w("        %s", badReq)
	g.w("    };")
	g.w("    let Ok(mut args) = parse_filters(&q) else {")
	g.w("        %s", badReq)
	g.w("    };")
	g.w("    // page 0 은 전체 행, pagesize 는 10000 을 넘지 않는다 (Go / Spring 과 같다).")
	g.w("    let pagesize = pagesize.min(10000);")
	g.guard(failAuth, true, true, false, false)
	for _, w := range sw {
		g.w("    args.push(%s);", w)
	}
	g.w("    // 개수는 페이지 / 정렬 없이 같은 조건으로 센다.")
	g.w("    let count_args = args.clone();")
	g.w("    let paged = page != 0 && pagesize != 0;")
	g.w("    if paged {")
	g.w("        args.push(Arg::Paging { page, pagesize });")
	g.w("    }")
	g.w("    // orderby 는 이 테이블 컬럼 + asc/desc 만 남긴다 (모르는 항목은 버린다). 없으면 페이지일 때 %s desc.", t.OrderCol)
	g.w("    let orderby = %s::parse_order(q.get(\"orderby\"));", t.Model)
	g.w("    if !orderby.is_empty() {")
	g.w("        args.push(Arg::Ordering(orderby));")
	g.w("    }")
	g.w("    let items = %s::find(&pool, &args).await?;", t.Model)
	g.w("    let mut result = ApiResult::ok().set(\"items\", &items);")
	g.w("    if paged {")
	g.w("        result.insert(\"total\", %s::count(&pool, &count_args).await?);", t.Model)
	g.w("    }")
	g.w("    Ok(result.into())")
	g.w("}")
}

func (g *hgen) count() {
	t := g.t
	g.route("post", "/"+t.Domain+"/count", "count")
	g.route("get", "/"+t.Domain+"/count", "count")
	sw := g.sessionWheres()
	mut := ""
	if len(sw) > 0 {
		mut = "mut "
	}
	g.w("")
	g.w("/// GET|POST /%s/count: 개수 (Go Count). total 과 count 가 같다.", t.Domain)
	g.w("async fn count(%s%s, query: QueryArg) -> Reply {", stateParam, g.sessionParam())
	g.w("    let Some(q) = params::query_params(query) else {")
	g.w("        %s", badReq)
	g.w("    };")
	g.w("    let Ok(%sargs) = parse_filters(&q) else {", mut)
	g.w("        %s", badReq)
	g.w("    };")
	g.guard(failAuth, true, true, false, false)
	for _, w := range sw {
		g.w("    args.push(%s);", w)
	}
	g.w("    let total = %s::count(&pool, &args).await?;", t.Model)
	g.w("    Ok(ApiResult::ok().set(\"total\", total).set(\"count\", total).into())")
	g.w("}")
}

func (g *hgen) read() {
	t := g.t
	g.route("get", "/"+t.Domain+"/{id}", "read")
	owned := t.Jwt && len(t.Sessions) > 0
	g.w("")
	g.w("/// GET /%s/{id}: 한 행 (Go Read). 없으면 item: null%s.", t.Domain, map[bool]string{true: ", 세션이 있으면 없는 행은 error, 남의 행은 auth error"}[owned])
	g.w("async fn read(%s%s, id: IdArg) -> Reply {", stateParam, g.sessionParam())
	g.w("    let Some(id) = params::path_id::<%s>(id) else {", t.IdType)
	g.w("        %s", badReq)
	g.w("    };")
	g.guard(failAuth, true, owned, owned, false)
	g.w("    let item = %s::get(&pool, id).await?;", t.Model)
	if owned {
		g.w("    let Some(item) = item else {")
		g.w("        %s", ret(failError))
		g.w("    };")
		for _, s := range t.Sessions {
			g.w("    if item.%s != %s {", s.Col.Field, ownValue(s))
			g.w("        %s", ret(failAuth))
			g.w("    }")
		}
	}
	g.w("    Ok(ApiResult::ok().set(\"item\", &item).into())")
	g.w("}")
}

// searchParse 는 검색 값 v(&str)를 컬럼 타입으로 바꾸는 코드와 Bind 식이다 (Go searchParamCode, enum 은 코드).
func searchParse(c rustCol) (string, string) {
	switch {
	case c.Base == "String":
		return "", "Bind::from(v)"
	case c.Base == "bool":
		return "params::parse_bool(v)", "Bind::from(v)"
	case c.Base == "f64":
		return "params::parse_float(v)", "Bind::from(v)"
	}
	return "params::parse_int(v)", "Bind::from(v)"
}

func (g *hgen) search() {
	t := g.t
	cols := make([]rustCol, 0)
	for _, c := range t.Cols {
		if isSearchColumn(c) {
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		return
	}
	sw := g.sessionWheres()
	g.w("")
	g.w("/// GET /%s/search/{field}?{key}=값 (Go Search): 컬럼이 값과 같은 행 목록. 세션이 있으면 본인 소유 행만.", t.Domain)
	g.w("async fn search(pool: MySqlPool%s, column: &'static str, value: Bind) -> Reply {", g.sessionParam())
	g.guard(failAuth, true, true, false, false)
	args := append([]string{"Where::new(column, value, \"=\").into()"}, sw...)
	g.w("    let args: Vec<Arg> = vec![%s];", strings.Join(args, ", "))
	g.w("    let items = %s::find(&pool, &args).await?;", t.Model)
	g.w("    Ok(ApiResult::ok().set(\"items\", &items).into())")
	g.w("}")
	used := map[string]bool{}
	for _, c := range cols {
		seg := util.SearchSegment(c.Col)
		fn := util.DedupeIdent("search_"+util.RustIdent(strings.ToLower(util.SanitizeIdent(seg, "v"))), used)
		g.route("get", "/"+t.Domain+"/search/"+seg, fn)
		g.w("")
		g.w("async fn %s(%s%s, query: QueryArg) -> Reply {", fn, stateParam, g.sessionParam())
		g.w("    let Some(q) = params::query_params(query) else {")
		g.w("        %s", badReq)
		g.w("    };")
		g.w("    let v = q.get(%s);", q(c.JSON))
		g.w("    if v.is_empty() {")
		g.w("        %s", badReq)
		g.w("    }")
		parse, bind := searchParse(c)
		if parse != "" {
			g.w("    let Some(v) = %s else {", parse)
			g.w("        %s", badReq)
			g.w("    };")
		}
		g.w("    search(pool%s, %s, %s).await", g.sessionArg(), q(c.Col.Key), bind)
		g.w("}")
	}
}

// protectInsert 는 Insert 의 보호 컬럼 처리다 (Go: 관리자가 아니면 0 값).
func (g *hgen) protectInsert(item string, indent string, admin string) {
	t := g.t
	if len(t.Protects) == 0 {
		return
	}
	g.w("%s// 보호 컬럼은 클라이언트 값을 받지 않는다%s.", indent, map[bool]string{true: " (관리자 제외)"}[t.AdminCheck])
	inner := indent
	if t.AdminCheck {
		g.w("%sif !%s {", indent, admin)
		inner = indent + "    "
	}
	for _, c := range t.Protects {
		g.w("%s%s.%s = Default::default();", inner, item, c.Field)
	}
	if t.AdminCheck {
		g.w("%s}", indent)
	}
}

// prepareInsert 는 Insert / Insertbatch 의 준비다 (id, createddate, 보호 컬럼, 비밀번호).
func (g *hgen) prepareInsert(item string, indent string, admin string) {
	t := g.t
	if t.HasId {
		g.w("%s// id 는 자동 증가 값을 쓴다 (클라이언트가 보낸 id 는 무시한다).", indent)
		g.w("%s%s.id = 0;", indent, item)
	}
	if t.CreatedNow {
		g.w("%s// createddate 는 서버가 현재 시각으로 넣는다.", indent)
		if t.Created.Nullable {
			g.w("%s%s.%s = None;", indent, item, t.Created.Field)
		} else {
			g.w("%s%s.%s = String::new();", indent, item, t.Created.Field)
		}
	}
	g.protectInsert(item, indent, admin)
	if p := t.Password; p != nil {
		g.w("%s// 비밀번호는 해시해서 저장한다 (Go jwt.GeneratePasswd 와 같은 bcrypt).", indent)
		if p.Nullable {
			g.w("%sif let Some(pw) = %s.%s.clone().filter(|pw| !pw.is_empty()) {", indent, item, p.Field)
			g.w("%s    %s.%s = Some(password::generate_passwd_blocking(pw).await?);", indent, item, p.Field)
		} else {
			g.w("%sif !%s.%s.is_empty() {", indent, item, p.Field)
			g.w("%s    %s.%s = password::generate_passwd_blocking(%s.%s.clone()).await?;", indent, item, p.Field, item, p.Field)
		}
		g.w("%s}", indent)
	}
}

func (g *hgen) needsPrepare() bool {
	t := g.t
	return t.HasId || t.CreatedNow || len(t.Protects) > 0 || t.Password != nil
}

func (g *hgen) insert() {
	t := g.t
	g.route("post", "/"+t.Domain, "insert")
	user := t.isUser()
	withSession := t.Jwt && (!user || t.AdminCheck)
	sp := ""
	if withSession {
		sp = ", session: Option<Session>"
	}
	g.w("")
	g.w("/// POST /%s: 추가 (Go Insert). 응답 id 는 자동 증가 값이다.%s", t.Domain, map[bool]string{true: " 회원가입이라 로그인 없이 부를 수 있다."}[t.Jwt && user])
	g.w("async fn insert(%s%s, body: BodyArg) -> Reply {", stateParam, sp)
	g.w("    let Some((mut item, _)) = params::body_bytes(body).and_then(|b| %s::from_body(&b)) else {", t.Model)
	g.w("        %s", badReq)
	g.w("    };")
	admin := g.adminExpr()
	if t.Jwt && !user {
		g.guard(failError, true, false, true, t.AdminCheck)
		for _, s := range t.Sessions {
			g.w("    item.%s = %s;", s.Col.Field, ownValue(s))
		}
	} else if t.Jwt && user && t.AdminCheck {
		admin = "is_admin(session.as_ref(), Some(ADMIN_LEVEL))"
	}
	g.prepareInsert("item", "    ", admin)
	g.w("    let id = %s::insert(&pool, &mut item).await?;", t.Model)
	g.w("    Ok(ApiResult::ok().set(\"id\", id).into())")
	g.w("}")
}

func (g *hgen) insertBatch() {
	t := g.t
	g.route("post", "/"+t.Domain+"/batch", "insert_batch")
	user := t.isUser()
	g.w("")
	g.w("/// POST /%s/batch: 여러 행 추가 (Go Insertbatch). 모든 행을 준비(해시 등)한 다음 저장한다 (트랜잭션은 아니다).", t.Domain)
	g.w("async fn insert_batch(%s%s, body: BodyArg) -> Reply {", stateParam, g.sessionParam())
	g.w("    let Some(mut items) = params::body_bytes(body).and_then(|b| %s::list_from_body(&b)) else {", t.Model)
	g.w("        %s", badReq)
	g.w("    };")
	g.w("    if items.is_empty() {")
	g.w("        return Ok(ApiResult::ok().into());")
	g.w("    }")
	sessions := t.Jwt && !user && len(t.Sessions) > 0
	g.guard(failError, !user, false, sessions, t.AdminCheck)
	if t.AdminCheck {
		g.w("    let admin = %s;", g.adminExpr())
	}
	if sessions || g.needsPrepare() {
		g.w("    for item in items.iter_mut() {")
		if sessions {
			for _, s := range t.Sessions {
				g.w("        item.%s = %s;", s.Col.Field, ownValue(s))
			}
		}
		g.prepareInsert("item", "        ", "admin")
		g.w("    }")
	}
	g.w("    for item in items.iter_mut() {")
	g.w("        %s::insert(&pool, item).await?;", t.Model)
	g.w("    }")
	g.w("    Ok(ApiResult::ok().into())")
	g.w("}")
}

func (g *hgen) update() {
	t := g.t
	g.route("put", "/"+t.Domain, "update_body")
	g.route("put", "/"+t.Domain+"/{id}", "update_path")
	g.route("patch", "/"+t.Domain+"/{id}", "update_path")
	g.w("")
	g.w("/// PUT /%s: 부분 수정, id 는 본문 (Go Update).", t.Domain)
	g.w("async fn update_body(%s%s, body: BodyArg) -> Reply {", stateParam, g.sessionParam())
	g.w("    update(pool%s, None, body).await", g.sessionArg())
	g.w("}")
	g.w("")
	g.w("/// PUT|PATCH /%s/{id}: 부분 수정, 경로 id 가 본문 id 보다 우선한다.", t.Domain)
	g.w("async fn update_path(%s%s, id: IdArg, body: BodyArg) -> Reply {", stateParam, g.sessionParam())
	g.w("    let Some(id) = params::path_id::<%s>(id) else {", t.IdType)
	g.w("        %s", badReq)
	g.w("    };")
	g.w("    update(pool%s, Some(id), body).await", g.sessionArg())
	g.w("}")

	owned := t.Jwt && len(t.Sessions) > 0
	user := t.Jwt && t.isUser()
	adminUsed := t.AdminCheck
	existingUsed := t.Created != nil || len(t.Protects) > 0 || t.Password != nil
	g.w("")
	g.w("/// 본문에 있는 key 만 기존 행(GetRaw)에 덮어쓴다 (Go Update): 없는 key 는 DB 값 유지, null 허용 필드의 null 은 NULL 로 지운다.")
	g.w("/// 그다음 id, 세션 컬럼, createddate, 보호 컬럼, 비밀번호 규칙을 다시 적용하고 id 를 뺀 모든 컬럼을 저장한다.")
	g.w("async fn update(pool: MySqlPool%s, path_id: Option<%s>, body: BodyArg) -> Reply {", g.sessionParam(), t.IdType)
	g.w("    let Some((patch, keys)) = params::body_bytes(body).and_then(|b| %s::from_body(&b)) else {", t.Model)
	g.w("        %s", badReq)
	g.w("    };")
	g.w("    let id = path_id.unwrap_or(patch.id);")
	g.guard(failError, true, false, owned, user || adminUsed)
	if adminUsed {
		g.w("    let admin = %s;", g.adminExpr())
	}
	if user {
		g.w("    // 회원 정보는 본인만 수정할 수 있다%s.", map[bool]string{true: " (관리자 제외)"}[adminUsed])
		cond := g.idNeSession("id")
		if adminUsed {
			cond = "!admin && " + cond
		}
		g.w("    if %s {", cond)
		g.w("        %s", ret(failAuth))
		g.w("    }")
	}
	g.w("    let Some(existing) = %s::get_raw(&pool, id).await? else {", t.Model)
	g.w("        %s", ret(failError))
	g.w("    };")
	if owned {
		g.w("    // 소유권 검사: 기존 행이 현재 세션 사용자 것인지 확인한다 (NULL 소유자도 남의 행).")
		for _, s := range t.Sessions {
			g.w("    if existing.%s != %s {", s.Col.Field, ownValue(s))
			g.w("        %s", ret(failAuth))
			g.w("    }")
		}
	}
	if existingUsed {
		g.w("    let mut merged = existing.clone();")
	} else {
		g.w("    let mut merged = existing;")
	}
	g.w("    merged.merge(&patch, &keys);")
	g.w("    merged.id = id;")
	if owned {
		for _, s := range t.Sessions {
			g.w("    merged.%s = %s;", s.Col.Field, ownValue(s))
		}
	}
	if c := t.Created; c != nil {
		g.w("    // createddate 는 기존 값을 유지한다.")
		g.w("    merged.%s = existing.%s%s;", c.Field, c.Field, map[bool]string{false: ".clone()"}[c.Copy])
	}
	if len(t.Protects) > 0 {
		g.w("    // 보호 컬럼은 기존 값(NULL 포함)을 유지한다%s.", map[bool]string{true: " (관리자 제외)"}[adminUsed])
		indent := "    "
		if adminUsed {
			g.w("    if !admin {")
			indent = "        "
		}
		for _, c := range t.Protects {
			g.w("%smerged.%s = existing.%s%s;", indent, c.Field, c.Field, map[bool]string{false: ".clone()"}[c.Copy])
		}
		if adminUsed {
			g.w("    }")
		}
	}
	if p := t.Password; p != nil {
		g.w("    // 비밀번호를 보내지 않거나 비워 보내거나 기존 해시와 같으면 기존 해시를 유지한다.")
		if p.Nullable {
			g.w("    match merged.%s.clone().filter(|pw| !pw.is_empty() && Some(pw) != existing.%s.as_ref()) {", p.Field, p.Field)
			g.w("        Some(pw) => merged.%s = Some(password::generate_passwd_blocking(pw).await?),", p.Field)
			g.w("        None => merged.%s = existing.%s.clone(),", p.Field, p.Field)
			g.w("    }")
		} else {
			g.w("    if merged.%s.is_empty() || merged.%s == existing.%s {", p.Field, p.Field, p.Field)
			g.w("        merged.%s = existing.%s.clone();", p.Field, p.Field)
			g.w("    } else {")
			g.w("        merged.%s = password::generate_passwd_blocking(merged.%s.clone()).await?;", p.Field, p.Field)
			g.w("    }")
		}
	}
	g.w("    %s::update(&pool, &mut merged).await?;", t.Model)
	g.w("    Ok(ApiResult::ok().into())")
	g.w("}")
}

func (g *hgen) delete() {
	t := g.t
	g.route("delete", "/"+t.Domain, "delete_body")
	g.route("delete", "/"+t.Domain+"/{id}", "delete_path")
	g.w("")
	g.w("/// DELETE /%s: 삭제, id 는 본문 (Go Delete).", t.Domain)
	g.w("async fn delete_body(%s%s, body: BodyArg) -> Reply {", stateParam, g.sessionParam())
	g.w("    let Some((item, _)) = params::body_bytes(body).and_then(|b| %s::from_body(&b)) else {", t.Model)
	g.w("        %s", badReq)
	g.w("    };")
	g.w("    delete_one(pool%s, item.id).await", g.sessionArg())
	g.w("}")
	g.w("")
	g.w("/// DELETE /%s/{id}: 삭제.", t.Domain)
	g.w("async fn delete_path(%s%s, id: IdArg) -> Reply {", stateParam, g.sessionParam())
	g.w("    let Some(id) = params::path_id::<%s>(id) else {", t.IdType)
	g.w("        %s", badReq)
	g.w("    };")
	g.w("    delete_one(pool%s, id).await", g.sessionArg())
	g.w("}")

	owned := t.Jwt && len(t.Sessions) > 0
	user := t.Jwt && t.isUser()
	g.w("")
	g.w("async fn delete_one(pool: MySqlPool%s, id: %s) -> Reply {", g.sessionParam(), t.IdType)
	g.guard(failError, false, false, owned, user)
	if owned {
		g.w("    // 소유권 검사: 기존 행이 현재 세션 사용자 것인지 확인한다 (없는 행은 error).")
		g.w("    let Some(existing) = %s::get_raw(&pool, id).await? else {", t.Model)
		g.w("        %s", ret(failError))
		g.w("    };")
		for _, s := range t.Sessions {
			g.w("    if existing.%s != %s {", s.Col.Field, ownValue(s))
			g.w("        %s", ret(failAuth))
			g.w("    }")
		}
	}
	if user {
		g.userSelfCheck("id", "    ")
	}
	g.w("    %s::delete(&pool, id).await?;", t.Model)
	g.w("    Ok(ApiResult::ok().into())")
	g.w("}")
}

func (g *hgen) userSelfCheck(id string, indent string) {
	t := g.t
	g.w("%s// 회원 정보는 본인만 수정 / 삭제할 수 있다%s.", indent, map[bool]string{true: " (관리자 제외)"}[t.AdminCheck])
	cond := g.idNeSession(id)
	if t.AdminCheck {
		cond = "!" + g.adminExpr() + " && " + cond
	}
	g.w("%sif %s {", indent, cond)
	g.w("%s    %s", indent, ret(failAuth))
	g.w("%s}", indent)
}

func (g *hgen) deleteBatch() {
	t := g.t
	g.route("delete", "/"+t.Domain+"/batch", "delete_batch")
	owned := t.Jwt && len(t.Sessions) > 0
	user := t.Jwt && t.isUser()
	g.w("")
	g.w("/// DELETE /%s/batch: 본문 [{\"id\": 1}, ...] 삭제 (Go Deletebatch). 모든 행의 권한을 먼저 검사하고 (없는 행은 건너뛴다),", t.Domain)
	g.w("/// 남의 행이 하나라도 있으면 아무것도 지우지 않는다. 삭제는 트랜잭션이 아니다.")
	g.w("async fn delete_batch(%s%s, body: BodyArg) -> Reply {", stateParam, g.sessionParam())
	g.w("    let Some(items) = params::body_bytes(body).and_then(|b| %s::list_from_body(&b)) else {", t.Model)
	g.w("        %s", badReq)
	g.w("    };")
	g.guard(failError, false, false, owned, user)
	if owned || user {
		g.w("    for v in &items {")
		if owned {
			g.w("        let Some(existing) = %s::get_raw(&pool, v.id).await? else {", t.Model)
			g.w("            continue;")
			g.w("        };")
			for _, s := range t.Sessions {
				g.w("        if existing.%s != %s {", s.Col.Field, ownValue(s))
				g.w("            %s", ret(failAuth))
				g.w("        }")
			}
		}
		if user {
			g.userSelfCheck("v.id", "        ")
		}
		g.w("    }")
	}
	g.w("    for v in &items {")
	g.w("        %s::delete(&pool, v.id).await?;", t.Model)
	g.w("    }")
	g.w("    Ok(ApiResult::ok().into())")
	g.w("}")
}

// customRoute 는 커스텀 메서드의 경로와 경로 변수 이름이다 (Go routeFromFunc).
func customRoute(t *table, f rustFunc) (string, string, []string) {
	names := make([]string, 0)
	switch f.Kind {
	case "Get", "Find", "Count", "Delete":
		segment := strings.ToLower(f.Name[len(f.Kind)+2:])
		path := "/" + t.Domain + "/" + strings.ToLower(f.Kind) + "/" + segment
		for _, w := range f.Wheres {
			name := strings.ToLower(w.Column)
			if len(f.Wheres) == 1 {
				name = segment
			}
			path += "/{" + name + "}"
			names = append(names, name)
		}
		method := "get"
		if f.Kind == "Delete" {
			method = "delete"
		}
		return method, path, names
	case "Update":
		sets, wheres := "", ""
		for _, w := range f.Wheres {
			if w.Compare == "column" {
				sets += strings.ToLower(w.Column)
			} else {
				wheres += strings.ToLower(w.Column)
			}
			names = append(names, util.LowerCamel(w.Key))
		}
		return "put", "/" + t.Domain + "/" + sets + "/" + wheres, names
	}
	return "", "", nil
}

func hasCompare(f rustFunc, compare string) bool {
	for _, w := range f.Wheres {
		if w.Compare == compare {
			return true
		}
	}
	return false
}

func (g *hgen) custom(f rustFunc) {
	t := g.t
	if f.Kind == "Delete" && hasCompare(f, "like") {
		// like 로 지우는 경로는 만들지 않는다 (Go 와 같다). 모델 메서드는 있다.
		return
	}
	method, path, names := customRoute(t, f)
	if method == "" {
		return
	}
	fn := f.Fn
	g.route(method, path, fn)

	read := f.Kind == "Get" || f.Kind == "Find" || f.Kind == "Count"
	locked := false
	if f.Kind == "Update" {
		for _, w := range f.Wheres {
			if w.Compare == "column" {
				for _, p := range t.Protects {
					if p.Col.Name == w.Column {
						locked = true
					}
				}
			}
		}
	}
	noOwner := t.Jwt && t.isUser() && !t.HasId && (f.Kind == "Update" || f.Kind == "Delete")
	rejectAll := noOwner || (locked && !t.AdminCheck)
	owned := t.Jwt && len(t.Sessions) > 0

	// 세션 값으로 바꾸는 매개변수 (Go: 세션 컬럼과 같은 조건, in 제외)
	override := map[int]rustSession{}
	if owned {
		for _, s := range t.Sessions {
			for i, w := range f.Wheres {
				if s.Col.Col.Name == w.Column && !w.Slice {
					override[i] = s
				}
			}
		}
	}
	userCheck := !rejectAll && !read && t.Jwt && t.isUser() && t.HasId
	useS := userCheck || (locked && t.AdminCheck && !rejectAll)

	params := make([]string, 0)
	if !rejectAll {
		params = append(params, stateParam)
	}
	if t.Jwt {
		params = append(params, "session: Option<Session>")
	}
	if f.Kind == "Update" {
		params = append(params, "body: BodyArg")
	} else {
		params = append(params, "path: PathArg")
	}
	g.w("")
	g.w("/// %s %s (Go %s)", strings.ToUpper(method), path, f.Name)
	g.w("async fn %s(%s) -> Reply {", fn, strings.Join(params, ", "))
	if f.Kind == "Update" {
		g.w("    let Some(body) = params::body_bytes(body).and_then(|b| params::body_object(&b)) else {")
	} else {
		g.w("    let Some(path) = params::path_map(path) else {")
	}
	g.w("        %s", badReq)
	g.w("    };")
	for i, w := range f.Wheres {
		var expr string
		switch {
		case f.Kind == "Update":
			expr = fmt.Sprintf("params::body_value::<%s>(&body, %s)", w.PType, q(names[i]))
		case w.Slice:
			expr = fmt.Sprintf("params::path_list::<%s>(&path, %s)", w.Col.Base, q(names[i]))
		default:
			expr = fmt.Sprintf("params::path_value::<%s>(&path, %s)", w.PType, q(names[i]))
		}
		_, over := override[i]
		if rejectAll || over {
			g.w("    if %s.is_none() {", expr)
			g.w("        %s", badReq)
			g.w("    }")
		} else {
			mut := ""
			if f.Kind == "Update" && t.Password != nil && w.Compare == "column" && w.Column == t.Password.Col.Name {
				mut = "mut "
			}
			g.w("    let Some(%s%s) = %s else {", mut, w.Param, expr)
			g.w("        %s", badReq)
			g.w("    };")
		}
	}
	g.guard(failAuth, true, owned && !rejectAll, owned && len(override) > 0 && !rejectAll, useS)
	if rejectAll {
		if noOwner {
			g.w("    // id 컬럼이 없어 본인 행인지 확인할 수 없으므로 회원 정보 수정 / 삭제는 거부한다.")
		} else {
			g.w("    // 보호 컬럼은 클라이언트가 바꿀 수 없다.")
		}
		g.w("    Ok(ApiResult::auth_error().into())")
		g.w("}")
		return
	}
	for i, w := range f.Wheres {
		if s, ok := override[i]; ok {
			g.w("    let %s = %s;", w.Param, ownVar(s))
		}
	}
	if locked {
		g.w("    // 보호 컬럼은 관리자만 바꿀 수 있다.")
		g.w("    if !%s {", g.adminExpr())
		g.w("        %s", ret(failAuth))
		g.w("    }")
	}
	args := make([]string, 0)
	for _, w := range f.Wheres {
		args = append(args, w.Param)
	}
	if read {
		sw := g.sessionWheres()
		g.w("    let item = %s::%s(&pool, %s, &[%s]).await?;", t.Model, f.Fn, strings.Join(args, ", "), strings.Join(sw, ", "))
		key := map[string]string{"Get": "item", "Find": "items", "Count": "count"}[f.Kind]
		ref := "&"
		if f.Kind == "Count" {
			ref = ""
		}
		g.w("    Ok(ApiResult::ok().set(%s, %sitem).into())", q(key), ref)
		g.w("}")
		return
	}
	// 소유권 검사 조건 (Go: Delete 는 모든 조건, Update 는 where 조건)
	conds := make([]string, 0)
	for _, w := range f.Wheres {
		if f.Kind == "Delete" || w.Compare == "where" {
			cmp := w.Compare
			if cmp == "where" {
				cmp = "="
			}
			val := w.Param
			if !w.Col.Copy || w.Slice {
				val += ".clone()"
			}
			if w.Slice {
				val = "Bind::list(" + val + ")"
			}
			conds = append(conds, fmt.Sprintf("Where::new(%s, %s, %s).into()", q(w.Key), val, q(cmp)))
		}
	}
	if owned {
		g.w("    // 소유권 검사: 조건에 맞는 행 중 다른 사용자(또는 NULL) 소유 행이 있으면 거부한다.")
		for _, s := range t.Sessions {
			c := append(append([]string{}, conds...), fmt.Sprintf("Custom::new(%s, vec![%s.into()]).into()", q("("+s.SQL+" <> ? or "+s.SQL+" is null)"), sessVar(s)))
			g.w("    if %s::count(&pool, &[%s]).await? > 0 {", t.Model, strings.Join(c, ", "))
			g.w("        %s", ret(failAuth))
			g.w("    }")
		}
	}
	if userCheck {
		g.w("    // 회원 정보는 본인만 수정 / 삭제할 수 있다%s.", map[bool]string{true: " (관리자 제외)"}[t.AdminCheck])
		c := append(append([]string{}, conds...), fmt.Sprintf("Where::new(%s, s.id, \"<>\").into()", q(t.OrderKey)))
		cond := fmt.Sprintf("%s::count(&pool, &[%s]).await? > 0", t.Model, strings.Join(c, ", "))
		if t.AdminCheck {
			cond = "!" + g.adminExpr() + " && " + cond
		}
		g.w("    if %s {", cond)
		g.w("        %s", ret(failAuth))
		g.w("    }")
	}
	if f.Kind == "Update" && t.Password != nil {
		for _, w := range f.Wheres {
			if w.Compare == "column" && w.Column == t.Password.Col.Name {
				g.w("    // 비밀번호는 해시해서 저장한다. 빈 비밀번호는 받지 않는다.")
				g.w("    if %s.is_empty() {", w.Param)
				g.w("        return Ok(ApiResult::with_code(CODE_BAD_REQUEST).into());")
				g.w("    }")
				g.w("    %s = password::generate_passwd_blocking(%s).await?;", w.Param, w.Param)
			}
		}
	}
	g.w("    %s::%s(&pool, %s).await?;", t.Model, f.Fn, strings.Join(args, ", "))
	g.w("    Ok(ApiResult::ok().into())")
	g.w("}")
}

// buildHandlers 는 핸들러 본문과 경로 묶음이다.
func buildHandlers(t *table) (string, []routeGroup) {
	g := &hgen{t: t}
	g.filters()
	g.index()
	if t.HasId {
		g.read()
	}
	if !t.IsView {
		g.insert()
		g.insertBatch()
	}
	g.count()
	g.search()
	if !t.IsView && t.HasId {
		g.update()
		g.delete()
		g.deleteBatch()
	}
	for _, f := range t.Funcs {
		g.custom(f)
	}
	return g.b.String(), groupRoutes(t.TableName, g.routes)
}

// shape 는 경로 변수 이름을 뺀 모양이다 (/t/{id} → /t/{}). axum 은 같은 모양에 다른 변수 이름이면 시작할 때 panic 한다.
func shape(path string) string {
	var b strings.Builder
	in := false
	for _, r := range path {
		switch {
		case r == '{':
			in = true
			b.WriteString("{}")
		case r == '}':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// groupRoutes 는 같은 경로의 메서드를 묶는다 (등록 순서 유지). 같은 경로 / 메서드가 또 나오면(같은 이름의 커스텀 메서드) 경고하고 건너뛴다.
func groupRoutes(tableName string, routes []route) []routeGroup {
	groups := make([]routeGroup, 0)
	idx := map[string]int{}
	for _, r := range routes {
		sh := shape(r.Path)
		i, ok := idx[sh]
		if !ok {
			groups = append(groups, routeGroup{Path: r.Path, Chain: r.Method + "(" + r.Handler + ")", shape: sh, method: map[string]bool{r.Method: true}})
			idx[sh] = len(groups) - 1
			continue
		}
		g := &groups[i]
		if g.Path != r.Path || g.method[r.Method] {
			log.Printf("WARNING: rust: %s route %s %s conflicts with %s, skipped", tableName, strings.ToUpper(r.Method), r.Path, g.Path)
			continue
		}
		g.method[r.Method] = true
		g.Chain += "." + r.Method + "(" + r.Handler + ")"
	}
	return groups
}
