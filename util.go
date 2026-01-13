package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"git.0xf0xx0.eth.limo/0xf0xx0/oigiki"
	"github.com/urfave/cli/v3"
)

func getLineBlock(reader *bufio.Reader, max_line int) (lines []string, err error) {
	/// ensure the commit buffer has max_line commits in it (inch right)
	for len(global_commitBuffer) < max_line {
		line := ""
		line, err = reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
				global_commitBuffer = append(global_commitBuffer, cleanLine(line))
				break
			}
			return []string{}, err
		}
		global_commitBuffer = append(global_commitBuffer, cleanLine(line))
	}

	lines = make([]string, 0, max_line)
	furstLine := global_commitBuffer[0]           /// steal the furst commit to mark it visited
	global_commitBuffer = global_commitBuffer[1:] /// inch right by one
	lines = append(lines, furstLine)

	/// copy the next `max_line-2` commits for lookahead
	for i := 2; i <= max_line; i++ {
		lines = append(lines, global_commitBuffer[i-2])
	}
	return lines, err
}

func parseLine(line string) (sha, miniSha, message string, parents []string) {
	matches := lineRegex.FindStringSubmatch(line)
	if len(matches) == 0 {
		return
	}
	if len(matches) > 0 {
		matches = matches[1:]
	}
	sha = matches[0]
	miniSha = matches[1]
	if matches[2] != "" {
		parents = strings.Split(matches[2], " ")
	}
	message = matches[3]
	return
}
func splitMessage(msg string) (hash string, timestamp time.Time, author, refs, message string) {
	split := strings.Split(msg, "\t")
	if len(split) == 0 {
		return
	}
	hash = split[0]
	// TODO: error
	t, _ := strconv.Atoi(split[1])
	timestamp = time.Unix(int64(t), 0)
	author = split[2]
	refs = split[3]
	message = split[4]
	return
}
func roundDown2(n int) int {
	if n < 0 {
		return n
	}
	return n & ^1
}
func strExpand(s *string, l int) {
	x := l - len(*s)
	if x > 0 {
		(*s) += strings.Repeat(" ", x)
	}
}
func replaceAt(s *string, r string, n int) {
	split := strings.Split(*s, "")
	split[n] = r
	(*s) = strings.Join(split, "")
}
func removeTrailingBlanks(vine *[]string) {
	for len(*vine) > 0 && (*vine)[len(*vine)-1] == "" {
		*vine = (*vine)[:len(*vine)-1]
	}
}
func cleanLine(l string) string {
	return strings.Trim(l, "\r\n\t")
}

func fileExistsInRepo(path string) bool {
	_, err := fs.Stat(global_repoRoot, path)
	return err == nil
}
func fileExists(path string) bool {
	_, err := fs.Stat(global_root, path)
	return err == nil
}
func recursivelyLookForGitRoot(path string, maxDepth uint) (string, error) {
	if maxDepth == 0 {
		return "", errors.New("ran out of depth looking for git root; is this actually a git repository?")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	path = filepath.Clean(path)
	potentialRoot := filepath.Join(path, "./.git")
	/// [1:] because fs.root doesnt treat base / as root
	if fileExists(potentialRoot[1:]) {
		return potentialRoot, nil
	}
	maxDepth--
	path = filepath.Dir(path)
	return recursivelyLookForGitRoot(path, maxDepth)
}

// useful func generated while throwing perl at gpt-oss
func tr(source string, from, to string) string {
	var b strings.Builder
	for _, r := range source {
		idx := strings.IndexRune(from, r)
		if idx >= 0 {
			b.WriteRune([]rune(to)[idx])
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// STOLEN EVILLY from urfave/cli
// commithash: 2240690b11d48e705bd123082c787e05a9ac196c
// modified for my uses, some funcs inlined, etc
// stolen funcs all under the urfave/cli license, which is MIT
func stolenFlagStringer(f cli.Flag) string {
	// enforce DocGeneration interface on flags to avoid reflection
	df, ok := f.(cli.DocGenerationFlag)
	if !ok {
		return ""
	}
	/// inlined `unquoteUsage`
	placeholder, usage := func() (string, string) {
		var usage string = df.GetUsage()
		for i := 0; i < len(usage); i++ {
			if usage[i] == '`' {
				for j := i + 1; j < len(usage); j++ {
					if usage[j] == '`' {
						name := usage[i+1 : j]
						usage = usage[:i] + name + usage[j+1:]
						return name, usage
					}
				}
				break
			}
		}
		return "", usage
	}()
	needsPlaceholder := df.TakesValue()
	// if needsPlaceholder is true, placeholder is empty
	if needsPlaceholder && placeholder == "" {
		// try to get type from flag
		if tname := df.TypeName(); tname != "" {
			placeholder = tname
		} else {
			placeholder = "value"
		}
	}

	defaultValueString := ""

	// don't print default text for required flags
	if rf, ok := f.(cli.RequiredFlag); !ok || !rf.IsRequired() {
		if df.IsDefaultVisible() {
			if s := df.GetDefaultText(); s != "" {
				defaultValueString = fmt.Sprintf(" (default: %s)", s)
			} else if df.TakesValue() && df.GetValue() != "" {
				defaultValueString = fmt.Sprintf(" (default: %s)", df.GetValue())
			}
		}
	}

	usageWithDefault := strings.TrimSpace(usage + defaultValueString)

	/// inlined prefixedNames
	pn := func() string {
		var (
			names       []string = f.Names()
			placeholder string   = placeholder
		)
		var prefixed string
		for i, name := range names {
			if name == "" {
				continue
			}
			prefix := ""
			if len(name) == 1 {
				prefix = "-"
			} else {
				prefix = "--"
			}
			prefixed += prefix + name
			if placeholder != "" {
				prefixed += " " + "{yellow}" + placeholder + "{/yellow}"
			}
			if i < len(names)-1 {
				prefixed += ", "
			}
		}

		return prefixed
	}()
	sliceFlag, ok := f.(cli.DocGenerationMultiValueFlag)
	if ok && sliceFlag.IsMultiValueFlag() {
		pn = pn + " [ " + pn + " ]"
	}
	pn = "{bluebright}" + pn + "{/bluebright}"

	/// no withEnvHint cause we dont care about env
	/// the entire point of copying this
	l := len(oigiki.StripTags(pn))
	p := strings.Repeat(" ", 40-l) /// ugly ugly hardcoded length
	return fmt.Sprintf("%s%s%s", oigiki.ProcessTags(pn), p, usageWithDefault)
}
