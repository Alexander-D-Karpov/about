// Command projects is a small CLI to prepare a portfolio-projects list and push
// it to a running about instance via the admin API.
//
// Projects are kept in a local JSON file (default ./projects.json) with the exact
// shape the projects plugin expects:
//
//	[{"name","description","image","github","live","technologies":[...]}]
//
// Usage:
//
//	projects add        # interactive prompt (or use -name/-desc/... flags)
//	projects list       # print the local list
//	projects pull       # download prod's current list into the local file
//	projects push       # merge local list into prod (by name) and save
//
// Target + credentials come from flags or the environment (.env is loaded from
// the working dir): DEPLOY_URL / ADMIN_USER / ADMIN_PASS. Defaults target
// https://about.akarpov.ru with basic auth.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Project struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Image        string   `json:"image,omitempty"`
	GitHub       string   `json:"github,omitempty"`
	Live         string   `json:"live,omitempty"`
	Technologies []string `json:"technologies,omitempty"`
}

func main() {
	_ = godotenv.Load()

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "add":
		cmdAdd(os.Args[2:])
	case "list":
		cmdList()
	case "pull":
		cmdPull(os.Args[2:])
	case "push":
		cmdPush(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `projects - prepare & push portfolio projects

  projects add     add a project (interactive, or -name -desc -image -tech -github -live)
  projects list    print the local projects.json
  projects pull    download the target's current projects into projects.json
  projects push    merge local projects into the target (by name) and save

Flags:
  -file   local JSON file (default ./projects.json)
  -url    target base URL (default $DEPLOY_URL or https://about.akarpov.ru)
  -user   admin user (default $ADMIN_USER)
  -pass   admin password (default $ADMIN_PASS)
  -replace  (push) replace the target list entirely instead of merging
`)
}

// ---- flag parsing (tiny, so subcommands can share) ----

type opts struct {
	file, url, user, pass                 string
	name, desc, image, tech, github, live string
	replace                               bool
}

func parse(args []string) opts {
	o := opts{
		file: "projects.json",
		url:  envOr("DEPLOY_URL", "https://about.akarpov.ru"),
		user: os.Getenv("ADMIN_USER"),
		pass: os.Getenv("ADMIN_PASS"),
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch a {
		case "-file":
			o.file = next()
		case "-url":
			o.url = strings.TrimRight(next(), "/")
		case "-user":
			o.user = next()
		case "-pass":
			o.pass = next()
		case "-name":
			o.name = next()
		case "-desc":
			o.desc = next()
		case "-image":
			o.image = next()
		case "-tech":
			o.tech = next()
		case "-github":
			o.github = next()
		case "-live":
			o.live = next()
		case "-replace":
			o.replace = true
		}
	}
	o.url = strings.TrimRight(o.url, "/")
	return o
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// ---- local file helpers ----

func load(file string) ([]Project, error) {
	b, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ps []Project
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(b, &ps); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return ps, nil
}

func save(file string, ps []Project) error {
	b, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(b, '\n'), 0644)
}

// mergeByName appends/overrides src into dst keyed by lower-cased name.
func mergeByName(dst, src []Project) []Project {
	idx := map[string]int{}
	for i, p := range dst {
		idx[strings.ToLower(strings.TrimSpace(p.Name))] = i
	}
	for _, p := range src {
		k := strings.ToLower(strings.TrimSpace(p.Name))
		if j, ok := idx[k]; ok {
			dst[j] = p
		} else {
			idx[k] = len(dst)
			dst = append(dst, p)
		}
	}
	return dst
}

// ---- subcommands ----

func cmdAdd(args []string) {
	o := parse(args)
	p := Project{
		Name:        o.name,
		Description: o.desc,
		Image:       o.image,
		GitHub:      o.github,
		Live:        o.live,
	}
	if o.tech != "" {
		p.Technologies = splitTech(o.tech)
	}

	// Interactive fill for anything not passed via flags.
	rd := bufio.NewReader(os.Stdin)
	if p.Name == "" {
		p.Name = prompt(rd, "Name")
	}
	if p.Name == "" {
		fmt.Fprintln(os.Stderr, "name is required")
		os.Exit(1)
	}
	if p.Description == "" {
		p.Description = prompt(rd, "Description")
	}
	if p.Image == "" {
		p.Image = prompt(rd, "Image URL")
	}
	if len(p.Technologies) == 0 {
		p.Technologies = splitTech(prompt(rd, "Tech stack (comma-separated)"))
	}
	if p.GitHub == "" {
		p.GitHub = prompt(rd, "GitHub URL (optional)")
	}
	if p.Live == "" {
		p.Live = prompt(rd, "Demo/Live URL (optional)")
	}

	ps, err := load(o.file)
	check(err)
	ps = mergeByName(ps, []Project{p})
	check(save(o.file, ps))
	fmt.Printf("saved %q -> %s (%d total)\n", p.Name, o.file, len(ps))
}

func cmdList() {
	o := parse(os.Args[2:])
	ps, err := load(o.file)
	check(err)
	if len(ps) == 0 {
		fmt.Println("(no projects in", o.file+")")
		return
	}
	for i, p := range ps {
		fmt.Printf("%2d. %s\n", i+1, p.Name)
		if p.Description != "" {
			fmt.Printf("    %s\n", p.Description)
		}
		if len(p.Technologies) > 0 {
			fmt.Printf("    tech: %s\n", strings.Join(p.Technologies, ", "))
		}
		if p.Image != "" {
			fmt.Printf("    image: %s\n", p.Image)
		}
	}
	fmt.Printf("\n%d project(s) in %s\n", len(ps), o.file)
}

func cmdPull(args []string) {
	o := parse(args)
	remote, err := fetch(o)
	check(err)
	check(save(o.file, remote))
	fmt.Printf("pulled %d project(s) from %s -> %s\n", len(remote), o.url, o.file)
}

func cmdPush(args []string) {
	o := parse(args)
	local, err := load(o.file)
	check(err)
	if len(local) == 0 {
		fmt.Fprintln(os.Stderr, "nothing to push:", o.file, "is empty")
		os.Exit(1)
	}

	var out []Project
	if o.replace {
		out = local
	} else {
		remote, err := fetch(o)
		check(err)
		out = mergeByName(remote, local)
	}

	body, _ := json.Marshal(out)
	req, _ := http.NewRequest("POST", o.url+"/admin/api/projects", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(o.user, o.pass)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	check(err)
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "push failed (%s): %s\n", resp.Status, strings.TrimSpace(string(rb)))
		os.Exit(1)
	}
	fmt.Printf("pushed %d project(s) to %s: %s\n", len(out), o.url, strings.TrimSpace(string(rb)))
}

func fetch(o opts) ([]Project, error) {
	req, _ := http.NewRequest("GET", o.url+"/admin/api/projects", nil)
	req.SetBasicAuth(o.user, o.pass)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET projects: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var ps []Project
	if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
		return nil, err
	}
	return ps, nil
}

// ---- misc ----

func prompt(rd *bufio.Reader, label string) string {
	fmt.Printf("%s: ", label)
	line, _ := rd.ReadString('\n')
	return strings.TrimSpace(line)
}

func splitTech(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
