package main

import "html/template"

// Icônes de menu : traits simples, héritant de la couleur du texte. Elles
// aident au repérage et ne portent aucune information à elles seules — chaque
// entrée garde son libellé écrit.
var icones = map[string]string{
	"candidats": `<circle cx="8" cy="5.5" r="2.6"/><path d="M2.6 14c0-2.7 2.4-4.4 5.4-4.4s5.4 1.7 5.4 4.4"/>`,
	"partis":    `<path d="M4 13.5V4.2l7-1.6v9.3"/><circle cx="3" cy="13.6" r="1.6"/><circle cx="12" cy="11.7" r="1.6"/>`,
	"assemblee": `<path d="M2 13.5h12M3.5 13.5V7M7 13.5V7M9 13.5V7M12.5 13.5V7M1.6 6.6 8 2.5l6.4 4.1z"/>`,
	"senat":     `<path d="M2.2 12.8a6.5 6.5 0 0 1 11.6 0"/><path d="M8 12.8V6.3M4.7 12.8l-1.2-4M11.3 12.8l1.2-4"/>`,
	"europe":    `<circle cx="8" cy="8" r="5.6"/><path d="M8 2.4v11.2M2.4 8h11.2M5 3.4a9 9 0 0 0 0 9.2M11 3.4a9 9 0 0 1 0 9.2"/>`,
	"themes":    `<path d="M2.6 4.2h10.8M2.6 8h10.8M2.6 11.8h6.6"/>`,
	"sources":   `<path d="M4 2.4h5.2L12 5.2v8.4H4z"/><path d="M9.2 2.4v2.8H12"/><path d="M6 8.6h4M6 11h3"/>`,
	"loupe":     `<circle cx="7.2" cy="7.2" r="4.6"/><path d="m14 14-3.5-3.5"/>`,
}

// Icone retourne le SVG d'une entrée de menu. L'icône est décorative :
// aria-hidden, et le libellé reste le porteur du sens.
func Icone(cle string) template.HTML {
	d, ok := icones[cle]
	if !ok {
		return ""
	}
	return template.HTML(`<svg class="ico" viewBox="0 0 16 16" aria-hidden="true" ` +
		`fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" ` +
		`stroke-linejoin="round">` + d + `</svg>`)
}
