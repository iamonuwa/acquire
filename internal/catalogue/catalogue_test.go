package catalogue

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func row(width int, vals map[int]string) []string {
	r := make([]string, width)
	for i, v := range vals {
		r[i] = v
	}
	return r
}

func member(rows [][]string) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.WriteAll(rows)
	return b.Bytes()
}

func extractZip(t *testing.T, members map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range members {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func sampleExtract(t *testing.T) []byte {
	return extractZip(t, map[string][]byte{
		"drug.txt": member([][]string{
			row(14, map[int]string{0: "1", 2: "Human", 3: "00000001", 4: "BRAND A", 8: "1"}),
			row(14, map[int]string{0: "2", 2: "Human", 3: "00000002", 4: "COMBO", 8: "2"}),
		}),
		"ingred.txt": member([][]string{
			row(15, map[int]string{0: "1", 2: "METFORMIN HYDROCHLORIDE", 4: "500", 5: "MG"}),
			row(15, map[int]string{0: "2", 2: "AMLODIPINE", 4: "5", 5: "MG"}),
			row(15, map[int]string{0: "2", 2: "VALSARTAN", 4: "160", 5: "MG"}),
		}),
		"comp.txt": member([][]string{
			row(18, map[int]string{0: "1", 3: "ACME PHARMA", 4: "DIN_OWNER"}),
			row(18, map[int]string{0: "2", 3: "BETA LABS", 4: "DIN_OWNER"}),
		}),
		"form.txt":       member([][]string{row(4, map[int]string{0: "1", 2: "TABLET"}), row(4, map[int]string{0: "2", 2: "TABLET"})}),
		"route.txt":      member([][]string{row(4, map[int]string{0: "1", 2: "ORAL"}), row(4, map[int]string{0: "2", 2: "ORAL"})}),
		"schedule.txt":   member([][]string{row(3, map[int]string{0: "1", 1: "OTC"})}),
		"status.txt":     member([][]string{row(7, map[int]string{0: "1", 1: "Y", 2: "MARKETED"}), row(7, map[int]string{0: "2", 1: "Y", 2: "MARKETED"})}),
		"biosimilar.txt": member(nil),
	})
}

func TestBuild(t *testing.T) {
	dir := t.TempDir()
	meta, err := Build(sampleExtract(t), dir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.IngredientCount != 3 || meta.DINCount != 2 {
		t.Fatalf("meta = %+v, want 3 ingredients / 2 DINs", meta)
	}

	var ing Ingredient
	readJSON(t, filepath.Join(dir, "ingredients", "metformin-hydrochloride.json"), &ing)
	if len(ing.Products) != 1 {
		t.Fatalf("metformin products = %d", len(ing.Products))
	}
	p := ing.Products[0]
	if p.DIN != "00000001" || p.Brand != "BRAND A" || p.Strength != "500" ||
		p.StrengthUnit != "MG" || p.Status != "MARKETED" || p.ProductRole != "DIN_OWNER" ||
		len(p.Companies) != 1 || p.Companies[0] != "ACME PHARMA" {
		t.Errorf("metformin product = %+v", p)
	}

	var idx map[string][]string
	readJSON(t, filepath.Join(dir, "din-index.json"), &idx)
	if got := idx["00000002"]; len(got) != 2 { // combination -> two slugs
		t.Errorf("combo DIN slugs = %v, want 2", got)
	}
}

func TestBuild_ColumnAssertionFails(t *testing.T) {
	z := extractZip(t, map[string][]byte{
		"drug.txt": member([][]string{row(13, map[int]string{0: "1"})}), // 13, want 14
	})
	if _, err := Build(z, t.TempDir()); err == nil {
		t.Error("a wrong column count must fail the build")
	}
}
