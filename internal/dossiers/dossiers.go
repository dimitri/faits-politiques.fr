// Package dossiers charge les faits des dossiers documentaires (docs/*.md)
// dans la structure commune de D-066 : contexte, enjeux, cadre, contrôles et
// évaluations, situation chiffrée. Chaque fait porte sa preuve — document
// scellé relu, texte du Journal officiel chargé, ou prise de parole à
// l'Assemblée nationale chargée — et, quand il cite une personne publique, le
// lien vers sa fiche. Les mots suivis dans les débats et les acteurs nommés par
// les dossiers sont chargés avec eux.
package dossiers

import (
	"context"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "dossiers-v1"

var SourceFaitsDossiers = archive.Source{
	Slug: "faits-dossiers", Label: "Faits des dossiers : textes, constats, évaluations et déclarations sourcés",
	Publisher: "Institutions citées fait par fait (Parlement, juridictions, autorités, ministères, Journal officiel) ; " +
		"déclarations de personnes et d'organismes signalées comme telles",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Documents publics des institutions, cités avec lien et page",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Sources citées fait par fait (colonne source_url)",
	Cadence:     "au fil des dossiers",
	Notes: "Faits transcrits un par un ; chaque preuve est relue au chargement : la phrase attendue doit figurer " +
		"dans le document scellé, le texte du Journal officiel ou la prise de parole chargée. Qualités : " +
		"ref.qualite_fait. Une prise de parole en séance est toujours DECLARATIF.",
}

// URL des comptes rendus de l'Assemblée chargés (internal/an) : la preuve d'une
// prise de parole est le paragraphe chargé, identifié par son slug.
const urlComptesRendusAN = "https://data.assemblee-nationale.fr/static/openData/repository/17/vp/syceronbrut/syseron.xml.zip"

// Fait : une affirmation d'un dossier et sa preuve. Exactement une preuve
// parmi : Seance (prise de parole de Personne à l'Assemblée ce jour-là), JO
// (texte du Journal officiel chargé), URL (document scellé) ; PRESSE seul
// dispense de preuve archivée.
type Fait struct {
	ID, Dossier, Section, Theme, Type string
	Date, Auteur                      string
	Personne                          string // slug de core.person : lie la fiche
	Groupe, Intitule                  string
	Montant, Nature                   string
	Constat, URL, Page, Qualite       string
	JO, JOArticle                     string
	Seance                            string
	Attendus                          []string
}

// Terme : une expression par laquelle un dossier est repéré dans les débats.
type Terme struct{ Dossier, Libelle, Motif string }

// Mission : une mission budgétaire de l'État suivie par un dossier. Le motif est
// une expression régulière sur core.budget_programme.mission_libelle, parce que
// le libellé change d'un projet de loi de finances à l'autre.
type Mission struct{ Dossier, Libelle, Motif string }

var (
	faits    []Fait
	termes   []Terme
	missions []Mission
)

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	for _, e := range []func(context.Context, *pgxpool.Pool, *archive.Archive) error{IngestFaits, IngestActeurs} {
		if err := e(ctx, pool, arch); err != nil {
			return err
		}
	}
	return nil
}

func executer(ctx context.Context, arch *archive.Archive, src archive.Source,
	f func(srcID, runID int64) (map[string]any, error)) error {
	srcID, err := arch.EnsureSource(ctx, src)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	stats, err := f(srcID, runID)
	if err != nil {
		err = fmt.Errorf("%s : %w", src.Slug, err)
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}
	arch.EndRun(ctx, runID, "SUCCESS", stats, "")
	fmt.Printf("  %-34s %v\n", src.Slug, stats)
	return nil
}

var (
	reBalises = regexp.MustCompile(`(?s)<script.*?</script>|<style.*?</style>|<[^>]+>`)
	reBlancs  = regexp.MustCompile(`\s+`)
)

// normaliser ramène espaces insécables, apostrophes droites ou courbes et
// retours à la ligne à une forme unique : un compte rendu écrit « l’État », un
// texte du JORF « l'Etat », et la phrase attendue doit se retrouver dans les deux.
func normaliser(s string) string {
	s = strings.NewReplacer("\u00a0", " ", "\u202f", " ", "\u2009", " ", "\u2019", "'").Replace(s)
	return strings.TrimSpace(reBlancs.ReplaceAllString(s, " "))
}

