package plugins

import "strings"

var LanguageColors = map[string]string{
	"Go":           "#00D9FF",
	"Python":       "#4B8BBE",
	"JavaScript":   "#F7DF1E",
	"TypeScript":   "#3178C6",
	"Java":         "#ED8B00",
	"C++":          "#F34B7D",
	"C":            "#A8B9CC",
	"C#":           "#68217A",
	"Rust":         "#FF6B35",
	"HTML":         "#E34F26",
	"CSS":          "#264DE4",
	"Shell":        "#89E051",
	"Bash":         "#4EAA25",
	"PHP":          "#777BB4",
	"Ruby":         "#CC342D",
	"Swift":        "#F05138",
	"Kotlin":       "#B125EA",
	"Dart":         "#00D2B8",
	"Vue":          "#42B883",
	"React":        "#61DAFB",
	"JSON":         "#5C5C5C",
	"XML":          "#0060AC",
	"YAML":         "#CB171E",
	"Markdown":     "#083FA1",
	"SQL":          "#F29111",
	"Dockerfile":   "#2496ED",
	"Vim script":   "#199F4B",
	"Lua":          "#000080",
	"PowerShell":   "#5391FE",
	"Assembly":     "#6E4C13",
	"SCSS":         "#CF649A",
	"Less":         "#1D365D",
	"Sass":         "#CF649A",
	"Makefile":     "#427819",
	"CMake":        "#064F8C",
	"Perl":         "#39457E",
	"R":            "#276DC3",
	"MATLAB":       "#E16737",
	"Scala":        "#DC322F",
	"Clojure":      "#63B132",
	"Elixir":       "#6E4A7E",
	"Erlang":       "#A90533",
	"Haskell":      "#5E5086",
	"F#":           "#B845FC",
	"OCaml":        "#EC6813",
	"Reason":       "#FF5847",
	"Elm":          "#1293D8",
	"PureScript":   "#1D222D",
	"CoffeeScript": "#244776",
	"LiveScript":   "#499886",
	"Nim":          "#FFE953",
	"Crystal":      "#000100",
	"D":            "#BA595E",
	"Zig":          "#F7A41D",
	"V":            "#5D87BF",
	"Odin":         "#60AFFE",
	"Text":         "#6E7681",
	"Plain Text":   "#6E7681",
	"Svelte":       "#FF3E00",
	"Astro":        "#FF5D01",
	"Nix":          "#7EBAE4",
	"Gleam":        "#FFAFF3",
	"Hy":           "#7790B2",
	"Julia":        "#9558B2",
	"Other":        "#8B949E",
}

func GetLanguageColor(languageName string) string {
	if color, exists := LanguageColors[languageName]; exists {
		return color
	}

	lowerName := strings.ToLower(languageName)
	for lang, color := range LanguageColors {
		if strings.ToLower(lang) == lowerName {
			return color
		}
	}

	return "#8B949E"
}
