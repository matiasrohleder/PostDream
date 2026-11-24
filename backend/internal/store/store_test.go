package store

import (
	"strings"
	"testing"
)

func TestLikeEscape(t *testing.T) {
	if got := likeEscape(`50%_off\`); got != `%50\%\_off\\%` {
		t.Errorf("got %s", got)
	}
}

func TestFilterWhereScopesInboxes(t *testing.T) {
	f := MessageFilter{InboxIDs: []string{}, Q: "foo bar"}
	where, args := f.where()
	if !strings.Contains(where, "inbox_id = ANY") || len(args) != 3 {
		t.Errorf("where=%s args=%v", where, args)
	}
}

func TestSlugify(t *testing.T) {
	if s := Slugify("  Proyecto X — Staging! "); s != "proyecto-x-staging" {
		t.Errorf("got %q", s)
	}
}

func TestTruncateUTF8(t *testing.T) {
	if s := truncate("ñandú", 2); s != "ñ" {
		t.Errorf("got %q", s)
	}
}
