# DPD extract column layouts

Layouts **as documented** by Health Canada in the extract Read Me. Verified as documentation 2026-07-27.

Every layout here is a claim, not a fact. `ther.txt` is documented with seven columns and observed with four, which means the documentation is wrong somewhere and is assumed wrong elsewhere. Assert the actual column count per file at ingest and fail loudly on divergence.

`DRUG_CODE` is the join key across every file.

## Contents

- [drug.txt](#drugtxt): core product record
- [comp.txt](#comptxt): companies, carries product role
- [ingred.txt](#ingredtxt): active ingredients and strengths
- [form.txt](#formtxt): dosage form
- [route.txt](#routetxt): route of administration
- [schedule.txt](#scheduletxt): regulatory schedule
- [status.txt](#statustxt): product status
- [ther.txt](#thertxt): therapeutic class, **documentation known wrong**
- [bios.txt](#biostxt): biosimilar indicator
- [pharm.txt](#pharmtxt): pharmaceutical standard
- [package.txt](#packagetxt): packaging
- [vet.txt](#vettxt): veterinary species
- [inactive.txt](#inactivetxt): inactive products, separate extract

---

## drug.txt

Table `QRYM_DRUG_PRODUCT`. The core product record.

```
DRUG_CODE
PRODUCT_CATEGORIZATION
CLASS
DRUG_IDENTIFICATION_NUMBER
BRAND_NAME
DESCRIPTOR
PEDIATRIC_FLAG
ACCESSION_NUMBER
NUMBER_OF_AIS
LAST_UPDATE_DATE
AI_GROUP_NO
CLASS_F
BRAND_NAME_F
DESCRIPTOR_F
```

`DRUG_IDENTIFICATION_NUMBER` is the DIN provinces cite. It is never a rules key; it resolves to an ingredient through the catalogue.

`AI_GROUP_NO` looks like a molecule grouping key and is not usable as one.

`_F` suffixed fields are French. Ignore for catalogue generation unless a French-language requirement appears.

## comp.txt

Table `QRYM_COMPANIES`. Carries product role.

```
DRUG_CODE
MFR_CODE
COMPANY_CODE
COMPANY_NAME
COMPANY_TYPE
ADDRESS_MAILING_FLAG
ADDRESS_BILLING_FLAG
ADDRESS_NOTIFICATION_FLAG
ADDRESS_OTHER
SUITE_NUMBER
STREET_NAME
CITY_NAME
PROVINCE
COUNTRY
POSTAL_CODE
POST_OFFICE_BOX
PROVINCE_F
COUNTRY_F
```

`COMPANY_TYPE` carries the literal space-separated string `DIN OWNER`. There is no `DIN_OWNER` column in this or any other file. Match the string with a space; do not split on an underscore.

## ingred.txt

Table `QRYM_ACTIVE_INGREDIENTS`. Several rows per product for combination products.

```
DRUG_CODE
ACTIVE_INGREDIENT_CODE
INGREDIENT
INGREDIENT_SUPPLIED_IND
STRENGTH
STRENGTH_UNIT
STRENGTH_TYPE
DOSAGE_VALUE
BASE
DOSAGE_UNIT
NOTES
INGREDIENT_F
STRENGTH_UNIT_F
STRENGTH_TYPE_F
DOSAGE_UNIT_F
```

`INGREDIENT` is the string slug generation runs against. Expect salts, esters, hydrates, and inconsistent punctuation.

`NUMBER_OF_AIS` on `drug.txt` should equal the row count here for that `DRUG_CODE`. Worth asserting; a mismatch indicates a join problem.

## form.txt

Table `QRYM_FORM`.

```
DRUG_CODE
PHARM_FORM_CODE
PHARMACEUTICAL_FORM
PHARMACEUTICAL_FORM_F
```

## route.txt

Table `QRYM_ROUTE`.

```
DRUG_CODE
ROUTE_OF_ADMINISTRATION_CODE
ROUTE_OF_ADMINISTRATION
ROUTE_OF_ADMINISTRATION_F
```

## schedule.txt

Table `QRYM_SCHEDULE`.

```
DRUG_CODE
SCHEDULE
SCHEDULE_F
```

## status.txt

Table `QRYM_STATUS`. Multiple historical rows per product.

```
DRUG_CODE
CURRENT_STATUS_FLAG
STATUS
HISTORY_DATE
STATUS_F
LOT_NUMBER
EXPIRATION_DATE
```

`CURRENT_STATUS_FLAG` selects the live row. Do not assume the last row by file order is current.

## ther.txt

Table `QRYM_THERAPEUTIC_CLASS`. **The documentation for this file is wrong.**

Documented as seven columns:

```
DRUG_CODE
TC_ATC_NUMBER
TC_ATC
TC_AHFS_NUMBER
TC_AHFS
TC_ATC_F
TC_AHFS_F
```

Observed with four. Do not write a parser against the documented layout. Read the header or first data row, determine the actual layout, and assert it.

This file is the reason the column-count assertion exists. Treat it as the canonical example rather than an isolated exception.

## bios.txt

Table `QRYM_BIOSIMILARS`. The biosimilar indicator, joined on `DRUG_CODE`.

```
DRUG_CODE
SI_DESC_E
SI_DESC_F
SI_CODE
```

Biosimilar status is not a flag on `drug.txt`. It is this file.

Do not rely on this indicator alone. Derive product role from the company relationship in `comp.txt` and use this as corroboration.

## pharm.txt

Table `QRYM_PHARMACEUTICAL_STD`.

```
DRUG_CODE
PHARMACEUTICAL_STD
```

## package.txt

Table `QRYM_PACKAGING`.

```
DRUG_CODE
UPC
PACKAGE_SIZE_UNIT
PACKAGE_TYPE
PACKAGE_SIZE
PRODUCT_INFORMATION
PACKAGE_SIZE_UNIT_F
PACKAGE_TYPE_F
```

## vet.txt

Table `QRYM_VETERINARY_SPECIES`.

```
DRUG_CODE
VET_SPECIES
VET_SUB_SPECIES
VET_SPECIES_F
```

Not relevant to human drug coverage. Ingested because the decision is to ingest the whole extract without filtering; scope is enforced in `release.yaml`, not at ingestion.

## inactive.txt

Table `QRYM_INACTIVE_PRODUCTS`. Ships as a separate extract, not inside `allfiles.zip`.

```
DRUG_CODE
DRUG_IDENTIFICATION_NUMBER
BRAND_NAME
HISTORY_DATE
BRAND_NAME_F
```

---

## Field to concept mapping

For resolving what the catalogue needs from where.

| Concept | File | Field |
|---|---|---|
| Join key | all | `DRUG_CODE` |
| DIN | `drug.txt` | `DRUG_IDENTIFICATION_NUMBER` |
| Brand name | `drug.txt` | `BRAND_NAME` |
| Class | `drug.txt` | `CLASS` |
| Company | `comp.txt` | `COMPANY_NAME` |
| Product role | `comp.txt` | `COMPANY_TYPE` = `DIN OWNER` |
| Ingredient | `ingred.txt` | `INGREDIENT` |
| Strength | `ingred.txt` | `STRENGTH`, `STRENGTH_UNIT` |
| Dosage form | `form.txt` | `PHARMACEUTICAL_FORM` |
| Route | `route.txt` | `ROUTE_OF_ADMINISTRATION` |
| Schedule | `schedule.txt` | `SCHEDULE` |
| Status | `status.txt` | `CURRENT_STATUS_FLAG`, `STATUS` |
| Biosimilar | `bios.txt` | `SI_CODE`, `SI_DESC_E` |
| Not a molecule key | `drug.txt` | `AI_GROUP_NO` |