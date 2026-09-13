package jorf

import "testing"

// Le texte du décret du 12 octobre 2025 (JORFTEXT000052382035), tel que le
// connecteur l'extrait du Journal officiel. Extrait réduit : les trois formes
// de section y figurent, et les noms difficiles aussi — particule en minuscules
// (« Amélie de MONTCHALIN »), particule capitalisée (« Anne Le HÉNANFF »),
// patronyme composé (« Charlotte PARMENTIER-LECOCQ »), capitale accentuée
// (« Aurore BERGÉ »).
const decret20251012 = `Sont nommés ministres : M. Laurent NUNEZ, ministre de l'intérieur ; Mme Amélie de MONTCHALIN, ministre de l'action et des comptes publics ; M. Gérald DARMANIN, garde des sceaux, ministre de la justice ; Mme Charlotte PARMENTIER-LECOCQ, ministre des solidarités.
Sont nommés ministres délégués auprès du Premier ministre et participent au conseil des ministres : M. Laurent PANIFOUS, chargé des relations avec le Parlement ; Mme Maud BREGEON, porte-parole du Gouvernement.
Sont nommés ministres délégués et participent au conseil des ministres pour les affaires relevant de leurs attributions : - Auprès du Premier ministre : Mme Aurore BERGÉ, chargée de l'égalité entre les femmes et les hommes ; - Auprès du ministre de l'économie, des finances et de la souveraineté industrielle : Mme Anne Le HÉNANFF, chargée de l'intelligence artificielle et du numérique.
Le présent décret sera publié au Journal officiel de la République française.`

func TestLireComposition(t *testing.T) {
	m := LireComposition(decret20251012)
	if len(m) != 8 {
		for _, x := range m {
			t.Logf("%+v", x)
		}
		t.Fatalf("8 membres attendus, %d trouvés", len(m))
	}

	attendu := []Membre{
		{Rang: 1, Sens: SensNomination, Fonction: FonctionMinistre, Civilite: "M.",
			Prenom: "Laurent", Nom: "NUNEZ", Portefeuille: "ministre de l'intérieur"},
		{Rang: 2, Sens: SensNomination, Fonction: FonctionMinistre, Civilite: "Mme",
			Prenom: "Amélie", Nom: "de MONTCHALIN", Portefeuille: "ministre de l'action et des comptes publics"},
		{Rang: 3, Sens: SensNomination, Fonction: FonctionMinistre, Civilite: "M.",
			Prenom: "Gérald", Nom: "DARMANIN", Portefeuille: "garde des sceaux, ministre de la justice"},
		{Rang: 4, Sens: SensNomination, Fonction: FonctionMinistre, Civilite: "Mme",
			Prenom: "Charlotte", Nom: "PARMENTIER-LECOCQ", Portefeuille: "ministre des solidarités"},
		{Rang: 5, Sens: SensNomination, Fonction: FonctionMinistreDelegue, Civilite: "M.",
			Prenom: "Laurent", Nom: "PANIFOUS", Portefeuille: "chargé des relations avec le Parlement"},
		{Rang: 6, Sens: SensNomination, Fonction: FonctionMinistreDelegue, Civilite: "Mme",
			Prenom: "Maud", Nom: "BREGEON", Portefeuille: "porte-parole du Gouvernement"},
		{Rang: 7, Sens: SensNomination, Fonction: FonctionMinistreDelegue, Civilite: "Mme",
			Prenom: "Aurore", Nom: "BERGÉ", Rattachement: "Premier ministre",
			Portefeuille: "chargée de l'égalité entre les femmes et les hommes"},
		{Rang: 8, Sens: SensNomination, Fonction: FonctionMinistreDelegue, Civilite: "Mme",
			Prenom: "Anne", Nom: "Le HÉNANFF",
			Rattachement: "ministre de l'économie, des finances et de la souveraineté industrielle",
			Portefeuille: "chargée de l'intelligence artificielle et du numérique"},
	}
	for i, a := range attendu {
		if m[i] != a {
			t.Errorf("membre %d :\n  obtenu  %+v\n  attendu %+v", i+1, m[i], a)
		}
	}
}

