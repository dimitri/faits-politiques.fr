package an

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceDossiers = archive.Source{
	Slug: "an-dossiers", Label: "AN — Dossiers législatifs (17e législature)",
	Publisher: "Assemblée nationale", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Assemblée nationale, open data",
	Cadence:     "continue",
	Notes: "Contient les dossiers et les documents (textes déposés, rapports). " +
		"Les exposés des motifs ne figurent PAS dans le JSON : seuls les titres, " +
		"auteurs, dates et étapes y sont. Un résumé rédigé ne peut donc pas en être " +
		"tiré mécaniquement.",
}

const DossiersURL = Base + "/loi/dossiers_legislatifs/Dossiers_Legislatifs.json.zip"

// rawBox conserve le JSON brut d'un sous-objet dont la forme varie.
type rawBox struct{ Raw json.RawMessage }

func (b *rawBox) UnmarshalJSON(data []byte) error { b.Raw = append([]byte(nil), data...); return nil }

type dossierParlementaire struct {
	UID          flexStr `json:"uid"`
	Legislature  flexStr `json:"legislature"`
	TitreDossier struct {
		Titre       flexStr `json:"titre"`
		TitreChemin flexStr `json:"titreChemin"`
		SenatChemin flexStr `json:"senatChemin"`
	} `json:"titreDossier"`
	ProcedureParlementaire struct {
		Libelle flexStr `json:"libelle"`
	} `json:"procedureParlementaire"`
	Initiateur struct {
		Acteurs rawBox `json:"acteurs"`
		Organes rawBox `json:"organes"`
	} `json:"initiateur"`
	ActesLegislatifs struct {
		ActeLegislatif json.RawMessage `json:"acteLegislatif"`
	} `json:"actesLegislatifs"`
}

type documentAN struct {
	UID    flexStr `json:"uid"`
	Titres struct {
		TitrePrincipal      flexStr `json:"titrePrincipal"`
		TitrePrincipalCourt flexStr `json:"titrePrincipalCourt"`
	} `json:"titres"`
	DenominationStructurelle flexStr `json:"denominationStructurelle"`
	DossierRef               flexStr `json:"dossierRef"`
	CycleDeVie               struct {
		Chrono struct {
			DateDepot flexStr `json:"dateDepot"`
		} `json:"chrono"`
	} `json:"cycleDeVie"`
	Classification struct {
		Type struct {
			Code    flexStr `json:"code"`
			Libelle flexStr `json:"libelle"`
		} `json:"type"`
	} `json:"classification"`
	Auteurs       json.RawMessage `json:"auteurs"`
	CoSignataires json.RawMessage `json:"coSignataires"`
}

// Les types de documents retenus comme « textes » au sens du modèle. Les
// rapports et avis sont des documents de travail, pas des textes soumis au vote.
var documentKind = map[string]string{
	"PRJL":  "PROJET_DE_LOI",
	"PION":  "PROPOSITION_DE_LOI",
	"PIONR": "PROPOSITION_DE_RESOLUTION",
	"PRJLC": "PROJET_DE_LOI",
	"PIONC": "PROPOSITION_DE_LOI",
	"PRJLO": "PROJET_DE_LOI",
	"PIONO": "PROPOSITION_DE_LOI",
}

