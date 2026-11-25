package logs

import (
	"hash/fnv"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// Codex's skill list and skill uses (docs/33 for Codex). Neither is documented as a log format; the shapes follow
// Codex's source and what its rollouts hold, keyed on which fields are present, not on the CLI version.
//
// Listed: the union, over the session, of the lists Codex sent: a developer message holding <skills_instructions>,
// and world_state host_skills.body (merge patches applied). Lines read "- name: description (file: <root
// alias>/dir/SKILL.md)", with the aliases under "### Skill roots". A skill with another locator (an executor or
// cloud package) has no folder a command could touch, so it is left out: it could never be seen as used.
// Used: a pick (a user message with a <skill><name>…</name><path>…</path> block), or any tool call whose input
// names the skill's folder (see mentions). The input is not read as a shell command: whatever the command does with
// the folder (reads it, runs a script, edits it), naming it counts, so "not used" is never said of a skill a call
// named. The list text itself is not a tool call and never counts.

type codexSkills struct {
	by         map[string]int    // name -> index in Setup.Listed
	dirs       map[string]string // listed folder -> name
	overhead   int               // the longest list text's characters outside its skill lines (headings, roots, how-to)
	body       string            // world_state host_skills.body, patches applied
	hidden     bool              // world_state host_skills.includeInstructions is false
	hiddenEver bool
	entries    map[string]bool // delta form: SKILL.md path -> shown, patches applied
	// listedHash are the hash keys of this file's listed folders (also written from home), kept past the bound
	listedHash map[string]bool
	shown      map[string]bool // folders an entry showed at some point in the session
	shownAt    int64
}

var (
	// the description is greedy, so a "(file: …)" inside it does not end the line early
	cxItemRE = regexp.MustCompile(`^- (.+?):(?: (.*))? ?\((file|executor package|cloud package|custom resource): ([^()]+)\)$`)
	cxRootRE = regexp.MustCompile("^- `([^`]+)` = `([^`]+)`$")
	cxPickRE = regexp.MustCompile(`<skill>\s*<name>([^<]+)</name>(?:\s*<path>([^<]+)</path>)?`)
	// a path in a tool call's input: quoted (spaces kept) or bare
	cxTokRE = regexp.MustCompile(`'([^']*)'|"([^"]*)"|([^\s'"` + "`" + `;|&()<>,{}\[\]=]+)`)
)

// cxOmitted starts the body Codex sends when every skill was left out for the budget.
const cxOmitted = "Host skills are available but omitted"

func (s *Setup) cxState() *codexSkills {
	if s.cx == nil {
		s.cx = &codexSkills{by: map[string]int{}, dirs: map[string]string{}}
	}
	return s.cx
}

// slash makes a Windows path comparable: forward slashes, a lower-case drive letter.
func slash(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	if len(p) >= 2 && p[1] == ':' {
		p = strings.ToLower(p[:1]) + p[1:]
	}
	return p
}

// codexSkillList adds the skills of one list text (a <skills_instructions> block or a host_skills body).
func (s *Setup) codexSkillList(text string, ts int64) {
	if strings.Contains(text, cxOmitted) {
		s.SkillsUncertain = true
		return
	}
	roots := map[string]string{}
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		if m := cxRootRE.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			roots[m[1]] = strings.TrimRight(slash(m[2]), "/")
		}
	}
	c := s.cxState()
	items, described, itemChars := 0, 0, 0
	for _, l := range lines {
		m := cxItemRE.FindStringSubmatch(strings.TrimSpace(strings.TrimRight(l, "\r")))
		if m == nil {
			continue
		}
		items++
		chars := utf8.RuneCountInString(l) + 1
		itemChars += chars
		if strings.TrimSpace(m[2]) != "" {
			described++
		}
		if m[3] != "file" {
			continue
		}
		loc := slash(strings.TrimSpace(m[4]))
		if a, rest, ok := strings.Cut(loc, "/"); ok && roots[a] != "" {
			loc = roots[a] + "/" + rest
		}
		dir := path.Dir(loc)
		if _, seen := c.dirs[dir]; !seen {
			c.listHashes(dir)
		}
		c.dirs[dir] = m[1]
		if _, ok := c.by[m[1]]; ok {
			continue
		}
		c.by[m[1]] = len(s.Listed)
		s.Listed = append(s.Listed, SkillLine{Name: m[1], Chars: chars, Source: cxSource(dir), Dir: dir})
		s.ListLen += chars
	}
	if items == 0 {
		return
	}
	if o := utf8.RuneCountInString(text) - itemChars; o > c.overhead {
		c.overhead = o
	}
	if described == 0 { // every description dropped: the budget may have left skills out, unnamed
		s.SkillsUncertain = true
	}
	if s.SkillsAt == NaT {
		s.SkillsAt = ts
	}
	s.Skills = len(s.Listed)
}

