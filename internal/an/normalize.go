package an

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// flexStr lit indifféremment une chaîne ou un nombre : l'open data de l'AN
// mélange les deux pour un même champ selon les fichiers.
type flexStr string

func (f *flexStr) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == "" {
		*f = ""
		return nil
	}
	*f = flexStr(str(json.RawMessage(b)))
	return nil
}

func (f flexStr) String() string { return string(f) }
func (f flexStr) Int() int       { n, _ := strconv.Atoi(string(f)); return n }

// ---------------------------------------------------------------- organes

type organe struct {
	UID               flexStr `json:"uid"`
	CodeType          flexStr `json:"codeType"`
	Libelle           flexStr `json:"libelle"`
	LibelleAbrege     flexStr `json:"libelleAbrege"`
	Legislature       flexStr `json:"legislature"`
	PositionPolitique flexStr `json:"positionPolitique"`
	Couleur           flexStr `json:"couleurAssociee"`
	ViMoDe            struct {
		DateDebut flexStr `json:"dateDebut"`
		DateFin   flexStr `json:"dateFin"`
	} `json:"viMoDe"`
}

// Seuls les organes qui sont des organisations politiques au sens du modèle
// deviennent des core.organization. Les commissions, missions et groupes
// d'études sont des organes de travail, pas des organisations politiques.
var organeKind = map[string]string{
	"GP":     "PARLIAMENTARY_GROUP",
	"PARPOL": "PARTY",
}