func texteHTML(fragment string) string {
	return normaliser(html.UnescapeString(reBalises.ReplaceAllString(fragment, " ")))
}

// textePDF passe par pdftotext (poppler-utils), comme internal/numerique : le
// projet n'embarque pas de lecteur PDF.
func textePDF(ctx context.Context, path string) (string, error) {
	out, err := exec.CommandContext(ctx, "pdftotext", path, "-").Output()
	if err != nil {
		return "", fmt.Errorf("pdftotext (paquet poppler-utils) : %w", err)
	}
	return normaliser(string(out)), nil
}

func nul(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func IngestFaits(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executer(ctx, arch, SourceFaitsDossiers, func(srcID, runID int64) (map[string]any, error) {
		vus := map[string]bool{}
		for _, f := range faits {
			if vus[f.ID] {
				return nil, fmt.Errorf("fait %s déclaré deux fois", f.ID)
			}
			vus[f.ID] = true
		}
		docs := map[string]int64{}
		textesDocs := map[string]string{}
		type preuve struct {
			doc, jo, intervention, personne any
			url, page                       string
		}
		preuves := map[string]preuve{}
		parDossier := map[string]int{}
		for _, f := range faits {
			p := preuve{url: f.URL, page: f.Page}
			var texte string
			switch {
			case f.Seance != "":
				// La prise de parole de la personne ce jour-là qui contient
				// toutes les phrases attendues.
				rows, err := pool.Query(ctx, `SELECT i.slug, i.person_id, i.numero_seance, i.contenu
					FROM core.intervention i JOIN core.person pe ON pe.id = i.person_id
					WHERE pe.slug = $1 AND i.date_seance = $2::date ORDER BY i.ordre`, f.Personne, f.Seance)
				if err != nil {
					return nil, err
				}
				trouve := false
				for rows.Next() {
					var slug, contenu string
					var pid int64
					var num *string
					if err := rows.Scan(&slug, &pid, &num, &contenu); err != nil {
						rows.Close()
						return nil, err
					}
					t := normaliser(contenu)
					ok := true
					for _, a := range f.Attendus {
						if !strings.Contains(t, normaliser(a)) {
							ok = false
							break
						}
					}
					if ok && !trouve {
						trouve = true
						p.intervention, p.personne = slug, pid
						if p.url == "" {
							p.url = urlComptesRendusAN
						}
						if p.page == "" && num != nil {
							p.page = "séance n° " + *num
						}
					}
				}
				rows.Close()
				if !trouve {
					return nil, fmt.Errorf("%s : aucune prise de parole de %s le %s ne contient les phrases attendues (charger -only=interventions)", f.ID, f.Personne, f.Seance)
				}
				preuves[f.ID] = p
				parDossier[f.Dossier]++
				continue
			case f.JO != "":
				q := `SELECT coalesce(titre_complet, titre, '') FROM jo.texte WHERE id = $1`
				args := []any{f.JO}
				if f.JOArticle != "" {
					q = `SELECT coalesce(string_agg(contenu, ' '), '') FROM jo.bloc WHERE texte_id = $1 AND article_num = $2`
					args = append(args, f.JOArticle)
				}
				if err := pool.QueryRow(ctx, q, args...).Scan(&texte); err != nil {
					return nil, fmt.Errorf("%s : texte %s absent du corpus JORF (charger -only=jorf-complet) : %w", f.ID, f.JO, err)
				}
				texte = texteHTML(texte)
				p.jo = f.JO
				if p.url == "" {
					p.url = "https://www.legifrance.gouv.fr/jorf/id/" + f.JO
				}
			case f.Qualite == "PRESSE":
			default:
				if f.URL == "" {
					return nil, fmt.Errorf("%s : ni séance, ni texte du JORF, ni document", f.ID)
				}
				if _, ok := docs[f.URL]; !ok {
					ext := ".html"
					if strings.HasSuffix(f.URL, ".pdf") {
						ext = ".pdf"
					}
					d, err := arch.Fetch(ctx, srcID, runID, f.URL, ext)
					if err != nil {
						return nil, fmt.Errorf("%s : %w", f.ID, err)
					}
					docs[f.URL] = d.DocumentID
					// Certains registres officiels (le registre public du Conseil de
					// l'UE, par exemple) servent un PDF sans extension .pdf dans
					// l'URL — se fier à la suite d'octets réelle plutôt qu'au seul
					// nom, pour ne pas lire un PDF comme du HTML.
					entete := make([]byte, 5)
					fEntete, err := os.Open(d.Path)
					if err != nil {
						return nil, err
					}
					_, err = io.ReadFull(fEntete, entete)
					fEntete.Close()
					estPDF := ext == ".pdf" || (err == nil && string(entete) == "%PDF-")
					if estPDF {
						if textesDocs[f.URL], err = textePDF(ctx, d.Path); err != nil {
							return nil, err
						}
					} else {
						b, err := os.ReadFile(d.Path)
						if err != nil {
							return nil, err
						}
						textesDocs[f.URL] = texteHTML(string(b))
					}
				}
				texte = textesDocs[f.URL]
				p.doc = docs[f.URL]
			}
			for _, a := range f.Attendus {
				if !strings.Contains(texte, normaliser(a)) {
					return nil, fmt.Errorf("%s : « %s » absent de %s", f.ID, a, p.url)
				}
			}
			if f.Personne != "" {
				var pid int64
				if err := pool.QueryRow(ctx, `SELECT id FROM core.person WHERE slug = $1`, f.Personne).Scan(&pid); err != nil {
					return nil, fmt.Errorf("%s : personne %s inconnue : %w", f.ID, f.Personne, err)
				}
				p.personne = pid
			}
			preuves[f.ID] = p
			parDossier[f.Dossier]++
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `DELETE FROM ref.fait_dossier`); err != nil {
			return nil, err
		}
		for _, f := range faits {
			p := preuves[f.ID]
			if _, err := tx.Exec(ctx, `INSERT INTO ref.fait_dossier
				(id, dossier, section, theme, type, date_fait, auteur, person_id, groupe, intitule, montant_eur, nature_montant,
				 constat, source_url, page, qualite, source_id, document_id, jo_texte_id, intervention_slug)
				VALUES ($1,$2,$3,$4,$5,$6::date,$7,$8,$9,$10,$11::numeric,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
				f.ID, f.Dossier, f.Section, nul(f.Theme), f.Type, nul(f.Date), f.Auteur, p.personne, nul(f.Groupe), f.Intitule,
				nul(f.Montant), nul(f.Nature), f.Constat, p.url, nul(p.page), f.Qualite, srcID, p.doc, p.jo, p.intervention); err != nil {
				return nil, fmt.Errorf("%s : %w", f.ID, err)
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM ref.dossier_terme`); err != nil {
			return nil, err
		}
		for _, t := range termes {
			if _, err := tx.Exec(ctx, `INSERT INTO ref.dossier_terme (dossier, libelle, motif) VALUES ($1,$2,$3)`,
				t.Dossier, t.Libelle, t.Motif); err != nil {
				return nil, fmt.Errorf("terme %s/%s : %w", t.Dossier, t.Libelle, err)
			}
		}
		// Une mission déclarée qui ne correspond à aucune ligne du budget chargé
		// laisserait un tableau vide sans le dire : c'est une erreur de chargement.
		if _, err := tx.Exec(ctx, `DELETE FROM ref.dossier_mission`); err != nil {
			return nil, err
		}
		for _, m := range missions {
			var n int
			if err := tx.QueryRow(ctx, `SELECT count(DISTINCT mission_libelle) FROM core.budget_programme WHERE mission_libelle ~ $1`, m.Motif).Scan(&n); err != nil {
				return nil, fmt.Errorf("mission %s/%s : %w", m.Dossier, m.Libelle, err)
			}
			if n == 0 {
				return nil, fmt.Errorf("mission %s/%s : aucune ligne de core.budget_programme ne correspond à %q (charger -only=budget)", m.Dossier, m.Libelle, m.Motif)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO ref.dossier_mission (dossier, libelle, motif) VALUES ($1,$2,$3)`,
				m.Dossier, m.Libelle, m.Motif); err != nil {
				return nil, fmt.Errorf("mission %s/%s : %w", m.Dossier, m.Libelle, err)
			}
		}
		stats := map[string]any{"faits": len(faits), "documents": len(docs), "termes": len(termes), "missions": len(missions)}
		for d, n := range parDossier {
			stats[d] = n
		}
		return stats, tx.Commit(ctx)
	})
}
