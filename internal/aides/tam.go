package aides

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le registre européen de transparence des aides d'État (Transparency Award
// Module). La Commission n'en publie ni API ni fichier complet : la recherche
// publique passe par un formulaire (pays, puis filtres), dont la page de
// résultats propose un export CSV lié à la session. Le connecteur reproduit ce
// parcours, avec l'accord explicite du responsable du projet (14 septembre
// 2026) : les requêtes d'un visiteur, un client qui s'identifie, une pause de
// deux secondes entre chacune, aucune protection contournée — le site n'en
// présente pas.
var SourceTAM = archive.Source{
	Slug: "ue-tam-aides-etat-france", Label: "Commission européenne — registre de transparence des aides d'État (TAM), France",
	Publisher: "Commission européenne, DG Concurrence (données déclarées par les autorités françaises)", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Avis de réutilisation de la Commission européenne : réutilisation avec mention de la source",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : Commission européenne, State Aid Transparency Public Search",
	Cadence:     "continue (publication dans les 6 à 12 mois suivant l'octroi)",
	Notes: "Seuil de publication : 500 k€ (RGEC 2014), 100 k€ après la révision de 2023 et pour les " +
		"encadrements temporaires Covid et Ukraine. Le type « PME / grande entreprise » est DÉCLARÉ par " +
		"l'autorité et souvent faux : le confronter à la catégorie INSEE. Avantages fiscaux publiés par " +
		"tranches de montant. Montant nominal souvent vide : l'élément d'aide (ESB) est la colonne complète. " +
		"Export récupéré trimestre par trimestre de date d'octroi ; le nombre de lignes est contrôlé contre la " +
		"pagination de la page de résultats.",
}

// Au-delà d'environ 1 000 lignes (951 servies, 1 021 refusées le 14 septembre
// 2026), l'export est proposé par courriel seulement.
const (
	seuilExportTAM  = 950
	cibleTrancheTAM = 800
)

const tamBase = "https://webgate.ec.europa.eu/competition/transparency/public"

var (
	reJeton     = regexp.MustCompile(`name="CSRFTOKEN" value="([^"]+)"`)
	rePage      = regexp.MustCompile(`search/results\?offset=(\d+)&amp;max=(\d+)`)
	reAideLigne = regexp.MustCompile(`/public/aidAward/show/\d+`)
)

type tamSession struct {
	client *http.Client
	pause  time.Duration
}

// requete tente jusqu'à quatre fois, avec une attente croissante : sur
// plusieurs centaines de requêtes, une coupure réseau passagère est certaine,
// et elle ne doit pas coûter tout le parcours.
func (s *tamSession) requete(ctx context.Context, methode, u string, form url.Values) (string, error) {
	var dernier error
	for essai := 1; essai <= 4; essai++ {
		page, reessayer, err := s.requeteUne(ctx, methode, u, form)
		if err == nil || !reessayer {
			return page, err
		}
		dernier = err
		fmt.Printf("    %v — nouvel essai dans %ds\n", err, 15*essai)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(15*essai) * time.Second):
		}
	}
	return "", dernier
}

func (s *tamSession) requeteUne(ctx context.Context, methode, u string, form url.Values) (string, bool, error) {
	time.Sleep(s.pause)
	var corps io.Reader
	if form != nil {
		corps = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, methode, u, corps)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("User-Agent", "faits-politiques.fr (ingestion open data)")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", true, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", true, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode >= 500, fmt.Errorf("%s %s : HTTP %d", methode, u, resp.StatusCode)
	}
	return string(b), false, nil
}

func jeton(page string) (string, error) {
	m := reJeton.FindStringSubmatch(page)
	if m == nil {
		return "", fmt.Errorf("jeton CSRF introuvable : la page du registre a changé")
	}
	return m[1], nil
}

