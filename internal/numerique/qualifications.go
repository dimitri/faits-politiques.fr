package numerique

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceANSSI = archive.Source{
	Slug: "anssi-catalogue-qualifications", Label: "ANSSI — catalogue des produits et services certifiés, qualifiés et agréés",
	Publisher:   "Agence nationale de la sécurité des systèmes d'information",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Document public de l'ANSSI, cité avec lien",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : ANSSI, catalogue des produits et services qualifiés",
	Cadence:     "mise à jour continue (date imprimée sur le catalogue)",
	Notes: "Seule la section 3.3.1 (services Cloud qualifiés SecNumCloud) est lue. Une " +
		"qualification porte sur un service nommé, pour trois ans ; les offres en cours d'évaluation n'y " +
		"figurent pas.",
}

const urlCatalogueANSSI = "https://messervices.cyber.gouv.fr/visas/catalogue-produits-services-profils-de-protection-sites-certifies-qualifies-agrees-anssi.pdf"

// La société française qui opère chaque fournisseur du catalogue, lorsque la
// dénomination Sirene (unité active) la désigne sans ambiguïté. « Cegedim »
// reste sans SIREN : le catalogue ne dit pas s'il s'agit de la société de tête
// ou de sa filiale Cegedim.cloud.
var sirenProviders = map[string]string{
	"Cloud Solutions":          "528893522", // éditeur de Wimi
	"Cloud Temple":             "825400336",
	"Index Education":          "384351599",
	"Numspot":                  "948608948",
	"OVH":                      "424761419", // OVH SAS, l'exploitant (les autres « OVH » sont des sociétés civiles ou sans salarié)
	"Oodrive":                  "432735082",
	"Orange Business Services": "345039416",
	"Outscale":                 "527594493",
	"Thales Cloud Sécurisé":    "908211980", // opérateur de l'offre S3NS
	"Whaller":                  "519139497",
	"Worldline":                "378901946",
}

var (
	reDate        = regexp.MustCompile(`\b(\d{2}/\d{2}/\d{4})\b`)
	reCatalogueDu = regexp.MustCompile(`mis à jour le (\d{2}/\d{2}/\d{4})`)
	reDecision    = regexp.MustCompile(`(\d+)\s*$`)
	reDeuxBlancs  = regexp.MustCompile(`\s{2,}`)
)

