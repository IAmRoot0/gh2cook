# gh2cook

Generate [cook](https://github.com/glitchedgitz/cook) ingredient manifests from any GitHub repository.

`gh2cook` walks an entire GitHub repo, lists every file with its `raw.githubusercontent.com` URL, and outputs a ready-to-use cook YAML file. Point it at a wordlist repo like SecLists and it produces a complete ingredient manifest in seconds — no manual copy-pasting of URLs.

---

## Features

- **Whole-repo enumeration** — recursively lists every file in a GitHub repository via a single API call
- **Cook YAML output** — `-tool cook` produces a `files:` manifest ready to drop into `~/.config/cook/cook-ingredients/`
- **Duplicate-key safe** — automatically disambiguates duplicate filenames (common in SecLists) so YAML parsing doesn't silently drop entries
- **Extension filtering** — `-filter .txt,.json` to keep only the file types you want
- **Flexible repo input** — accepts `https://`, `git@`, bare `github.com/...`, and `/tree/` subpath URLs
- **Branch auto-detection** — reads the repo's default branch automatically, or specify with `-branch`
- **Rate-limit friendly** — supports `GITHUB_TOKEN` for 5000 requests/hour instead of the anonymous 60/hour

---

## Installation

```bash
# clone or download gh2cook.go, then:
go build -o gh2cook gh2cook.go

# move to your PATH
sudo mv gh2cook ~/go/bin/       # or /usr/local/bin/
```

Requires Go 1.18+.

---

## Usage

```
gh2cook -r <repo-url> [options]
```

### Options

| Flag | Description |
|------|-------------|
| `-r` | GitHub repository URL **(required)** |
| `-tool` | Output format for a tool. Supported: `cook` (YAML ingredient file) |
| `-filter` | Comma-separated extensions to keep (e.g. `.txt,.json`). Empty = all files |
| `-branch` | Branch name. Auto-detects the default branch if omitted |
| `-o` | Write to a file instead of stdout |

---

## Examples

**Generate a cook manifest from SecLists:**

```bash
gh2cook -r https://github.com/danielmiessler/SecLists -tool cook -o seclists.yaml
```

**Only `.txt` wordlists, saved straight to the cook ingredients directory:**

```bash
gh2cook -r https://github.com/danielmiessler/SecLists \
  -tool cook \
  -filter .txt \
  -o ~/.config/cook/cook-ingredients/seclists.yaml
```

**Plain raw-URL list (no cook wrapping):**

```bash
gh2cook -r https://github.com/danielmiessler/SecLists -filter .txt
```

**Other wordlist repos:**

```bash
gh2cook -r https://github.com/six2dez/OneListForAll -tool cook -o oneforall.yaml
gh2cook -r https://github.com/assetnote/wordlists -tool cook -o assetnote.yaml
```

**Specific branch:**

```bash
gh2cook -r https://github.com/user/repo -branch develop -o out.yaml
```

---

## Output Format

With `-tool cook`, the output is a YAML manifest:

```yaml
files:
    bitquark-subdomains-top100000.txt: [https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/DNS/bitquark-subdomains-top100000.txt]
    combined_subdomains.txt: [https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/DNS/combined_subdomains.txt]
    dns-Jhaddix.txt: [https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/DNS/dns-Jhaddix.txt]
    n0kovo_subdomains.txt: [https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/DNS/n0kovo_subdomains.txt]
```

Without `-tool`, the output is the raw manifest lines only (no `files:` header).

---

## GitHub Rate Limits

The GitHub API allows **60 requests/hour** without authentication. Large repos need only one API call, but if you run `gh2cook` frequently, set a token:

```bash
export GITHUB_TOKEN=ghp_your_token_here
gh2cook -r https://github.com/danielmiessler/SecLists -tool cook -o seclists.yaml
```

A token with the `public_repo` scope is sufficient and raises the limit to **5000 requests/hour**. Create one at <https://github.com/settings/tokens>.

---

## Notes

- **Large repos** like SecLists have 100k+ files and sit near GitHub's tree-listing limit. If the tree is truncated, `gh2cook` prints a warning and returns partial results.
- **Duplicate filenames** across different folders (e.g. multiple `shell.php` in SecLists' Web-Shells) are automatically renamed `shell_2.php`, `shell_3.php`, etc. so no entry is lost in the YAML.

---

## Download the wordlists with cook

Once you have the manifest, cook can fetch and use the ingredients:

```bash
# cook reads the manifest and can pull the wordlists it references
cook seclists.yaml
```

Or fetch everything directly with `wget`:

```bash
gh2cook -r https://github.com/danielmiessler/SecLists -filter .txt \
  | grep -oP 'https?://\S+?(?=\])' > urls.txt
wget -i urls.txt -P ./seclists/
```

---

## License

MIT — do what you like, no warranty.