// rechercher lance la recherche France sur une période de date d'octroi et
// renvoie le nombre de résultats annoncé par la pagination : [min, max].
func (s *tamSession) rechercher(ctx context.Context, debut, fin time.Time) (int, int, error) {
	accueil, err := s.requete(ctx, http.MethodGet, tamBase+"?lang=en", nil)
	if err != nil {
		return 0, 0, err
	}
	tok, err := jeton(accueil)
	if err != nil {
		return 0, 0, err
	}
	etape, err := s.requete(ctx, http.MethodPost, tamBase+"/search", url.Values{
		"CSRFTOKEN": {tok}, "resetSearch": {"true"}, "_selectAll": {"on"}, "_countries": {"on"}, "countries": {"CountryFRA"},
	})
	if err != nil {
		return 0, 0, err
	}
	if tok, err = jeton(etape); err != nil {
		return 0, 0, err
	}
	res, err := s.requete(ctx, http.MethodPost, tamBase+"/search/results", url.Values{
		"CSRFTOKEN": {tok}, "resetSearch": {"true"}, "currency": {"EUR"},
		"dateGrantedFrom": {debut.Format("02/01/2006")}, "dateGrantedTo": {fin.Format("02/01/2006")},
	})
	if err != nil {
		return 0, 0, err
	}
	lignes := len(reAideLigne.FindAllString(res, -1))
	dernier, taille := -1, 10
	for _, m := range rePage.FindAllStringSubmatch(res, -1) {
		o, _ := strconv.Atoi(m[1])
		t, _ := strconv.Atoi(m[2])
		if o > dernier {
			dernier, taille = o, t
		}
	}
	if dernier < 0 {
		// Une seule page : le nombre de liens de détail est le nombre exact.
		return lignes, lignes, nil
	}
	return dernier + 1, dernier + taille, nil
}

var enTeteTAM = []string{"Country", "Another Beneficiary Member State", "Aid Measure Title", "Aid Measure Title [EN]",
	"SA.Number", "Ref-no.", "National ID", "Name of the beneficiary", "Name of the beneficiary [EN]", "Beneficiary Type",
	"Region", "Sector (NACE)", "Aid Instrument", "Aid Instrument [EN]", "Objectives of the Aid", "Objectives of the Aid [EN]",
	"Nominal Amount, expressed as full amount", "Aid element, expressed as full amount", "Currency", "Date of granting",
	"Granting Authority Name", "Granting Authority Name [EN]", "Published Date", "Entrusted Entity",
	"Financial Intermediaries", "Third country outside of the EU"}

// trancheTAM lit les montants publiés par tranches : « 500,000 - 1,000,000 »,
// « > 1,000,000 - 2,000,000 », et les tranches ouvertes « > 30,000,000 »
// (minimum sans maximum) ou « < 500,000 » (maximum sans minimum).
func trancheTAM(s string) (*float64, *float64, bool) {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, ">") && !strings.Contains(s, " - "):
		v, ok := montantPoint(strings.TrimPrefix(s, ">"))
		return v, nil, ok && v != nil
	case strings.HasPrefix(s, "<"):
		v, ok := montantPoint(strings.TrimPrefix(s, "<"))
		return nil, v, ok && v != nil
	}
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(s, ">")), " - ")
	if len(parts) != 2 {
		return nil, nil, false
	}
	a, ok1 := montantPoint(parts[0])
	b, ok2 := montantPoint(parts[1])
	if !ok1 || !ok2 || a == nil || b == nil {
		return nil, nil, false
	}
	return a, b, true
}

func dateTAM(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("02/01/2006", s)
	if err != nil {
		return nil, fmt.Errorf("date illisible %q", s)
	}
	return &t, nil
}

// anomaliesTAM recense les valeurs qu'on ne sait pas lire. Le parcours
// complet du registre prend une vingtaine de minutes : échouer à la première
// valeur inattendue obligerait à le refaire autant de fois qu'il y a de
// formats inconnus. On lit tout, on compte, et on échoue une fois, à la fin,
// avec la liste complète — sans rien charger.
type anomaliesTAM map[string]int

// typesTAM : le registre publie le type de bénéficiaire dans la langue de
// saisie de l'autorité.
var typesTAM = map[string]string{
	"small and medium-sized entreprises": "PME", "small and medium-sized enterprises": "PME", "sme": "PME",
	"petites et moyennes entreprises": "PME", "pme": "PME",
	"only large enterprises": "GRANDE_ENTREPRISE", "large enterprises": "GRANDE_ENTREPRISE",
	"grandes entreprises": "GRANDE_ENTREPRISE", "uniquement les grandes entreprises": "GRANDE_ENTREPRISE",
	"small mid-caps": "PETITE_ETI", "petites entreprises à moyenne capitalisation": "PETITE_ETI",
}

