package catalogue

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

const generatorVersion = "catalogue-1"

// dinOwnerType is the COMPANY_TYPE value for the DIN owner. Underscore, not a
// space, verified against allfiles.zip; it is the only value in the extract.
const dinOwnerType = "DIN_OWNER"

// memberColumns are the observed column counts, verified against the file, not
// Health Canada's docs. A row that diverges fails the build. ther.txt is
// documented 7 / observed 4 and is not read here.
var memberColumns = map[string]int{
	"drug.txt":       14,
	"ingred.txt":     15,
	"comp.txt":       18,
	"form.txt":       4,
	"route.txt":      4,
	"schedule.txt":   3,
	"status.txt":     7,
	"biosimilar.txt": 4,
}

// Field indices into each row (0-based), from the observed layouts.
const (
	fDrugCode = 0

	fDrugClass = 2
	fDrugDIN   = 3
	fDrugBrand = 4

	fIngName     = 2
	fIngStrength = 4
	fIngUnit     = 5

	fCompName = 3
	fCompType = 4

	fFormName  = 2
	fRouteName = 2
	fSchedName = 1

	fStatusFlag = 1
	fStatusName = 2
)

// Product is one drug product listed under an ingredient.
type Product struct {
	DIN          string   `json:"din"`
	Brand        string   `json:"brand"`
	Class        string   `json:"class,omitempty"`
	Companies    []string `json:"companies,omitempty"`
	ProductRole  string   `json:"product_role,omitempty"`
	Strength     string   `json:"strength,omitempty"`
	StrengthUnit string   `json:"strength_unit,omitempty"`
	Forms        []string `json:"forms,omitempty"`
	Routes       []string `json:"routes,omitempty"`
	Schedule     []string `json:"schedule,omitempty"`
	Status       string   `json:"status,omitempty"`
	Biosimilar   bool     `json:"biosimilar"`
}

// Ingredient is one catalogue entry.
type Ingredient struct {
	Ingredient string    `json:"ingredient"`
	Slug       string    `json:"slug"`
	Products   []Product `json:"products"`
}

// Meta is the generation record.
type Meta struct {
	SourceHash       string `json:"source_hash"`
	GeneratorVersion string `json:"generator_version"`
	IngredientCount  int    `json:"ingredient_count"`
	DINCount         int    `json:"din_count"`
}

