package normalize

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func makeZip(t *testing.T, members map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range members {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestZipMembers_SortedWithHeaders(t *testing.T) {
	raw := makeZip(t, map[string]string{
		"drug.txt": "D1\nD2\n",
		"comp.txt": "C1\n",
		"bios.txt": "B1\n",
	})
	out, err := zipMembers(raw)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	// Members must appear in sorted order under headers.
	wantOrder := []string{"===== bios.txt =====", "===== comp.txt =====", "===== drug.txt ====="}
	last := -1
	for _, h := range wantOrder {
		i := strings.Index(got, h)
		if i < 0 {
			t.Fatalf("missing header %q in output", h)
		}
		if i < last {
			t.Errorf("members not in sorted order: %q", h)
		}
		last = i
	}
	if !strings.Contains(got, "B1") || !strings.Contains(got, "C1") || !strings.Contains(got, "D1") {
		t.Error("member content missing")
	}
	// Deterministic across runs.
	out2, _ := zipMembers(raw)
	if !bytes.Equal(out, out2) {
		t.Error("zipMembers not deterministic")
	}
}

func TestZipMembers_Traversal(t *testing.T) {
	for _, bad := range []string{"../evil.txt", "/etc/passwd", "a/../../b"} {
		raw := makeZip(t, map[string]string{bad: "x"})
		if _, err := zipMembers(raw); err == nil {
			t.Errorf("member %q should be rejected", bad)
		}
	}
}

func TestZipMembers_TooManyMembers(t *testing.T) {
	members := make(map[string]string, maxMembers+1)
	for i := 0; i <= maxMembers; i++ {
		members[fmt.Sprintf("f%03d.txt", i)] = "x"
	}
	if _, err := zipMembers(makeZip(t, members)); err == nil {
		t.Error("over-limit member count should be rejected")
	}
}

func TestZipMembers_NotAZip(t *testing.T) {
	if _, err := zipMembers([]byte("plain text, not a zip")); err == nil {
		t.Error("non-archive should error, not panic")
	}
}
