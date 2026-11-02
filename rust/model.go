package rustcodegen

import (
	"fmt"
	"gomachine/util"
	"strconv"
	"strings"
)

// 모델 템플릿(model.jet) 변수: Go model.jet 과 같은 SQL 을 만든다.

func joinQuoted(list []string) string {
	ret := make([]string, 0, len(list))
	for _, s := range list {
		ret = append(ret, util.RustString(s))
	}
	return strings.Join(ret, ", ")
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

func columnNames(cols []rustCol) string {
	names := make([]string, 0, len(cols))
	for _, c := range cols {
		names = append(names, c.Col.Column)
	}
	return strings.Join(names, ", ")
}

func writeValues(cols []rustCol) []string {
	ret := make([]string, 0, len(cols))
	for _, c := range cols {
		ret = append(ret, writeExpr(c, "item"))
	}
	return ret
}

// modelVars 는 model.jet 변수다.
func modelVars(t *table) map[string]interface{} {
	keys := make([]string, 0)
	nullable := make([]string, 0)
	decodes := make([]string, 0)
	for i, c := range t.Cols {
		keys = append(keys, c.JSON)
		if c.Nullable {
			nullable = append(nullable, c.JSON)
		}
		_, kind := selectExpr(c.Col, c.Base, c.Enum != "", c.Col.Column)
		decodes = append(decodes, decode(c.Base, c.Nullable, kind, c.Enum, strconv.Itoa(i)))
	}

	// 정렬 이름 → 컬럼 (같은 컬럼의 이름은 한 arm 으로)
	arms := make([][2]string, 0)
	armIdx := map[string]int{}
	for _, oc := range t.OrderCols {
		if i, ok := armIdx[oc[1]]; ok {
			arms[i][0] += " | " + util.RustString(oc[0])
			continue
		}
		armIdx[oc[1]] = len(arms)
		arms = append(arms, [2]string{util.RustString(oc[0]), util.RustString(oc[1])})
	}

	v := map[string]interface{}{
		"t":            t,
		"keys":         joinQuoted(keys),
		"nullableKeys": joinQuoted(nullable),
		"decodes":      decodes,
		"orderArms":    arms,
		"funcs":        funcsCode(t),
		"joinFns":      joinFnsCode(t),
		"createdFill":  "",
	}
	if t.HasId {
		v["getSQL"] = t.Select + " where 1=1 and " + t.Qual + t.Prefix + "_id = ?"
		v["getRawSQL"] = t.RawSelect + " where " + t.Prefix + "_id = ?"
		v["deleteSQL"] = "delete from " + t.TableName + " where " + t.Prefix + "_id = ?"
		sets := make([]string, 0)
		for _, c := range t.NonId {
			sets = append(sets, c.Col.Column+" = ?")
		}
		v["updateSQL"] = "update " + t.TableName + " set " + strings.Join(sets, ", ") + " where " + t.Prefix + "_id = ?"
		v["updateValues"] = strings.Join(append(writeValues(t.NonId), "Bind::from(item.id)"), ", ")
		v["insertIdSQL"] = fmt.Sprintf("insert into %s (%s) values (%s)", t.TableName, columnNames(t.Cols), placeholders(len(t.Cols)))
		v["insertIdValues"] = strings.Join(writeValues(t.Cols), ", ")
	}
	insertCols := t.Cols
	if t.HasId {
		insertCols = t.NonId
	}
	v["insertSQL"] = fmt.Sprintf("insert into %s (%s) values (%s)", t.TableName, columnNames(insertCols), placeholders(len(insertCols)))
	v["insertValues"] = strings.Join(writeValues(insertCols), ", ")
	if t.CreatedNow {
		f := t.Created.Field
		if t.Created.Nullable {
			v["createdFill"] = fmt.Sprintf("if item.%s.as_deref().unwrap_or(\"\").is_empty() {\n            item.%s = Some(clock::now());\n        }", f, f)
		} else {
			v["createdFill"] = fmt.Sprintf("if item.%s.is_empty() {\n            item.%s = clock::now();\n        }", f, f)
		}
	}
	return v
}

// funcsCode 는 커스텀 메서드(Go GetBy/FindBy/CountBy/DeleteBy/UpdateXByY)다.
func funcsCode(t *table) string {
	var b strings.Builder
	w := func(format string, args ...interface{}) {
		fmt.Fprintf(&b, format, args...)
		b.WriteByte('\n')
	}
	for _, f := range t.Funcs {
		params := make([]string, 0)
		args := make([]string, 0)
		for _, wh := range f.Wheres {
			params = append(params, wh.Param+": "+wh.PType)
			args = append(args, wh.Param)
		}
		where := func(wh rustWhere, cmp string) string {
			val := wh.Param
			if wh.Slice {
				val = "Bind::list(" + val + ")"
			}
			return fmt.Sprintf("Where::new(%s, %s, %s)", util.RustString(wh.Key), val, util.RustString(cmp))
		}
		switch f.Kind {
		case "Get", "Find", "Count":
			w("")
			w("    /// %s 의 조건 (Go): args 다음에 붙이고, 값이 없는(0 / \"\") 매개변수는 뺀다 (bool / in / 코드 0 이 실제 값인 enum 은 항상).", f.Name)
			w("    pub fn %s_args(%s, args: &[Arg]) -> Vec<Arg> {", f.Fn, strings.Join(params, ", "))
			w("        let mut rets = args.to_vec();")
			for _, wh := range f.Wheres {
				push := "rets.push(" + where(wh, wh.Compare) + ".into());"
				if wh.Check != "" {
					w("        if %s {", wh.Check)
					w("            %s", push)
					w("        }")
				} else {
					w("        %s", push)
				}
			}
			w("        rets")
			w("    }")
			w("")
			ret := map[string]string{"Get": "Option<Self>", "Find": "Vec<Self>", "Count": "i64"}[f.Kind]
			w("    pub async fn %s(pool: &MySqlPool, %s, args: &[Arg]) -> Result<%s, Error> {", f.Fn, strings.Join(params, ", "), ret)
			w("        let rets = Self::%s_args(%s, args);", f.Fn, strings.Join(args, ", "))
			switch f.Kind {
			case "Get":
				w("        Ok(Self::find(pool, &rets).await?.into_iter().next())")
			case "Find":
				w("        Self::find(pool, &rets).await")
			default:
				w("        Self::count(pool, &rets).await")
			}
			w("    }")
		case "Delete":
			w("")
			w("    /// %s (Go): 조건은 Find 와 같은 Where 규칙 (in / like).", f.Name)
			w("    pub fn %s_query(%s) -> QueryBuilder<MySql> {", f.Fn, strings.Join(params, ", "))
			w("        let mut qb = QueryBuilder::<MySql>::new(%s);", util.RustString("delete from "+t.TableName+" where 1=1"))
			for _, wh := range f.Wheres {
				w("        query::push_where(&mut qb, WHERE_PREFIX, &%s);", where(wh, wh.Compare))
			}
			w("        qb")
			w("    }")
			w("")
			w("    pub async fn %s(pool: &MySqlPool, %s) -> Result<(), Error> {", f.Fn, strings.Join(params, ", "))
			w("        let mut qb = Self::%s_query(%s);", f.Fn, strings.Join(args, ", "))
			w("        log_query(&qb);")
			w("        qb.build().execute(pool).await?;")
			w("        Ok(())")
			w("    }")
		case "Update":
			sets := make([]string, 0)
			conds := ""
			values := make([]string, 0)
			for _, wh := range f.Wheres {
				if wh.Compare == "column" {
					sets = append(sets, t.Prefix+"_"+wh.Key+" = ?")
					values = append(values, paramWriteExpr(wh))
				}
			}
			for _, wh := range f.Wheres {
				if wh.Compare == "where" {
					conds += " and " + t.Prefix + "_" + wh.Key + " = ?"
					values = append(values, "Bind::from("+wh.Param+")")
				}
			}
			sql := "update " + t.TableName + " set " + strings.Join(sets, ", ") + " where 1=1" + conds
			w("")
			w("    /// %s (Go).", f.Name)
			w("    pub fn %s_query(%s) -> QueryBuilder<MySql> {", f.Fn, strings.Join(params, ", "))
			w("        query::bound(%s, &[%s])", util.RustString(sql), strings.Join(values, ", "))
			w("    }")
			w("")
			w("    pub async fn %s(pool: &MySqlPool, %s) -> Result<(), Error> {", f.Fn, strings.Join(params, ", "))
			w("        let mut qb = Self::%s_query(%s);", f.Fn, strings.Join(args, ", "))
			w("        log_query(&qb);")
			w("        qb.build().execute(pool).await?;")
			w("        Ok(())")
			w("    }")
		}
	}
	return b.String()
}

// joinFnsCode 는 조인 행 읽기 함수다. 값은 조인 테이블 모델과 같은 JSON (비밀번호 제외, extra 는 enum 표시 이름).
func joinFnsCode(t *table) string {
	var b strings.Builder
	w := func(format string, args ...interface{}) {
		fmt.Fprintf(&b, format, args...)
		b.WriteByte('\n')
	}
	for _, j := range t.Joins {
		w("")
		w("/// 조인 %s (left join): 조인 행이 없으면 null. NOT NULL 컬럼의 NULL 은 0 값 (Go 와 같다).", j.Var)
		w("fn %s(r: &MySqlRow) -> Result<Value, sqlx::Error> {", j.Fn)
		idVar := ""
		for i, c := range j.Cols {
			if c.Hidden {
				continue
			}
			idx := strconv.Itoa(j.Offset + i)
			v := "v" + strconv.Itoa(i)
			w("    let %s = %s;", v, decode(c.Base, true, c.Kind, "", idx))
			if c.IsId && idVar == "" {
				idVar = v
			}
		}
		if idVar != "" {
			w("    if %s.is_none() {", idVar)
			w("        return Ok(Value::Null);")
			w("    }")
		}
		w("    let mut m = Map::new();")
		labels := make([]string, 0)
		for i, c := range j.Cols {
			if c.Hidden {
				continue
			}
			v := "v" + strconv.Itoa(i)
			var val string
			switch {
			case c.Base == "Vec<u8>" && c.Nullable:
				val = v + ".as_deref().map(serde_ext::b64)"
			case c.Base == "Vec<u8>":
				val = "serde_ext::b64(&" + v + ".unwrap_or_default())"
			case c.Nullable:
				val = v
			default:
				val = v + ".unwrap_or_default()"
			}
			w("    m.insert(%s.into(), json!(%s));", util.RustString(c.JSON), val)
			if c.EnumKey != "" {
				label := "row::label(" + c.Labels + ", " + v + ".unwrap_or_default())"
				if c.Nullable {
					label = v + ".map(|code| row::label(" + c.Labels + ", code)).unwrap_or(\"\")"
				}
				labels = append(labels, fmt.Sprintf("    extra.insert(%s.into(), json!(%s));", util.RustString(c.EnumKey), label))
			}
		}
		if len(labels) > 0 {
			w("    let mut extra = Map::new();")
			for _, l := range labels {
				w("%s", l)
			}
		} else {
			w("    let extra = Map::new();")
		}
		w("    m.insert(\"extra\".into(), Value::Object(extra));")
		w("    Ok(Value::Object(m))")
		w("}")
	}
	return b.String()
}
