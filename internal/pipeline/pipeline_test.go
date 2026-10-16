package pipeline

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// étape minimale : exécute f, renvoie nom pour que les dépendantes puissent
// le lire dans Results si besoin.
func addStep(reg *Registry, nom string, deps []string, f func(ctx context.Context, d Results) (any, error)) {
	reg.Add(Step{Name: nom, Description: nom, Dependencies: deps, Run: f})
}

func TestRunRespectsDependencies(t *testing.T) {
	reg := NewRegistry(nil)
	var ordre []string
	var mu sync.Mutex
	noter := func(nom string) func(ctx context.Context, d Results) (any, error) {
		return func(ctx context.Context, d Results) (any, error) {
			mu.Lock()
			ordre = append(ordre, nom)
			mu.Unlock()
			return nom, nil
		}
	}
	addStep(reg, "a", nil, noter("a"))
	addStep(reg, "b", []string{"a"}, noter("b"))
	addStep(reg, "c", []string{"b"}, noter("c"))

	resultats, err := reg.Run(context.Background(), reg.Names(), Options{Concurrency: 4})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := []string{ordre[0], ordre[1], ordre[2]}; got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("ordre attendu a,b,c ; obtenu %v", ordre)
	}
	if resultats["a"] != "a" || resultats["b"] != "b" || resultats["c"] != "c" {
		t.Fatalf("resultats inattendus : %v", resultats)
	}
}

