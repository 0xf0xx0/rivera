/*
git-rivera/git-河流

display the git river, like git-forest

options:

	--repository path, --repo path                              repository path to use (default: ".")
	--hashlength len, --hashlen len, -l len                     length of the commit hash (default: 8)
	--style num, -s num                                         style num to select (1-5) (default: 1)
	--subvinedepth uint, --svdepth uint, --depth uint, -d uint  maximum length of merge subvines (default: 2)
	--graphmarginleft uint, --marginl uint                      left margin of the commit graph (default: 2)
	--graphmarginright uint, --marginr uint                     right margin of the commit graph (default: 1)
	--all, -a                                                   display all branches
	--reverse, -r                                               reverse the flow
	--branchcolors color,color[,color]                          comma separated color,color[,color] used for branches, passed straight to oigiki (default: "red, blue, yellow, green, cyan, magenta, white")
	--maxgitrecursedepth uint, --mgrd uint                      maximum depth to search for a .git dir (default: 8)
	--help, -h                                                  show help
	--version, -v                                               print the version
	--color                                                     force color output
	--nocolor, --stdout                                         disable color output
*/
package main

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"

	"git.0xf0xx0.eth.limo/0xf0xx0/oigiki"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

const (
	DATE_FMT = "2006-01-02 15:04"
)

const commandHelpTemplate = `Name:
   {bold}{green}{{.Name}} {/}- {blue}{{.Usage}}{/}

Usage:
   {green}{{.Name}} {blue}[options]{/}

Options:{blue}
   {{range .VisibleFlags}}{{.String}}
   {{end}}{/}
Version:
   {green}v{{.Version}}
`

// regex
var (
	lineRegex    = regexp.MustCompile(`^<(.*?)><(.*?)><(.*?)>(.*)`)
	nextShaRegex = regexp.MustCompile(`^<(.*?)>`)
	/// fmt pt 1
	overpassRegex = regexp.MustCompile(`O[DO]+O`)
	fanRegex      = regexp.MustCompile(`(?i)s.*s`)
	fanLMR        = regexp.MustCompile(`(s.*)S(.*s)`)
	fanLM         = regexp.MustCompile(`(s.*)S`)
	fanMR         = regexp.MustCompile(`S(.*s)`)
	/// fmt pt 2
	leftxB  = regexp.MustCompile(`(x\w*B)`)
	leftgI  = regexp.MustCompile(`(g\w*I)`)
	leftAg  = regexp.MustCompile(`A(\w*g)`)
	righteB = regexp.MustCompile(`e(\w*B)`)
	rightzI = regexp.MustCompile(`z(\w*I)`)
	rightAz = regexp.MustCompile(`(A\w*z)`)
)

// global
var (
	global_commitBuffer          []string
	global_branchColors          []string /// populated in flag
	global_root, global_repoRoot fs.FS
)

var config = struct {
	repoPath                     string
	hashLen, style, subvineDepth uint8
	leftMargin, rightMargin      uint8
	reverse, displayAll          bool
}{}

var buildCommit = func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value[:8]
			}
		}
	}

	return ""
}()