func lireExportTAM(path string, doc int64, anomalies anomaliesTAM) ([]aide, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cr := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))))
	cr.FieldsPerRecord = -1
	recs, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("export vide, sans en-tête")
	}
	if strings.Join(recs[0], "|") != strings.Join(enTeteTAM, "|") {
		return nil, fmt.Errorf("en-tête de l'export modifié : %v", recs[0])
	}
	var out []aide
	for i, r := range recs[1:] {
		if len(r) != len(enTeteTAM) {
			return nil, fmt.Errorf("ligne %d : %d colonnes", i+2, len(r))
		}
		if r[18] != "" && r[18] != "EUR" {
			anomalies["devise "+r[18]]++
			continue
		}
		a := aide{reference: r[5], identifiant: r[6], nom: r[7], regime: r[4], intitule: r[2], instrument: r[12],
			objectif: r[14], secteur: r[11], region: r[10], autorite: r[20], document: doc}
		if t := strings.TrimSpace(r[9]); t != "" {
			if v, ok := typesTAM[strings.ToLower(t)]; ok {
				a.typeDeclare = v
			} else {
				anomalies["type de bénéficiaire "+t]++
			}
		}
		// Chaque montant est soit un nombre, soit une tranche (avantages
		// fiscaux). Une tranche n'est jamais ramenée à un point.
		for j, champ := range []int{16, 17} {
			v := strings.TrimSpace(r[champ])
			if v == "" {
				continue
			}
			if m, ok := montantPoint(v); ok {
				if j == 0 {
					a.nominal = m
				} else {
					a.esb = m
				}
				continue
			}
			lo, hi, ok := trancheTAM(v)
			if !ok {
				anomalies["montant "+v]++
				continue
			}
			if j == 1 || (a.trancheMin == nil && a.trancheMax == nil) {
				a.trancheMin, a.trancheMax = lo, hi
			}
		}
		if a.dateOctroi, err = dateTAM(r[19]); err != nil {
			anomalies[err.Error()]++
		}
		if a.datePublication, err = dateTAM(r[22]); err != nil {
			anomalies[err.Error()]++
		}
		if a.reference == "" {
			return nil, fmt.Errorf("ligne %d : référence vide", i+2)
		}
		out = append(out, a)
	}
	return out, nil
}

// exportParCourriel reconnaît la réponse JSON qui remplace le CSV quand
// l'export est trop volumineux pour être servi directement.
func exportParCourriel(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	debut := make([]byte, 4096)
	n, _ := io.ReadFull(f, debut)
	return bytes.HasPrefix(bytes.TrimSpace(debut[:n]), []byte("{")) &&
		bytes.Contains(debut[:n], []byte("userCriteriaToExportCsvCommand"))
}