// Build parses the DPD extract and writes the three catalogue artifacts under
// outDir: ingredients/<slug>.json, din-index.json, and _meta.json.
func Build(zipBytes []byte, outDir string) (*Meta, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("catalogue: open extract: %w", err)
	}

	drug, err := readMember(zr, "drug.txt")
	if err != nil {
		return nil, err
	}
	ingred, err := readMember(zr, "ingred.txt")
	if err != nil {
		return nil, err
	}
	comp, err := readMember(zr, "comp.txt")
	if err != nil {
		return nil, err
	}
	form, err := readMember(zr, "form.txt")
	if err != nil {
		return nil, err
	}
	route, err := readMember(zr, "route.txt")
	if err != nil {
		return nil, err
	}
	sched, err := readMember(zr, "schedule.txt")
	if err != nil {
		return nil, err
	}
	status, err := readMember(zr, "status.txt")
	if err != nil {
		return nil, err
	}
	bios, err := readMember(zr, "biosimilar.txt")
	if err != nil {
		return nil, err
	}

	drugByCode := make(map[string][]string, len(drug))
	for _, r := range drug {
		drugByCode[r[fDrugCode]] = r
	}
	ingredByCode := groupBy(ingred)
	companies := multiVal(comp, fCompName, func(r []string) bool { return r[fCompType] == dinOwnerType })
	forms := multiVal(form, fFormName, nil)
	routes := multiVal(route, fRouteName, nil)
	schedules := multiVal(sched, fSchedName, nil)
	biosByCode := make(map[string]bool, len(bios))
	for _, r := range bios {
		biosByCode[r[fDrugCode]] = true
	}
	statusByCode := make(map[string]string)
	for _, r := range status {
		if r[fStatusFlag] == "Y" {
			statusByCode[r[fDrugCode]] = r[fStatusName]
		}
	}

	catalog := make(map[string]*Ingredient)
	dinIndex := make(map[string][]string)
	for code, drow := range drugByCode {
		din := drow[fDrugDIN]
		for _, irow := range ingredByCode[code] {
			slug := Slug(irow[fIngName])
			if slug == "" {
				continue
			}
			ing := catalog[slug]
			if ing == nil {
				ing = &Ingredient{Ingredient: irow[fIngName], Slug: slug}
				catalog[slug] = ing
			}
			ing.Products = append(ing.Products, Product{
				DIN:          din,
				Brand:        drow[fDrugBrand],
				Class:        drow[fDrugClass],
				Companies:    companies[code],
				ProductRole:  roleOf(companies[code]),
				Strength:     irow[fIngStrength],
				StrengthUnit: irow[fIngUnit],
				Forms:        forms[code],
				Routes:       routes[code],
				Schedule:     schedules[code],
				Status:       statusByCode[code],
				Biosimilar:   biosByCode[code],
			})
			dinIndex[din] = appendUnique(dinIndex[din], slug)
		}
	}

	meta := &Meta{
		SourceHash:       "sha256:" + hexSum(zipBytes),
		GeneratorVersion: generatorVersion,
		IngredientCount:  len(catalog),
		DINCount:         len(dinIndex),
	}
	if err := emit(outDir, catalog, dinIndex, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

// readMember reads a zip member as CSV, asserting the observed column count on
// every row and failing loudly on divergence.
func readMember(zr *zip.Reader, name string) ([][]string, error) {
	want, ok := memberColumns[name]
	if !ok {
		return nil, fmt.Errorf("catalogue: no expected column count for %s", name)
	}
	var f *zip.File
	for _, zf := range zr.File {
		if filepath.Base(zf.Name) == name {
			f = zf
			break
		}
	}
	if f == nil {
		return nil, fmt.Errorf("catalogue: extract is missing %s", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("catalogue: open %s: %w", name, err)
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("catalogue: read %s: %w", name, err)
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")) // strip UTF-8 BOM if present

	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var rows [][]string
	for line := 1; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("catalogue: %s line %d: %w", name, line, err)
		}
		if len(rec) != want {
			return nil, fmt.Errorf("catalogue: %s line %d has %d columns, expected %d", name, line, len(rec), want)
		}
		rows = append(rows, rec)
	}
	return rows, nil
}

func groupBy(rows [][]string) map[string][][]string {
	m := make(map[string][][]string)
	for _, r := range rows {
		m[r[fDrugCode]] = append(m[r[fDrugCode]], r)
	}
	return m
}

// multiVal collects, per DRUG_CODE, the deduped sorted values at field idx from
// rows passing keep (keep nil accepts all).
func multiVal(rows [][]string, idx int, keep func([]string) bool) map[string][]string {
	m := make(map[string][]string)
	for _, r := range rows {
		if keep != nil && !keep(r) {
			continue
		}
		m[r[fDrugCode]] = appendUnique(m[r[fDrugCode]], r[idx])
	}
	for k := range m {
		sort.Strings(m[k])
	}
	return m
}

func roleOf(companies []string) string {
	if len(companies) == 0 {
		return ""
	}
	return dinOwnerType
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func hexSum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func emit(outDir string, catalog map[string]*Ingredient, dinIndex map[string][]string, meta *Meta) error {
	ingDir := filepath.Join(outDir, "ingredients")
	if err := os.MkdirAll(ingDir, 0o755); err != nil {
		return fmt.Errorf("catalogue: mkdir: %w", err)
	}
	for slug, ing := range catalog {
		sort.Slice(ing.Products, func(i, j int) bool {
			a, b := &ing.Products[i], &ing.Products[j]
			if a.DIN != b.DIN {
				return a.DIN < b.DIN
			}
			if a.Strength != b.Strength {
				return a.Strength < b.Strength
			}
			if a.StrengthUnit != b.StrengthUnit {
				return a.StrengthUnit < b.StrengthUnit
			}
			return a.Brand < b.Brand
		})
		if err := writeJSON(filepath.Join(ingDir, slug+".json"), ing); err != nil {
			return err
		}
	}
	for din := range dinIndex {
		sort.Strings(dinIndex[din])
	}
	if err := writeJSON(filepath.Join(outDir, "din-index.json"), dinIndex); err != nil {
		return err
	}
	return writeJSON(filepath.Join(outDir, "_meta.json"), meta)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("catalogue: marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("catalogue: write %s: %w", path, err)
	}
	return nil
}