func main() {
	/// discard sigpipe
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGPIPE)
		<-c
	}()
	cli.RootCommandHelpTemplate = oigiki.ProcessTags(commandHelpTemplate)

	app := &cli.Command{
		Name:                   "git-rivera",
		Version:                "1.0.0+g" + buildCommit,
		Usage:                  "display the git river, like git-forest",
		UseShortOptionHandling: true,
		/// TODO: pass unknown flags to git
		MutuallyExclusiveFlags: []cli.MutuallyExclusiveFlags{
			{
				Flags: [][]cli.Flag{
					{
						&cli.BoolFlag{
							Name:  "color",
							Usage: "force color output",
						},
					},
					{
						&cli.BoolFlag{
							Name:    "nocolor",
							Aliases: []string{"stdout"},
							Usage:   "disable color output",
						},
					},
				},
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repository",
				Usage:   "repository `path` to use",
				Aliases: []string{"repo"},
				Value:   ".",
			},
			&cli.Uint8Flag{
				Name:    "hashlength",
				Usage:   "`len`gth of the commit hash",
				Aliases: []string{"hashlen", "l"},
				Value:   8,
			},
			&cli.Uint8Flag{
				Name:    "style",
				Usage:   "style `num` to select (1-5)",
				Aliases: []string{"s"},
				Value:   1,
			},
			&cli.Uint8Flag{
				Name:    "subvinedepth",
				Usage:   "maximum length of merge subvines",
				Aliases: []string{"svdepth", "depth", "d"},
				Value:   2,
			},
			&cli.Uint8Flag{
				Name:    "graphmarginleft",
				Usage:   "left margin of the commit graph",
				Aliases: []string{"marginl"},
				Value:   2,
			},
			&cli.Uint8Flag{
				Name:    "graphmarginright",
				Usage:   "right margin of the commit graph",
				Aliases: []string{"marginr"},
				Value:   1,
			},
			&cli.BoolFlag{
				Name:    "all",
				Usage:   "display all branches",
				Aliases: []string{"a"},
				Value:   false,
			},
			&cli.BoolFlag{
				Name:    "reverse",
				Usage:   "reverse the flow",
				Aliases: []string{"r"},
				Value:   false,
			},
			&cli.StringFlag{
				Name:  "branchcolors",
				Usage: "comma separated `color,color[,color]` used for branches, passed straight to oigiki",
				Value: "red, blue, yellow, green, cyan, magenta, white",
			},
			&cli.UintFlag{
				Name:    "maxgitrecursedepth",
				Aliases: []string{"mgrd"},
				Usage:   "maximum depth to search for a .git dir",
				Value:   8,
			},
		},
		Action: func(_ context.Context, ctx *cli.Command) error {
			oigiki.NoColor = !term.IsTerminal(int(os.Stdout.Fd()))

			if ctx.Bool("color") {
				oigiki.NoColor = false
			} else if ctx.Bool("nocolor") {
				oigiki.NoColor = true
			}

			/// my dumb ass
			global_root = os.DirFS("/")

			repoRoot, err := recursivelyLookForGitRoot(ctx.String("repository"), ctx.Uint("maxgitrecursedepth"))
			if err != nil {
				return err
			}
			/// yyyyyyoink
			global_repoRoot = os.DirFS(repoRoot)

			config.repoPath = repoRoot
			config.displayAll = ctx.Bool("all")
			config.reverse = !ctx.Bool("reverse")
			config.style = ctx.Uint8("style")
			config.hashLen = ctx.Uint8("hashlength")
			config.leftMargin = ctx.Uint8("graphmarginleft")
			config.rightMargin = ctx.Uint8("graphmarginright")
			config.subvineDepth = ctx.Uint8("svdepth") + 1 /// TODO: figure out why +1
			global_branchColors = strings.Split(ctx.String("branchcolors"), ",")
			for color := range global_branchColors {
				global_branchColors[color] = strings.TrimSpace(cleanLine(global_branchColors[color]))
			}

			//////
			/// now, we build the river
			//////
			return processCommits()
		},
	}
	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func processCommits() error {
	/// NOTE: getLineBlock inches the slice along, ensure the backing array has enough capacity
	/// the buffer stores the next commits to look at and is filled by getLineBlock
	global_commitBuffer = make([]string, 0, config.subvineDepth*32)
	/// each vine is a git branch
	vine := make([]string, 0, config.subvineDepth)
	refMap, err := getRefs()
	if err != nil {
		return cli.Exit(err.Error(), 1)
	}
	status, err := getStatus()
	if err != nil {
		return cli.Exit(err.Error(), 1)
	}

	/// TODO: make option...? this might be something im too lazy to do
	PRETTY := "%H\t%at\t%an\t%C(reset)%C(auto)%d%C(reset)\t%s"

	cmd := exec.Command("git", "--git-dir="+config.repoPath,
		"log", "--date-order", "--pretty=format:<%H><%h><%P>"+PRETTY)
	if config.displayAll {
		cmd.Args = append(cmd.Args, "--all", "HEAD")
	}
	if !oigiki.NoColor {
		cmd.Args = append(cmd.Args, "--color")
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return cli.Exit(err.Error(), 1)
	}

	if err := cmd.Start(); err != nil {
		return cli.Exit(err.Error(), 1)
	}
	reader := bufio.NewReader(stdout)

	collectedLines := []string{}
	if config.reverse {
		/// we only collect lines when reversing
		collectedLines = make([]string, 0, 128)
	}
	for {
		lines, err := getLineBlock(reader, int(config.subvineDepth))
		if err != nil {
			return cli.Exit(err.Error(), 1)
		}
		if len(lines) == 0 {
			break
		}

		line := strings.TrimSpace(lines[0])
		if line == "" {
			break
		}

		nextLines := []string{}
		if len(lines) > 1 {
			nextLines = lines[1:]
		}

		nextShas := make([]string, 0, len(nextLines))
		for idx := range nextLines {
			matches := nextShaRegex.FindStringSubmatch(nextLines[idx])
			if len(matches) == 0 {
				continue
			}
			nextShas = append(nextShas, matches[1])
		}

		sha, _, msg, parents := parseLine(line)
		_, t, author, autoRefs, message := splitMessage(msg)

		ret := strings.Builder{}
		ret.Grow(256)
		ret.WriteString(vineBranch(&vine, sha))

		ret.WriteString(fmt.Sprintf("{magenta}%s {blue}%s%s",
			sha[:config.hashLen], t.Format(DATE_FMT), strings.Repeat(" ", int(config.leftMargin)),
		))

		ret.WriteString(vineCommit(&vine, sha, parents))

		ret.WriteString(fmt.Sprintf("%s{yellow}%s",
			strings.Repeat(" ", int(config.rightMargin)), author))
		if _, ok := refMap[sha]; ok {
			/// TODO: /HEAD
			autoRefs = strings.Replace(autoRefs, "HEAD", "HEAD"+status, 1)
			autoRefs = strings.ReplaceAll(autoRefs, "tag:", "{magenta}tag:{/magenta}")
		}
		ret.WriteString(autoRefs)
		ret.WriteString("{/} ")
		ret.WriteString(message)
		ret.WriteRune('\n')

		ret.WriteString(vineMerge(&vine, sha, nextShas, parents))

		if config.reverse {
			/// split each line for proper reversal (theyre printed in clumps)
			collectedLines = append(collectedLines, strings.Split(ret.String(), "\n")...)
		} else {
			/// otherwise print as clumps
			fmt.Print(oigiki.ProcessTags(ret.String()))
		}
	}

	if config.reverse {
		for _, x := range slices.Backward(collectedLines) {
			if x == "" {
				continue
			}
			fmt.Println(oigiki.ProcessTags(x))
		}
	}

	if err := cmd.Wait(); err != nil {
		return cli.Exit(err.Error(), 1)
	}
	return nil
}

