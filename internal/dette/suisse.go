package dette

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/unicode/norm"
)

// « Pourquoi la Suisse n'a pas de dette » : la prémisse est fausse, la Suisse
// a une dette (autour de 40 % du PIB au sens du FMI). Ce qui la distingue est
// un mécanisme, le frein à l'endettement inscrit à l'article 126 de la
// Constitution fédérale, et un résultat : une dette stable en francs pendant
// que le PIB croît. L'Administration fédérale des finances (AFF) publie le
// bilan consolidé des collectivités et ses ratios d'endettement par niveau
// (Confédération, cantons, communes, assurances sociales).
var SourceAFF = archive.Source{
	Slug: "aff-statistique-financiere", Label: "AFF — statistique financière de la Suisse (bilans et indicateurs)",
	Publisher: "Administration fédérale des finances (Suisse)", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Open Government Data Suisse : utilisation libre avec indication de la source",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : Administration fédérale des finances, statistique financière",
	Cadence:     "annuelle (fin août)",
	Notes: "Modèle SF de l'AFF, pas le SEC 2010 : les montants ne se comparent pas directement à la " +
		"dette Maastricht. Bilan en milliers de francs, converti à l'unité. Les ratios sont des " +
		"fractions (0,45 = 45 %). La dernière année est généralement une estimation de l'AFF.",
}

var SourceBNS = archive.Source{
	Slug: "bns-rendements-obligataires", Label: "BNS — rendements des obligations de la Confédération",
	Publisher: "Banque nationale suisse", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Conditions de data.snb.ch : utilisation libre avec indication de la source",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : Banque nationale suisse, cube rendoblim",
	Cadence:     "mensuelle (en principe)",
	Notes: "Le cube rendoblim n'était plus mis à jour depuis juillet 2025 lors de sa première " +
		"intégration (date de publication du cube : 1er septembre 2025) : vérifier la fraîcheur " +
		"avant de citer un taux récent.",
}

const affBase = "https://www.data.finance.admin.ch/static/assets/datasets/fs_dashboard/"

// Les classeurs retenus, et le secteur qu'ils couvrent. La statistique
// consolidée « Confédération, cantons, communes » exclut les assurances
// sociales : ce n'est donc pas S13 au sens européen.
var affBilans = []struct{ file, sector string }{
	{"bund_ktn_gdn-f.xlsx", "S13_HORS_S1314"},
	{"bund-f.xlsx", "S1311"},
}

var affIndicators = map[string]string{
	"staat": "S13", "bund": "S1311", "ktn": "S1312", "gdn": "S1313", "sv": "S1314",
}

var reYear = regexp.MustCompile(`^(19|20)[0-9]{2}$`)

// yearHeader trouve la ligne dont les cellules à partir de C sont des
// années, et renvoie la correspondance colonne -> année avec l'index de ligne.
func yearHeader(rows []map[string]string) (int, map[string]string, error) {
	for i, l := range rows {
		if !reYear.MatchString(l["C"]) {
			continue
		}
		cols := map[string]string{}
		for c, v := range l {
			if reYear.MatchString(v) {
				cols[c] = v
			}
		}
		return i, cols, nil
	}
	return 0, nil, fmt.Errorf("aucune ligne d'années trouvée")
}

func IngestSuisse(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := run(ctx, pool, arch, SourceAFF, readAFF(ctx, arch)); err != nil {
		return err
	}
	return run(ctx, pool, arch, SourceBNS, readBNS(ctx, arch))
}

