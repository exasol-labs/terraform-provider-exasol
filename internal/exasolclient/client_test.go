package exasolclient

import "testing"

func TestRenameLog(t *testing.T) {
	l := NewRenameLog()
	if _, ok := l.Lookup(RenamePrincipal, "A"); ok {
		t.Fatal("empty log returned a rename")
	}
	l.Record(RenamePrincipal, "a", "B")
	l.Record(RenamePrincipal, "B", "C") // chain: A -> B -> C
	if got, _ := l.Lookup(RenamePrincipal, "A"); got != "C" {
		t.Fatalf("A resolves to %q, want C", got)
	}
	if got, _ := l.Lookup(RenamePrincipal, "b"); got != "C" {
		t.Fatalf("B resolves to %q, want C", got)
	}
	// Another object takes the old name A: grants recorded under A still
	// belong to the object that is now C.
	l.Record(RenamePrincipal, "X", "A")
	if got, _ := l.Lookup(RenamePrincipal, "A"); got != "C" {
		t.Fatalf("A resolves to %q after reuse, want C", got)
	}
	if got, _ := l.Lookup(RenamePrincipal, "X"); got != "A" {
		t.Fatalf("X resolves to %q, want A", got)
	}
	// Kinds are separate
	if _, ok := l.Lookup(RenameConnection, "A"); ok {
		t.Fatal("principal rename leaked into connection kind")
	}
	var nilLog *RenameLog
	nilLog.Record(RenameSchema, "S", "T")
	if _, ok := nilLog.Lookup(RenameSchema, "S"); ok {
		t.Fatal("nil log returned a rename")
	}
}