// NormalizeDossiers reconstruit dossiers, textes et auteurs depuis raw, puis
// rattache les scrutins à leur dossier.
func NormalizeDossiers(ctx context.Context, pool *pgxpool.Pool,
	personByUID, orgByUID map[string]int64) error {

	// La remise à zéro est faite en amont, dans Normalize, en une seule passe
	// ordonnée : les dossiers doivent disparaître avant les organisations, dont
	// dossier_author et texte_author dépendent.

	var legID int64
	if err := pool.QueryRow(ctx,
		`SELECT id FROM core.legislature WHERE institution='ASSEMBLEE_NATIONALE' AND numero=17`).
		Scan(&legID); err != nil {
		return fmt.Errorf("législature : %w", err)
	}

	rows, err := pool.Query(ctx,
		`SELECT DISTINCT ON (natural_key) payload FROM raw.record
		  WHERE record_type = 'an.dossierParlementaire'
		  ORDER BY natural_key, extracted_at DESC, id DESC`)
	if err != nil {
		return err
	}
	dossierID := map[string]int64{}
	seenSlug := map[string]bool{}
	nInitiateurs := 0
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var d dossierParlementaire
		if err := json.Unmarshal(raw, &d); err != nil || d.UID == "" {
			continue
		}
		titre := strings.TrimSpace(d.TitreDossier.Titre.String())
		if titre == "" {
			titre = d.UID.String()
		}
		slug := slugify(titre)
		if slug == "" || seenSlug[slug] {
			slug = strings.Trim(slug+"-"+strings.ToLower(d.UID.String()), "-")
		}
		seenSlug[slug] = true

		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO core.dossier
			  (slug, institution, source_uid, legislature_id, titre, titre_chemin, senat_chemin)
			VALUES ($1,'ASSEMBLEE_NATIONALE',$2,$3,$4,$5,$6)
			ON CONFLICT (institution, source_uid) DO UPDATE SET titre = EXCLUDED.titre
			RETURNING id`, slug, d.UID.String(), legID, titre,
			nullable(d.TitreDossier.TitreChemin.String()),
			nullable(d.TitreDossier.SenatChemin.String())).Scan(&id); err != nil {
			return fmt.Errorf("dossier %s : %w", d.UID, err)
		}
		dossierID[d.UID.String()] = id

		if n, err := appliquerInitiateur(ctx, pool, id, d, personByUID, orgByUID); err != nil {
			return err
		} else {
			nInitiateurs += n
		}
	}
	rows.Close()
	fmt.Printf("  dossiers        %d (%d initiateurs)\n", len(dossierID), nInitiateurs)

	nTextes, nAuteurs, err := normalizeDocuments(ctx, pool, dossierID, personByUID, orgByUID, seenSlug)
	if err != nil {
		return err
	}
	fmt.Printf("  textes          %d (%d auteurs et cosignataires)\n", nTextes, nAuteurs)

	nLies, err := lierScrutins(ctx, pool, dossierID)
	if err != nil {
		return err
	}
	fmt.Printf("  scrutins rattachés à leur dossier %d\n", nLies)
	return nil
}

// appliquerInitiateur transcrit l'initiateur publié par la source. Un dossier
// sans initiateur — environ un sur huit — reste sans auteur : l'absence est
// affichée, jamais comblée par une déduction à partir du titre.
func appliquerInitiateur(ctx context.Context, pool *pgxpool.Pool, dossierID int64,
	d dossierParlementaire, personByUID, orgByUID map[string]int64) (int, error) {

	n, rang := 0, 1
	var acteurs struct {
		Acteur json.RawMessage `json:"acteur"`
	}
	if len(d.Initiateur.Acteurs.Raw) > 0 {
		_ = json.Unmarshal(d.Initiateur.Acteurs.Raw, &acteurs)
		for _, e := range asSlice(acteurs.Acteur) {
			var a struct {
				ActeurRef json.RawMessage `json:"acteurRef"`
			}
			if json.Unmarshal(e, &a) != nil {
				continue
			}
			pid, ok := personByUID[str(a.ActeurRef)]
			if !ok {
				continue
			}
			if _, err := pool.Exec(ctx, `
				INSERT INTO core.dossier_author (dossier_id, person_id, role, rang)
				VALUES ($1,$2,'INITIATEUR',$3)`, dossierID, pid, rang); err != nil {
				return 0, err
			}
			n++
			rang++
		}
	}
	var organes struct {
		Organe json.RawMessage `json:"organe"`
	}
	if len(d.Initiateur.Organes.Raw) > 0 {
		_ = json.Unmarshal(d.Initiateur.Organes.Raw, &organes)
		for _, e := range asSlice(organes.Organe) {
			var o struct {
				OrganeRef json.RawMessage `json:"organeRef"`
			}
			if json.Unmarshal(e, &o) != nil {
				continue
			}
			if oid, ok := orgByUID[str(o.OrganeRef)]; ok {
				if _, err := pool.Exec(ctx, `
					INSERT INTO core.dossier_author (dossier_id, organization_id, role, rang)
					VALUES ($1,$2,'GOUVERNEMENT',$3)`, dossierID, oid, rang); err != nil {
					return 0, err
				}
				n++
			}
		}
	}
	return n, nil
}

func normalizeDocuments(ctx context.Context, pool *pgxpool.Pool, dossierID map[string]int64,
	personByUID, orgByUID map[string]int64, seenSlug map[string]bool) (int, int, error) {

	rows, err := pool.Query(ctx,
		`SELECT DISTINCT ON (natural_key) payload FROM raw.record
		  WHERE record_type = 'an.document'
		  ORDER BY natural_key, extracted_at DESC, id DESC`)
	if err != nil {
		return 0, 0, err
	}
	var docs []documentAN
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return 0, 0, err
		}
		var d documentAN
		if err := json.Unmarshal(raw, &d); err != nil || d.UID == "" {
			continue
		}
		docs = append(docs, d)
	}
	rows.Close()

	nTextes, nAuteurs := 0, 0
	for _, d := range docs {
		kind, ok := documentKind[d.Classification.Type.Code.String()]
		if !ok {
			continue // rapport, avis, annexe : documents de travail
		}
		did, ok := dossierID[d.DossierRef.String()]
		if !ok {
			continue // un texte sans dossier rattaché n'est pas exploitable ici
		}
		titre := strings.TrimSpace(d.Titres.TitrePrincipal.String())
		if titre == "" {
			titre = d.UID.String()
		}
		slug := slugify(titre)
		if slug == "" || seenSlug["t-"+slug] {
			slug = strings.Trim(slug+"-"+strings.ToLower(d.UID.String()), "-")
		}
		seenSlug["t-"+slug] = true

		var tid int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO core.texte (slug, dossier_id, institution, source_uid, kind, titre, date_depot)
			VALUES ($1,$2,'ASSEMBLEE_NATIONALE',$3,$4,$5,$6::date)
			ON CONFLICT (institution, source_uid) DO UPDATE SET titre = EXCLUDED.titre
			RETURNING id`, slug, did, d.UID.String(), kind, titre,
			nullable(dateOnly(d.CycleDeVie.Chrono.DateDepot.String()))).Scan(&tid); err != nil {
			return 0, 0, fmt.Errorf("texte %s : %w", d.UID, err)
		}
		nTextes++

		// « Qui propose » est une dimension distincte de « qui vote », et c'est
		// celle qu'aucun outil français n'exploite. Elle est transcrite ici
		// telle que publiée, sans interprétation.
		n, err := appliquerAuteurs(ctx, pool, tid, d, personByUID, orgByUID)
		if err != nil {
			return 0, 0, err
		}
		nAuteurs += n
	}
	return nTextes, nAuteurs, nil
}