func getRefs() (map[string][]string, error) {
	m := make(map[string][]string, 32)
	cmd := exec.Command("git", "--git-dir="+config.repoPath, "show-ref")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return m, cli.Exit(err.Error(), 1)
	}
	if err := cmd.Start(); err != nil {
		return m, cli.Exit(err.Error(), 1)
	}
	reader := bufio.NewScanner(stdout)
	for reader.Scan() {
		line := reader.Text()
		split := strings.Split(line, " ")
		if _, ok := m[split[0]]; !ok {
			m[split[0]] = make([]string, 0, 3)
		}
		m[split[0]] = append(m[split[0]], split[1])
	}
	if err := cmd.Wait(); err != nil {
		return m, cli.Exit(err.Error(), 1)
	}

	/// TODO: the rest of the rebaes stuff
	return m, nil
}
func getStatus() (string, error) {
	dirty := ""
	midFlow := ""

	hasChangeUnstagedCmd := exec.Command("git", "--git-dir="+config.repoPath, "diff", "--shortstat")
	hasChangeStagedCmd := exec.Command("git", "--git-dir="+config.repoPath, "diff", "--shortstat", "--cached")
	hasStashCmd := exec.Command("git", "--git-dir="+config.repoPath, "stash", "list")
	hasUntrackedCmd := exec.Command("git", "--git-dir="+config.repoPath, "ls-files", "--others", "--exclude-standard")

	x, err := hasChangeUnstagedCmd.Output()
	if err != nil {
		return "", err
	}
	/// unstaged
	if len(x) > 0 {
		dirty += "*"
	}

	x, err = hasChangeStagedCmd.Output()
	if err != nil {
		return "", err
	}
	/// staged
	if len(x) > 0 {
		dirty += "+"
	}

	x, err = hasStashCmd.Output()
	if err != nil {
		return "", err
	}
	/// stash exists
	if len(x) > 0 {
		dirty += "$"
	}

	x, err = hasUntrackedCmd.Output()
	if err != nil {
		return "", err
	}
	/// untracked
	if len(x) > 0 {
		dirty += "%"
	}
	if len(dirty) > 1 {
		dirty = " " + dirty
	}

	/// midflow
	if fileExistsInRepo("/rebase-merge") {
		if fileExistsInRepo("/rebase-merge/interactive") {
			midFlow = "|REBASE-i"
		} else {
			midFlow = "|REBASE-m"
		}
	} else if fileExistsInRepo("/rebase-apply") {
		if fileExistsInRepo("/rebase-apply/rebasing") {
			midFlow = "|REBASE"
		} else if fileExistsInRepo("/rebase-apply/applying") {
			midFlow = "|AM"
		} else {
			midFlow = "|AM/REBASE"
		}
	} else if fileExistsInRepo("/MERGE_HEAD") {
		midFlow = "|MERGING"
	} else if fileExistsInRepo("/CHERRY_PICK_HEAD") {
		midFlow = "|CHERRY-PICKING"
	} else if fileExistsInRepo("/REVERT_HEAD") {
		midFlow = "|REVERTING"
	} else if fileExistsInRepo("/BISECT_LOG") {
		midFlow = "|BISECTING"
	}
	return dirty + midFlow, nil
}