// listHashes keeps the hash keys a listed folder answers to (SkillKeys with this computer's home).
func (c *codexSkills) listHashes(dir string) {
	if c.listedHash == nil {
		c.listedHash = map[string]bool{}
	}
	home, _ := os.UserHomeDir()
	for _, k := range SkillKeys(dir, home) {
		if strings.HasPrefix(k, "#") {
			c.listedHash[k] = true
		}
	}
}

// hasString says xs holds s.
func hasString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// cxSource is a listed skill's group: the plugin it came with (".../plugins/cache/<marketplace>/<plugin>/..."),
// else "" (a skill of its own).
func cxSource(dir string) string {
	parts := strings.Split(dir, "/")
	for i := 0; i+3 < len(parts); i++ {
		if parts[i] == "plugins" && parts[i+1] == "cache" {
			return parts[i+3]
		}
	}
	return ""
}

// codexHostSkills reads world_state host_skills: a full state or a JSON merge patch on the last one. In the delta
// form an entry shown is a listed skill: its list text is in a developer message, but not always (a session can
// hold entries only, or show a skill no list text names), so the entries count too (named at the end of the file).
func (s *Setup) codexHostSkills(p py.Value, ts int64) {
	st := p.Get("state")
	if st.K != py.Obj {
		return
	}
	c := s.cxState()
	if p.Get("full").K == py.Bool && p.Get("full").Truthy() {
		c.body, c.hidden, c.entries = "", false, nil
	}
	if !st.Has("host_skills") {
		return
	}
	h := st.Get("host_skills")
	if h.K != py.Obj { // null: removed
		c.body, c.hidden, c.entries = "", false, nil
		return
	}
	if e := h.Get("entries"); e.K == py.Obj {
		if c.entries == nil {
			c.entries = map[string]bool{}
		}
		for _, k := range iterKeys(e) {
			j, err := py.Loads(k) // ["host", <authority>, ".../SKILL.md"]
			if err != nil || j.K != py.Arr || len(j.A) == 0 {
				continue
			}
			pth := slash(py.PyStr(j.A[len(j.A)-1]))
			switch v := e.Get(k); {
			case v.K != py.Obj: // null: removed
				delete(c.entries, pth)
			case v.Has("shown"):
				c.entries[pth] = v.Get("shown").Truthy()
			case !c.entries[pth]: // a patch without "shown" keeps the last value; a new entry without it is not shown
				c.entries[pth] = false
			}
		}
	} else if h.Has("entries") { // null
		c.entries = nil
	}
	if h.Has("body") {
		c.body = ""
		if b := h.Get("body"); b.K == py.Str {
			c.body = b.S
		}
	}
	if h.Has("includeInstructions") {
		v := h.Get("includeInstructions")
		c.hidden = v.K == py.Bool && !v.Truthy() // null: back to the default (shown)
	}
	if c.hidden {
		s.SkillsUncertain, c.hiddenEver = true, true
		return
	}
	if c.body != "" && h.Has("body") {
		s.codexSkillList(c.body, ts)
	}
	for pth, on := range c.entries {
		if on {
			if c.shown == nil {
				c.shown = map[string]bool{}
			}
			if d := path.Dir(pth); !c.shown[d] {
				c.shown[d] = true
				c.listHashes(d) // a skill shown only here is listed too: its hashes join and pass the bound as well
			}
			if c.shownAt == 0 {
				c.shownAt = ts
			}
		}
	}
}

