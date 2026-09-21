package an

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/logs"
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
// Le type d'organe publié par l'Assemblée, traduit vers la nature
// d'organisation du modèle. Les organes absents de cette table ne deviennent
// pas des organisations : ASSEMBLEE, SENAT et PRESREP sont des institutions,
// et les mandats qui s'y rapportent sont des mandats, pas des adhésions.
var organeKind = map[string]string{
	"GP":           "PARLIAMENTARY_GROUP",
	"GROUPESENAT":  "PARLIAMENTARY_GROUP",
	"PARPOL":       "PARTY",
	"GOUVERNEMENT": "GOVERNMENT",
	"MINISTERE":    "GOVERNMENT",

	// Les commissions, au sens strict.
	"COMPER":     "COMMITTEE",
	"COMNL":      "COMMITTEE",
	"CMP":        "COMMITTEE",
	"COMSENAT":   "COMMITTEE",
	"COMSPSENAT": "COMMITTEE",

	// Tout le reste des organes parlementaires. Ils ne sont pas des
	// commissions et les ranger comme telles serait faux ; leur code d'origine
	// est conservé dans organ_type, qui les distingue sans les regrouper.
	"DELEG":       "PARLIAMENTARY_BODY",
	"DELEGSENAT":  "PARLIAMENTARY_BODY",
	"DELEGBUREAU": "PARLIAMENTARY_BODY",
	"MISINFO":     "PARLIAMENTARY_BODY",
	"MISINFOPRE":  "PARLIAMENTARY_BODY",
	"MISINFOCOM":  "PARLIAMENTARY_BODY",
	"CNPE":        "PARLIAMENTARY_BODY",
	"CNPS":        "PARLIAMENTARY_BODY",
	"GE":          "PARLIAMENTARY_BODY",
	"GEVI":        "PARLIAMENTARY_BODY",
	"GA":          "PARLIAMENTARY_BODY",
	"ORGEXTPARL":  "PARLIAMENTARY_BODY",
	"BUREAU":      "PARLIAMENTARY_BODY",
	"API":         "PARLIAMENTARY_BODY",
	"OFFPAR":      "PARLIAMENTARY_BODY",
	"CJR":         "PARLIAMENTARY_BODY",
	"CONFPT":      "PARLIAMENTARY_BODY",
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

	// Le slug une fois choisi en Go (seenSlug ci-dessous a besoin de voir les
	// slugs déjà attribués dans CET ordre, avant toute écriture), une seule
	// table temporaire porte les organisations candidates et un seul
	// INSERT ... SELECT les écrit — aucune contrainte d'exclusion ici (slug
	// est une simple clé unique), donc aucune dépendance à l'ordre
	// contrairement à applyMandat/copierMandats plus bas.
	type ligne struct {
		uid, slug, kind, nom, abrege, debut, fin, organType string
	}
	seenSlug := map[string]bool{}
	lignes := make([]ligne, 0, len(items))
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
		lignes = append(lignes, ligne{
			uid: it.o.UID.String(), slug: slug, kind: it.kind, nom: it.o.Libelle.String(),
			abrege: it.o.LibelleAbrege.String(), debut: it.o.ViMoDe.DateDebut.String(),
			fin: it.o.ViMoDe.DateFin.String(), organType: it.o.CodeType.String(),
		})
	}
	if len(lignes) == 0 {
		return map[string]int64{}, nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_organe (
			uid text, slug text, kind text, nom text, abrege text,
			debut text, fin text, organ_type text
		) ON COMMIT DROP`); err != nil {
		return nil, err
	}
	copieOrganes := make([][]any, len(lignes))
	for i, l := range lignes {
		copieOrganes[i] = []any{l.uid, l.slug, l.kind, l.nom, nullable(l.abrege), nullable(l.debut), nullable(l.fin), l.organType}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_organe"},
		[]string{"uid", "slug", "kind", "nom", "abrege", "debut", "fin", "organ_type"},
		pgx.CopyFromRows(copieOrganes)); err != nil {
		return nil, err
	}
	res, err := tx.Query(ctx, `
		WITH upsert AS (
			INSERT INTO core.organization (slug, kind, name, short_name, validity, organ_type)
			SELECT slug, kind::core.organization_kind, nom, abrege, daterange(debut::date, fin::date), organ_type
			  FROM tmp_organe
			ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, organ_type = EXCLUDED.organ_type
			RETURNING id, slug
		)
		SELECT t.uid, u.id FROM upsert u JOIN tmp_organe t ON t.slug = u.slug`)
	if err != nil {
		return nil, fmt.Errorf("organisations : %w", err)
	}
	byUID := map[string]int64{}
	for res.Next() {
		var uid string
		var id int64
		if err := res.Scan(&uid, &id); err != nil {
			res.Close()
			return nil, err
		}
		byUID[uid] = id
	}
	res.Close()
	if err := res.Err(); err != nil {
		return nil, err
	}

	idRows := make([][]any, 0, len(lignes))
	for _, l := range lignes {
		if id, ok := byUID[l.uid]; ok {
			idRows = append(idRows, []any{id, l.uid})
		}
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_organe_id (organization_id bigint, uid text) ON COMMIT DROP`); err != nil {
		return nil, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_organe_id"},
		[]string{"organization_id", "uid"}, pgx.CopyFromRows(idRows)); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.organization_identifier (organization_id, scheme, value)
		SELECT organization_id, 'AN_ORGANE', uid FROM tmp_organe_id
		ON CONFLICT (scheme, value) DO NOTHING`); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
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

		nom := a.EtatCivil.Ident.Nom.String()
		prenom := a.EtatCivil.Ident.Prenom.String()
		naissance := nullable(a.EtatCivil.InfoNaissance.DateNais.String())

		// Trois chemins, du plus sûr au moins sûr, et jamais de création si un
		// des deux premiers aboutit :
		//   1. l'identifiant de l'Assemblée, s'il est déjà connu ;
		//   2. le triplet exact (nom, prénom, date de naissance), qui rattrape
		//      une personne créée d'abord par le RNE — sans lui, un député par
		//      ailleurs conseiller municipal existerait en double ;
		//   3. le slug, qui est un permalien public et ne doit jamais changer.
		pid, err := trouverPersonne(ctx, pool, a.UID.String(), nom, prenom, naissance)
		if err != nil {
			return nil, fmt.Errorf("personne %s : %w", a.UID, err)
		}
		if pid == 0 {
			if err := pool.QueryRow(ctx, `
				INSERT INTO core.person (slug, family_name, given_name, birth_date)
				VALUES ($1,$2,$3,$4::date)
				ON CONFLICT (slug) DO UPDATE SET family_name = EXCLUDED.family_name
				RETURNING id`, slug, nom, prenom, naissance).Scan(&pid); err != nil {
				return nil, fmt.Errorf("personne %s : %w", a.UID, err)
			}
		} else if _, err := pool.Exec(ctx, `
			UPDATE core.person SET family_name = $2, given_name = $3,
			       birth_date = coalesce($4::date, birth_date)
			 WHERE id = $1`, pid, nom, prenom, naissance); err != nil {
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
		// crosswalk gratuite (docs/perimetre.md §4.5).
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

// normalizeMandats lit les mandats aux DEUX endroits où l'Assemblée les publie.
//
// AMO50 les publie comme des objets autonomes, porteurs de acteurRef : c'était
// la seule lecture jusqu'ici, et elle paraissait la plus sûre — pas de
// dépendance à une sérialisation imbriquée. Elle était surtout la plus pauvre.
// AMO50 est le fichier « divisé », qui omet les députés partis en cours de
// législature : 1 258 mandats de député pour 577 personnes, soit exactement la
// promotion en exercice.
//
// AMO30, le fichier « tous acteurs », porte les mêmes mandats DANS chaque
// acteur, sous `mandats.mandat` — et il en porte 3 952, pour 2 120 personnes,
// remontant au 19 juin 2002. C'est trois fois plus de mandats et près de quatre
// fois plus de députés. La structure imbriquée qu'on avait voulu éviter était
// l'endroit où se trouvait l'histoire.
//
// Les deux sources sont lues et réunies sur l'identifiant du mandat (`uid`) :
// aucun rapprochement approximatif, et aucun doublon.
func normalizeMandats(ctx context.Context, pool *pgxpool.Pool,
	personByUID, orgByUID map[string]int64, labels map[string]string) (int, error) {

	rows, err := pool.Query(ctx,
		`WITH acteurs AS (
		   SELECT DISTINCT ON (natural_key) payload FROM raw.record
		    WHERE record_type = 'an.acteur'
		    ORDER BY natural_key, extracted_at DESC, id DESC
		 )
		 SELECT payload FROM (
		   SELECT DISTINCT ON (natural_key) payload FROM raw.record
		    WHERE record_type = 'an.mandat'
		    ORDER BY natural_key, extracted_at DESC, id DESC
		 ) autonomes
		 UNION ALL
		 -- Cinq acteurs n'ont qu'un mandat, et le JSON le sérialise alors comme
		 -- un objet et non comme un tableau. Les ignorer perdrait cinq
		 -- carrières sans le dire.
		 SELECT jsonb_array_elements(payload #> '{mandats,mandat}') FROM acteurs
		  WHERE jsonb_typeof(payload #> '{mandats,mandat}') = 'array'
		 UNION ALL
		 SELECT payload #> '{mandats,mandat}' FROM acteurs
		  WHERE jsonb_typeof(payload #> '{mandats,mandat}') = 'object'`)
	if err != nil {
		return 0, err
	}
	var mandats []mandat
	vus := map[string]bool{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return 0, err
		}
		var m mandat
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		if u := m.UID.String(); u != "" {
			if vus[u] {
				continue
			}
			vus[u] = true
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

	fins := recoller(mandats, personByUID)

	// Deux lots, jamais une INSERT par mandat (jusqu'à plusieurs milliers
	// d'allers-retours sur une base distante) : chaque mandat est d'abord
	// classé en Go (même logique de branchement qu'avant, voir classerMandat)
	// dans l'un des deux lots — core.mandate (député/ministre/sénateur) ou
	// core.affiliation (groupe, commission, parti déclaré) —, copiés chacun
	// dans une table temporaire, puis un SEUL INSERT ... SELECT par lot.
	// L'ordre du lot (le plus ancien d'abord, comme mandats est déjà trié)
	// est préservé par rn : Postgres traite les lignes d'un INSERT ... SELECT
	// ... ORDER BY dans cet ordre pour la résolution des contraintes
	// d'exclusion, exactement comme la boucle ligne à ligne qu'il remplace
	// (vérifié directement contre Postgres avant d'écrire ceci — voir la
	// session qui a introduit ce commentaire).
	var lotMandats []ligneMandat
	var lotAffiliations []ligneAffiliation
	for _, m := range mandats {
		pid, ok := personByUID[m.ActeurRef.String()]
		if !ok {
			continue
		}
		lm, la := classerMandat(pid, m, orgByUID, labels, fins[m.UID.String()])
		if lm != nil {
			lm.rn = len(lotMandats)
			lotMandats = append(lotMandats, *lm)
		}
		if la != nil {
			la.rn = len(lotAffiliations)
			lotAffiliations = append(lotAffiliations, *la)
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	nMandats, err := copierMandats(ctx, tx, lotMandats)
	if err != nil {
		return 0, fmt.Errorf("mandats (core.mandate) : %w", err)
	}
	nAffiliations, err := copierAffiliations(ctx, tx, lotAffiliations)
	if err != nil {
		return 0, fmt.Errorf("appartenances (core.affiliation) : %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return nMandats + nAffiliations, nil
}

// ligneMandat/ligneAffiliation : une ligne prête pour la table temporaire de
// son lot — mêmes colonnes que l'INSERT qu'applyMandat faisait une par une
// avant ce lot, jamais recalculées.
type ligneMandat struct {
	rn                                     int
	personID                               int64
	mandateType, institution, constituency string
	role, portefeuille                     string
	debut, fin                             string
}

type ligneAffiliation struct {
	rn                          int
	personID, organizationID    int64
	organizationKind, role, via string
	debut, fin                  string
}

// classerMandat reprend exactement le branchement et les anomalies signalées
// par l'absence qu'applyMandat traitait ligne à ligne — seule la sortie
// change : une ligne pour l'un des deux lots plutôt qu'un INSERT immédiat.
// PRESREP et les organes/types inconnus renvoient (nil, nil) : ignorés, comme
// avant.
func classerMandat(personID int64, m mandat, orgByUID map[string]int64,
	labels map[string]string, finRecollee string) (*ligneMandat, *ligneAffiliation) {
	debut := m.DateDebut.String()
	if debut == "" {
		return nil, nil // un mandat sans date de début n'est pas exploitable
	}
	brute := m.DateFin.String()
	if finRecollee != "" {
		brute = finRecollee
	}
	fin := brute
	if fin != "" && fin < debut {
		return nil, nil // anomalie de source : signalée par l'absence, jamais devinée
	}

	switch m.TypeOrgane.String() {
	case "ASSEMBLEE":
		circo := strings.TrimSpace(m.Election.Lieu.Departement.String())
		if n := m.Election.Lieu.NumCirco.String(); n != "" {
			circo = strings.TrimSpace(circo + ", " + n + "e circonscription")
		}
		return &ligneMandat{
			personID: personID, mandateType: "DEPUTE", institution: "ASSEMBLEE_NATIONALE",
			constituency: circo, role: m.InfosQualite.LibQualite.String(), debut: debut, fin: fin,
		}, nil

	case "MINISTERE":
		return &ligneMandat{
			personID: personID, mandateType: "MINISTRE",
			role: m.InfosQualite.LibQualite.String(), portefeuille: labels[str(m.Organes.OrganeRef)],
			debut: debut, fin: fin,
		}, nil

	case "SENAT":
		return &ligneMandat{
			personID: personID, mandateType: "SENATEUR", institution: "SENAT",
			role: m.InfosQualite.LibQualite.String(), debut: debut, fin: fin,
		}, nil

	case "PRESREP":
		// Ignoré volontairement : data/presidents.csv fait autorité sur les
		// présidences, avec les intérims et les dates de passation. Insérer
		// ici une seconde version, plus pauvre, ferait doublon.
		return nil, nil

	default:
		orgUID := str(m.Organes.OrganeRef)
		orgID, ok := orgByUID[orgUID]
		if !ok {
			return nil, nil
		}
		kind, ok := organeKind[m.TypeOrgane.String()]
		if !ok {
			return nil, nil
		}
		// D-009 : le canal de connaissance est stocké. L'appartenance à un
		// groupe, à une commission ou à une délégation est publiée par
		// l'assemblée ; le rattachement à un parti dans ce fichier est
		// déclaratif, et n'a pas la valeur du rattachement publié au Journal
		// officiel.
		via := "INSTITUTION"
		if kind == "PARTY" {
			via = "PARTY_DECLARATION"
		}
		return nil, &ligneAffiliation{
			personID: personID, organizationID: orgID, organizationKind: kind,
			role: m.InfosQualite.LibQualite.String(), via: via, debut: debut, fin: fin,
		}
	}
}

// copierMandats : une table temporaire, une copie, un INSERT ... SELECT —
// les contraintes d'exclusion de core.mandate (un DEPUTE/SENATEUR/MINISTRE
// ne peut pas chevaucher un autre mandat du même type pour la même
// personne) refusent un chevauchement exactement comme le ON CONFLICT DO
// NOTHING ligne à ligne qu'elles remplacent — ORDER BY rn préserve le
// « plus ancien d'abord » dont recoller/le tri de normalizeMandats dépendent.
func copierMandats(ctx context.Context, tx pgx.Tx, lignes []ligneMandat) (int, error) {
	if len(lignes) == 0 {
		return 0, nil
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_mandat (
			rn int, person_id bigint, mandate_type text, institution text,
			constituency text, role text, portefeuille text, debut text, fin text
		) ON COMMIT DROP`); err != nil {
		return 0, err
	}
	rows := make([][]any, len(lignes))
	for i, l := range lignes {
		rows[i] = []any{l.rn, l.personID, l.mandateType, nullable(l.institution),
			nullable(l.constituency), nullable(l.role), nullable(l.portefeuille),
			l.debut, nullable(l.fin)}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_mandat"},
		[]string{"rn", "person_id", "mandate_type", "institution", "constituency",
			"role", "portefeuille", "debut", "fin"},
		pgx.CopyFromRows(rows)); err != nil {
		return 0, err
	}
	res, err := tx.Exec(ctx, `
		INSERT INTO core.mandate (person_id, mandate_type, institution, constituency, role, portefeuille, validity)
		SELECT person_id, mandate_type::core.mandate_type, institution::core.institution,
		       constituency, role, portefeuille, daterange(debut::date, fin::date, '[]')
		  FROM tmp_mandat
		 ORDER BY rn
		ON CONFLICT DO NOTHING`)
	if err != nil {
		return 0, err
	}
	return int(res.RowsAffected()), nil
}

// copierAffiliations : même patron que copierMandats pour core.affiliation —
// la contrainte d'exclusion n'y interdit qu'appartenir à deux groupes
// parlementaires en même temps (siéger dans plusieurs commissions à la fois
// est normal), donc ORDER BY rn a la même portée limitée qu'avant.
func copierAffiliations(ctx context.Context, tx pgx.Tx, lignes []ligneAffiliation) (int, error) {
	if len(lignes) == 0 {
		return 0, nil
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_affiliation (
			rn int, person_id bigint, organization_id bigint, organization_kind text,
			role text, via text, debut text, fin text
		) ON COMMIT DROP`); err != nil {
		return 0, err
	}
	rows := make([][]any, len(lignes))
	for i, l := range lignes {
		rows[i] = []any{l.rn, l.personID, l.organizationID, l.organizationKind,
			nullable(l.role), l.via, l.debut, nullable(l.fin)}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_affiliation"},
		[]string{"rn", "person_id", "organization_id", "organization_kind", "role", "via", "debut", "fin"},
		pgx.CopyFromRows(rows)); err != nil {
		return 0, err
	}
	res, err := tx.Exec(ctx, `
		INSERT INTO core.affiliation (person_id, organization_id, organization_kind, role, validity, declared_via)
		SELECT person_id, organization_id, organization_kind::core.organization_kind, role,
		       daterange(debut::date, fin::date, '[]'), via::core.affiliation_source
		  FROM tmp_affiliation
		 ORDER BY rn
		ON CONFLICT DO NOTHING`)
	if err != nil {
		return 0, err
	}
	return int(res.RowsAffected()), nil
}

// recoller règle le chevauchement de deux jours que la source publie à chaque
// changement de législature.
//
// La quatorzième législature s'achève le 20 juin 2017 et la quinzième débute
// le 18 : les deux dates sont justes — l'Assemblée sortante siège jusqu'à ce
// que la nouvelle soit constituée — mais la contrainte d'exclusion, elle, voit
// deux mandats simultanés et refuse le second. Le prix était lourd : 1 258
// mandats de député dans la source, 64 en base, et pas un message. Chaque
// député n'y gardait que sa première élection.
//
// Le recollement clôt un mandat la veille du suivant. C'est ce que la source
// veut dire, et c'est la seule lecture compatible avec un siège qui ne se
// détient pas deux fois. Elle ne vaut QUE pour les sièges parlementaires : un
// ministre peut cumuler deux portefeuilles, et ses chevauchements restent des
// faits, pas des bavures.
func recoller(mandats []mandat, personByUID map[string]int64) map[string]string {
	type cle struct {
		pid   int64
		type_ string
	}
	suites := map[cle][]*mandat{}
	for i := range mandats {
		m := &mandats[i]
		t := m.TypeOrgane.String()
		if t != "ASSEMBLEE" && t != "SENAT" {
			continue
		}
		pid, ok := personByUID[m.ActeurRef.String()]
		if !ok || m.DateDebut.String() == "" {
			continue
		}
		suites[cle{pid, t}] = append(suites[cle{pid, t}], m)
	}

	fins := map[string]string{}
	for _, suite := range suites {
		// mandats est déjà trié par date de début ; suite en hérite.
		for i := 0; i < len(suite)-1; i++ {
			fin, debutSuivant := suite[i].DateFin.String(), suite[i+1].DateDebut.String()
			if fin == "" || fin < debutSuivant {
				continue
			}
			d, err := time.Parse("2006-01-02", debutSuivant)
			if err != nil {
				continue
			}
			veille := d.AddDate(0, 0, -1).Format("2006-01-02")
			// Un mandat qui commencerait après sa propre fin corrigée est une
			// vraie anomalie de source : elle reste signalée par l'absence.
			if veille < suite[i].DateDebut.String() {
				continue
			}
			fins[suite[i].UID.String()] = veille
		}
	}
	return fins
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
	// EN UNE TRANSACTION. Ces suppressions s'enchaînent dans l'ordre des
	// dépendances ; si l'une échoue à mi-parcours et que les précédentes ont
	// déjà été validées, la base reste à moitié vidée — 1,27 million de votes
	// effacés, les organisations encore là, et plus aucun moyen de savoir où on
	// en était. C'est arrivé : deux exécutions concurrentes se sont marché
	// dessus, l'une reconstruisant ce que l'autre effaçait, et le message final
	// parlait d'une clé étrangère au lieu de parler d'une course.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, q := range []string{
		// Portée limitée à l'Assemblée : un DELETE global emporterait les votes
		// du Parlement européen, qui viennent d'un autre connecteur.
		`DELETE FROM core.ballot_group_exception e USING core.scrutin s
		  WHERE s.id = e.scrutin_id AND s.institution = 'ASSEMBLEE_NATIONALE'`,
		`DELETE FROM core.ballot_group g USING core.scrutin s
		  WHERE s.id = g.scrutin_id AND s.institution = 'ASSEMBLEE_NATIONALE'`,
		`DELETE FROM core.ballot b USING core.scrutin s
		  WHERE s.id = b.scrutin_id AND s.institution = 'ASSEMBLEE_NATIONALE'`,
		// Les affiliations proviennent toutes de l'Assemblée aujourd'hui ; la
		// portée est bornée malgré tout, pour que l'arrivée d'un connecteur
		// d'affiliations sénatoriales ne fasse pas de dégât silencieux.
		`DELETE FROM core.affiliation a
		  WHERE EXISTS (SELECT 1 FROM core.organization_identifier i
		                WHERE i.organization_id = a.organization_id AND i.scheme = 'AN_ORGANE')`,
		// Les nuances sont attribuées à des mandats locaux par le connecteur
		// des communes. L'Assemblée n'en produit aucune, et n'a donc rien à
		// effacer ici : la ligne est retirée plutôt que bornée.
		// Portée limitée aux mandats que CE connecteur crée. « institution IS
		// NULL » couvrait autrefois les seuls mandats ministériels ; depuis que
		// le RNE est chargé, il couvrirait aussi 613 000 mandats locaux et les
		// présidences de la République, qu'une simple renormalisation de
		// l'Assemblée effacerait sans un mot.
		`DELETE FROM core.mandate
		  WHERE institution = 'ASSEMBLEE_NATIONALE'
		     OR (institution IS NULL AND mandate_type = 'MINISTRE')`,
		// Les dossiers doivent partir avant les organisations : texte_author et
		// dossier_author y renvoient.
		// TOUTES ces suppressions sont bornées à l'Assemblée. Elles ne
		// l'étaient pas, et le prix a été payé : un `DELETE FROM core.dossier`
		// sans clause a emporté les 8 412 dossiers du Sénat et les 17 660
		// assignations de thèmes qui s'y rattachaient, faisant tomber
		// l'héritage de thèmes par la navette (D-019) de 4 806 à 0.
		`UPDATE core.scrutin SET dossier_id = NULL, texte_id = NULL, amendement_id = NULL
		  WHERE institution = 'ASSEMBLEE_NATIONALE'`,
		// Les interventions en séance pointent le dossier discuté. Elles
		// survivent à la renormalisation — elles viennent d'un autre jeu — mais
		// leur pointeur, lui, désigne des dossiers sur le point de disparaître.
		`UPDATE core.intervention SET dossier_id = NULL
		  WHERE institution = 'ASSEMBLEE_NATIONALE'`,
		// Les amendements se rattachent aux textes par clé étrangère. Rebâtir
		// core.texte sans les effacer d'abord faisait échouer toute la
		// normalisation — ce qui est le bon comportement : la contrainte a
		// tenu. Ils sont rechargés par le connecteur des amendements, qui
		// s'exécute après la normalisation dans la chaîne.
		`DELETE FROM core.amendement WHERE institution = 'ASSEMBLEE_NATIONALE'`,
		`DELETE FROM core.lecture l USING core.dossier d
		  WHERE d.id = l.dossier_id AND d.institution = 'ASSEMBLEE_NATIONALE'`,
		`DELETE FROM core.dossier_author a USING core.dossier d
		  WHERE d.id = a.dossier_id AND d.institution = 'ASSEMBLEE_NATIONALE'`,
		`DELETE FROM core.texte_author a USING core.texte t
		  WHERE t.id = a.texte_id AND t.institution = 'ASSEMBLEE_NATIONALE'`,
		`DELETE FROM core.texte WHERE institution = 'ASSEMBLEE_NATIONALE'`,
		`DELETE FROM core.dossier WHERE institution = 'ASSEMBLEE_NATIONALE'`,
		// La cartographie éditoriale pointe les groupes de l'Assemblée. Elle est
		// rechargée juste après la normalisation (cmd/ingest), mais tant qu'elle
		// pointe des organisations sur le point d'être détruites, la clé
		// étrangère refuse la suppression — et le connecteur échouerait sans
		// qu'on comprenne pourquoi.
		`DELETE FROM core.party_group_link l
		  WHERE EXISTS (SELECT 1 FROM core.organization_identifier i
		                WHERE i.organization_id = l.group_id AND i.scheme = 'AN_ORGANE')`,
		// Enfin les entités portant un identifiant de l'Assemblée, et elles seules.
		`DELETE FROM core.organization o
		  WHERE EXISTS (SELECT 1 FROM core.organization_identifier i
		                 WHERE i.organization_id = o.id AND i.scheme = 'AN_ORGANE')`,
		// Les PERSONNES ne sont plus détruites. Elles l'étaient quand
		// l'Assemblée en était le seul producteur ; depuis que le RNE et la
		// HATVP y rattachent des mandats et des déclarations, effacer une
		// personne parce qu'elle est députée emporterait son mandat de maire.
		// Elles sont donc mises à jour en place, par leur identifiant.
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return fmt.Errorf("remise à zéro : %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("remise à zéro : %w", err)
	}

	// Un NOTICE avant chaque phase, pas seulement le compte après : sans ça,
	// « etape=normalize » puis plus rien pendant plusieurs minutes se lit
	// comme une commande bloquée plutôt que comme une phase en cours — voir
	// la session qui a introduit ce commentaire (normalize seule, sur une
	// base réelle, prend facilement deux minutes).
	logs.Notice("normalisation : organes")
	orgByUID, err := normalizeOrganes(ctx, pool)
	if err != nil {
		return fmt.Errorf("organes : %w", err)
	}
	logs.Notice("organes normalisés", "count", len(orgByUID))

	logs.Notice("normalisation : acteurs")
	personByUID, err := normalizeActeurs(ctx, pool)
	if err != nil {
		return fmt.Errorf("acteurs : %w", err)
	}
	logs.Notice("acteurs normalisés", "count", len(personByUID))

	labels, err := organeLabels(ctx, pool)
	if err != nil {
		return fmt.Errorf("libellés d'organes : %w", err)
	}

	logs.Notice("normalisation : mandats et appartenances")
	nMandats, err := normalizeMandats(ctx, pool, personByUID, orgByUID, labels)
	if err != nil {
		return fmt.Errorf("mandats : %w", err)
	}
	logs.Notice("mandats et appartenances normalisés", "count", nMandats)

	logs.Notice("normalisation : scrutins")
	nScr, nBal, err := normalizeScrutins(ctx, pool, personByUID, orgByUID)
	if err != nil {
		return fmt.Errorf("scrutins : %w", err)
	}
	logs.Notice("scrutins normalisés", "scrutins", nScr, "votes_nominatifs", nBal)

	logs.Notice("normalisation : dossiers")
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
	logs.Notice("scrutins normalisés", "scrutins", nScr, "votes_nominatifs", nBal)
	return nil
}

var _ = pgx.Identifier{}

// trouverPersonne renvoie l'identifiant d'une personne déjà connue, ou 0.
// L'ordre compte : l'identifiant de l'Assemblée fait foi ; à défaut, le triplet
// exact (nom, prénom, date de naissance) rattrape les personnes créées par un
// autre connecteur. Sans date de naissance, aucun rapprochement n'est tenté —
// la même règle que pour le RNE (D-025).
func trouverPersonne(ctx context.Context, pool *pgxpool.Pool, uid, nom, prenom string, naissance any) (int64, error) {
	var id int64
	err := pool.QueryRow(ctx, `
		SELECT person_id FROM core.person_identifier
		 WHERE scheme = 'AN_ACTEUR' AND value = $1`, uid).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != pgx.ErrNoRows {
		return 0, err
	}
	if naissance == nil {
		return 0, nil
	}
	err = pool.QueryRow(ctx, `
		SELECT id FROM core.person
		 WHERE birth_date = $3::date
		   AND core.f_unaccent(lower(family_name)) = core.f_unaccent(lower($1))
		   AND core.f_unaccent(lower(given_name))  = core.f_unaccent(lower($2))
		 ORDER BY id LIMIT 1`, nom, prenom, naissance).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return id, err
}