func IngestTAM(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executerAides(ctx, pool, arch, SourceTAM, "TAM", func(srcID, runID int64) ([]aide, map[string]any, error) {
		jar, _ := cookiejar.New(nil)
		s := &tamSession{client: &http.Client{Jar: jar, Timeout: 10 * time.Minute}, pause: 2 * time.Second}

		var toutes []aide
		periodes, repris := 0, 0
		anomalies := anomaliesTAM{}
		// Le registre existe depuis le 1er juillet 2016 ; on part de 2014 pour
		// les octrois antérieurs publiés tardivement.
		var charger func(debut, fin time.Time) error
		charger = func(debut, fin time.Time) error {
			exportURL := tamBase + "/search/export?format=CSV"
			archivee := fmt.Sprintf("%s#pays=FRA&octroi=%s..%s", exportURL, debut.Format("2006-01-02"), fin.Format("2006-01-02"))
			// Reprise : un export de cette période exacte, scellé depuis moins de
			// deux jours et lisible, est réutilisé sans rien redemander au
			// registre. Le contrôle contre la pagination a été fait lors du
			// scellement.
			var cle string
			var docID int64
			if err := pool.QueryRow(ctx, `
				SELECT d.storage_key, d.id FROM raw.retrieval r JOIN raw.document d ON d.id = r.document_id
				WHERE r.source_id = $1 AND r.url = $2 AND r.fetched_at > now() - interval '2 days'
				  AND d.content_type LIKE 'text/csv%'
				ORDER BY r.id DESC LIMIT 1`, srcID, archivee).Scan(&cle, &docID); err == nil {
				if aides, err := lireExportTAM(filepath.Join(arch.Root, cle), docID, anomalies); err == nil {
					periodes++
					repris++
					toutes = append(toutes, aides...)
					return nil
				}
			}
			min, max, err := s.rechercher(ctx, debut, fin)
			if err != nil {
				return err
			}
			if max == 0 {
				return nil
			}
			// Le registre n'exporte directement que jusqu'à un millier de lignes
			// environ. Le nombre de résultats étant connu, on découpe d'emblée
			// en tranches d'environ 800 aides, plutôt que d'essuyer un refus
			// d'export par niveau de découpage.
			if max > seuilExportTAM && fin.After(debut) {
				parts := (max + cibleTrancheTAM - 1) / cibleTrancheTAM
				jours := int(fin.Sub(debut).Hours()/24) + 1
				if parts > jours {
					parts = jours
				}
				pas := jours / parts
				fmt.Printf("    %s..%s : %d-%d aides, découpage en %d\n", debut.Format("2006-01-02"), fin.Format("2006-01-02"), min, max, parts)
				for k := 0; k < parts; k++ {
					d := debut.AddDate(0, 0, k*pas)
					f := d.AddDate(0, 0, pas-1)
					if k == parts-1 {
						f = fin
					}
					if err := charger(d, f); err != nil {
						return err
					}
				}
				return nil
			}
			f, err := arch.FetchSession(ctx, srcID, runID, exportURL, archivee, ".csv", s.client)
			if err != nil {
				return err
			}
			// Au-delà d'un certain volume, le registre ne sert plus l'export : il
			// répond par un formulaire (prénom, nom, e-mail) pour l'envoyer par
			// courriel. On ne le remplit pas — pas de données personnelles
			// saisies, et un envoi par courriel ne s'automatise pas : la période
			// est découpée jusqu'à redevenir exportable directement.
			trop := exportParCourriel(f.Path)
			var aides []aide
			if !trop {
				if aides, err = lireExportTAM(f.Path, f.DocumentID, anomalies); err != nil {
					return fmt.Errorf("export %s : %w", archivee, err)
				}
			}
			if trop || len(aides) < min || len(aides) > max {
				// Export tronqué ou pagination trompeuse : on découpe la période
				// en deux plutôt que de charger un trimestre incomplet.
				if !fin.After(debut) {
					if trop {
						return fmt.Errorf("export %s : une seule journée (%d à %d aides) dépasse le volume exportable sans courriel", archivee, min, max)
					}
					return fmt.Errorf("export %s : %d lignes pour %d à %d annoncées", archivee, len(aides), min, max)
				}
				milieu := debut.Add(fin.Sub(debut) / 2).Truncate(24 * time.Hour)
				motif := fmt.Sprintf("%d lignes pour %d-%d annoncées", len(aides), min, max)
				if trop {
					motif = fmt.Sprintf("%d-%d aides : export envoyé par courriel seulement", min, max)
				}
				fmt.Printf("    %s..%s : %s, découpage\n", debut.Format("2006-01-02"), fin.Format("2006-01-02"), motif)
				if err := charger(debut, milieu); err != nil {
					return err
				}
				return charger(milieu.AddDate(0, 0, 1), fin)
			}
			periodes++
			fmt.Printf("    %s..%s : %d aides\n", debut.Format("2006-01-02"), fin.Format("2006-01-02"), len(aides))
			toutes = append(toutes, aides...)
			return nil
		}
		aujourdhui := time.Now().UTC()
		for d := time.Date(2014, 1, 1, 0, 0, 0, 0, time.UTC); d.Before(aujourdhui); d = d.AddDate(0, 3, 0) {
			if err := charger(d, d.AddDate(0, 3, -1)); err != nil {
				return nil, nil, err
			}
		}
		if len(anomalies) > 0 {
			var liste []string
			for k, n := range anomalies {
				liste = append(liste, fmt.Sprintf("%q ×%d", k, n))
			}
			sort.Strings(liste)
			return nil, nil, fmt.Errorf("%d valeurs illisibles, rien n'est chargé : %s", len(liste), strings.Join(liste, " ; "))
		}
		fmt.Printf("    %d périodes, dont %d reprises de l'archive\n", periodes, repris)
		return toutes, map[string]any{"periodes": periodes, "reprises": repris}, nil
	})
}