// codexPick counts the skill a user message's <skill> block names, and its folder when the block gives the path.
func (s *Setup) codexPick(text string) {
	for _, m := range cxPickRE.FindAllStringSubmatch(text, -1) {
		s.countUse(m[1])
		s.mentions(m[2])
	}
}

// Matching a tool call's input to listed skill folders. The input is decoded the way it was written (a JSON value
// to its string leaves, code-mode input as a JavaScript string), then normalized once (normArg), then read for
// path tokens. Each path token leaves keys the report matches against any session's list (SkillKeys): the
// folder's tail ("skills/<folder>", ".system/<folder>"), the folder of a SKILL.md or scripts folder
// ("*/<folder>"), and a short hash of every folder the path passes through (a custom root, a references/ read).
// The input is not read as a shell command: naming a skill's folder counts, whatever the command does with it.

// argText is a tool call's input as text: the string leaves of a JSON value (a string holding JSON decoded too).
func argText(v py.Value) string {
	var out []string
	var walk func(v py.Value, depth int)
	walk = func(v py.Value, depth int) {
		switch v.K {
		case py.Str:
			if t := strings.TrimSpace(v.S); depth < 4 && (strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")) {
				if j, err := py.Loads(t); err == nil && (j.K == py.Obj || j.K == py.Arr) {
					walk(j, depth+1)
					return
				}
			}
			out = append(out, v.S)
		case py.Arr:
			for _, x := range v.A {
				walk(x, depth)
			}
		case py.Obj:
			for _, k := range iterKeys(v) {
				walk(v.Get(k), depth)
			}
		}
	}
	walk(v, 0)
	return strings.Join(out, "\n")
}

// argTextRaw decodes a call's raw input: JSON when it parses, else code (a JavaScript-escaped script).
func argTextRaw(raw string) string {
	if j, err := py.Loads(raw); err == nil {
		return argText(j)
	}
	if u := jsUnescape(raw); u != raw {
		return raw + "\n" + u // also as written: a raw patch's C:\…\notes is a folder, not "\n"
	}
	return raw
}