func IngestQualifications(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return run(ctx, arch, SourceANSSI, func(srcID, runID int64) (map[string]any, error) {
		f, err := arch.Fetch(ctx, srcID, runID, urlCatalogueANSSI, ".pdf")
		if err != nil {
			return nil, err
		}
		t, err := pdfText(ctx, f.Path, true)
		if err != nil {
			return nil, err
		}
		m := reCatalogueDu.FindStringSubmatch(t)
		if m == nil {
			return nil, fmt.Errorf("date de mise à jour du catalogue introuvable")
		}
		catalogDate, _ := time.Parse("02/01/2006", m[1])
		qualifications, err := readSecNumCloud(t)
		if err != nil {
			return nil, err
		}
		// Une vingtaine de services en 2025-2026 : moins de 15 trahirait une
		// mise en page que la lecture ne suit plus.
		if len(qualifications) < 15 {
			return nil, fmt.Errorf("%d services qualifiés lus seulement", len(qualifications))
		}
		var rows [][]any
		for _, q := range qualifications {
			var thirdParty any
			switch {
			case strings.Contains(strings.ToLower(q.service), "s3ns"):
				thirdParty = "Google Cloud (Alphabet, États-Unis)"
			case strings.Contains(strings.ToLower(q.service), "vmware"):
				thirdParty = "VMware (Broadcom, États-Unis)"
			}
			rows = append(rows, []any{q.provider, q.service, q.types[0], q.types[1], q.types[2], q.types[3],
				q.start, q.end, nilIfEmpty(q.decision), catalogDate, nilIfEmpty(sirenProviders[q.provider]), thirdParty, f.DocumentID})
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		// MERGE plutôt que DELETE+COPY, scopé au seul catalogue_du de cette
		// republication : les catalogues précédents restent en base (verify et
		// sujet_page lisent max(catalogue_du), acteurs.go l'historique). Une
		// vue plutôt que la table réelle : sans elle, WHEN NOT MATCHED BY
		// SOURCE effacerait aussi les catalogues des autres dates.
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			CREATE TEMP TABLE tmp_qualification_secnumcloud (
				fournisseur text, service text, saas boolean, paas boolean, caas boolean, iaas boolean,
				date_debut date, date_fin date, decision text, catalogue_du date, siren text,
				technologie_tierce text, document_id bigint
			) ON COMMIT DROP;
			CREATE OR REPLACE TEMPORARY VIEW qualification_secnumcloud_scope AS
			  SELECT * FROM core.qualification_secnumcloud WHERE catalogue_du = %s
			  WITH LOCAL CHECK OPTION`, "'"+catalogDate.Format("2006-01-02")+"'::date")); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_qualification_secnumcloud"},
			[]string{"fournisseur", "service", "saas", "paas", "caas", "iaas", "date_debut", "date_fin", "decision",
				"catalogue_du", "siren", "technologie_tierce", "document_id"}, pgx.CopyFromRows(rows)); err != nil {
			return nil, err
		}
		ct, err := tx.Exec(ctx, `
			MERGE INTO qualification_secnumcloud_scope AS tgt
			USING tmp_qualification_secnumcloud AS src
			ON tgt.fournisseur = src.fournisseur AND tgt.service = src.service AND tgt.catalogue_du = src.catalogue_du
			WHEN MATCHED AND (tgt.saas, tgt.paas, tgt.caas, tgt.iaas, tgt.date_debut, tgt.date_fin, tgt.decision,
			                   tgt.siren, tgt.technologie_tierce, tgt.document_id)
			                  IS DISTINCT FROM
			                  (src.saas, src.paas, src.caas, src.iaas, src.date_debut, src.date_fin, src.decision,
			                   src.siren, src.technologie_tierce, src.document_id) THEN
			    UPDATE SET saas = src.saas, paas = src.paas, caas = src.caas, iaas = src.iaas,
			               date_debut = src.date_debut, date_fin = src.date_fin, decision = src.decision,
			               siren = src.siren, technologie_tierce = src.technologie_tierce,
			               document_id = src.document_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (fournisseur, service, saas, paas, caas, iaas, date_debut, date_fin, decision,
			            catalogue_du, siren, technologie_tierce, document_id)
			    VALUES (src.fournisseur, src.service, src.saas, src.paas, src.caas, src.iaas, src.date_debut,
			            src.date_fin, src.decision, src.catalogue_du, src.siren, src.technologie_tierce,
			            src.document_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return nil, err
		}
		return map[string]any{"catalogue_du": m[1], "services": len(qualifications), "touchees": ct.RowsAffected()}, tx.Commit(ctx)
	})
}

type qualification struct {
	provider, service, decision string
	types                       [4]bool // SaaS, PaaS, CaaS, IaaS
	start, end                  time.Time
}

// readSecNumCloud lit le tableau de la section 3.3.1 dans la sortie
// « -layout » de pdftotext. Le tableau est en colonnes à largeur fixe, dont les
// positions changent d'une page à l'autre : elles sont relues sur chaque ligne
// d'en-tête. Un service occupe un bloc de lignes séparé des autres par une
// ligne vide ; son nom peut déborder au-dessus et au-dessous de la ligne qui
// porte le fournisseur, les coches et les dates.
func readSecNumCloud(t string) ([]qualification, error) {
	lines := strings.Split(t, "\n")
	start, end := -1, -1
	for i, l := range lines {
		s := strings.TrimSpace(l)
		if start < 0 && strings.HasPrefix(s, "3.3.1") && strings.Contains(s, "services qualifiés") && !strings.Contains(s, "...") {
			start = i
		}
		if start >= 0 && strings.HasPrefix(s, "3.3.2") {
			end = i
			break
		}
	}
	if start < 0 || end < 0 {
		return nil, fmt.Errorf("section 3.3.1 introuvable dans le catalogue")
	}
	var (
		out         []qualification
		typeColumns [4]int
		block       [][]rune
		headerRead  bool
	)
	process := func() error {
		defer func() { block = nil }()
		datedLine := -1
		for i, l := range block {
			if len(reDate.FindAllString(string(l), -1)) >= 2 {
				if datedLine >= 0 {
					return fmt.Errorf("deux lignes datées dans un même bloc : %q", string(l))
				}
				datedLine = i
			}
		}
		if datedLine < 0 {
			return nil
		}
		if !headerRead {
			return fmt.Errorf("ligne de service avant tout en-tête : %q", string(block[datedLine]))
		}
		l := block[datedLine]
		// Le fournisseur est en tête de la ligne datée, séparé du reste par au
		// moins deux espaces ; les en-têtes ne donnent pas la colonne du service
		// (son intitulé est centré, pas aligné sur les noms).
		lineText := string(l)
		cut := reDeuxBlancs.FindStringIndex(lineText)
		if cut == nil {
			return fmt.Errorf("ligne de service sans colonnes : %q", lineText)
		}
		q := qualification{provider: strings.TrimSpace(lineText[:cut[0]])}
		var pieces []string
		for i, b := range block {
			if i == datedLine {
				// Sur la ligne datée, le nom du service s'arrête avant la
				// première coche.
				rest := []rune(lineText[cut[1]:])
				restStart := len([]rune(lineText[:cut[1]]))
				if restStart < typeColumns[0]-2 {
					cutEnd := min(len(rest), typeColumns[0]-2-restStart)
					if s := strings.TrimSpace(string(rest[:cutEnd])); s != "" {
						pieces = append(pieces, s)
					}
				}
				continue
			}
			// Les lignes de débordement ne portent que le nom du service.
			if s := strings.TrimSpace(string(b)); s != "" {
				pieces = append(pieces, s)
			}
		}
		for i, s := range pieces {
			// « co-» + « edition » se recolle ; « Services -» + « Secured » garde
			// son espace.
			if i > 0 && strings.HasSuffix(q.service, "-") && !strings.HasSuffix(q.service, " -") {
				q.service += s
			} else if i > 0 {
				q.service += " " + s
			} else {
				q.service = s
			}
		}
		// Les coches sont entre la colonne SaaS et la première date.
		datePos := strings.Index(string(l), reDate.FindString(string(l)))
		limit := len([]rune(string(l)[:datePos]))
		for i := typeColumns[0] - 2; i < limit && i < len(l); i++ {
			if l[i] != 'X' {
				continue
			}
			k, d := 0, 1<<30
			for j, c := range typeColumns {
				if e := abs(i - (c + 2)); e < d {
					k, d = j, e
				}
			}
			q.types[k] = true
		}
		dates := reDate.FindAllString(string(l), 2)
		q.start, _ = time.Parse("02/01/2006", dates[0])
		q.end, _ = time.Parse("02/01/2006", dates[1])
		if m := reDecision.FindStringSubmatch(string(l)); m != nil {
			q.decision = m[1]
		}
		if q.provider == "" || q.service == "" || !(q.types[0] || q.types[1] || q.types[2] || q.types[3]) {
			return fmt.Errorf("ligne de service incomplète : %q", string(l))
		}
		out = append(out, q)
		return nil
	}
	for _, raw := range lines[start+1 : end] {
		raw = strings.TrimLeft(raw, "\f")
		r := []rune(raw)
		s := string(r)
		if strings.Contains(s, "Nom du service") && strings.Contains(s, "SaaS") && strings.Contains(s, "IaaS") {
			if err := process(); err != nil {
				return nil, err
			}
			for k, name := range []string{"SaaS", "PaaS", "CaaS", "IaaS"} {
				typeColumns[k] = runeIndex(s, name)
			}
			headerRead = true
			continue
		}
		if strings.TrimSpace(s) == "" {
			if err := process(); err != nil {
				return nil, err
			}
			continue
		}
		block = append(block, r)
	}
	if err := process(); err != nil {
		return nil, err
	}
	return out, nil
}

func runeIndex(s, sub string) int {
	i := strings.Index(s, sub)
	if i < 0 {
		return -1
	}
	return len([]rune(s[:i]))
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
