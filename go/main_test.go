package gocodegen

import (
	"gomachine/util"
	"strings"
	"testing"
)

func TestDateFillExpr(t *testing.T) {
	cases := []struct {
		col  util.Column
		want string
	}{
		{util.Column{OriginalType: "datetime"}, `"1000-01-01 00:00:00"`},
		{util.Column{OriginalType: "date"}, `"1000-01-01"`},
		{util.Column{OriginalType: "time"}, `"00:00:00"`},
		{util.Column{OriginalType: "timestamp"}, `time.Now().Format("2006-01-02 15:04:05")`},
		{util.Column{OriginalType: "datetime", HasDefault: true, Default: "CURRENT_TIMESTAMP"}, `time.Now().Format("2006-01-02 15:04:05")`},
		{util.Column{OriginalType: "date", HasDefault: true, Default: "curdate()"}, `time.Now().Format("2006-01-02")`},
		{util.Column{OriginalType: "date", HasDefault: true, Default: "'2000-01-01'"}, `"2000-01-01"`},
	}
	for _, c := range cases {
		if got := dateFillExpr(c.col); got != c.want {
			t.Errorf("dateFillExpr(%+v) = %s, want %s", c.col, got, c.want)
		}
	}
}

func TestScanCode(t *testing.T) {
	s := newScanCode()
	s.add("item", "n_Id", util.Column{Name: "Id", Type: "int64"}, "x")
	s.add("item", "n_Memo", util.Column{Name: "Memo", Type: "string", Nullable: true}, "x")
	s.add("item", "n_Cnt", util.Column{Name: "Cnt", Type: "int", Nullable: true}, "x")
	s.add("item", "n_Status", util.Column{Name: "Status", Type: "x.Status", Nullable: true}, "x")
	s.add("item", "n_Data", util.Column{Name: "Data", Type: "[]byte", Nullable: true}, "x")
	s.add("_j", "n_j_Kind", util.Column{Name: "Kind", Type: "y.Kind", Nullable: true}, "")

	if got := strings.Join(s.targets, ", "); got != "&item.Id, &n_Memo, &n_Cnt, &n_Status, &item.Data, &_j.Kind" {
		t.Errorf("targets = %s", got)
	}
	for _, want := range []string{"item.Memo = n_Memo.String", "item.Cnt = int(n_Cnt.Int64)", "item.Status = x.Status(n_Status.Int64)"} {
		if !strings.Contains(s.assigns.String(), want) {
			t.Errorf("assigns missing %q:\n%s", want, s.assigns.String())
		}
	}
}
