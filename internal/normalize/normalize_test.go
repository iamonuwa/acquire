package normalize

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"gitlab.com/cail-health/cail-acquire/internal/config"
)

// makeTestPDF builds a minimal single-page PDF containing text, so no binary
// fixture is committed. text must not contain PDF-special chars.
func makeTestPDF(text string) []byte {
	var buf bytes.Buffer
	offsets := make([]int, 6)
	obj := func(n int, body string) {
		offsets[n] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", n, body)
	}
	buf.WriteString("%PDF-1.4\n")
	obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	obj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	obj(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")
	content := fmt.Sprintf("BT /F1 24 Tf 72 700 Td (%s) Tj ET", text)
	obj(4, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	obj(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	xref := buf.Len()
	buf.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", xref)
	return buf.Bytes()
}

func requirePdftotext(t *testing.T) {
	t.Helper()
	if err := AssertAvailable(); err != nil {
		t.Skip("pdftotext not installed")
	}
}

func TestPDF_ExtractsText(t *testing.T) {
	requirePdftotext(t)
	out, err := PDF(context.Background(), makeTestPDF("Criteria Test Doc"))
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}
	if !strings.Contains(string(out), "Criteria Test Doc") {
		t.Errorf("extracted text missing expected content: %q", out)
	}
}

func TestPDF_Deterministic(t *testing.T) {
	requirePdftotext(t)
	pdf := makeTestPDF("Stable Hash")
	a, err := PDF(context.Background(), pdf)
	if err != nil {
		t.Fatal(err)
	}
	b, err := PDF(context.Background(), pdf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Error("pdftotext output not deterministic across runs")
	}
}

func TestPDF_InvalidInput(t *testing.T) {
	requirePdftotext(t)
	if _, err := PDF(context.Background(), []byte("not a pdf")); err == nil {
		t.Error("expected failure on non-PDF input")
	}
}

func TestFingerprint(t *testing.T) {
	requirePdftotext(t)
	fp, err := Fingerprint(context.Background(), config.NormalizePDF)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(fp, "pdftotext-") {
		t.Errorf("fingerprint = %q, want pdftotext-<version>", fp)
	}
}

func TestApply_Dispatch(t *testing.T) {
	if _, err := Apply(context.Background(), config.NormalizeZipMembers, nil); err == nil {
		t.Error("zip_members should be unimplemented")
	}
	if _, err := Apply(context.Background(), config.Normalize("bogus"), nil); err == nil {
		t.Error("unknown normalizer should error")
	}
}