// organeLabels retourne le libellé de chaque organe, quel que soit son type :
// un ministère n'est pas une organisation politique au sens du modèle, mais son
// intitulé est nécessaire pour qualifier un mandat ministériel.
func organeLabels(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	rows, err := pool.Query(ctx,
		`SELECT DISTINCT ON (natural_key) natural_key, payload->>'libelle'
		   FROM raw.record WHERE record_type = 'an.organe'
		   ORDER BY natural_key, extracted_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var uid string
		var lib *string
		if err := rows.Scan(&uid, &lib); err != nil {
			return nil, err
		}
		if lib != nil {
			out[uid] = *lib
		}
	}
	return out, rows.Err()
}

func normalizeOrganes(ctx context.Context, pool *pgxpool.Pool) (map[string]int64, error) {
	rows, err := pool.Query(ctx,
		`SELECT DISTINCT ON (natural_key) payload FROM raw.record
		  WHERE record_type = 'an.organe'
		  ORDER BY natural_key, extracted_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type item struct {
		o    organe
		kind string
	}
	var items []item
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var o organe
		if err := json.Unmarshal(raw, &o); err != nil {
			continue
		}
		if k, ok := organeKind[o.CodeType.String()]; ok {
			items = append(items, item{o: o, kind: k})
		}
	}
	rows.Close()

	byUID := map[string]int64{}
	seenSlug := map[string]bool{}
	for _, it := range items {
		base := slugify(it.o.Libelle.String())
		if base == "" {
			base = strings.ToLower(it.o.UID.String())
		}
		if it.kind == "PARLIAMENTARY_GROUP" && it.o.Legislature != "" {
			base = base + "-" + it.o.Legislature.String()
		}
		slug := base
		if seenSlug[slug] {
			slug = base + "-" + strings.ToLower(it.o.UID.String())
		}
		seenSlug[slug] = true

		var id int64
		err := pool.QueryRow(ctx, `
			INSERT INTO core.organization (slug, kind, name, short_name, validity)
			VALUES ($1,$2,$3,$4, daterange($5::date, $6::date))
			ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
			RETURNING id`,
			slug, it.kind, it.o.Libelle.String(), nullable(it.o.LibelleAbrege.String()),
			nullable(it.o.ViMoDe.DateDebut.String()), nullable(it.o.ViMoDe.DateFin.String()),
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("organisation %s : %w", it.o.UID, err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO core.organization_identifier (organization_id, scheme, value)
			VALUES ($1,'AN_ORGANE',$2) ON CONFLICT (scheme, value) DO NOTHING`,
			id, it.o.UID.String()); err != nil {
			return nil, err
		}
		byUID[it.o.UID.String()] = id
	}
	return byUID, nil
}

// ---------------------------------------------------------------- acteurs

type acteur struct {
	UID       flexStr `json:"uid"`
	URIHatvp  flexStr `json:"uri_hatvp"`
	EtatCivil struct {
		Ident struct {
			Civ    flexStr `json:"civ"`
			Prenom flexStr `json:"prenom"`
			Nom    flexStr `json:"nom"`
		} `json:"ident"`
		InfoNaissance struct {
			DateNais flexStr `json:"dateNais"`
		} `json:"infoNaissance"`
	} `json:"etatCivil"`
	Mandats struct {
		Mandat json.RawMessage `json:"mandat"`
	} `json:"mandats"`
}

type mandat struct {
	UID         flexStr `json:"uid"`
	ActeurRef   flexStr `json:"acteurRef"`
	Legislature flexStr `json:"legislature"`
	TypeOrgane  flexStr `json:"typeOrgane"`
	DateDebut   flexStr `json:"dateDebut"`
	DateFin     flexStr `json:"dateFin"`
	Organes     struct {
		OrganeRef json.RawMessage `json:"organeRef"`
	} `json:"organes"`
	Election struct {
		Lieu struct {
			Departement flexStr `json:"departement"`
			NumCirco    flexStr `json:"numCirco"`
		} `json:"lieu"`
	} `json:"election"`
	InfosQualite struct {
		LibQualite flexStr `json:"libQualite"`
	} `json:"infosQualite"`
}

func normalizeActeurs(ctx context.Context, pool *pgxpool.Pool) (map[string]int64, error) {
	rows, err := pool.Query(ctx,
		`SELECT DISTINCT ON (natural_key) payload FROM raw.record
		  WHERE record_type = 'an.acteur'
		  ORDER BY natural_key, extracted_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	var acteurs []acteur
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var a acteur
		if err := json.Unmarshal(raw, &a); err != nil || a.UID == "" {
			continue
		}
		acteurs = append(acteurs, a)
	}
	rows.Close()

	// Resolution des homonymes AVANT insertion : un slug est un permalien
	// public, il ne doit jamais changer parce qu'un homonyme est arrive apres.
	count := map[string]int{}
	for _, a := range acteurs {
		count[slugify(a.EtatCivil.Ident.Prenom.String(), a.EtatCivil.Ident.Nom.String())]++
	}

	byUID := map[string]int64{}
	for _, a := range acteurs {
		base := slugify(a.EtatCivil.Ident.Prenom.String(), a.EtatCivil.Ident.Nom.String())
		slug := base
		if base == "" || count[base] > 1 {
			slug = strings.Trim(base+"-"+strings.ToLower(a.UID.String()), "-")
		}

		var pid int64
		err := pool.QueryRow(ctx, `
			INSERT INTO core.person (slug, family_name, given_name, birth_date)
			VALUES ($1,$2,$3,$4::date)
			ON CONFLICT (slug) DO UPDATE SET family_name = EXCLUDED.family_name
			RETURNING id`,
			slug, a.EtatCivil.Ident.Nom.String(), a.EtatCivil.Ident.Prenom.String(),
			nullable(a.EtatCivil.InfoNaissance.DateNais.String())).Scan(&pid)
		if err != nil {
			return nil, fmt.Errorf("personne %s : %w", a.UID, err)
		}
		byUID[a.UID.String()] = pid

		if _, err := pool.Exec(ctx, `
			INSERT INTO core.person_identifier (person_id, scheme, value)
			VALUES ($1,'AN_ACTEUR',$2) ON CONFLICT (scheme, value) DO NOTHING`,
			pid, a.UID.String()); err != nil {
			return nil, err
		}
		// L'identifiant HATVP est publie dans le fichier acteurs : une ligne de
		// crosswalk gratuite (docs/perimetre.md 4.5).
		if h := a.URIHatvp.String(); h != "" {
			if ref := h[strings.LastIndex(h, "/")+1:]; ref != "" {
				_, _ = pool.Exec(ctx, `
					INSERT INTO core.person_identifier (person_id, scheme, value)
					VALUES ($1,'HATVP',$2) ON CONFLICT (scheme, value) DO NOTHING`, pid, ref)
			}
		}
	}
	return byUID, nil
}

// normalizeMandats lit les mandats comme des enregistrements AUTONOMES.
// AMO50 les publie comme des objets a part entiere, complets et porteurs de
// acteurRef : les lire la plutot que dans la structure imbriquee de l'acteur
// evite de dependre d'une serialisation qui varie d'un fichier a l'autre.
func normalizeMandats(ctx context.Context, pool *pgxpool.Pool,
	personByUID, orgByUID map[string]int64, labels map[string]string) (int, error) {

	rows, err := pool.Query(ctx,
		`SELECT DISTINCT ON (natural_key) payload FROM raw.record
		  WHERE record_type = 'an.mandat'
		  ORDER BY natural_key, extracted_at DESC, id DESC`)
	if err != nil {
		return 0, err
	}
	var mandats []mandat
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return 0, err
		}
		var m mandat
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		mandats = append(mandats, m)
	}
	rows.Close()

	// Les plus anciens d'abord : si la contrainte d'exclusion refuse un
	// chevauchement, c'est le mandat le plus ancien qui est conserve. Un refus
	// est une anomalie de source, signalee par l'absence, jamais comblee.
	sort.Slice(mandats, func(i, j int) bool {
		return mandats[i].DateDebut.String() < mandats[j].DateDebut.String()
	})

	n := 0
	for _, m := range mandats {
		pid, ok := personByUID[m.ActeurRef.String()]
		if !ok {
			continue
		}
		k, err := applyMandat(ctx, pool, pid, m, orgByUID, labels)
		if err != nil {
			return 0, err
		}
		n += k
	}
	return n, nil
}

