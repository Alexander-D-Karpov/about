package plugins

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type ProjectsPlugin struct {
	storage *storage.Storage
	hub     *stream.Hub
}

func NewProjectsPlugin(storage *storage.Storage, hub *stream.Hub) *ProjectsPlugin {
	return &ProjectsPlugin{
		storage: storage,
		hub:     hub,
	}
}

func (p *ProjectsPlugin) Name() string {
	return "projects"
}

func (p *ProjectsPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	projects, ok := config.Settings["projects"].([]interface{})
	if !ok {
		return "", nil
	}

	tmpl := `
	<div class="projects-section plugin" data-w="3">
		<div class="plugin-header">
			<h3 class="plugin-title">Projects</h3>
		</div>
		<div class="plugin__inner">
			<div class="projects-grid">
				{{range .Projects}}
				<article class="project-card">
					{{if .Image}}
					<div class="project-image">
						<img src="{{.Image}}" alt="{{.Name}}" loading="lazy">
					</div>
					{{end}}
					<div class="project-content">
						<h3 class="project-title">{{.Name}}</h3>
						<p class="project-description">{{.Description}}</p>
						<div class="project-links">
							{{if .GitHub}}
							<a href="{{.GitHub}}" target="_blank" rel="noopener" class="project-link project-link--github">
								<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor">
									<path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
								</svg>
								Source
							</a>
							{{end}}
							{{if .Live}}
							<a href="{{.Live}}" target="_blank" rel="noopener" class="project-link project-link--live">
								<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor">
									<path d="M14 3v2h3.59l-9.83 9.83 1.41 1.41L19 6.41V10h2V3m-2 16H5V5h7V3H5c-1.11 0-2 .89-2 2v14c0 1.11.89 2 2 2h14c1.11 0 2-.89 2-2v-7h-2v7Z"/>
								</svg>
								Demo
							</a>
							{{end}}
						</div>
						{{if .Technologies}}
						<div class="project-tech">
							{{range .Technologies}}
							<span class="tech-tag">{{.}}</span>
							{{end}}
						</div>
						{{end}}
					</div>
				</article>
				{{end}}
			</div>
		</div>
	</div>`

	type project struct {
		Name         string
		Description  string
		Image        string
		GitHub       string
		Live         string
		Technologies []string
	}

	var projectList []project
	for _, proj := range projects {
		projMap, ok := proj.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := projMap["name"].(string)
		desc, _ := projMap["description"].(string)
		image, _ := projMap["image"].(string)
		github, _ := projMap["github"].(string)
		live, _ := projMap["live"].(string)

		var technologies []string
		if techs, ok := projMap["technologies"].([]interface{}); ok {
			for _, tech := range techs {
				if techStr, ok := tech.(string); ok {
					technologies = append(technologies, techStr)
				}
			}
		}

		projectList = append(projectList, project{
			Name:         name,
			Description:  desc,
			Image:        image,
			GitHub:       github,
			Live:         live,
			Technologies: technologies,
		})
	}

	funcMap := template.FuncMap{
		"printf": fmt.Sprintf,
	}

	tmplParsed, err := template.New("projects").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = tmplParsed.Execute(&buf, struct {
		Projects []project
	}{
		Projects: projectList,
	})
	return buf.String(), err
}

func (p *ProjectsPlugin) UpdateData(ctx context.Context) error {
	return nil
}

func (p *ProjectsPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *ProjectsPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings
	return p.storage.SetPluginConfig(p.Name(), config)
}

func (p *ProjectsPlugin) RenderText(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	projects, ok := config.Settings["projects"].([]interface{})
	if !ok || len(projects) == 0 {
		return "Projects: No projects configured", nil
	}

	var projectNames []string
	for _, proj := range projects {
		projMap, ok := proj.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := projMap["name"].(string); ok {
			projectNames = append(projectNames, name)
		}
	}

	if len(projectNames) == 0 {
		return "Projects: No valid projects", nil
	}

	return fmt.Sprintf("Projects: %s (%d total)", strings.Join(projectNames, ", "), len(projectNames)), nil
}
