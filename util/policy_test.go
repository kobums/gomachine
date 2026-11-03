package util

import (
	"gomachine/config"
	"testing"
)

func TestReadPolicy(t *testing.T) {
	sess := []config.SessionPair{{Key: "id", Column: "user"}}
	tests := []struct {
		model string
		gpa   *config.Gpa
		auth  string
		want  string
		err   bool
	}{
		{"Board", nil, "", ReadPublic, false},
		{"Board", &config.Gpa{Session: sess}, "", ReadPublic, false},
		{"Board", &config.Gpa{Read: "owner"}, "", ReadPublic, false},
		{"Board", nil, "jwt", ReadLogin, false},
		{"Board", &config.Gpa{Session: sess}, "jwt", ReadOwner, false},
		{"Board", &config.Gpa{Session: sess, Read: "login"}, "jwt", ReadLogin, false},
		{"Board", &config.Gpa{Session: sess, Read: " Public "}, "jwt", ReadPublic, false},
		{"Board", &config.Gpa{Read: "public"}, "jwt", ReadPublic, false},
		{"Board", &config.Gpa{Read: "owner"}, "jwt", "", true},
		{"Board", &config.Gpa{Read: "everyone"}, "jwt", "", true},
		{"Board", &config.Gpa{Read: "everyone"}, "", "", true},
		{"User", nil, "jwt", ReadSelf, false},
		{"User", &config.Gpa{Read: "public"}, "jwt", ReadSelf, false},
		{"User", &config.Gpa{Session: []config.SessionPair{{Key: "id", Column: "id"}}}, "jwt", ReadSelf, false},
		{"User", nil, "", ReadPublic, false},
	}
	for _, tt := range tests {
		got, err := ReadPolicy(tt.model, tt.gpa, tt.auth)
		if got != tt.want || (err != nil) != tt.err {
			t.Errorf("ReadPolicy(%s, %+v, %q) = %q, %v; want %q, err %v", tt.model, tt.gpa, tt.auth, got, err, tt.want, tt.err)
		}
	}
}

func TestLikePattern(t *testing.T) {
	for in, want := range map[string]string{"abc": "%abc%", "50%": "%50!%%", "a_b": "%a!_b%", "x!y": "%x!!y%", `a\b`: `%a\b%`, "": "%%"} {
		if got := LikePattern(in); got != want {
			t.Errorf("LikePattern(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilterCompare(t *testing.T) {
	gpa := &config.Gpa{Name: "t", Map: []config.GpaMap{{Name: "status", Data: []string{"a:A"}}},
		Compare: []config.GpaCompare{{Name: "code", Type: "="}, {Name: "cnt", Type: ">="}, {Name: "memo", Type: "Not  Like"}, {Name: "price", Type: "like"}, {Name: "nick_name", Type: "!="}}}
	col := func(name, typ string) Column {
		return NewColumnFromRaw("t_tb", RawColumn{Name: name, DataType: typ}, gpa, config.ModelConfig{Language: "go"})
	}
	tests := []struct {
		col  Column
		want string
	}{
		{col("t_title", "varchar"), "like"},
		{col("t_name", "varchar"), "like"},
		{col("t_content", "text"), "like"},
		{col("t_code", "char"), "="},
		{col("t_memo", "text"), "not like"},
		{col("t_nick_name", "varchar"), "<>"},
		{col("t_cnt", "int"), ">="},
		{col("t_price", "int"), "="}, // like 는 숫자에 쓸 수 없다
		{col("t_status", "int"), "="},
		{col("t_flag", "tinyint"), "="},
		{col("t_tm", "time"), "like"},
		{col("t_geo", "geometry"), "like"},
		{col("t_createddate", "datetime"), ""},
		{col("t_data", "blob"), ""},
	}
	for _, tt := range tests {
		if got := FilterCompare(tt.col, gpa); got != tt.want {
			t.Errorf("FilterCompare(%s) = %q, want %q", tt.col.Column, got, tt.want)
		}
	}
}

func TestDefaultLiteral(t *testing.T) {
	tests := []struct {
		typ, def string
		has      bool
		want     string
		ok       bool
	}{
		{"int", "1", true, "1", true},
		{"int", "'1'", true, "1", true},
		{"int", "NULL", true, "", false},
		{"int", "", false, "", false},
		{"int", "abc", true, "", false},
		{"decimal", "12.50", true, "12.50", true},
		{"tinyint", "0", true, "0", true},
		{"bit", "b'1'", true, "1", true},
		{"varchar", "'it''s'", true, "it's", true},
		{"varchar", "abc", true, "abc", true},
		{"varchar", "''", true, "", true},
		{"varchar", "uuid()", true, "", false},
		{"varchar", "'uuid()'", true, "uuid()", true},
		{"datetime", "current_timestamp()", true, "", false},
		{"date", "'2000-01-01'", true, "2000-01-01", true},
		{"blob", "'x'", true, "", false},
	}
	for _, tt := range tests {
		got, ok := DefaultLiteral(Column{OriginalType: tt.typ, HasDefault: tt.has, Default: tt.def})
		if got != tt.want || ok != tt.ok {
			t.Errorf("DefaultLiteral(%s %q) = %q, %v; want %q, %v", tt.typ, tt.def, got, ok, tt.want, tt.ok)
		}
	}
}