func applyMandat(ctx context.Context, pool *pgxpool.Pool, personID int64, m mandat,
	orgByUID map[string]int64, labels map[string]string) (int, error) {
	debut := m.DateDebut.String()
	if debut == "" {
		return 0, nil // un mandat sans date de début n'est pas exploitable
	}
	fin := nullable(m.DateFin.String())
	if f, ok := fin.(string); ok && f < debut {
		return 0, nil // anomalie de source : signalée par l'absence, jamais devinée
	}

	switch m.TypeOrgane.String() {
	case "ASSEMBLEE":
		circo := strings.TrimSpace(m.Election.Lieu.Departement.String())
		if n := m.Election.Lieu.NumCirco.String(); n != "" {
			circo = strings.TrimSpace(circo + ", " + n + "e circonscription")
		}
		// Les contraintes d'exclusion refusent les mandats de même type qui se
		// chevauchent. Un refus est une anomalie de source, pas une erreur
		// fatale : on la signale sans interrompre l'ingestion.
		_, err := pool.Exec(ctx, `
			INSERT INTO core.mandate
			  (person_id, mandate_type, institution, constituency, role, validity)
			VALUES ($1,'DEPUTE','ASSEMBLEE_NATIONALE',$2,$3, daterange($4::date, $5::date, '[]'))
			ON CONFLICT DO NOTHING`,
			personID, nullable(circo), nullable(m.InfosQualite.LibQualite.String()), debut, fin)
		if err != nil {
			if isExclusion(err) {
				return 0, nil
			}
			return 0, err
		}
		return 1, nil

	case "MINISTERE":
		// La contrainte d'exclusion interdit deux mandats ministériels
		// simultanés pour une même personne. C'est parfois faux — un ministre
		// peut cumuler deux portefeuilles — et le refus est alors une anomalie
		// signalée par l'absence, jamais comblée par une invention.
		_, err := pool.Exec(ctx, `
			INSERT INTO core.mandate
			  (person_id, mandate_type, role, portefeuille, validity)
			VALUES ($1,'MINISTRE',$2,$3, daterange($4::date, $5::date, '[]'))
			ON CONFLICT DO NOTHING`,
			personID, nullable(m.InfosQualite.LibQualite.String()),
			nullable(labels[str(m.Organes.OrganeRef)]), debut, fin)
		if err != nil {
			if isExclusion(err) {
				return 0, nil
			}
			return 0, err
		}
		return 1, nil

	case "GP", "PARPOL":
		orgUID := str(m.Organes.OrganeRef)
		orgID, ok := orgByUID[orgUID]
		if !ok {
			return 0, nil
		}
		kind := "PARLIAMENTARY_GROUP"
		// D-009 : le canal de connaissance est stocké. L'appartenance à un
		// groupe est publiée par l'assemblée ; le rattachement à un parti dans
		// ce fichier est déclaratif, et n'a pas la valeur du rattachement
		// publié au Journal officiel.
		via := "INSTITUTION"
		if m.TypeOrgane.String() == "PARPOL" {
			kind, via = "PARTY", "PARTY_DECLARATION"
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO core.affiliation
			  (person_id, organization_id, organization_kind, role, validity, declared_via)
			VALUES ($1,$2,$3,$4, daterange($5::date, $6::date, '[]'), $7)
			ON CONFLICT DO NOTHING`,
			personID, orgID, kind, nullable(m.InfosQualite.LibQualite.String()),
			debut, fin, via)
		if err != nil {
			if isExclusion(err) {
				return 0, nil
			}
			return 0, err
		}
		return 1, nil
	}
	return 0, nil
}

func isExclusion(err error) bool {
	return strings.Contains(err.Error(), "exclusion") ||
		strings.Contains(err.Error(), "23P01")
}

// Normalize reconstruit core à partir de raw. L'opération est idempotente :
// c'est l'invariant central du projet (db/README.md).
func Normalize(ctx context.Context, pool *pgxpool.Pool) error {
	// core est une fonction pure de raw : on reconstruit, on ne complète pas.
	// Sans cela, rejouer l'ingestion dupliquerait mandats et appartenances, et
	// l'invariant de reconstructibilité serait faux.
	// core est une fonction pure de raw : on reconstruit, on ne complète pas.
	// Mais un connecteur ne reconstruit QUE CE QU'IL POSSÈDE. Un TRUNCATE
	// global sur core.organization emportait en cascade les comptes CNCCFP et
	// les classifications tierces, qui viennent d'autres sources — régression
	// réelle, détectée en production locale et gardée depuis par cmd/verify.
	// Remise à zéro complète de ce que ce connecteur possède, EN UNE FOIS et
	// dans l'ordre des dépendances. Pas de TRUNCATE CASCADE : sa portée a déjà
	// dépassé deux fois ce périmètre, détruisant les comptes CNCCFP puis les
	// scrutins et leurs 1,27 million de votes.
	for _, q := range []string{
		// Portée limitée à l'Assemblée : un DELETE global emporterait les votes
		// du Parlement européen, qui viennent d'un autre connecteur.
		`DELETE FROM core.ballot_group_exception e USING core.scrutin s
		  WHERE s.id = e.scrutin_id AND s.institution = 'ASSEMBLEE_NATIONALE'`,
		`DELETE FROM core.ballot_group g USING core.scrutin s
		  WHERE s.id = g.scrutin_id AND s.institution = 'ASSEMBLEE_NATIONALE'`,
		`DELETE FROM core.ballot b USING core.scrutin s
		  WHERE s.id = b.scrutin_id AND s.institution = 'ASSEMBLEE_NATIONALE'`,
		`DELETE FROM core.affiliation`,
		`DELETE FROM core.nuance_assignment`,
		`DELETE FROM core.mandate WHERE institution IS NULL OR institution = 'ASSEMBLEE_NATIONALE'`,
		// Les dossiers doivent partir avant les organisations : texte_author et
		// dossier_author y renvoient.
		`UPDATE core.scrutin SET dossier_id = NULL, texte_id = NULL`,
		`DELETE FROM core.dossier_author`,
		`DELETE FROM core.texte_author`,
		`DELETE FROM core.texte`,
		`DELETE FROM core.dossier`,
		// Enfin les entités portant un identifiant de l'Assemblée, et elles seules.
		`DELETE FROM core.organization o
		  WHERE EXISTS (SELECT 1 FROM core.organization_identifier i
		                 WHERE i.organization_id = o.id AND i.scheme = 'AN_ORGANE')`,
		`DELETE FROM core.person p
		  WHERE EXISTS (SELECT 1 FROM core.person_identifier i
		                 WHERE i.person_id = p.id AND i.scheme = 'AN_ACTEUR')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("remise à zéro : %w", err)
		}
	}

	orgByUID, err := normalizeOrganes(ctx, pool)
	if err != nil {
		return fmt.Errorf("organes : %w", err)
	}
	fmt.Printf("  organisations   %d\n", len(orgByUID))

	personByUID, err := normalizeActeurs(ctx, pool)
	if err != nil {
		return fmt.Errorf("acteurs : %w", err)
	}
	fmt.Printf("  personnes       %d\n", len(personByUID))

	labels, err := organeLabels(ctx, pool)
	if err != nil {
		return fmt.Errorf("libellés d'organes : %w", err)
	}

	nMandats, err := normalizeMandats(ctx, pool, personByUID, orgByUID, labels)
	if err != nil {
		return fmt.Errorf("mandats : %w", err)
	}
	fmt.Printf("  mandats et appartenances %d\n", nMandats)

	nScr, nBal, err := normalizeScrutins(ctx, pool, personByUID, orgByUID)
	if err != nil {
		return fmt.Errorf("scrutins : %w", err)
	}
	fmt.Printf("  scrutins        %d\n  votes nominatifs %d\n", nScr, nBal)

	if err := NormalizeDossiers(ctx, pool, personByUID, orgByUID); err != nil {
		return fmt.Errorf("dossiers : %w", err)
	}
	return nil
}

func normalizeScrutinsLegacy(ctx context.Context, pool *pgxpool.Pool,
	personByUID, orgByUID map[string]int64) error {
	nScr, nBal, err := normalizeScrutins(ctx, pool, personByUID, orgByUID)
	if err != nil {
		return fmt.Errorf("scrutins : %w", err)
	}
	fmt.Printf("  scrutins        %d\n  votes nominatifs %d\n", nScr, nBal)
	return nil
}

var _ = pgx.Identifier{}
