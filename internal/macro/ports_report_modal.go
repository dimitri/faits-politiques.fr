package macro

import (
	"context"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceReportModalPort = archive.Source{
	Slug: "dgitm-observatoire-performance-portuaire-2024", Label: "Observatoire de la performance portuaire et des chaînes logistiques, édition 2024",
	Publisher: "DGITM (ministère de la Transition écologique)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : DGITM, Observatoire de la performance portuaire, édition 2024 (données 2023)",
	Cadence:     "irrégulière",
	Notes: "Un seul millésime (2023), pas une série. « Part massifiée » = fer + fleuve, tous " +
		"trafics confondus (marchandise et conteneur mélangés) — pas comparable, en l'état, aux " +
		"répartitions conteneurs seules publiées par Anvers/Rotterdam (voir core.report_modal_conteneurs).",
}

// reportModalPortPoint : vérifié directement dans le rapport DGITM 2024
// (télécharger et lire le PDF), recoupé avec les publications propres de
// chaque port (RSE Dunkerque 2024, Distriport GPMM, HAROPA S1 2024) —
// Marseille-Fos n'est cité par le rapport que comme « moins de 20 % »,
// sans chiffre exact publié : est_plafond=true plutôt qu'une fausse
// précision.
type reportModalPortPoint struct {
	port                   string
	anneeRef               int
	massifiee, fer, fleuve *float64
	estPlafond             bool
	note                   string
}

var reportModalPorts = []reportModalPortPoint{
	{
		port: "Dunkerque", anneeRef: 2023,
		massifiee: f64p(52), fer: f64p(31), fleuve: f64p(21),
		note: "1ʳᵉ place portuaire française de fret ferroviaire selon la DGITM ; recoupé avec le RSE 2024 de Dunkerque-Port (fer 2,6 Mt, fluvial 1,7 Mt, sur un trafic hors pipeline de 8,3 Mt).",
	},
	{
		port: "HAROPA", anneeRef: 2023,
		massifiee: f64p(29),
		note:      "Majoritairement fluvial (6,3 Mt en 2022) ; la part ferroviaire seule reste sous les 10 % (environ 3 Mt en 2023) mais aucun pourcentage exact n'est publié pour ce seul mode.",
	},
	{
		port: "Marseille-Fos", anneeRef: 2023,
		estPlafond: true, massifiee: f64p(20),
		note: "La DGITM ne publie qu'un plafond (« moins de 20 % ») pour l'ensemble du trafic, la route repassant au-dessus de 80 % pour les conteneurs. Pour les seuls conteneurs, le bilan Distriport du GPMM (2024) donne un chiffre exact : 17 % fer, 6 % fleuve (voir la comparaison européenne, core.report_modal_conteneurs).",
	},
	{
		port: "Nantes-Saint-Nazaire", anneeRef: 2023,
		massifiee: f64p(4),
		note:      "Quasi intégralement fluvial ; le trafic fluvial du port (essentiellement du charbon vers Cordemais) s'est effondré de 1,2 Mt à 0,3 Mt entre 2022 et 2023, ce qui explique la faiblesse du chiffre autant qu'une politique modale.",
	},
}

func IngestReportModalPort(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceReportModalPort)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "report-modal-port-v1")
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
	if _, err := tx.Exec(ctx, `DELETE FROM core.report_modal_port`); err != nil {
		return fail(err)
	}
	for _, p := range reportModalPorts {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.report_modal_port
				(port, annee, part_massifiee_pct, part_fer_pct, part_fleuve_pct, est_plafond, note, source_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			p.port, p.anneeRef, p.massifiee, p.fer, p.fleuve, p.estPlafond, p.note, srcID); err != nil {
			return fail(fmt.Errorf("%s : insertion : %w", p.port, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"ports": len(reportModalPorts)}, "")
	fmt.Printf("  report modal par port : %d ports\n", len(reportModalPorts))
	return nil
}

var SourceReportModalConteneurs = archive.Source{
	Slug: "report-modal-conteneurs-europe", Label: "Répartition modale du transport de conteneurs vers l'arrière-pays, ports comparés",
	Publisher: "GPMM (Distriport), Port of Antwerp-Bruges, Havenbedrijf Rotterdam", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : bilan Distriport du GPMM (2024), Port of Antwerp-Bruges (2024), Havenbedrijf Rotterdam (2023)",
	Cadence:     "irrégulière",
	Notes: "Restreint aux CONTENEURS uniquement, jamais à comparer aux chiffres tous-trafics de " +
		"core.report_modal_port. HAROPA et Hambourg exclus : aucun pourcentage conteneurs seuls " +
		"publié pour HAROPA (seulement des volumes bruts en EVP) ; la publication de Hambourg " +
		"n'a pas pu être vérifiée directement.",
}

type reportModalConteneursPoint struct {
	port, pays         string
	annee              int
	fer, fleuve, route float64
	routeCalculee      bool
}

var reportModalConteneurs = []reportModalConteneursPoint{
	// GPMM : bilan Distriport, 2024 — seuls fer et fleuve publiés, route
	// déduite par complément à 100 (signalé par routeCalculee).
	{"Marseille-Fos", "France", 2024, 17, 6, 77, true},
	{"Anvers-Bruges", "Belgique", 2024, 6.9, 33.9, 59.2, false},
	{"Rotterdam", "Pays-Bas", 2023, 10.3, 30.5, 59.2, false},
}

func IngestReportModalConteneurs(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceReportModalConteneurs)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "report-modal-conteneurs-v1")
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
	if _, err := tx.Exec(ctx, `DELETE FROM core.report_modal_conteneurs`); err != nil {
		return fail(err)
	}
	for _, p := range reportModalConteneurs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.report_modal_conteneurs
				(port, pays, annee, part_fer_pct, part_fleuve_pct, part_route_pct, route_calculee, source_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			p.port, p.pays, p.annee, p.fer, p.fleuve, p.route, p.routeCalculee, srcID); err != nil {
			return fail(fmt.Errorf("%s : insertion : %w", p.port, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"ports": len(reportModalConteneurs)}, "")
	fmt.Printf("  report modal conteneurs (comparaison européenne) : %d ports\n", len(reportModalConteneurs))
	return nil
}