/// layout

func vineBranch(vine *[]string, sha string) string {
	matchedCount := 0
	masterDrawn := false
	output := strings.Builder{}
	output.Grow(len(*vine))

	for columnIndex := range *vine {
		if (*vine)[columnIndex] == "" {
			output.WriteRune(' ')
		} else if (*vine)[columnIndex] != sha {
			output.WriteRune('I') // Straight line (other branch continues)
		} else {
			/// This column points to our commit
			if !masterDrawn && columnIndex%2 == 0 {
				output.WriteRune('S') /// Main branch split
				masterDrawn = true
			} else {
				output.WriteRune('s')     /// Secondary branch split
				(*vine)[columnIndex] = "" /// Clear
			}
			matchedCount++
		}
	}
	// Only print if multiple branches converged
	if matchedCount < 2 {
		return ""
	}
	removeTrailingBlanks(vine)
	/// +1 for space between hash and date
	return fmt.Sprintln(strings.Repeat(" ", int(config.hashLen)+1+len(DATE_FMT)+int(config.leftMargin)) + visPost(visFan(output.String(), "branch")))
}
func vineCommit(vine *[]string, sha string, parents []string) string {
	output := "" /// its too much of a pain to use strings.Builder here

	for columnIndex := range *vine {
		if (*vine)[columnIndex] == "" {
			output += " "
		} else if (*vine)[columnIndex] == sha {
			output += "C"
		} else {
			output += "I"
		}
	}
	if !strings.Contains(output, "C") {
		i := 0
		for i = roundDown2(len(*vine) - 1); i >= 0; i -= 2 {
			if output[i] == ' ' {
				/// tip
				replaceAt(&output, "t", i)
				(*vine)[i] = sha
				break
			}
		}
		if i < 0 {
			if len(*vine)%2 != 0 {
				output += " "
				*vine = append(*vine, "")
			}
			/// also tip
			output += "t"
			*vine = append(*vine, sha)
		}
	}

	removeTrailingBlanks(vine)

	if len(parents) == 0 {
		/// root
		output = strings.Replace(output, "C", "r", 1)
	} else if len(parents) > 1 {
		/// merge
		output = strings.Replace(output, "C", "M", 1)
	}

	return visPost(output)
}
func vineMerge(vine *[]string, sha string, nextShas, parents []string) string {
	originalColumn := -1
	output := "" /// ditto
	slot := make([]int, 0, 8)

	for idx := range *vine {
		if (*vine)[idx] == sha {
			originalColumn = idx
			break
		}
	}
	if originalColumn == -1 {
		panic("vineCommit didn't add this vine")
	}

	if len(parents) < 2 {
		if len(parents) > 0 {
			(*vine)[originalColumn] = parents[0]
		}
		removeTrailingBlanks(vine)
		return ""
	}
	for j := 0; j < len(parents) && len(parents) > 1; j++ {
		for idx := range *vine {
			if (*vine)[idx] != parents[j] || !slices.Contains(nextShas, (*vine)[idx]) {
				continue
			}
			if idx == originalColumn {
				panic("shouldnt really happen?")
			}
			pos := -1
			if idx < originalColumn {
				pos = idx + 1
				/// TODO: is empty string "undefined"?
				if (*vine)[pos] != "" {
					pos = idx - 1
				}
				if pos < 0 || (*vine)[pos] != "" {
					break
				}
			} else {
				pos = idx - 1
				if pos < 0 || (*vine)[pos] != "" {
					pos = idx + 1
				}
				if (*vine)[pos] != "" {
					break
				}
			}

			(*vine)[pos] = parents[j]
			/// TODO: maybe fixme?
			strExpand(&output, pos+1)
			replaceAt(&output, "s", pos)
			parents = append(parents[:j], parents[j+1:]...)
			j = j - 1
			break
		}
	}

	/// slotting
	slot = append(slot, originalColumn)
	parentCounter := 0

	for seeker := 2; parentCounter < len(parents)-1 && seeker < 2+(len(*vine)-1); seeker++ {
		idx := 1
		if seeker%2 == 0 {
			idx = -1
		}
		idx *= (seeker / 2) * 2
		idx += originalColumn

		if idx >= 0 && idx < len(*vine) && (*vine)[idx] == "" {
			slot = append(slot, idx)
			(*vine)[idx] = strings.Repeat("0", 40)
			parentCounter++
		}
	}
	for idx := originalColumn + 2; parentCounter < len(parents)-1; idx += 2 {
		// fmt.Printf("%q, %d, %d %d\n", *vine, idx, parentCounter, len(parents))
		/// TODO: is this how we interpret `undef`?
		if idx >= len(*vine) || (*vine)[idx] == "" {
			slot = append(slot, idx)
			parentCounter++
		}
	}

	if len(slot) != len(parents) {
		println(len(slot), len(parents))
		fmt.Printf("%q\n", slot)
		panic("serious internal error")
	}

	slices.Sort(slot)
	maxLen := len(*vine) + 2*len(slot)
	for i := 0; i < maxLen; i++ {
		strExpand(&output, i+1)
		if len(slot) > 0 && i == slot[0] {
			slot = slot[1:]
			/// fml
			if i >= len(*vine) {
				newVine := make([]string, i+1)
				copy(newVine, *vine)
				*vine = newVine
			}
			(*vine)[i] = parents[0]
			parents = parents[1:]
			if i == originalColumn {
				replaceAt(&output, "S", i)
			} else {
				replaceAt(&output, "s", i)
			}
		} else if output[i] == 's' {
			/// *crickets*
			/// NOTE: bug? remove i < len?
		} else if i < len(*vine) && (*vine)[i] != "" {
			replaceAt(&output, "I", i)
		} else {
			replaceAt(&output, " ", i)
		}
	}
	return fmt.Sprintln(strings.Repeat(" ", int(config.hashLen)+1+len(DATE_FMT)+int(config.leftMargin)) + visPost(visFan(output, "merge")))
}

