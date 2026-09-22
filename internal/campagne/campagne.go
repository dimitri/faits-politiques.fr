// Package campagne charge les comptes de campagne publiés par la Commission
// nationale des comptes de campagne et des financements politiques.
//
// Le jeu contient, pour chaque candidat, ce qu'il a DÉCLARÉ avoir dépensé et
// reçu, et ce que la Commission a RETENU après contrôle. L'écart entre les deux
// est le fait le plus intéressant : il dit ce qui a été rejeté.
//
// Aucun rattachement automatique à une personne n'est fait. Le fichier ne porte
// ni date de naissance ni identifiant, et 41,9 % des élus de notre base ont un
// homonyme exact. Un lien nom-à-nom produirait des attributions fausses sur un
// sujet — le financement — où une erreur est particulièrement coûteuse.
package campagne

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "campagne-v1"

var Source = archive.Source{
	Slug: "cnccfp-campagne", Label: "CNCCFP — comptes de campagne",
	Publisher:   "Commission nationale des comptes de campagne et des financements politiques",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Aucune licence déclarée par le producteur",
	ReuseClass:  "RESTRICTED",
	Attribution: "Source : Commission nationale des comptes de campagne et des financements politiques",
	Cadence:     "par scrutin",
	Notes: "Licence non déclarée : même situation que les résultats municipaux de " +
		"2020. Classé RESTRICTED en attendant une décision explicite ; à ne pas " +
		"reverser dans un export ouvert.",
}