// jsUnescape undoes a script's string escapes (\\, \", \/, \n, \t, \uXXXX), as its strings run.
func jsUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		switch c := s[i+1]; c {
		case '\\', '"', '/', '\'':
			b.WriteByte(c)
			i++
		case 'n', 't', 'r':
			b.WriteByte('\n')
			i++
		case 'u':
			if i+6 <= len(s) {
				if r, err := strconv.ParseUint(s[i+2:i+6], 16, 32); err == nil {
					b.WriteRune(rune(r))
					i += 5
					continue
				}
			}
			b.WriteByte(s[i])
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

var (
	cxURLEsc  = strings.NewReplacer("%2F", "/", "%2f", "/", "%5C", `\`, "%5c", `\`, "%20", cxSpace, `\u002f`, "/", `\u002F`, "/", `\u005c`, `\`, `\u005C`, `\`)
	cxHomeEsc = strings.NewReplacer("${home}", "~", "$home", "~", "%userprofile%", "~", "$env:userprofile", "~",
		"${codex_home}", "$codex_home", "%codex_home%", "$codex_home", "$env:codex_home", "$codex_home")
	cxSlashes = regexp.MustCompile(`/{2,}`)
	// a simple brace list in a path: skills/{a,b}/SKILL.md
	cxBraces = regexp.MustCompile(`([^\s'"{}]*)\{([^{}\s'"]*,[^{}\s'"]*)\}([^\s'"{}]*)`)
)

// cxSpace stands for a space written inside a path ("\ ", %20), so the path stays one token; it is a space again
// in the keys.
const cxSpace = "\x1f"

// normArg is the one normalization before matching, in order: URL escapes (and a \u002f left in decoded text),
// a shell-escaped space, backslashes to slashes, runs of slashes to one, case (Windows paths), home notations.
func normArg(s string) string {
	s = norm.NFC.String(s) // macOS writes names decomposed (NFD)
	s = cxURLEsc.Replace(s)
	s = strings.ReplaceAll(s, `\ `, cxSpace)
	s = strings.ReplaceAll(s, `\`, "/")
	s = cxSlashes.ReplaceAllString(s, "/")
	s = strings.ToLower(s)
	return cxHomeEsc.Replace(s)
}

// cxMaxHashes bounds the folder hashes one file keeps.
const cxMaxHashes = 20000

// mentions keeps the keys of the skill folders a call's input (decoded text) names. runIn are the folders the call
// runs in, as its record gives them (a JSON workdir or cwd, a command's cwd; a file URL is read as its path).
func (s *Setup) mentions(text string, runIn ...string) {
	if text == "" || !strings.Contains(text, "/") && !strings.Contains(text, `\`) && !strings.Contains(text, "%") {
		return
	}
	var cp callPaths
	for _, d := range runIn {
		d = strings.TrimSpace(d)
		if strings.HasPrefix(strings.ToLower(d), "file:") {
			d = fileURLPath(normArg(d))
		} else {
			d = normArg(d)
		}
		if c, abs := cleanTok(d); abs {
			cp.anchor(c)
		}
	}
	// -C, --chdir and --directory name the folder only after env, make, git and tar (ls -C and wc -c do not); read
	// before case is folded
	if strings.Contains(text, "-C") || strings.Contains(text, "--chdir") || strings.Contains(text, "--directory") {
		for _, m := range cxDashC.FindAllStringSubmatch(text, cxMaxRoots) {
			if c, abs := cleanTok(normArg(strings.Trim(m[1], `"'`))); abs {
				cp.anchor(c)
			}
		}
	}
	t := normArg(text)
	if strings.Contains(t, "{") {
		t = cxBraces.ReplaceAllStringFunc(t, func(m string) string {
			p := cxBraces.FindStringSubmatch(m)
			alts := strings.Split(p[2], ",")
			if len(alts) > 16 {
				return m
			}
			var out []string
			for _, a := range alts {
				out = append(out, p[1]+a+p[3])
			}
			return strings.Join(out, " ")
		})
	}
	s.pathTokens(t, 0, &cp)
	// the value of a workdir or cwd key inside a code-mode script ({cmd:"…", workdir:'…'})
	if strings.Contains(t, "dir") || strings.Contains(t, "cwd") {
		for _, m := range cxDirKeyRE.FindAllStringSubmatch(t, cxMaxRoots) {
			if c, abs := cleanTok(m[1] + m[2] + m[3]); abs {
				cp.anchor(c)
			}
		}
	}
	// a relative path in a call that says which folder it runs in (workdir or cwd, cd, -C): its leading parts (up
	// to three, after one leading ".." resolved against the folder), joined to that folder, are folders the call may
	// name (only hashes: matched to listed folders). A folder the call only names is not one it runs in:
	// "rg x ~/.codex/skills review/notes.md" names no "review" skill. Kept when it can matter: the joined folder is
	// one of this file's listed folders, or the root is a skills folder, home or $CODEX_HOME (a sub-agent's file
	// has no list of its own)
	for i, root := range cp.roots {
		if !cp.anchored[i] {
			continue
		}
		up := path.Dir(root)
		if root == "~" || root == "$codex_home" || up == "." {
			up = ""
		}
		for _, rel := range cp.rels {
			base := root
			if r, ok := strings.CutPrefix(rel, "../"); ok {
				if up == "" {
					continue
				}
				base, rel = up, r
			}
			any := base == "~" || base == "$codex_home" || strings.HasSuffix(base, "/skills") || strings.HasSuffix(base, "/.system")
			k := fnvMore(fnvMore(fnvOffset, strings.TrimRight(base, "/")+"/"), rel)
			if any || (s.cx != nil && s.cx.listedHash["#"+strconv.FormatUint(k, 16)]) {
				s.joinedKey(k)
			}
		}
	}
}

// fileURLPath is the path of a normalized file URL (file:///C:/… is C:/…).
func fileURLPath(u string) string {
	p := strings.TrimPrefix(pctDecode(strings.TrimPrefix(u, "file:")), "/localhost")
	if len(p) > 3 && p[0] == '/' && p[2] == ':' && p[3] == '/' {
		p = p[1:]
	}
	return p
}

// runDirs are the folders a JSON tool input says its call runs in (workdir, cwd, working_directory).
func runDirs(v py.Value) []string {
	if v.K == py.Str {
		j, err := py.Loads(v.S)
		if err != nil {
			return nil
		}
		v = j
	}
	if v.K != py.Obj {
		return nil
	}
	var out []string
	for _, k := range []string{"workdir", "cwd", "working_directory", "workingDirectory"} {
		if x := v.Get(k); x.K == py.Str && x.S != "" {
			out = append(out, x.S)
		}
	}
	return out
}

// callPaths are one call's folders that relative paths may be under (absolute, from home, from $CODEX_HOME), whether
// the call runs in each (anchored), and the leading parts of its relative paths (up to three deep), bounded.
type callPaths struct {
	roots    []string
	anchored []bool
	rels     []string
	afterCd  bool // the previous token was cd (or pushd, Set-Location, git -C)
}

const cxMaxRoots, cxMaxRels, cxRelDepth = 16, 64, 3

// cxCdWords are the commands whose next path is the folder the rest of the call runs in ("cd -- <path>" too).
var cxCdWords = map[string]bool{"cd": true, "pushd": true, "set-location": true, "sl": true, "chdir": true}

// cxDashC is env, make, git or tar with a -C, --chdir or --directory folder (original case: ls -C is no folder).
var cxDashC = regexp.MustCompile(`(?:^|[\s;&|(])(?:env|make|gmake|git|tar)\s[^;&|\n]*?(?:-C\s*|--chdir[=\s]\s*|--directory[=\s]\s*)("[^"\n]*"|'[^'\n]*'|[^\s;&|)]+)`)

// cxDirKeyRE is a working-folder key and its string value in a script or object literal (normalized text): the
// key spellings seen in Codex tool-call inputs (workdir, cwd; dir), and their longer variants.
var cxDirKeyRE = regexp.MustCompile("(?:^|[^a-z0-9_$])(?:workdir|cwd|working_?dir(?:ectory)?|dir)[\"']?\\s*[:=]\\s*(?:\"([^\"\\n]*)\"|'([^'\\n]*)'|`([^`\\n]*)`)")

// cleanTok is a path token without "." parts and trailing punctuation, and whether it is absolute (from the root,
// a drive, home or $CODEX_HOME).
func cleanTok(tok string) (string, bool) {
	var parts []string
	for _, p := range strings.Split(strings.ReplaceAll(tok, cxSpace, " "), "/") {
		if p != "." {
			parts = append(parts, p)
		}
	}
	clean := strings.TrimRight(strings.Join(parts, "/"), "/.:,;")
	abs := clean == "~" || clean == "$codex_home" || strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "~/") ||
		strings.HasPrefix(clean, "$codex_home/") || (len(clean) > 2 && clean[1] == ':' && clean[2] == '/')
	return clean, abs
}

