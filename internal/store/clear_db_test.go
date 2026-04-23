package store

import (
	"testing"
)

func TestClearDB(t *testing.T) {
	db, err := New("d:/THG/THG_sale/data/local.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, _ = db.db.Exec("DELETE FROM outbound_messages")
	_, _ = db.db.Exec("DELETE FROM leads")
	_, _ = db.db.Exec("DELETE FROM comments")
	_, _ = db.db.Exec("DELETE FROM posts")
	t.Log("Cleared outbox, leads, comments, and posts successfully!")
}