// Un scrutin par fichier. Les URL sont fixées : c'est cette version-là qui est
// scellée dans l'archive.
var scrutins = []struct {
	typeElection string
	annee        int
	url          string
}{
	{"LEGISLATIVE", 2022,
		"https://static.data.gouv.fr/resources/comptes-de-campagne-elections-legislatives-generales-des-12-et-19-juin-2022/20231012-135051/publications-2022-lg.csv"},
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, Source)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_compte (
			type_election text, annee int, candidat_ref text, candidat_nom text,
			circonscription text, departement text, code_departement text, nuance text,
			monnaie text, depenses_declarees numeric, depenses_retenues numeric,
			recettes_declarees numeric, recettes_retenues numeric
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}

	var nComptes, nPostes int
	for _, s := range scrutins {
		f, err := arch.Fetch(ctx, srcID, runID, s.url, ".csv")
		if err != nil {
			return fail(fmt.Errorf("%s %d : %w", s.typeElection, s.annee, err))
		}
		recs, entetes, err := lireCSV(f.Path)
		if err != nil {
			return fail(err)
		}
		if _, err := tx.Exec(ctx, `TRUNCATE tmp_compte`); err != nil {
			return fail(err)
		}
		// MERGE plutôt que DELETE+COPY, scopé au scrutin (type_election, annee)
		// par une vue temporaire : la table est réutilisée par tous les
		// scrutins de la boucle, et l'ancien DELETE payait le prix des
		// triggers RI pour l'intégralité d'UN scrutin (jusqu'à quelques
		// milliers de comptes) à chaque republication, changement ou non.
		// IS NOT DISTINCT FROM sur circonscription : la contrainte
		// d'unicité traite deux NULL comme distincts (comportement standard
		// SQL), mais le MERGE doit au contraire les reconnaître comme « même
		// candidat, pas de circonscription » pour ne pas dupliquer sa ligne
		// à chaque run.
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			CREATE OR REPLACE TEMPORARY VIEW compte_campagne_scope AS
			  SELECT * FROM core.compte_campagne
			   WHERE type_election = %s AND annee = %d
			  WITH LOCAL CHECK OPTION`, quoteLiteral(s.typeElection), s.annee)); err != nil {
			return fail(err)
		}

		var comptes []map[string]string
		var compteRows [][]any
		for _, r := range recs {
			nom := strings.TrimSpace(r["nom"])
			if nom == "" {
				continue
			}
			comptes = append(comptes, r)
			compteRows = append(compteRows, []any{
				s.typeElection, s.annee, nul(r["candidat"]), nom,
				nul(r["circonscription"]), nul(r["département"]), nul(r["code département"]),
				nul(r["nuance"]), nul(r["monnaie"]),
				montant(r["dépenses totales déclarées"]), montant(r["depenses totales retenues"]),
				montant(r["recettes totales déclarées"]), montant(r["recettes totales retenues"]),
			})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_compte"},
			[]string{"type_election", "annee", "candidat_ref", "candidat_nom", "circonscription",
				"departement", "code_departement", "nuance", "monnaie", "depenses_declarees",
				"depenses_retenues", "recettes_declarees", "recettes_retenues"},
			pgx.CopyFromRows(compteRows)); err != nil {
			return fail(err)
		}
		// Pas de RETURNING : il n'émettrait une ligne que pour un compte dont
		// l'UPDATE a réellement changé quelque chose, laissant les comptes
		// inchangés hors de la carte candidat -> id. Un SELECT séparé après
		// coup, sans dépendre d'un WHEN, la reconstruit en entier.
		if _, err := tx.Exec(ctx, `
			MERGE INTO compte_campagne_scope AS tgt
			USING tmp_compte AS src
			ON tgt.candidat_nom = src.candidat_nom
			   AND tgt.circonscription IS NOT DISTINCT FROM src.circonscription
			WHEN MATCHED AND (tgt.candidat_ref, tgt.departement, tgt.code_departement, tgt.nuance,
			                   tgt.monnaie, tgt.depenses_declarees, tgt.depenses_retenues,
			                   tgt.recettes_declarees, tgt.recettes_retenues, tgt.source_id)
			                  IS DISTINCT FROM
			                  (src.candidat_ref, src.departement, src.code_departement, src.nuance,
			                   src.monnaie, src.depenses_declarees, src.depenses_retenues,
			                   src.recettes_declarees, src.recettes_retenues, $1) THEN
			    UPDATE SET candidat_ref = src.candidat_ref, departement = src.departement,
			               code_departement = src.code_departement, nuance = src.nuance,
			               monnaie = src.monnaie, depenses_declarees = src.depenses_declarees,
			               depenses_retenues = src.depenses_retenues,
			               recettes_declarees = src.recettes_declarees,
			               recettes_retenues = src.recettes_retenues, source_id = $1
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (type_election, annee, candidat_ref, candidat_nom, circonscription,
			            departement, code_departement, nuance, monnaie,
			            depenses_declarees, depenses_retenues, recettes_declarees, recettes_retenues, source_id)
			    VALUES (src.type_election, src.annee, src.candidat_ref, src.candidat_nom, src.circonscription,
			            src.departement, src.code_departement, src.nuance, src.monnaie,
			            src.depenses_declarees, src.depenses_retenues, src.recettes_declarees,
			            src.recettes_retenues, $1)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`, srcID); err != nil {
			return fail(fmt.Errorf("fusion des comptes : %w", err))
		}
		res, err := tx.Query(ctx, `
			SELECT t.candidat_nom, coalesce(t.circonscription, ''), c.id
			  FROM tmp_compte t
			  JOIN compte_campagne_scope c
			    ON c.candidat_nom = t.candidat_nom
			   AND c.circonscription IS NOT DISTINCT FROM t.circonscription`)
		if err != nil {
			return fail(err)
		}
		idParCandidat := map[[2]string]int64{}
		for res.Next() {
			var nom, circo string
			var id int64
			if err := res.Scan(&nom, &circo, &id); err != nil {
				res.Close()
				return fail(err)
			}
			idParCandidat[[2]string{nom, circo}] = id
		}
		res.Close()
		if err := res.Err(); err != nil {
			return fail(err)
		}
		nComptes += len(idParCandidat)

		// Les postes : toute colonne suffixée « (déclaré) » ou « (retenu) ».
		// Le libellé est conservé tel quel, sans regroupement. Un compte en
		// doublon (nom+circonscription déjà pris) n'a pas d'id : ses postes
		// sont ignorés, comme avant.
		type clePoste struct {
			id          int64
			poste, etat string
		}
		vusPostes := map[clePoste]bool{}
		var posteRows [][]any
		for _, r := range comptes {
			id, ok := idParCandidat[[2]string{strings.TrimSpace(r["nom"]), strings.TrimSpace(r["circonscription"])}]
			if !ok {
				continue
			}
			for _, h := range entetes {
				var etat string
				var poste string
				switch {
				case strings.HasSuffix(h, "(déclaré)"):
					etat, poste = "DECLARE", strings.TrimSpace(strings.TrimSuffix(h, "(déclaré)"))
				case strings.HasSuffix(h, "(retenu)"):
					etat, poste = "RETENU", strings.TrimSpace(strings.TrimSuffix(h, "(retenu)"))
				default:
					continue
				}
				m := montant(r[h])
				if m == nil {
					continue
				}
				// Un en-tête dupliqué dans le CSV source produirait la même
				// clé (compte_id, poste, etat) : la première valeur gagne,
				// comme le faisait l'ON CONFLICT DO NOTHING ligne à ligne.
				cle := clePoste{id, poste, etat}
				if vusPostes[cle] {
					continue
				}
				vusPostes[cle] = true
				posteRows = append(posteRows, []any{id, poste, etat, m})
			}
		}
		// MERGE plutôt que COPY directe : cette table n'était jamais wipée
		// elle-même (elle suivait le CASCADE de la DELETE sur core.
		// compte_campagne ci-dessus), donc son propre passage en MERGE suit
		// la même scope par scrutin, via une vue restreinte aux compte_id de
		// ce scrutin.
		if _, err := tx.Exec(ctx, `
			CREATE TEMP TABLE tmp_compte_poste (
				compte_id bigint, poste text, etat text, montant numeric
			) ON COMMIT DROP`); err != nil {
			return fail(err)
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_compte_poste"},
			[]string{"compte_id", "poste", "etat", "montant"}, pgx.CopyFromRows(posteRows)); err != nil {
			return fail(fmt.Errorf("copie des postes : %w", err))
		}
		if _, err := tx.Exec(ctx, `
			CREATE OR REPLACE TEMPORARY VIEW compte_campagne_poste_scope AS
			  SELECT * FROM core.compte_campagne_poste
			   WHERE compte_id IN (SELECT id FROM compte_campagne_scope)
			  WITH LOCAL CHECK OPTION`); err != nil {
			return fail(err)
		}
		var nTouchees int64
		err = bulkload.SansContraintesFK(ctx, tx, "core.compte_campagne_poste", func() error {
			ct, err := tx.Exec(ctx, `
				MERGE INTO compte_campagne_poste_scope AS tgt
				USING tmp_compte_poste AS src
				ON tgt.compte_id = src.compte_id AND tgt.poste = src.poste AND tgt.etat = src.etat
				WHEN MATCHED AND tgt.montant IS DISTINCT FROM src.montant THEN
				    UPDATE SET montant = src.montant
				WHEN NOT MATCHED BY TARGET THEN
				    INSERT (compte_id, poste, etat, montant)
				    VALUES (src.compte_id, src.poste, src.etat, src.montant)
				WHEN NOT MATCHED BY SOURCE THEN DELETE`)
			if err != nil {
				return err
			}
			nTouchees = ct.RowsAffected()
			return nil
		})
		if err != nil {
			return fail(fmt.Errorf("fusion des postes : %w", err))
		}
		nPostes += len(posteRows)
		logs.Notice(fmt.Sprintf("campaign accounts %s %d: %s, %d line items touched by the merge",
			s.typeElection, s.annee, logs.Plural(len(idParCandidat), "account"), nTouchees))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"comptes": nComptes, "postes": nPostes}, "")
	logs.Notice(fmt.Sprintf("campaign accounts done: %s, %s",
		logs.Plural(nComptes, "account"), logs.Plural(nPostes, "line item")))
	return nil
}

func lireCSV(path string) ([]map[string]string, []string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	texte := string(b)
	if !utf8.Valid(b) {
		r := make([]rune, len(b))
		for i, c := range b {
			r[i] = rune(c)
		}
		texte = string(r)
	}
	rd := csv.NewReader(strings.NewReader(texte))
	rd.Comma = ';'
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	recs, err := rd.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("%s : %w", path, err)
	}
	if len(recs) < 2 {
		return nil, nil, fmt.Errorf("%s : aucun compte", path)
	}
	head := make([]string, len(recs[0]))
	for i, h := range recs[0] {
		head[i] = strings.TrimSpace(strings.Trim(h, "\ufeff\""))
	}
	out := make([]map[string]string, 0, len(recs)-1)
	for _, rec := range recs[1:] {
		m := make(map[string]string, len(head))
		for i, h := range head {
			if i < len(rec) {
				m[h] = strings.TrimSpace(rec[i])
			}
		}
		out = append(out, m)
	}
	return out, head, nil
}

func montant(s string) any {
	s = strings.NewReplacer(" ", "", " ", "", "€", "", ",", ".").Replace(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return v
}

func nul(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// quoteLiteral échappe un littéral SQL. N'est appelé que sur s.typeElection,
// une constante Go du tableau scrutins ci-dessus — jamais sur une donnée
// venue du fichier source — mais une vue temporaire ne peut pas se
// paramétrer autrement qu'en construisant son texte.
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
