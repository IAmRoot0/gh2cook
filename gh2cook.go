// gh2cook - given a GitHub repository URL, list every file in the repo
// with its raw.githubusercontent.com URL, formatted as:
//
//     filename.txt: [https://raw.githubusercontent.com/owner/repo/branch/path/filename.txt]
//
// Usage:
//   gh2cook -r https://github.com/danielmiessler/SecLists
//   gh2cook -r https://github.com/danielmiessler/SecLists -o urls.txt
//   gh2cook -r https://github.com/user/repo -branch main
//   gh2cook -r https://github.com/user/repo -filter .txt,.json
//   gh2cook -r https://github.com/danielmiessler/SecLists -tool cook -o seclists.yaml
//
// A GITHUB_TOKEN env var raises the API rate limit from 60/hr to 5000/hr.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
)

// treeResp mirrors the GitHub Git Trees API response.
type treeResp struct {
	Tree      []treeEntry `json:"tree"`
	Truncated bool        `json:"truncated"`
}

type treeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"` // "blob" (file) or "tree" (dir)
}

func main() {
	repoURL := flag.String("r", "", "GitHub repository URL (e.g. https://github.com/user/repo) [required]")
	branch := flag.String("branch", "", "Branch (auto-detect if empty: tries main, then master)")
	filter := flag.String("filter", "", "Comma-separated extensions to keep (e.g. .txt,.json). Empty = all files.")
	out := flag.String("o", "", "Write to file instead of stdout")
	tool := flag.String("tool", "", "Output format for a specific tool. Supported: cook (YAML ingredient file)")
	flag.Parse()

	if *repoURL == "" {
		fmt.Fprintln(os.Stderr, "error: -r <repo URL> is required")
		flag.Usage()
		os.Exit(1)
	}

	owner, repo, err := parseRepoURL(*repoURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// resolve the branch: user-supplied, else auto-detect
	resolvedBranch := *branch
	if resolvedBranch == "" {
		resolvedBranch, err = detectBranch(owner, repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error detecting branch: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Fprintf(os.Stderr, "[+] repo=%s/%s branch=%s\n", owner, repo, resolvedBranch)

	// fetch the full recursive tree
	tree, err := fetchTree(owner, repo, resolvedBranch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching tree: %v\n", err)
		os.Exit(1)
	}
	if tree.Truncated {
		fmt.Fprintln(os.Stderr, "[!] warning: GitHub truncated the tree (repo has > 100k files). Results incomplete.")
	}

	// prepare extension filter
	var exts []string
	if *filter != "" {
		for _, e := range strings.Split(*filter, ",") {
			e = strings.TrimSpace(strings.ToLower(e))
			if e != "" && !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			exts = append(exts, e)
		}
	}

	// build the output lines
	var lines []string
	for _, entry := range tree.Tree {
		if entry.Type != "blob" {
			continue
		}
		if len(exts) > 0 && !hasAnyExt(entry.Path, exts) {
			continue
		}
		filename := path.Base(entry.Path)
		rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
			owner, repo, resolvedBranch, entry.Path)
		lines = append(lines, fmt.Sprintf("    %s: [%s]", filename, rawURL))
	}

	// tool-specific output wrapping
	var output string
	switch strings.ToLower(*tool) {
	case "":
		output = strings.Join(lines, "\n") + "\n"
	case "cook":
		// cook expects a YAML file with a top-level "files:" key.
		// Duplicate filenames within a repo would collide in YAML, so we
		// disambiguate them here by appending _2, _3, etc. to duplicates.
		lines = disambiguateForCook(lines)
		output = "files:\n" + strings.Join(lines, "\n") + "\n"
	default:
		fmt.Fprintf(os.Stderr, "error: unknown -tool value %q (supported: cook)\n", *tool)
		os.Exit(1)
	}

	if *out != "" {
		if err := os.WriteFile(*out, []byte(output), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[+] wrote %d entries to %s\n", len(lines), *out)
	} else {
		fmt.Print(output)
	}
}

// parseRepoURL accepts URLs in several common forms and returns owner/repo.
// Accepted:
//   https://github.com/user/repo
//   https://github.com/user/repo.git
//   http://github.com/user/repo
//   github.com/user/repo
//   git@github.com:user/repo.git
func parseRepoURL(raw string) (string, string, error) {
	s := strings.TrimSpace(raw)
	// git@ form
	if strings.HasPrefix(s, "git@github.com:") {
		s = strings.TrimPrefix(s, "git@github.com:")
	} else {
		re := regexp.MustCompile(`^https?://`)
		s = re.ReplaceAllString(s, "")
		s = strings.TrimPrefix(s, "github.com/")
	}
	s = strings.TrimSuffix(s, ".git")
	// drop any /tree/... or /blob/... suffix if the user pasted a subpath URL
	if i := strings.Index(s, "/tree/"); i != -1 {
		s = s[:i]
	}
	if i := strings.Index(s, "/blob/"); i != -1 {
		s = s[:i]
	}
	parts := strings.Split(s, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("could not parse owner/repo from %q", raw)
	}
	return parts[0], parts[1], nil
}

// detectBranch calls the repo API to read default_branch. Falls back to
// trying "main" then "master" if the metadata call fails.
func detectBranch(owner, repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	body, status, err := ghGet(url)
	if err == nil && status == 200 {
		var meta struct {
			DefaultBranch string `json:"default_branch"`
		}
		if err := json.Unmarshal(body, &meta); err == nil && meta.DefaultBranch != "" {
			return meta.DefaultBranch, nil
		}
	}
	for _, guess := range []string{"main", "master"} {
		test := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s", owner, repo, guess)
		if _, s, _ := ghGet(test); s == 200 {
			return guess, nil
		}
	}
	return "", fmt.Errorf("could not determine default branch (last status=%d)", status)
}

// fetchTree pulls the entire recursive tree for a given branch.
func fetchTree(owner, repo, branch string) (*treeResp, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1",
		owner, repo, branch)
	body, status, err := ghGet(url)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("API returned %d: %s", status, string(body))
	}
	var t treeResp
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &t, nil
}

// ghGet performs an authenticated GET to the GitHub API. If GITHUB_TOKEN is
// set in the environment, it's used to raise the rate limit from 60/hr to
// 5000/hr.
func ghGet(url string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "gh2cook/1.0")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// disambiguateForCook makes filename keys unique within the manifest so YAML
// parsing doesn't silently drop duplicates. SecLists (and similar wordlist
// repos) commonly have the same filename in multiple folders — without this,
// only the last occurrence would survive YAML parsing.
//
// Input lines look like: "    filename.txt: [url]"
// If "filename.txt" appears twice, the second becomes "filename_2.txt", etc.
func disambiguateForCook(lines []string) []string {
	seen := make(map[string]int)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		idx := strings.Index(line, ": [")
		if idx == -1 {
			out = append(out, line)
			continue
		}
		indent := "    "
		name := strings.TrimPrefix(line[:idx], indent)
		rest := line[idx:] // starts with ": ["

		if count, ok := seen[name]; ok {
			seen[name] = count + 1
			ext := path.Ext(name)
			base := strings.TrimSuffix(name, ext)
			name = fmt.Sprintf("%s_%d%s", base, count+1, ext)
		} else {
			seen[name] = 1
		}
		out = append(out, indent+name+rest)
	}
	return out
}

// hasAnyExt reports whether p ends with any extension in exts (lowercase compare).
func hasAnyExt(p string, exts []string) bool {
	lp := strings.ToLower(p)
	for _, e := range exts {
		if strings.HasSuffix(lp, e) {
			return true
		}
	}
	return false
}