func readAFF(ctx context.Context, arch *archive.Archive) func(srcID, runID int64) (*batch, error) {
	return func(srcID, runID int64) (*batch, error) {
		l := newBatch()
		for _, b := range affBilans {
			url := affBase + b.file
			f, err := arch.Fetch(ctx, srcID, runID, url, ".xlsx")
			if err != nil {
				return nil, err
			}
			x, err := openXLSX(f.Path)
			if err != nil {
				return nil, fmt.Errorf("%s : %w", b.file, err)
			}
			rows, err := x.rows("bilanz")
			x.Close()
			if err != nil {
				return nil, fmt.Errorf("%s : %w", b.file, err)
			}
			header, cols, err := yearHeader(rows)
			if err != nil {
				return nil, fmt.Errorf("%s : %w", b.file, err)
			}
			// L'unité est écrite dans la cellule d'angle ; si elle change, le
			// multiplicateur change aussi, et on préfère l'apprendre ici.
			if u := strings.NewReplacer("\u00a0", " ", "\u202f", " ").Replace(rows[header]["B"]); u != "1 000 CHF" {
				return nil, fmt.Errorf("%s : unité %q, attendu « 1 000 CHF »", b.file, u)
			}
			name := strings.TrimSuffix(b.file, "-f.xlsx")
			for _, ln := range rows[header+1:] {
				compte, libelle := ln["A"], ln["B"]
				if compte == "" || libelle == "" {
					continue
				}
				s := &Series{
					Code: "aff:bilanz:" + name + ":" + compte, CodeSource: b.file + "#bilanz!" + compte,
					Libelle: compte + " " + libelle, Pays: "CH", Frequence: "A", Unite: "CHF",
					Concept: "BILAN_APU_CH", Mesure: "ENCOURS", SecteurEmetteur: b.sector,
					Instrument: compte, URL: url,
				}
				l.add(s, annualValues(ln, cols, 1000, f.DocumentID))
			}
		}

		url := affBase + "finanzkennz-f.xlsx"
		f, err := arch.Fetch(ctx, srcID, runID, url, ".xlsx")
		if err != nil {
			return nil, err
		}
		x, err := openXLSX(f.Path)
		if err != nil {
			return nil, err
		}
		defer x.Close()
		for sheet, sector := range affIndicators {
			rows, err := x.rows(sheet)
			if err != nil {
				return nil, fmt.Errorf("finanzkennz : %w", err)
			}
			header, cols, err := yearHeader(rows)
			if err != nil {
				return nil, fmt.Errorf("finanzkennz/%s : %w", sheet, err)
			}
			for _, ln := range rows[header+1:] {
				libelle := ln["B"]
				if libelle == "" {
					continue
				}
				s := &Series{
					Code: "aff:finanzkennz:" + sheet + ":" + slug(libelle), CodeSource: "finanzkennz-f.xlsx#" + sheet,
					Libelle: libelle, Pays: "CH", Frequence: "A", Unite: "RATIO",
					Concept: "INDICATEUR_AFF", Mesure: "RATIO", SecteurEmetteur: sector, URL: url,
				}
				l.add(s, annualValues(ln, cols, 1, f.DocumentID))
			}
		}
		return l, nil
	}
}

func annualValues(ln map[string]string, cols map[string]string, mult float64, doc int64) []Obs {
	var obs []Obs
	for c, year := range cols {
		v, err := strconv.ParseFloat(ln[c], 64)
		if err != nil {
			continue // cellule vide : l'AFF ne calcule pas l'indicateur cette année-là
		}
		obs = append(obs, Obs{Periode: year, Valeur: v * mult, DocumentID: doc})
	}
	return obs
}

// slug : identifiant stable tiré d'un libellé (« Quotient d'endettement net »
// -> « quotient-d-endettement-net »).
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

const bnsURL = "https://data.snb.ch/api/cube/rendoblim/data/csv/fr"

func readBNS(ctx context.Context, arch *archive.Archive) func(srcID, runID int64) (*batch, error) {
	return func(srcID, runID int64) (*batch, error) {
		f, err := arch.Fetch(ctx, srcID, runID, bnsURL, ".csv")
		if err != nil {
			return nil, err
		}
		fh, err := os.Open(f.Path)
		if err != nil {
			return nil, err
		}
		defer fh.Close()
		var obs []Obs
		publication := ""
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			fields := strings.Split(strings.TrimPrefix(sc.Text(), "\uFEFF"), ";")
			for i := range fields {
				fields[i] = strings.Trim(fields[i], `"`)
			}
			if len(fields) == 2 && fields[0] == "PublishingDate" {
				publication = fields[1]
			}
			if len(fields) != 3 || fields[1] != "10J" {
				continue
			}
			v, err := strconv.ParseFloat(fields[2], 64)
			if err != nil {
				continue // mois sans cotation
			}
			obs = append(obs, Obs{Periode: fields[0], Valeur: v, DocumentID: f.DocumentID})
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
		l := newBatch()
		l.add(&Series{
			Code: "bns:rendoblim:10J", CodeSource: "rendoblim/10J",
			Libelle: "Rendement des obligations de la Confédération à 10 ans", Pays: "CH", Frequence: "M",
			Unite: "PCT", Concept: "TAUX_LONG_TERME", Mesure: "TAUX", SecteurEmetteur: "S1311",
			Notes: "Date de publication du cube : " + publication, URL: bnsURL,
		}, obs)
		return l, nil
	}
}
