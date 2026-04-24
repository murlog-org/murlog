package id

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	a := New()
	b := New()

	if a.IsNil() {
		t.Fatal("New() returned nil ID")
	}
	if a == b {
		t.Fatal("two New() calls returned same ID")
	}

	// Version should be 7.
	if v := a[6] >> 4; v != 7 {
		t.Fatalf("version = %d, want 7", v)
	}

	// Variant should be 0b10.
	if v := a[8] >> 6; v != 2 {
		t.Fatalf("variant = %d, want 2", v)
	}
}

func TestTime(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	id := NewWithTime(now)
	got := id.Time()

	if !got.Equal(now) {
		t.Fatalf("Time() = %v, want %v", got, now)
	}
}

func TestString(t *testing.T) {
	id := New()
	s := id.String()

	if len(s) != 36 {
		t.Fatalf("String() length = %d, want 36", len(s))
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		t.Fatalf("String() format invalid: %s", s)
	}
}

func TestParse(t *testing.T) {
	id := New()
	s := id.String()

	parsed, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	if parsed != id {
		t.Fatalf("Parse roundtrip failed: got %v, want %v", parsed, id)
	}
}

func TestParseNoHyphens(t *testing.T) {
	id := New()
	// Build string without hyphens.
	s := id.String()
	nohyphen := s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:36]

	parsed, err := Parse(nohyphen)
	if err != nil {
		t.Fatalf("Parse(%q): %v", nohyphen, err)
	}
	if parsed != id {
		t.Fatalf("Parse no-hyphen roundtrip failed: got %v, want %v", parsed, id)
	}
}

func TestFromBytes(t *testing.T) {
	id := New()
	restored, err := FromBytes(id.Bytes())
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	if restored != id {
		t.Fatalf("FromBytes roundtrip failed")
	}
}

func TestOrdering(t *testing.T) {
	a := NewWithTime(time.Now())
	time.Sleep(2 * time.Millisecond)
	b := NewWithTime(time.Now())

	// b should be greater than a (lexicographic byte comparison).
	for i := range 16 {
		if a[i] < b[i] {
			return // OK, a < b
		}
		if a[i] > b[i] {
			t.Fatal("earlier ID is greater than later ID")
		}
	}
	t.Fatal("IDs are equal despite different timestamps")
}
