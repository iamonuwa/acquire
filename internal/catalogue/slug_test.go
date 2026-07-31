package catalogue

import "testing"

func TestSlug_HardCases(t *testing.T) {
	cases := map[string]string{
		"SODIUM CHLORIDE": "sodium-chloride",
		"NORTRIPTYLINE (NORTRIPTYLINE HYDROCHLORIDE)": "nortriptyline-nortriptyline-hydrochloride",
		"PAPAVERINE HYDROCHLORIDE":                    "papaverine-hydrochloride",
		"CAFFEINE HYDRATE":                            "caffeine-hydrate",
		"ACETYLSALICYLIC ACID":                        "acetylsalicylic-acid",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlug_CasePunctWhitespaceCollapse(t *testing.T) {
	// These must all reduce to the same stable slug.
	variants := []string{"Metformin HCl", "METFORMIN HCL", "metformin   hcl", "  metformin-hcl  ", "metformin.hcl"}
	want := "metformin-hcl"
	for _, v := range variants {
		if got := Slug(v); got != want {
			t.Errorf("Slug(%q) = %q, want %q", v, got, want)
		}
	}
}

func TestSlug_Idempotent(t *testing.T) {
	x := "TENOFOVIR DISOPROXIL FUMARATE"
	if Slug(Slug(x)) != Slug(x) {
		t.Error("Slug is not idempotent")
	}
}
