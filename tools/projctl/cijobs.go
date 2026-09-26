package main

import (
	"slices"

	"go.yaml.in/yaml/v3"
)

var (
	// gitlabGlobalKeys are the top-level keys in .gitlab-ci.yml that aren't
	// jobs. Keys starting with "." are templates, also not jobs.
	gitlabGlobalKeys = []string{"default", "include", "stages", "variables", "workflow",
		"image", "services", "cache", "before_script", "after_script"}
	// gitlabSkipKeys and githubSkipKeys let a job pass while its make target
	// fails, or not run at all: allow_failure, a manual or never `when`, rules,
	// a false `if`, continue-on-error.
	gitlabSkipKeys = []string{"allow_failure", "when", "rules", "only", "except"}
	githubSkipKeys = []string{"if", "continue-on-error"}
	// makeEnvVars change what make runs, whatever the command line says.
	makeEnvVars = []string{"MAKEFLAGS", "MFLAGS", "GNUMAKEFLAGS", "MAKEFILES"}
)

// A ciJob is one job in a forge CI file.
type ciJob struct {
	name  string
	image string   // the image it runs in, or "" if it names none
	skips []string // keys that can let it pass or not run while its make target fails
}

// gitlabJobs returns the jobs in a .gitlab-ci.yml. Each job's image and keys
// include what it inherits through extends, then default, then the deprecated
// top-level image, in GitLab's order of precedence.
func gitlabJobs(doc *yaml.Node) []ciJob {
	top := root(doc)
	var jobs []ciJob
	for _, name := range mapKeys(top) {
		if name == "" || name[0] == '.' || slices.Contains(gitlabGlobalKeys, name) {
			continue
		}
		job := ciJob{name: name}
		chain := gitlabChain(top, mapGet(top, name), map[*yaml.Node]bool{})
		for _, n := range chain {
			for _, k := range mapKeys(n) {
				if slices.Contains(gitlabSkipKeys, k) && !slices.Contains(job.skips, k) {
					job.skips = append(job.skips, k)
				}
			}
		}
		for _, n := range append(chain, mapGet(top, "default"), top) {
			if img := mapGet(n, "image"); img != nil {
				job.image = imageName(img)
				break
			}
		}
		jobs = append(jobs, job)
	}
	return jobs
}

// gitlabChain returns job followed by the templates it extends, highest
// precedence first: a later entry in extends overrides an earlier one.
func gitlabChain(top, job *yaml.Node, seen map[*yaml.Node]bool) []*yaml.Node {
	if job == nil || seen[job] {
		return nil
	}
	seen[job] = true
	chain := []*yaml.Node{job}
	var names []string
	switch ext := mapGet(job, "extends"); {
	case ext == nil:
	case ext.Kind == yaml.ScalarNode:
		names = []string{ext.Value}
	case ext.Kind == yaml.SequenceNode:
		for _, c := range ext.Content {
			names = append(names, deref(c).Value)
		}
	}
	for _, n := range slices.Backward(names) {
		chain = append(chain, gitlabChain(top, mapGet(top, n), seen)...)
	}
	return chain
}

// githubJobs returns the jobs in a GitHub workflow, with the image from each
// job's container and the skip keys set on the job or any of its steps.
func githubJobs(doc *yaml.Node) []ciJob {
	all := mapGet(root(doc), "jobs")
	var jobs []ciJob
	for _, name := range mapKeys(all) {
		n := mapGet(all, name)
		job := ciJob{name: name}
		if c := mapGet(n, "container"); c != nil {
			job.image = imageName(c)
		}
		nodes := []*yaml.Node{n}
		if steps := mapGet(n, "steps"); steps != nil && steps.Kind == yaml.SequenceNode {
			nodes = append(nodes, steps.Content...)
		}
		for _, s := range nodes {
			for _, k := range mapKeys(s) {
				if slices.Contains(githubSkipKeys, k) && !slices.Contains(job.skips, k) {
					job.skips = append(job.skips, k)
				}
			}
		}
		jobs = append(jobs, job)
	}
	return jobs
}

// envNames returns every variable name set under key (GitLab's variables,
// GitHub's env) anywhere in the document.
func envNames(n *yaml.Node, key string) []string {
	var out []string
	var walk func(*yaml.Node)
	walk = func(v *yaml.Node) {
		switch v.Kind {
		case yaml.DocumentNode, yaml.SequenceNode:
			for _, c := range v.Content {
				walk(c)
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(v.Content); i += 2 {
				if v.Content[i].Value == key {
					out = append(out, mapKeys(v.Content[i+1])...)
				}
				walk(v.Content[i+1])
			}
		default:
			// Scalars and aliases set no variables.
		}
	}
	walk(n)
	return out
}

// imageName is an image given as a scalar, or as a mapping's name (GitLab)
// or image (GitHub).
func imageName(n *yaml.Node) string {
	n = deref(n)
	if n.Kind == yaml.ScalarNode {
		return n.Value
	}
	for _, k := range []string{"name", "image"} {
		if v := mapGet(n, k); v != nil && v.Kind == yaml.ScalarNode {
			return v.Value
		}
	}
	return ""
}

// root returns the top-level node of a parsed document.
func root(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return deref(doc.Content[0])
	}
	return deref(doc)
}

func deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

// mapGet returns the value of key in a mapping, following aliases and YAML
// merge keys (<<), or nil. A key set directly wins over a merged one.
func mapGet(n *yaml.Node, key string) *yaml.Node {
	n = deref(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	var merged []*yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, deref(n.Content[i+1])
		switch k {
		case key:
			return v
		case "<<":
			if v.Kind == yaml.SequenceNode {
				merged = append(merged, v.Content...)
			} else {
				merged = append(merged, v)
			}
		}
	}
	for _, m := range merged {
		if v := mapGet(m, key); v != nil {
			return v
		}
	}
	return nil
}

// mapKeys returns a mapping's keys in order, including merged ones.
func mapKeys(n *yaml.Node) []string {
	n = deref(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	var keys []string
	for i := 0; i+1 < len(n.Content); i += 2 {
		if k := n.Content[i].Value; k == "<<" {
			v := deref(n.Content[i+1])
			if v.Kind == yaml.SequenceNode {
				for _, m := range v.Content {
					keys = append(keys, mapKeys(m)...)
				}
			} else {
				keys = append(keys, mapKeys(v)...)
			}
		} else {
			keys = append(keys, k)
		}
	}
	return keys
}
