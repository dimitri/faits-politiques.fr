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

var SourceAccordParis = archive.Source{
	Slug: "onu-accord-paris-ratifications", Label: "ONU — signature et ratification de l'Accord de Paris",
	Publisher: "Organisation des Nations unies (dépositaire des traités)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Domaine public (document officiel des Nations unies)", ReuseClass: "OPEN",
	Attribution: "Source : ONU, Collection des traités, chapitre XXVII.7.d",
	Cadence:     "ponctuelle",
	Notes: "La collection dépositaire officielle, pas une source secondaire. Une ratification " +
		"NULLE ne veut pas dire un rejet du traité : le Yémen, par exemple, a signé sans jamais " +
		"ratifier, dans un contexte de guerre civile qui a rendu le processus impossible.",
}

const urlAccordParis = "https://treaties.un.org/doc/Publication/MTDSG/Volume%20II/Chapter%20XXVII/xxvii-7-d.en.xml"

// latin1 convertit de l'ISO-8859-1 vers UTF-8 (même patron que internal/senat).
func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

var (
	reBalise      = regexp.MustCompile(`<[^>]*>`)
	reNoteBasPage = regexp.MustCompile(`(?:\d,?)+$`)
)

// nettoyerParticipant retire les balises <superscript> et les chiffres de
// notes de bas de page qu'elles encadraient (ex. "United States of
// America<superscript>7</superscript>" -> "United States of America") —
// les notes elles-mêmes documentent des cas particuliers (déclarations
// unilatérales de certains territoires britanniques, notamment), non
// reprises ici faute d'un besoin identifié pour ce dossier.
func nettoyerParticipant(s string) string {
	s = reBalise.ReplaceAllString(s, "")
	s = reNoteBasPage.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// parserDateAccordParis lit "22 Apr\t 2016 " ou, pour les cas particuliers
// (les États-Unis, ré-acceptés après leur retrait), "[20 Jan\t 2021 A]" —
// crochets et lettre de type (A = acceptation, AA = approbation, a =
// adhésion directe, sans signature préalable) inclus.
func parserDateAccordParis(s string) (*time.Time, string) {
	s = strings.TrimSpace(strings.Trim(strings.TrimSpace(s), "[]"))
	champs := strings.Fields(s)
	if len(champs) == 0 {
		return nil, ""
	}
	typeR := ""
	if len(champs) == 4 {
		typeR = champs[3]
		champs = champs[:3]
	}
	if len(champs) != 3 {
		return nil, ""
	}
	t, err := time.Parse("2 Jan 2006", strings.Join(champs, " "))
	if err != nil {
		return nil, ""
	}
	return &t, typeR
}

type entreeAccordParis struct {
	Entries []string `xml:"Entry"`
}

type docAccordParis struct {
	Rows []entreeAccordParis `xml:"Treaty>Participants>Table>TGroup>Tbody>Rows>Row"`
}

// IngestAccordParis charge la table officielle ONU de signature/ratification
// de l'Accord de Paris, pays par pays.
func IngestAccordParis(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceAccordParis)
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

	f, err := arch.Fetch(ctx, srcID, runID, urlAccordParis, ".xml")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	texte := latin1(raw)
	texte = strings.Replace(texte, `encoding="ISO-8859-1"`, `encoding="UTF-8"`, 1)

	var doc docAccordParis
	if err := xml.Unmarshal([]byte(texte), &doc); err != nil {
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
		pays := nettoyerParticipant(r.Entries[0])
		if pays == "" {
			return fail(fmt.Errorf("ligne %d : nom de pays vide", i+1))
		}
		dateSign, _ := parserDateAccordParis(r.Entries[1])
		dateRatif, typeRatif := parserDateAccordParis(r.Entries[2])
		var dSign, dRatif *string
		if dateSign != nil {
			s := dateSign.Format("2006-01-02")
			dSign = &s
		}
		if dateRatif != nil {
			s := dateRatif.Format("2006-01-02")
			dRatif = &s
		}
		var typeRatifPtr *string
		if typeRatif != "" {
			typeRatifPtr = &typeRatif
		}
		rows = append(rows, []any{pays, dSign, dRatif, typeRatifPtr, srcID})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.ratification_accord_paris`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "ratification_accord_paris"},
		[]string{"pays", "date_signature", "date_ratification", "type_ratification", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("core.ratification_accord_paris : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"pays": len(rows)}, "")
	fmt.Printf("  Accord de Paris, ratifications (ONU) : %d pays\n", len(rows))
	return nil
}
