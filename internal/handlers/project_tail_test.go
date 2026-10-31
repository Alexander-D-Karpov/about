package handlers

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestProjectTailIsOutsideMosaicInInitialHTML(t *testing.T) {
	tmpl, err := template.New("main.html").Funcs(template.FuncMap{"default": defaultFunc}).ParseFiles("../../templates/main.html")
	if err != nil {
		t.Fatal(err)
	}
	data := TemplateData{
		Plugins:     []template.HTML{`<section class="profile-section plugin"></section>`},
		ProjectTail: []template.HTML{`<div class="projects-section plugin"></div>`},
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(&out)
	if err != nil {
		t.Fatal(err)
	}
	var project *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for _, attr := range n.Attr {
			if attr.Key == "class" && strings.Contains(attr.Val, "projects-section") {
				project = n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if project == nil || project.Parent == nil || project.Parent.Data != "main" {
		t.Fatal("projects must be rendered as a direct child of main, outside the mosaic")
	}
}