// TestRunDoesNotWaitForWave reproduit le cas réel ayant motivé la
// réécriture : "media" ne dépend que de "carto" (rapide), pas de "communes"
// (lent), bien que les deux tombent dans la même vague nominale que
// "communes" sous l'ancien découpage par Levels. Une étape dont la seule
// dépendance est déjà prête ne doit pas attendre qu'un autre membre de sa
// vague nominale, sans rapport, ait fini.
func TestRunDoesNotWaitForWave(t *testing.T) {
	reg := NewRegistry(nil)
	var finCarto, finMedia, finCommunes time.Time
	var mu sync.Mutex

	addStep(reg, "carto", nil, func(ctx context.Context, d Results) (any, error) {
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		finCarto = time.Now()
		mu.Unlock()
		return nil, nil
	})
	// "communes" tombe dans la même vague nominale que carto (aucune
	// dépendance), mais est délibérément lent.
	addStep(reg, "communes", nil, func(ctx context.Context, d Results) (any, error) {
		time.Sleep(300 * time.Millisecond)
		mu.Lock()
		finCommunes = time.Now()
		mu.Unlock()
		return nil, nil
	})
	// "media" ne dépend que de carto : sous l'ancien découpage par vagues,
	// elle tombait dans la vague SUIVANTE (celle de communes) et attendait
	// sa fin pour rien.
	addStep(reg, "media", []string{"carto"}, func(ctx context.Context, d Results) (any, error) {
		mu.Lock()
		finMedia = time.Now()
		mu.Unlock()
		return nil, nil
	})

	if _, err := reg.Run(context.Background(), reg.Names(), Options{Concurrency: 4}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if finMedia.After(finCommunes) {
		t.Fatalf("media (dépend seulement de carto, fini à %v) a attendu communes (fini à %v) : "+
			"le répartiteur sérialise encore par vague nominale", finMedia, finCommunes)
	}
	if finMedia.Before(finCarto) {
		t.Fatalf("media a démarré avant que sa propre dépendance carto (fini à %v) ne soit prête (media à %v)",
			finCarto, finMedia)
	}
}

func TestRunLimitsConcurrency(t *testing.T) {
	reg := NewRegistry(nil)
	var enCours int32
	var maxVu int32
	for i := 0; i < 12; i++ {
		addStep(reg, string(rune('a'+i)), nil, func(ctx context.Context, d Results) (any, error) {
			n := atomic.AddInt32(&enCours, 1)
			for {
				m := atomic.LoadInt32(&maxVu)
				if n <= m || atomic.CompareAndSwapInt32(&maxVu, m, n) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&enCours, -1)
			return nil, nil
		})
	}
	if _, err := reg.Run(context.Background(), reg.Names(), Options{Concurrency: 3}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if maxVu > 3 {
		t.Fatalf("concurrence observée %d > limite 3", maxVu)
	}
	if maxVu < 2 {
		t.Fatalf("aucune exécution concurrente observée (maxVu=%d) : le test ne prouve rien", maxVu)
	}
}

// TestRunStopsOnError : une étape qui échoue ne doit jamais faire
// partir ses dépendantes (elles ne peuvent de toute façon pas devenir
// prêtes), et ne doit pas non plus laisser partir une étape sans rapport,
// pas encore lancée au moment de l'échec — la même garantie qu'avant,
// « aucune vague suivante ne démarre », au niveau du graphe entier.
func TestRunStopsOnError(t *testing.T) {
	reg := NewRegistry(nil)
	var aDemarre int32

	echoue := errors.New("échec simulé")
	addStep(reg, "rate", nil, func(ctx context.Context, d Results) (any, error) {
		return nil, echoue
	})
	addStep(reg, "depend-de-rate", []string{"rate"}, func(ctx context.Context, d Results) (any, error) {
		atomic.AddInt32(&aDemarre, 1)
		return nil, nil
	})
	// Sans rapport avec "rate", mais lancée avec Concurrency: 1 seulement
	// après elle dans l'ordre de déclaration : ne doit jamais démarrer.
	addStep(reg, "sans-rapport", nil, func(ctx context.Context, d Results) (any, error) {
		atomic.AddInt32(&aDemarre, 1)
		return nil, nil
	})

	_, err := reg.Run(context.Background(), reg.Names(), Options{Concurrency: 1})
	if err == nil {
		t.Fatal("Run : attendu une erreur")
	}
	if !errors.Is(err, echoue) {
		t.Fatalf("erreur attendue enveloppant %v, obtenu %v", echoue, err)
	}
	if aDemarre != 0 {
		t.Fatalf("%d étape(s) n'auraient jamais dû démarrer après l'échec", aDemarre)
	}
}

// TestRunContinueOnErrorRunsIndependentSteps : le cas visé par
// ContinueOnError — une source d'ingestion en échec (un site externe en
// panne) ne doit pas faire perdre les sources SANS RAPPORT de la même
// catégorie. "depend-de-rate" ne doit jamais démarrer (sa dépendance a
// échoué) ; "sans-rapport" doit, elle, tourner jusqu'au bout.
func TestRunContinueOnErrorRunsIndependentSteps(t *testing.T) {
	reg := NewRegistry(nil)
	var aDemarre int32

	echoue := errors.New("échec simulé")
	addStep(reg, "rate", nil, func(ctx context.Context, d Results) (any, error) {
		return nil, echoue
	})
	addStep(reg, "depend-de-rate", []string{"rate"}, func(ctx context.Context, d Results) (any, error) {
		atomic.AddInt32(&aDemarre, 1)
		return nil, nil
	})
	addStep(reg, "sans-rapport", nil, func(ctx context.Context, d Results) (any, error) {
		atomic.AddInt32(&aDemarre, 1)
		return "fait", nil
	})

	resultats, err := reg.Run(context.Background(), reg.Names(), Options{Concurrency: 2, ContinueOnError: true})
	if err == nil {
		t.Fatal("Run : attendu une erreur agrégée, les échecs ne doivent pas disparaître silencieusement")
	}
	if !errors.Is(err, echoue) {
		t.Fatalf("erreur attendue enveloppant %v, obtenu %v", echoue, err)
	}
	if resultats["sans-rapport"] != "fait" {
		t.Fatalf("« sans-rapport » aurait dû s'exécuter malgré l'échec de « rate » ; résultats = %v", resultats)
	}
	if _, ran := resultats["depend-de-rate"]; ran {
		t.Fatal("« depend-de-rate » n'aurait jamais dû s'exécuter : sa dépendance a échoué")
	}
}

// TestRunContinueOnErrorDoesNotCancelSiblings : la garantie qui distingue
// ContinueOnError de errgroup.WithContext tout seul — un échec ne doit PAS
// annuler le ctx que voient les étapes indépendantes déjà en vol : sans
// stepCtx (voir Run), toute étape encore active au moment de l'échec
// verrait son propre ctx.Err() non-nil et échouerait en cascade.
func TestRunContinueOnErrorDoesNotCancelSiblings(t *testing.T) {
	reg := NewRegistry(nil)
	var longueVitAnnulee int32

	addStep(reg, "rate-vite", nil, func(ctx context.Context, d Results) (any, error) {
		return nil, errors.New("échec immédiat")
	})
	addStep(reg, "longue", nil, func(ctx context.Context, d Results) (any, error) {
		time.Sleep(50 * time.Millisecond)
		if ctx.Err() != nil {
			atomic.AddInt32(&longueVitAnnulee, 1)
		}
		return nil, nil
	})

	if _, err := reg.Run(context.Background(), reg.Names(), Options{Concurrency: 2, ContinueOnError: true}); err == nil {
		t.Fatal("Run : attendu une erreur agrégée")
	}
	if longueVitAnnulee != 0 {
		t.Fatal("« longue » a vu son contexte annulé par l'échec de « rate-vite » : " +
			"ContinueOnError doit isoler les étapes indépendantes, pas seulement empêcher leurs dépendantes")
	}
}