/// beautification

func visFan(line, visType string) string {
	isBranch := visType == "branch"

	/// build the overpass, if applicable
	line = fanRegex.ReplaceAllStringFunc(line, func(match string) string {
		return tr(match, " I", "DO")
	})
	/// TODO: remove? make an option?
	// s = overpassRegex.ReplaceAllStringFunc(s, func(x string) string {
	// 	return strings.Repeat("O", len(x))
	// })

	/// match the various fan patterns
	if matches := fanLMR.FindStringSubmatch(line); len(matches) > 0 {
		line = strings.Replace(line, matches[0], visFan3(matches[1], matches[2]), 1)
	} else if matches := fanLM.FindStringSubmatch(line); len(matches) > 0 {
		line = strings.Replace(line, matches[0], visFan2L(matches[1])+"B", 1)
	} else if matches := fanMR.FindStringSubmatch(line); len(matches) > 0 {
		line = strings.Replace(line, matches[0], "A"+visFan2R(matches[1]), 1)
	} else {
		panic("FUUUUUCK")
	}

	/// replace with the branch chars
	if isBranch {
		line = tr(line, "efg", "xyz")
	}
	return line
}
func visFan2L(l string) string {
	l = strings.Replace(l, "s", "e", 1)
	l = strings.ReplaceAll(l, "s", "f")
	return l
}
func visFan2R(r string) string {
	if r[len(r)-1] == 's' {
		replaceAt(&r, "g", len(r)-1)
	}
	r = strings.ReplaceAll(r, "s", "f")
	return r
}
func visFan3(l, r string) string {
	sb := strings.Builder{}
	sb.WriteString(visFan2L(l))
	sb.WriteRune('K')
	sb.WriteString(visFan2L(r))
	return sb.String()
}

