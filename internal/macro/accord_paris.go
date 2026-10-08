package macro

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceParisAgreement = archive.Source{
	Slug: "onu-accord-paris-ratifications", Label: "ONU — signature et ratification de l'Accord de Paris",
	Publisher: "Organisation des Nations unies (dépositaire des traités)", Tier: "PRIMARY_OFFICIAL",
	License: "Domaine public (document officiel des Nations unies)", ReuseClass: "OPEN",
	Attribution: "Source : ONU, Collection des traités, chapitre XXVII.7.d",
	Cadence:     "ponctuelle",
	Notes: "La collection dépositaire officielle, pas une source secondaire. Une ratification " +
		"NULLE ne veut pas dire un rejet du traité : le Yémen, par exemple, a signé sans jamais " +
		"ratifier, dans un contexte de guerre civile qui a rendu le processus impossible.",
}

const parisAgreementURL = "https://treaties.un.org/doc/Publication/MTDSG/Volume%20II/Chapter%20XXVII/xxvii-7-d.en.xml"

// latin1 convertit de l'ISO-8859-1 vers UTF-8 (même patron que internal/senat).
func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

var (
	reTag      = regexp.MustCompile(`<[^>]*>`)
	reFootnote = regexp.MustCompile(`(?:\d,?)+$`)
)

// cleanParticipant retire les balises <superscript> et les chiffres de
// notes de bas de page qu'elles encadraient (ex. "United States of
// America<superscript>7</superscript>" -> "United States of America") —
// les notes elles-mêmes documentent des cas particuliers (déclarations
// unilatérales de certains territoires britanniques, notamment), non
// reprises ici faute d'un besoin identifié pour ce dossier.
func cleanParticipant(s string) string {
	s = reTag.ReplaceAllString(s, "")
	s = reFootnote.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// parseParisAgreementDate lit "22 Apr\t 2016 " ou, pour les cas particuliers
// (les États-Unis, ré-acceptés après leur retrait), "[20 Jan\t 2021 A]" —
// crochets et lettre de type (A = acceptation, AA = approbation, a =
// adhésion directe, sans signature préalable) inclus.
func parseParisAgreementDate(s string) (*time.Time, string) {
	s = strings.TrimSpace(strings.Trim(strings.TrimSpace(s), "[]"))
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, ""
	}
	kind := ""
	if len(fields) == 4 {
		kind = fields[3]
		fields = fields[:3]
	}
	if len(fields) != 3 {
		return nil, ""
	}
	t, err := time.Parse("2 Jan 2006", strings.Join(fields, " "))
	if err != nil {
		return nil, ""
	}
	return &t, kind
}

type parisAgreementEntry struct {
	Entries []string `xml:"Entry"`
}

type parisAgreementDoc struct {
	Rows []parisAgreementEntry `xml:"Treaty>Participants>Table>TGroup>Tbody>Rows>Row"`
}

// IngestParisAgreement charge la table officielle ONU de signature/ratification
// de l'Accord de Paris, pays par pays.
func IngestParisAgreement(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceParisAgreement)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "accord-paris-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, parisAgreementURL, ".xml")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	text := latin1(raw)
	text = strings.Replace(text, `encoding="ISO-8859-1"`, `encoding="UTF-8"`, 1)

	var doc parisAgreementDoc
	if err := xml.Unmarshal([]byte(text), &doc); err != nil {
		return fail(fmt.Errorf("XML illisible : %w", err))
	}
	if len(doc.Rows) < 150 {
		return fail(fmt.Errorf("seulement %d lignes lues — le format a peut-être changé", len(doc.Rows)))
	}

	var rows [][]any
	for i, r := range doc.Rows {
		if len(r.Entries) != 3 {
			return fail(fmt.Errorf("ligne %d : %d colonnes au lieu de 3", i+1, len(r.Entries)))
		}
		country := cleanParticipant(r.Entries[0])
		if country == "" {
			return fail(fmt.Errorf("ligne %d : nom de pays vide", i+1))
		}
		signDate, _ := parseParisAgreementDate(r.Entries[1])
		ratifDate, ratifKind := parseParisAgreementDate(r.Entries[2])
		var signDateStr, ratifDateStr *string
		if signDate != nil {
			s := signDate.Format("2006-01-02")
			signDateStr = &s
		}
		if ratifDate != nil {
			s := ratifDate.Format("2006-01-02")
			ratifDateStr = &s
		}
		var ratifKindPtr *string
		if ratifKind != "" {
			ratifKindPtr = &ratifKind
		}
		rows = append(rows, []any{country, signDateStr, ratifDateStr, ratifKindPtr, srcID})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY : l'ancien DELETE (table entière, ce
	// connecteur en est l'unique propriétaire) payait le prix des triggers RI
	// pour l'intégralité des pays à chaque republication de l'ONU, changement
	// ou non.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_ratification_accord_paris (
			pays text, date_signature date, date_ratification date,
			type_ratification text, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_ratification_accord_paris"},
		[]string{"pays", "date_signature", "date_ratification", "type_ratification", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("core.ratification_accord_paris : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.ratification_accord_paris AS tgt
		USING tmp_ratification_accord_paris AS src
		ON tgt.pays = src.pays
		WHEN MATCHED AND (tgt.date_signature, tgt.date_ratification, tgt.type_ratification, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.date_signature, src.date_ratification, src.type_ratification, src.source_id) THEN
		    UPDATE SET date_signature = src.date_signature, date_ratification = src.date_ratification,
		               type_ratification = src.type_ratification, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (pays, date_signature, date_ratification, type_ratification, source_id)
		    VALUES (src.pays, src.date_signature, src.date_ratification, src.type_ratification, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}
	affected := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"pays": len(rows), "touchees": affected}, "")
	fmt.Printf("  Accord de Paris, ratifications (ONU) : %d pays (%d touchés par la fusion)\n", len(rows), affected)
	return nil
}