func TestLireCompositionFormeIndividuelle(t *testing.T) {
	const texte = `Mme Catherine PÉGARD est nommée ministre de la culture.
M. Sébastien LECORNU est nommé Premier ministre.`
	m := LireComposition(texte)
	if len(m) != 2 {
		for _, x := range m {
			t.Logf("%+v", x)
		}
		t.Fatalf("2 membres attendus, %d trouvés", len(m))
	}
	if m[0].Nom != "PÉGARD" || m[0].Fonction != FonctionMinistre {
		t.Errorf("premier membre : %+v", m[0])
	}
	if m[1].Nom != "LECORNU" || m[1].Fonction != FonctionPremierMinistre {
		t.Errorf("second membre : %+v", m[1])
	}
}

func TestLireCompositionCessation(t *testing.T) {
	const texte = `Il est mis fin aux fonctions de : Mme Rachida DATI, ministre de la culture ; ` +
		`Mme Charlotte PARMENTIER-LECOCQ, ministre déléguée chargée de l'autonomie.`
	m := LireComposition(texte)
	if len(m) != 2 {
		for _, x := range m {
			t.Logf("%+v", x)
		}
		t.Fatalf("2 cessations attendues, %d trouvées", len(m))
	}
	for _, x := range m {
		if x.Sens != SensCessation {
			t.Errorf("%s : sens %q, attendu CESSATION", x.Nom, x.Sens)
		}
	}
	if m[0].Fonction != FonctionMinistre || m[1].Fonction != FonctionMinistreDelegue {
		t.Errorf("fonctions : %q et %q", m[0].Fonction, m[1].Fonction)
	}
}

// Le corps d'un acte doit être trouvé quelle que soit sa profondeur. La DILA le
// place directement sous <TEXTE> dans ses livraisons quotidiennes et sous
// <STRUCT><ARTICLE> dans sa base complète ; le second cas rendait vides deux
// cents décrets sur deux cent quatre, sans erreur.
func TestCorpsProfondeurLibre(t *testing.T) {
	const plat = `<TEXTE><ID>X</ID>
	  <NOTICE><CONTENU><p>Notice à ne pas lire.</p></CONTENU></NOTICE>
	  <BLOC_TEXTUEL><CONTENU><p><br/>Sont nommés ministres :<br/>M. Jean DUPONT, ministre.</p></CONTENU></BLOC_TEXTUEL>
	</TEXTE>`
	const profond = `<TEXTE><ID>X</ID>
	  <VISAS><CONTENU><p>Vu la Constitution ;</p></CONTENU></VISAS>
	  <STRUCT><ARTICLE><NUM>1</NUM>
	    <BLOC_TEXTUEL><CONTENU><p><br/>Sont nommés ministres :<br/>M. Jean DUPONT, ministre.</p></CONTENU></BLOC_TEXTUEL>
	  </ARTICLE></STRUCT>
	</TEXTE>`
	const attendu = "Sont nommés ministres :\nM. Jean DUPONT, ministre."
	for nom, x := range map[string]string{"plat": plat, "profond": profond} {
		if got := corps([]byte(x)); got != attendu {
			t.Errorf("%s : corps = %q, attendu %q", nom, got, attendu)
		}
	}
}

// Les visas et la notice ne sont pas le dispositif : les prendre ferait entrer
// dans le corps des phrases que l'acte n'édicte pas.
func TestCorpsIgnoreVisasEtNotice(t *testing.T) {
	const x = `<TEXTE><NOTICE><CONTENU><p>Publics concernés : tout le monde.</p></CONTENU></NOTICE>
	  <VISAS><CONTENU><p>Vu le code pénal ;</p></CONTENU></VISAS></TEXTE>`
	if got := corps([]byte(x)); got != "" {
		t.Errorf("corps = %q, attendu vide", got)
	}
}