func appliquerAuteurs(ctx context.Context, pool *pgxpool.Pool, texteID int64, d documentAN,
	personByUID, orgByUID map[string]int64) (int, error) {

	n := 0
	ajouter := func(raw json.RawMessage, role string, rang int) error {
		for _, e := range asSlice(raw) {
			var a struct {
				ActeurRef json.RawMessage `json:"acteurRef"`
				Organe    struct {
					OrganeRef json.RawMessage `json:"organeRef"`
				} `json:"organe"`
			}
			if err := json.Unmarshal(e, &a); err != nil {
				continue
			}
			if ref := str(a.ActeurRef); ref != "" {
				if pid, ok := personByUID[ref]; ok {
					if _, err := pool.Exec(ctx, `
						INSERT INTO core.texte_author (texte_id, person_id, role, rang)
						VALUES ($1,$2,$3,$4)`, texteID, pid, role, rang); err != nil {
						return err
					}
					n++
					rang++
				}
			}
			if ref := str(a.Organe.OrganeRef); ref != "" {
				if oid, ok := orgByUID[ref]; ok {
					if _, err := pool.Exec(ctx, `
						INSERT INTO core.texte_author (texte_id, organization_id, role, rang)
						VALUES ($1,$2,'GOUVERNEMENT',$3)`, texteID, oid, rang); err != nil {
						return err
					}
					n++
				}
			}
		}
		return nil
	}

	var box struct {
		Auteur json.RawMessage `json:"auteur"`
	}
	if len(d.Auteurs) > 0 {
		_ = json.Unmarshal(d.Auteurs, &box)
		if err := ajouter(box.Auteur, "AUTEUR", 1); err != nil {
			return 0, err
		}
	}
	var cbox struct {
		CoSignataire json.RawMessage `json:"coSignataire"`
	}
	if len(d.CoSignataires) > 0 {
		_ = json.Unmarshal(d.CoSignataires, &cbox)
		if err := ajouter(cbox.CoSignataire, "COSIGNATAIRE", 2); err != nil {
			return 0, err
		}
	}
	return n, nil
}

// lierScrutins rattache chaque scrutin à son dossier quand la source le publie.
// Environ deux scrutins sur trois portent cette référence ; pour les autres,
// l'absence est affichée comme telle plutôt que devinée à partir du libellé.
func lierScrutins(ctx context.Context, pool *pgxpool.Pool, dossierID map[string]int64) (int, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT ON (natural_key) natural_key,
		       payload->'objet'->>'dossierLegislatif'
		  FROM raw.record
		 WHERE record_type = 'an.scrutin' AND payload->'objet'->>'dossierLegislatif' IS NOT NULL
		 ORDER BY natural_key, extracted_at DESC, id DESC`)
	if err != nil {
		return 0, err
	}
	type lien struct{ uid, dossier string }
	var liens []lien
	for rows.Next() {
		var uid, obj string
		if err := rows.Scan(&uid, &obj); err != nil {
			return 0, err
		}
		var o struct {
			DossierRef string `json:"dossierRef"`
		}
		if json.Unmarshal([]byte(obj), &o) == nil && o.DossierRef != "" {
			liens = append(liens, lien{uid, o.DossierRef})
		}
	}
	rows.Close()

	n := 0
	for _, l := range liens {
		did, ok := dossierID[l.dossier]
		if !ok {
			continue
		}
		ct, err := pool.Exec(ctx, `
			UPDATE core.scrutin SET dossier_id = $1
			WHERE institution = 'ASSEMBLEE_NATIONALE' AND source_uid = $2`, did, l.uid)
		if err != nil {
			return 0, err
		}
		n += int(ct.RowsAffected())
	}
	return n, nil
}

func dateOnly(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return ""
}