// anchor marks a root as a folder the call runs in (added when new, within the bound).
func (cp *callPaths) anchor(root string) {
	for i, r := range cp.roots {
		if r == root {
			cp.anchored[i] = true
			return
		}
	}
	if len(cp.roots) < cxMaxRoots {
		cp.roots, cp.anchored = append(cp.roots, root), append(cp.anchored, true)
	}
}

func (cp *callPaths) add(tok string) {
	afterCd := cp.afterCd
	cp.afterCd = cxCdWords[tok] || (afterCd && tok == "--") // "cd -- <path>"
	if tok == "" || len(tok) > cxMaxToken {
		return
	}
	clean, abs := cleanTok(tok)
	switch {
	case abs:
		if afterCd {
			cp.anchor(clean)
		} else if len(cp.roots) < cxMaxRoots && !hasString(cp.roots, clean) {
			cp.roots, cp.anchored = append(cp.roots, clean), append(cp.anchored, false)
		}
	case strings.Contains(tok, "/") && !strings.Contains(tok, "://"): // "foo/" counts: the slash says it is a folder
		rp := strings.Split(clean, "/")
		lead := ""
		if len(rp) > 1 && rp[0] == ".." { // one leading "..": the folder above the root
			lead, rp = "../", rp[1:]
		}
		for d := 1; d <= cxRelDepth && d <= len(rp); d++ {
			last := strings.TrimRight(rp[d-1], ".:,;")
			if last == "" || last == ".." || last == "~" {
				break
			}
			pre := lead + strings.Join(append(append([]string(nil), rp[:d-1]...), last), "/")
			if len(cp.rels) < cxMaxRels && !hasString(cp.rels, pre) {
				cp.rels = append(cp.rels, pre)
			}
		}
	}
}

