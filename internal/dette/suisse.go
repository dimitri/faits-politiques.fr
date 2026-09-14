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
var affBilans = []struct{ fichier, secteur string }{
	{"bund_ktn_gdn-f.xlsx", "S13_HORS_S1314"},
	{"bund-f.xlsx", "S1311"},
}

var affIndicateurs = map[string]string{
	"staat": "S13", "bund": "S1311", "ktn": "S1312", "gdn": "S1313", "sv": "S1314",
}

var reAnnee = regexp.MustCompile(`^(19|20)[0-9]{2}$`)

// enTeteAnnees trouve la ligne dont les cellules à partir de C sont des
// années, et renvoie la correspondance colonne -> année avec l'index de ligne.
func enTeteAnnees(lignes []map[string]string) (int, map[string]string, error) {
	for i, l := range lignes {
		if !reAnnee.MatchString(l["C"]) {
			continue
		}
		cols := map[string]string{}
		for c, v := range l {
			if reAnnee.MatchString(v) {
				cols[c] = v
			}
		}
		return i, cols, nil
	}
	return 0, nil, fmt.Errorf("aucune ligne d'années trouvée")
}

func IngestSuisse(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := executer(ctx, pool, arch, SourceAFF, lireAFF(ctx, arch)); err != nil {
		return err
	}
	return executer(ctx, pool, arch, SourceBNS, lireBNS(ctx, arch))
}

func lireAFF(ctx context.Context, arch *archive.Archive) func(srcID, runID int64) (*lot, error) {
	return func(srcID, runID int64) (*lot, error) {
		l := nouveauLot()
		for _, b := range affBilans {
			url := affBase + b.fichier
			f, err := arch.Fetch(ctx, srcID, runID, url, ".xlsx")
			if err != nil {
				return nil, err
			}
			x, err := openXLSX(f.Path)
			if err != nil {
				return nil, fmt.Errorf("%s : %w", b.fichier, err)
			}
			lignes, err := x.rows("bilanz")
			x.Close()
			if err != nil {
				return nil, fmt.Errorf("%s : %w", b.fichier, err)
			}
			entete, cols, err := enTeteAnnees(lignes)
			if err != nil {
				return nil, fmt.Errorf("%s : %w", b.fichier, err)
			}
			// L'unité est écrite dans la cellule d'angle ; si elle change, le
			// multiplicateur change aussi, et on préfère l'apprendre ici.
			if u := strings.NewReplacer("\u00a0", " ", "\u202f", " ").Replace(lignes[entete]["B"]); u != "1 000 CHF" {
				return nil, fmt.Errorf("%s : unité %q, attendu « 1 000 CHF »", b.fichier, u)
			}
			nom := strings.TrimSuffix(b.fichier, "-f.xlsx")
			for _, ln := range lignes[entete+1:] {
				compte, libelle := ln["A"], ln["B"]
				if compte == "" || libelle == "" {
					continue
				}
				s := &Serie{
					Code: "aff:bilanz:" + nom + ":" + compte, CodeSource: b.fichier + "#bilanz!" + compte,
					Libelle: compte + " " + libelle, Pays: "CH", Frequence: "A", Unite: "CHF",
					Concept: "BILAN_APU_CH", Mesure: "ENCOURS", SecteurEmetteur: b.secteur,
					Instrument: compte, URL: url,
				}
				l.ajouter(s, valeursAnnuelles(ln, cols, 1000, f.DocumentID))
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
		for feuille, secteur := range affIndicateurs {
			lignes, err := x.rows(feuille)
			if err != nil {
				return nil, fmt.Errorf("finanzkennz : %w", err)
			}
			entete, cols, err := enTeteAnnees(lignes)
			if err != nil {
				return nil, fmt.Errorf("finanzkennz/%s : %w", feuille, err)
			}
			for _, ln := range lignes[entete+1:] {
				libelle := ln["B"]
				if libelle == "" {
					continue
				}
				s := &Serie{
					Code: "aff:finanzkennz:" + feuille + ":" + slug(libelle), CodeSource: "finanzkennz-f.xlsx#" + feuille,
					Libelle: libelle, Pays: "CH", Frequence: "A", Unite: "RATIO",
					Concept: "INDICATEUR_AFF", Mesure: "RATIO", SecteurEmetteur: secteur, URL: url,
				}
				l.ajouter(s, valeursAnnuelles(ln, cols, 1, f.DocumentID))
			}
		}
		return l, nil
	}
}

func valeursAnnuelles(ln map[string]string, cols map[string]string, mult float64, doc int64) []Obs {
	var obs []Obs
	for c, annee := range cols {
		v, err := strconv.ParseFloat(ln[c], 64)
		if err != nil {
			continue // cellule vide : l'AFF ne calcule pas l'indicateur cette année-là
		}
		obs = append(obs, Obs{Periode: annee, Valeur: v * mult, DocumentID: doc})
	}
	return obs
}

// slug : identifiant stable tiré d'un libellé (« Quotient d'endettement net »
// -> « quotient-d-endettement-net »).
func slug(s string) string {
	var b strings.Builder
	tiret := false
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			tiret = false
		default:
			if !tiret && b.Len() > 0 {
				b.WriteByte('-')
				tiret = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

const bnsURL = "https://data.snb.ch/api/cube/rendoblim/data/csv/fr"

func lireBNS(ctx context.Context, arch *archive.Archive) func(srcID, runID int64) (*lot, error) {
	return func(srcID, runID int64) (*lot, error) {
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
			champs := strings.Split(strings.TrimPrefix(sc.Text(), "\uFEFF"), ";")
			for i := range champs {
				champs[i] = strings.Trim(champs[i], `"`)
			}
			if len(champs) == 2 && champs[0] == "PublishingDate" {
				publication = champs[1]
			}
			if len(champs) != 3 || champs[1] != "10J" {
				continue
			}
			v, err := strconv.ParseFloat(champs[2], 64)
			if err != nil {
				continue // mois sans cotation
			}
			obs = append(obs, Obs{Periode: champs[0], Valeur: v, DocumentID: f.DocumentID})
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
		l := nouveauLot()
		l.ajouter(&Serie{
			Code: "bns:rendoblim:10J", CodeSource: "rendoblim/10J",
			Libelle: "Rendement des obligations de la Confédération à 10 ans", Pays: "CH", Frequence: "M",
			Unite: "PCT", Concept: "TAUX_LONG_TERME", Mesure: "TAUX", SecteurEmetteur: "S1311",
			Notes: "Date de publication du cube : " + publication, URL: bnsURL,
		}, obs)
		return l, nil
	}
}
