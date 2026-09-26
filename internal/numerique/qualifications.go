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
var sirenFournisseurs = map[string]string{
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
	return executer(ctx, arch, SourceANSSI, func(srcID, runID int64) (map[string]any, error) {
		f, err := arch.Fetch(ctx, srcID, runID, urlCatalogueANSSI, ".pdf")
		if err != nil {
			return nil, err
		}
		t, err := textePDF(ctx, f.Path, true)
		if err != nil {
			return nil, err
		}
		m := reCatalogueDu.FindStringSubmatch(t)
		if m == nil {
			return nil, fmt.Errorf("date de mise à jour du catalogue introuvable")
		}
		catalogueDu, _ := time.Parse("02/01/2006", m[1])
		qs, err := lireSecNumCloud(t)
		if err != nil {
			return nil, err
		}
		// Une vingtaine de services en 2025-2026 : moins de 15 trahirait une
		// mise en page que la lecture ne suit plus.
		if len(qs) < 15 {
			return nil, fmt.Errorf("%d services qualifiés lus seulement", len(qs))
		}
		var lignes [][]any
		for _, q := range qs {
			var tierce any
			switch {
			case strings.Contains(strings.ToLower(q.service), "s3ns"):
				tierce = "Google Cloud (Alphabet, États-Unis)"
			case strings.Contains(strings.ToLower(q.service), "vmware"):
				tierce = "VMware (Broadcom, États-Unis)"
			}
			lignes = append(lignes, []any{q.fournisseur, q.service, q.types[0], q.types[1], q.types[2], q.types[3],
				q.debut, q.fin, nul(q.decision), catalogueDu, nul(sirenFournisseurs[q.fournisseur]), tierce, f.DocumentID})
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
			  WITH LOCAL CHECK OPTION`, "'"+catalogueDu.Format("2006-01-02")+"'::date")); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_qualification_secnumcloud"},
			[]string{"fournisseur", "service", "saas", "paas", "caas", "iaas", "date_debut", "date_fin", "decision",
				"catalogue_du", "siren", "technologie_tierce", "document_id"}, pgx.CopyFromRows(lignes)); err != nil {
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
		return map[string]any{"catalogue_du": m[1], "services": len(qs), "touchees": ct.RowsAffected()}, tx.Commit(ctx)
	})
}

type qualification struct {
	fournisseur, service, decision string
	types                          [4]bool // SaaS, PaaS, CaaS, IaaS
	debut, fin                     time.Time
}

// lireSecNumCloud lit le tableau de la section 3.3.1 dans la sortie
// « -layout » de pdftotext. Le tableau est en colonnes à largeur fixe, dont les
// positions changent d'une page à l'autre : elles sont relues sur chaque ligne
// d'en-tête. Un service occupe un bloc de lignes séparé des autres par une
// ligne vide ; son nom peut déborder au-dessus et au-dessous de la ligne qui
// porte le fournisseur, les coches et les dates.
func lireSecNumCloud(t string) ([]qualification, error) {
	lignes := strings.Split(t, "\n")
	debut, fin := -1, -1
	for i, l := range lignes {
		s := strings.TrimSpace(l)
		if debut < 0 && strings.HasPrefix(s, "3.3.1") && strings.Contains(s, "services qualifiés") && !strings.Contains(s, "...") {
			debut = i
		}
		if debut >= 0 && strings.HasPrefix(s, "3.3.2") {
			fin = i
			break
		}
	}
	if debut < 0 || fin < 0 {
		return nil, fmt.Errorf("section 3.3.1 introuvable dans le catalogue")
	}
	var (
		out       []qualification
		colTypes  [4]int
		bloc      [][]rune
		enteteLue bool
	)
	traiter := func() error {
		defer func() { bloc = nil }()
		datee := -1
		for i, l := range bloc {
			if len(reDate.FindAllString(string(l), -1)) >= 2 {
				if datee >= 0 {
					return fmt.Errorf("deux lignes datées dans un même bloc : %q", string(l))
				}
				datee = i
			}
		}
		if datee < 0 {
			return nil
		}
		if !enteteLue {
			return fmt.Errorf("ligne de service avant tout en-tête : %q", string(bloc[datee]))
		}
		l := bloc[datee]
		// Le fournisseur est en tête de la ligne datée, séparé du reste par au
		// moins deux espaces ; les en-têtes ne donnent pas la colonne du service
		// (son intitulé est centré, pas aligné sur les noms).
		ligne := string(l)
		coupe := reDeuxBlancs.FindStringIndex(ligne)
		if coupe == nil {
			return fmt.Errorf("ligne de service sans colonnes : %q", ligne)
		}
		q := qualification{fournisseur: strings.TrimSpace(ligne[:coupe[0]])}
		var morceaux []string
		for i, b := range bloc {
			if i == datee {
				// Sur la ligne datée, le nom du service s'arrête avant la
				// première coche.
				reste := []rune(ligne[coupe[1]:])
				debutReste := len([]rune(ligne[:coupe[1]]))
				if debutReste < colTypes[0]-2 {
					fin := min(len(reste), colTypes[0]-2-debutReste)
					if s := strings.TrimSpace(string(reste[:fin])); s != "" {
						morceaux = append(morceaux, s)
					}
				}
				continue
			}
			// Les lignes de débordement ne portent que le nom du service.
			if s := strings.TrimSpace(string(b)); s != "" {
				morceaux = append(morceaux, s)
			}
		}
		for i, s := range morceaux {
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
		posDate := strings.Index(string(l), reDate.FindString(string(l)))
		limite := len([]rune(string(l)[:posDate]))
		for i := colTypes[0] - 2; i < limite && i < len(l); i++ {
			if l[i] != 'X' {
				continue
			}
			k, d := 0, 1<<30
			for j, c := range colTypes {
				if e := abs(i - (c + 2)); e < d {
					k, d = j, e
				}
			}
			q.types[k] = true
		}
		dates := reDate.FindAllString(string(l), 2)
		q.debut, _ = time.Parse("02/01/2006", dates[0])
		q.fin, _ = time.Parse("02/01/2006", dates[1])
		if m := reDecision.FindStringSubmatch(string(l)); m != nil {
			q.decision = m[1]
		}
		if q.fournisseur == "" || q.service == "" || !(q.types[0] || q.types[1] || q.types[2] || q.types[3]) {
			return fmt.Errorf("ligne de service incomplète : %q", string(l))
		}
		out = append(out, q)
		return nil
	}
	for _, brute := range lignes[debut+1 : fin] {
		brute = strings.TrimLeft(brute, "\f")
		r := []rune(brute)
		s := string(r)
		if strings.Contains(s, "Nom du service") && strings.Contains(s, "SaaS") && strings.Contains(s, "IaaS") {
			if err := traiter(); err != nil {
				return nil, err
			}
			for k, nom := range []string{"SaaS", "PaaS", "CaaS", "IaaS"} {
				colTypes[k] = runeIndex(s, nom)
			}
			enteteLue = true
			continue
		}
		if strings.TrimSpace(s) == "" {
			if err := traiter(); err != nil {
				return nil, err
			}
			continue
		}
		bloc = append(bloc, r)
	}
	if err := traiter(); err != nil {
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