// pathTokens reads the path tokens of normalized text; a quoted string is a token (spaces kept) and is read again
// for the tokens inside it (a whole command in quotes).
func (s *Setup) pathTokens(t string, depth int, cp *callPaths) {
	for _, m := range cxTokRE.FindAllStringSubmatch(t, -1) {
		if q := m[1] + m[2]; q != "" && depth < 3 {
			s.pathTokens(q, depth+1, cp)
		}
		tok := m[1] + m[2] + m[3]
		if strings.HasPrefix(tok, "file:/") { // a file URL: the path after the scheme (slashes already folded)
			tok = strings.TrimPrefix(pctDecode(tok[len("file:"):]), "/localhost")
			if len(tok) > 3 && tok[0] == '/' && tok[2] == ':' && tok[3] == '/' { // file:///C:/… is the path C:/…
				tok = tok[1:]
			}
		}
		cp.add(tok)
		s.pathToken(tok)
	}
}

// pctDecode undoes a file URL's percent escapes (%XX; a malformed one is kept as written), as normArg would see
// the path: NFC, lower case, a space as cxSpace, a backslash as a slash.
func pctDecode(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b = append(b, byte(v))
				i += 2
				continue
			}
		}
		b = append(b, s[i])
	}
	d := strings.ToLower(norm.NFC.String(string(b)))
	d = strings.ReplaceAll(strings.ReplaceAll(d, " ", cxSpace), `\`, "/")
	return cxSlashes.ReplaceAllString(d, "/")
}

// fnvMore runs FNV-1a on from state h over s (from fnvOffset it is the hash of s, as pathToken computes it).
func fnvMore(h uint64, s string) uint64 {
	for j := 0; j < len(s); j++ {
		h = (h ^ uint64(s[j])) * fnvPrime
	}
	return h
}

// cxMaxToken is the longest token read for folder hashes (a long blob names no folder).
const cxMaxToken = 4096

// pathToken keeps one path token's keys: the folder after "skills" or ".system" (punctuation after it trimmed), the
// folder of a SKILL.md or scripts folder, and a hash of every folder the path passes through.
func (s *Setup) pathToken(tok string) {
	tok = strings.TrimRight(tok, "/")
	if !strings.Contains(tok, "/") {
		return
	}
	var parts []string
	for _, p := range strings.Split(tok, "/") {
		if p != "." {
			parts = append(parts, p)
		}
	}
	for i := 0; i+1 < len(parts); i++ {
		part, next := parts[i], strings.TrimRight(parts[i+1], ".:,;")
		if part == "" || next == "" {
			continue
		}
		if (part == "skills" || part == ".system") && folderName(next) {
			s.mention(part + "/" + strings.ReplaceAll(next, cxSpace, " "))
		}
		if (next == "skill.md" || next == "scripts") && folderName(part) {
			s.mention("*/" + strings.ReplaceAll(part, cxSpace, " "))
		}
	}
	if len(tok) > cxMaxToken || strings.Count(tok, "/") < 2 {
		return
	}
	// one pass: FNV-1a runs on along the path, and its value at each "/" is the hash of the folder so far
	tok = strings.ReplaceAll(strings.TrimRight(strings.Join(parts, "/"), ".:,;"), cxSpace, " ")
	h := uint64(fnvOffset)
	for j := 0; j < len(tok); j++ {
		if tok[j] == '/' && j > 0 {
			s.hashKey(h)
		}
		h = (h ^ uint64(tok[j])) * fnvPrime
	}
	s.hashKey(h)
}

// folderName says a path part looks like a folder's name, so it may be kept as plain text: letters, digits,
// ". _ - + @" and a space written inside the path ("\ ", %20), at most 100 characters. A part holding anything
// else (quotes, operators, a bare space: command text) is never kept; the tokens inside it still match.
func folderName(p string) bool {
	return p != "." && p != ".." && plainName(p, "._-+@"+cxSpace, 100)
}

// plainName says s is 1 to max letters, digits and the given punctuation.
func plainName(s, punct string, max int) bool {
	n := 0
	for _, r := range s {
		n++
		if n > max || !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(punct, r)) {
			return false
		}
	}
	return n > 0
}

// cxMaxJoined bounds the joined relative-path hashes one file keeps, apart from cxMaxHashes.
const cxMaxJoined = 2000

// joinedKey keeps a hash of a relative path joined to its call's folder, on a budget of its own, so the hashes
// paths name themselves keep all of cxMaxHashes.
func (s *Setup) joinedKey(h uint64) {
	key := "#" + strconv.FormatUint(h, 16)
	if s.SkillMentions[key] == 0 {
		if s.joined >= cxMaxJoined && (s.cx == nil || !s.cx.listedHash[key]) {
			return
		}
		s.joined++
	}
	s.mention(key)
}

// hashKey keeps a folder hash, up to cxMaxHashes distinct ones per file (a repeat costs nothing).
func (s *Setup) hashKey(h uint64) {
	key := "#" + strconv.FormatUint(h, 16)
	if s.SkillMentions[key] == 0 {
		if s.hashes >= cxMaxHashes && (s.cx == nil || !s.cx.listedHash[key]) { // this file's own listed folders always count
			return
		}
		s.hashes++
	}
	s.mention(key)
}

const (
	fnvOffset = 14695981039346656037
	fnvPrime  = 1099511628211
)

// folderHash is a folder path's key: a short hash, so the logs keep no readable path.
func folderHash(p string) string {
	h := fnv.New64a()
	h.Write([]byte(p))
	return "#" + strconv.FormatUint(h.Sum64(), 16)
}

func (s *Setup) mention(key string) {
	if s.SkillMentions == nil {
		s.SkillMentions = map[string]int{}
	}
	s.SkillMentions[key]++
}

// codexResolve ends a file: entries shown that no list text named are listed under their folder's name; the
// list's characters include its headings and how-to once.
func (s *Setup) codexResolve() {
	c := s.cx
	if c == nil {
		return
	}
	var extra []string
	for d := range c.shown {
		if c.dirs[d] == "" {
			extra = append(extra, d)
		}
	}
	sort.Strings(extra)
	for _, d := range extra {
		name := path.Base(d)
		if _, ok := c.by[name]; ok {
			continue
		}
		c.dirs[d] = name
		c.by[name] = len(s.Listed)
		s.Listed = append(s.Listed, SkillLine{Name: name, Source: cxSource(d), Dir: d})
	}
	if len(extra) > 0 && !c.hiddenEver {
		if s.SkillsAt == NaT {
			s.SkillsAt = c.shownAt
		}
		s.Skills = len(s.Listed)
	}
	if len(s.Listed) > 0 {
		s.ListLen += c.overhead
	}
}

// SkillKeys are the keys a listed skill's folder answers to in SkillMentions: its tail, its name as the folder of a
// SKILL.md, and the hash of its whole path (also written from home, "~/…", or from $CODEX_HOME when it lies there).
func SkillKeys(dir, home string) []string {
	if dir == "" {
		return nil
	}
	d := strings.TrimRight(normArg(dir), "/")
	base := path.Base(d)
	keys := []string{path.Base(path.Dir(d)) + "/" + base, "*/" + base, folderHash(d)}
	if home != "" {
		h := strings.TrimRight(normArg(home), "/")
		if rest, ok := strings.CutPrefix(d, h+"/"); ok {
			keys = append(keys, folderHash("~/"+rest))
			if r2, ok := strings.CutPrefix(rest, ".codex/"); ok {
				keys = append(keys, folderHash("$codex_home/"+r2))
			}
		}
	}
	return keys
}
