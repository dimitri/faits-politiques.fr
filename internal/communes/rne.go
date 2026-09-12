package communes

import (
	"context"
	"fmt"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le RNE dit QUI détient quel mandat, et rien de plus : aucune nuance politique
// (D-022). C'est pourtant la seule source qui couvre tous les niveaux d'un coup
// — du conseiller municipal au député européen — et donc la seule qui permette
// de dresser la liste des mandats actuels d'une personne sans la recomposer
// source par source.
var SourceRNE = archive.Source{
	Slug: "rne", Label: "Répertoire national des élus",
	Publisher: "Ministère de l'Intérieur / DILA", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : Répertoire national des élus, ministère de l'Intérieur",
	Cadence:     "trimestrielle",
	Notes: "Ne contient AUCUNE nuance politique. Ne porte que la mandature en " +
		"cours : aucune profondeur historique.",
}

const rneBase = "https://static.data.gouv.fr/resources/repertoire-national-des-elus-1/"

// Les sept fichiers du répertoire, avec le type de mandat correspondant et la
// colonne qui situe le mandat dans l'espace. Les URL portent l'horodatage de
// la publication : c'est cette version-là qui est scellée, pas « la dernière ».
var rneFichiers = []struct {
	url, mandateType, colonneCommune, colonneCirco string
}{
	{rneBase + "20260811-155100/elus-maire-mai.csv", "MAIRE", "Code de la commune", ""},
	{rneBase + "20260811-154802/elus-conseiller-municipal-cm.csv", "CONSEILLER_MUNICIPAL", "Code de la commune", ""},
	{rneBase + "20260811-154854/elus-conseiller-communautaire-epci.csv", "CONSEILLER_COMMUNAUTAIRE", "", "Code de la commune"},
	{rneBase + "20260811-154909/elus-conseiller-departemental-cd.csv", "CONSEILLER_DEPARTEMENTAL", "", "Code du canton"},
	{rneBase + "20260811-154932/elus-conseiller-regional-cr.csv", "CONSEILLER_REGIONAL", "", "Code de la région"},
	{rneBase + "20260811-155016/elus-senateur-sen.csv", "SENATEUR", "", "Code du département"},
	{rneBase + "20260811-155035/elus-depute-dep.csv", "DEPUTE", "", "Code de la circonscription législative"},
	{rneBase + "20260811-155000/elus-representant-parlement-europeen-rpe.csv", "DEPUTE_EUROPEEN", "", ""},
}

func IngestRNE(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceRNE)
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

	// Le fichier des conseillers municipaux compte à lui seul un demi-million
	// de lignes. Une boucle qui interrogerait la base par ligne ferait plus
	// d'un million d'allers-retours ; tout se fait donc en deux temps : un COPY
	// des lignes brutes dans une table temporaire, puis des requêtes
	// ensemblistes qui ne touchent la base qu'une fois chacune.
	var lignes [][]any
	for _, f := range rneFichiers {
		fetched, err := arch.Fetch(ctx, srcID, runID, f.url, ".csv")
		if err != nil {
			return fail(err)
		}
		recs, err := lireCSV(fetched.Path, ';')
		if err != nil {
			return fail(err)
		}
		for _, r := range recs {
			nom, prenom := r["Nom de l'élu"], r["Prénom de l'élu"]
			debut := premierNonVide(r["Date de début de la fonction"], r["Date de début du mandat"])
			if nom == "" || prenom == "" || debut == "" {
				continue
			}
			var commune, circo any
			if f.colonneCommune != "" {
				if c := r[f.colonneCommune]; c != "" {
					commune = c
				}
			}
			if f.colonneCirco != "" {
				libelle := r[strings.Replace(f.colonneCirco, "Code", "Libellé", 1)]
				if v := strings.TrimSpace(r[f.colonneCirco] + " " + libelle); v != "" {
					circo = v
				}
			}
			lignes = append(lignes, []any{
				f.mandateType, commune, circo, nom, prenom,
				nul(r["Date de naissance"]), debut, nul(r["Libellé de la fonction"]),
				nul(r["Code de la catégorie socio-professionnelle"]),
				nul(r["Libellé de la catégorie socio-professionnelle"]),
			})
		}
		fmt.Printf("    %-26s %6d lignes\n", f.mandateType, len(recs))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE rne_in (
		  mandate_type text, commune_code text, constituency text,
		  nom text, prenom text, naissance date, debut date, fonction text,
		  csp_code text, csp_libelle text
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"rne_in"},
		[]string{"mandate_type", "commune_code", "constituency", "nom", "prenom",
			"naissance", "debut", "fonction", "csp_code", "csp_libelle"},
		pgx.CopyFromRows(lignes)); err != nil {
		return fail(fmt.Errorf("copie des élus : %w", err))
	}

	// Les élus de communes absentes du COG sont écartés, pas rattachés de force.
	var horsCOG int64
	if err := tx.QueryRow(ctx, `
		WITH d AS (
		  DELETE FROM rne_in r
		   WHERE r.commune_code IS NOT NULL
		     AND NOT EXISTS (SELECT 1 FROM ref.commune c
		                     WHERE c.code_insee = r.commune_code
		                       AND c.cog_millesime = $1)
		  RETURNING 1)
		SELECT count(*) FROM d`, COGMillesime).Scan(&horsCOG); err != nil {
		return fail(err)
	}

	// La clé naturelle : le RNE ne publie aucun identifiant d'élu. Elle est
	// lisible et reproductible, et dit exactement sur quoi repose l'identité.
	if _, err := tx.Exec(ctx, `
		ALTER TABLE rne_in ADD COLUMN cle text;
		UPDATE rne_in SET cle = 'RNE:' || core.f_unaccent(lower(nom || '-' || prenom))
		                      || ':' || coalesce(naissance::text, '');
		CREATE INDEX ON rne_in (cle);
		ANALYZE rne_in`); err != nil {
		return fail(err)
	}

	// Reconstruction, pas complétion : un élu remplacé entre deux millésimes du
	// RNE doit disparaître, sinon sa commune finirait avec deux maires
	// simultanés — que la contrainte d'exclusion du schéma ne verrait pas,
	// puisqu'elle porte sur la personne et non sur le territoire.
	if _, err := tx.Exec(ctx, `
		DELETE FROM core.mandate m
		 WHERE m.mandate_type IN ('MAIRE','CONSEILLER_MUNICIPAL','CONSEILLER_COMMUNAUTAIRE',
		                          'CONSEILLER_DEPARTEMENTAL','CONSEILLER_REGIONAL')
		    OR EXISTS (SELECT 1 FROM core.person_identifier i
		               WHERE i.person_id = m.person_id AND i.scheme = 'RNE'
		                 AND m.mandate_type IN ('DEPUTE','SENATEUR','DEPUTE_EUROPEEN'))`); err != nil {
		return fail(err)
	}

	// Réconciliation sur le triplet EXACT (nom, prénom, date de naissance),
	// insensible aux accents et à la casse. C'est assez discriminant pour valoir
	// identifiant, et assez strict pour qu'aucune approximation ne s'y glisse.
	// Sans date de naissance, aucun rapprochement n'est tenté : deux homonymes
	// resteront deux personnes, ce qui est le sens de l'erreur le moins grave.
	//
	// C'est ce rapprochement qui rend la frise possible : un candidat qui fut
	// conseiller municipal, puis député, puis ministre, est UNE personne.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE rne_pers ON COMMIT DROP AS
		  SELECT DISTINCT ON (cle) cle, nom, prenom, naissance FROM rne_in ORDER BY cle;
		ALTER TABLE rne_pers ADD COLUMN person_id bigint;
		CREATE UNIQUE INDEX ON rne_pers (cle)`); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE rne_pers r SET person_id = p.id
		  FROM core.person p
		 WHERE r.naissance IS NOT NULL
		   AND p.birth_date = r.naissance
		   AND core.f_unaccent(lower(p.family_name)) = core.f_unaccent(lower(r.nom))
		   AND core.f_unaccent(lower(p.given_name))  = core.f_unaccent(lower(r.prenom))`); err != nil {
		return fail(err)
	}
	var reconciliees int64
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM rne_pers WHERE person_id IS NOT NULL`).Scan(&reconciliees); err != nil {
		return fail(err)
	}

	if _, err := tx.Exec(ctx, `
		WITH neuves AS (
		  INSERT INTO core.person (slug, family_name, given_name, birth_date)
		  SELECT core.f_unaccent(lower(regexp_replace(prenom || '-' || nom, '[^a-zA-Z0-9]+', '-', 'g')))
		         || '-' || substr(md5(cle), 1, 8),
		         nom, prenom, naissance
		    FROM rne_pers WHERE person_id IS NULL
		  RETURNING id, family_name, given_name, birth_date)
		UPDATE rne_pers r SET person_id = n.id
		  FROM neuves n
		 WHERE r.person_id IS NULL AND r.nom = n.family_name
		   AND r.prenom = n.given_name AND r.naissance IS NOT DISTINCT FROM n.birth_date`); err != nil {
		return fail(fmt.Errorf("création des personnes : %w", err))
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.person_identifier (person_id, scheme, value)
		SELECT person_id, 'RNE', cle FROM rne_pers WHERE person_id IS NOT NULL
		ON CONFLICT (scheme, value) DO NOTHING`); err != nil {
		return fail(err)
	}

	// La catégorie socio-professionnelle, quand la source la publie. coalesce :
	// une valeur déjà présente n'est pas écrasée par une absence.
	if _, err := tx.Exec(ctx, `
		UPDATE core.person p
		   SET csp_code = coalesce(p.csp_code, x.csp_code),
		       csp_libelle = coalesce(p.csp_libelle, x.csp_libelle)
		  FROM (SELECT DISTINCT ON (cle) cle, csp_code, csp_libelle FROM rne_in
		         WHERE csp_libelle IS NOT NULL ORDER BY cle) x
		  JOIN rne_pers r USING (cle)
		 WHERE p.id = r.person_id`); err != nil {
		return fail(err)
	}

	// DISTINCT ON : une personne ne détient qu'un mandat de chaque type. Quand
	// la source en présente deux — un élu inscrit dans deux communes — on garde
	// le plus ancien plutôt que de laisser la contrainte d'exclusion faire
	// échouer tout le chargement.
	//
	// Le NOT EXISTS écarte ce que d'autres connecteurs savent déjà mieux. Un
	// député est publié par l'Assemblée avec ses dates de début ET de fin ; le
	// RNE n'en donne qu'un début. Réinsérer la version pauvre par-dessus la
	// version riche ferait doublon — et la contrainte d'exclusion le refuse, à
	// juste titre. La règle : le RNE complète, il n'écrase pas.
	res, err := tx.Exec(ctx, `
		INSERT INTO core.mandate
		  (person_id, mandate_type, commune_code, constituency, validity, role)
		SELECT DISTINCT ON (p.person_id, r.mandate_type)
		       p.person_id, r.mandate_type::core.mandate_type,
		       r.commune_code, r.constituency,
		       daterange(r.debut, NULL, '[)'), r.fonction
		  FROM rne_in r JOIN rne_pers p USING (cle)
		 WHERE NOT EXISTS (
		         SELECT 1 FROM core.mandate m
		          WHERE m.person_id = p.person_id
		            AND m.mandate_type = r.mandate_type::core.mandate_type
		            AND m.validity && daterange(r.debut, NULL, '[)'))
		 ORDER BY p.person_id, r.mandate_type, r.debut`)
	if err != nil {
		return fail(fmt.Errorf("insertion des mandats : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"mandats": res.RowsAffected(), "reconciliees": reconciliees, "hors_cog": horsCOG}, "")
	fmt.Printf("  RNE : %d mandats, %d personnes déjà connues reconnues, %d élus hors COG écartés\n",
		res.RowsAffected(), reconciliees, horsCOG)
	return nil
}

func premierNonVide(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