// this func is kinda dumb and needs a refactor
//
// basically it matches various patterns and notes down colors for them
//
// the magic happens in two stages:
//
// furst, every column is given a color based on its index
//
// then, the patterns are matched and the colors are updated and written before being turned into
// graph chars and returned
func visXfrm(line string) string {
	/* NOTE: from original perl:
			# A: branch to right
			# B: branch to left
			# C: commit
			# M: merge commit
			# D: overpass over empty space
			# e: merge visual left (╔)
			# f: merge visual center (╦)
			# g: merge visual right (╗)
			# I: straight line (║)
			# K: branch visual split (╬)
			# m: single line (─)
			# O: overpass (≡)
			# r: root (╙)
			# t: tip (╓)
			# x: branch visual left (╚)
			# y: branch visual center (╩)
			# z: branch visual right (╝)
			# *: filler
	*/
	if config.reverse {
		line = tr(line, "efg.xyz.tr", "xyz.efg.rt")
		/// TODO: option to not flip tip and root char?
		// line = tr(line, "efg.xyz", "xyz.efg")
	}

	colorHints := make([]string, len(line))

	/// set the initial branch colors
	for idx := range line {
		if idx%2 == 0 {
			colorHints[idx] = getBranchColor(idx / 2)
		}
	}

	/// color branches and overpasses based on source
	/// TODO: find a better way to color reverse output? duplicating the whole block is annoying
	if config.reverse {
		if matches := leftxB.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := offsetHelper(idx)
			/// colorHints needs clearing, the source branch color (right)
			/// needs to run all the way until it hits the target branch color
			/// if the x is on an odd column it inherits the color automatically
			clearColorHintsUnderMatch(offset, matches[1], &colorHints)
			if idx%2 == 0 {
				colorHints[offset] = getBranchColor(offset)
				colorHints[idx+1] = getBranchColor((idx + len(matches[1])) / 2)
			} else {
				colorHints[offset] = getBranchColor(offset - 1)
				colorHints[idx+len(matches[1])-1] = getBranchColor((idx + len(matches[1])) / 2)
			}
		} else if matches := leftgI.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := offsetHelper(idx)
			colorHints[idx] = getBranchColor(offset)
		} else if matches := righteB.FindStringSubmatch(line); len(matches) > 0 {
			offset := offsetHelper(strings.Index(line, matches[1]))
			colorHints[offset] = getBranchColor(offset)
		} else if matches := rightzI.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := offsetHelper(idx)
			/// TODO: none of my repos have a z...I, does this also need to clear colorHints?
			colorHints[idx] = getBranchColor(offset)
		}

		/// NOTE: the overpasses needs to be done separately because the regexes above may overlap
		/// NOTE: overpasses inherit only the base color, so we zero-out colorHints over the length of the match
		if matches := leftAg.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := offsetHelper(idx)
			clearColorHintsUnderMatch(idx, matches[1], &colorHints)
			colorHints[idx] = getBranchColor(offset - 1)
		} else if matches := rightAz.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := offsetHelper(idx)
			clearColorHintsUnderMatch(idx, matches[1], &colorHints)
			/// color the section (minus the A)
			colorHints[idx+1] = getBranchColor((idx + len(matches[1])) / 2)
			// /// color the "A"
			colorHints[idx] = getBranchColor(offset)
		}
	} else {
		/// everything above, but in normal order
		if matches := leftxB.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := offsetHelper(idx)
			colorHints[idx] = getBranchColor(offset)
		} else if matches := leftgI.FindStringSubmatch(line); len(matches) > 0 {
			offset := offsetHelper(strings.Index(line, matches[1]))
			colorHints[offset] = getBranchColor(offset)
		} else if matches := righteB.FindStringSubmatch(line); len(matches) > 0 {
			offset := offsetHelper(strings.Index(line, matches[1]))
			clearColorHintsUnderMatch(offset, matches[1], &colorHints)
			colorHints[offset] = getBranchColor(offset - 1)
		} else if matches := rightzI.FindStringSubmatch(line); len(matches) > 0 {
			offset := offsetHelper(strings.Index(line, matches[1]))
			/// TODO: none of my repos have a z...I, does this also need to clear colorHints?
			colorHints[offset] = getBranchColor(offset)
		}

		if matches := leftAg.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := offsetHelper(idx - 1)

			/// we take the index of `g` to get the correct color
			overpassIdx := idx + len(matches[1])
			clearColorHintsUnderMatch(idx, matches[1], &colorHints)
			colorHints[idx] = getBranchColor(overpassIdx / 2)
			colorHints[idx] = getBranchColor(offsetHelper(overpassIdx - 1))
			colorHints[idx-1] = getBranchColor(offset)
		} else if matches := rightAz.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := offsetHelper(idx)
			clearColorHintsUnderMatch(idx, matches[1], &colorHints)

			colorHints[idx] = getBranchColor(offset)
		}
	}

	/// now replace with graph chars
	switch config.style {
	/// thin
	case 1:
		{
			line = tr(line, "ABDO.efg.IKm.xyz.tCMr", "├┤──.┌┬┐.│┼─.└┴┘.┬├├┴")
		}
	/// thin with double bridge
	case 2:
		{
			line = tr(line, "ABDO.efg.IKm.xyz.tCMr", "╞╡═╪.╒╤╕.│┼─.╘╧╛.┬├├┴")
		}
	/// idk why the perl used 10 and 15, like ???
	/// double
	case 3:
		{
			line = tr(line, "ABDO.efg.IKm.xyz.tCMr", "╠╣══.╔╦╗.║╬─.╚╩╝.╓║║╙")
		}
	/// curves
	case 4:
		{
			line = tr(line, "ABDO.efg.IKm.xyz.tCMr", "├┤──.╭┬╮.│┼─.╰┴╯.┬├├┴")
		}
	/// thicc
	case 5:
		{
			line = tr(line, "ABDO.efg.IKm.xyz.tCMr", "┣┫━━.┏┳┓.┃╋━.┗┻┛.┳┣┣┻")
		}
	}
	/// TODO: user-defined replace

	/// finally, actually color the string using the hints
	sb := strings.Builder{}
	for idx, c := range []rune(line) {
		if colorHints[idx] != "" {
			sb.WriteString("{")
			sb.WriteString(colorHints[idx])
			sb.WriteString("}")
		}
		sb.WriteRune(c)
	}

	return sb.String()
}

func clearColorHintsUnderMatch(idx int, match string, colorHints *[]string) {
	for i := idx; i < idx+len(match); i++ {
		if i == 0 {
			/// leave the default color
			continue
		}
		(*colorHints)[i] = ""
	}
}

// reducing repetition, just ensures offset is %2 before halving
func offsetHelper(offset int) int {
	if offset%2 == 1 {
		offset++
	}
	if offset > 0 {
		offset /= 2
	}
	return offset
}

func visPost(line string) string {
	return visXfrm(cleanLine(strings.TrimSpace(line)))
}

func getBranchColor(n int) string {
	return global_branchColors[n%len(global_branchColors)]
}
