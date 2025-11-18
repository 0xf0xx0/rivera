// git-河流
//
// pretty git-log ported from git-forest(a)
//
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"

	"github.com/0xf0xx0/oigiki"
	"github.com/urfave/cli/v3"
)

const (
	DATE_FMT = "2006-01-02 15:04"
)

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
	global_commitBuffer []string
	BRANCH_COLORS       = []string{} /// populated in flag
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
	app := &cli.Command{
		Name:                   "rivera",
		Version:                "0.0.0+g" + buildCommit,
		Usage:                  "display the git river, like git-forest",
		UseShortOptionHandling: true,
		/// TODO: pass unknown flags to git
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
				Aliases: []string{"l"},
				Value:   8,
			},
			&cli.Uint8Flag{
				Name:    "style",
				Usage:   "style `num` to select (1-5)",
				Aliases: []string{"s"},
				Value:   1,
			},
			&cli.Uint8Flag{
				Name:    "svdepth",
				Usage:   "maximum length of merge subvines",
				Aliases: []string{"s"},
				Value:   2,
			},
			&cli.Uint8Flag{
				Name:    "graph-margin-left",
				Usage:   "left margin of the commit graph",
				Aliases: []string{"s"},
				Value:   2,
			},
			&cli.Uint8Flag{
				Name:    "graph-margin-right",
				Usage:   "right margin of the commit graph",
				Aliases: []string{"s"},
				Value:   1,
			},
			&cli.BoolFlag{
				Name:  "all",
				Usage: "display all branches",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "force-color",
				Usage: "force color output (useful for piping)",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "reverse",
				Usage: "reverse the flow",
				Aliases: []string{"r"},
				Value: false,
			},
			&cli.StringFlag{
				Name:  "branchcolors",
				Usage: "comma separated `color,color[,color]` used for branches, passed straight to oigiki",
				Value: "red, blue, yellow, green, cyan, magenta, white",
			},
		},
		Action: func(_ context.Context, ctx *cli.Command) error {
			if ctx.Bool("force-color") {
				os.Setenv("CLICOLOR_FORCE", "true")
			}
			config.repoPath = filepath.Join(ctx.String("repository"), "./.git")
			config.displayAll = ctx.Bool("all")
			config.reverse = ctx.Bool("reverse")
			config.style = ctx.Uint8("style")
			config.hashLen = ctx.Uint8("hashlength")
			config.leftMargin = ctx.Uint8("graph-margin-left")
			config.rightMargin = ctx.Uint8("graph-margin-right")
			config.subvineDepth = ctx.Uint8("svdepth") + 1
			BRANCH_COLORS = strings.Split(ctx.String("branchcolors"), ",")
			for color := range BRANCH_COLORS {
				BRANCH_COLORS[color] = strings.TrimSpace(cleanLine(BRANCH_COLORS[color]))
			}

			//////
			/// now, we build the river
			//////
			return processCommits()
		},
	}
	/// discard sigpipe
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGPIPE)
		<-c
	}()
	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func processCommits() error {
	// refs :=
	/// NOTE: getLineBlock inches the slice along, ensure the backing array has enough capacity
	global_commitBuffer = make([]string, 0, config.subvineDepth*32)
	vine := make([]string, 0, config.subvineDepth)

	/// TODO: make option...?
	/// this might be something im too lazy to do
	PRETTY := "%H\t%at\t%an\t%C(reset)%C(auto)%d%C(reset)\t%s"
	cmd := exec.Command("git", "--git-dir="+config.repoPath,
		"log", "--date-order", "--pretty=format:<%H><%h><%P>"+PRETTY)
	if config.displayAll {
		cmd.Args = append(cmd.Args, "--all", "HEAD")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return cli.Exit(err.Error(), 1)
	}
	if err := cmd.Start(); err != nil {
		return cli.Exit(err.Error(), 1)
	}
	reader := bufio.NewReader(stdout)

	collectedLines := make([]string, 0, 128)
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
		_, t, author, refs, message := splitMessage(msg)

		ret := vineBranch(&vine, sha)

		ret += fmt.Sprintf(oigiki.ProcessTags("{magenta}%s {blue}%s%s"), sha[:config.hashLen], t.Format(DATE_FMT), strings.Repeat(" ", int(config.leftMargin)))

		ret += vineCommit(&vine, sha, parents)

		/// TODO: auto refs
		if refs != "" {
			ret += fmt.Sprintf(oigiki.ProcessTags("%s{yellow}%s%s {/}%s\n"), strings.Repeat(" ", int(config.rightMargin)), author, refs, message)
		} else {
			ret += fmt.Sprintf(oigiki.ProcessTags("%s{yellow}%s {/}%s\n"), strings.Repeat(" ", int(config.rightMargin)), author, message)
		}

		if config.reverse {
			collectedLines = append(collectedLines, ret)
			collectedLines = append(collectedLines, vineMerge(&vine, sha, nextShas, parents))
		} else {
			ret += vineMerge(&vine, sha, nextShas, parents)
			fmt.Print(ret)
		}
	}

	if config.reverse {
		for _, x := range slices.Backward(collectedLines) {
			fmt.Print(x)
		}
	}

	if err := cmd.Wait(); err != nil {
		return cli.Exit(err.Error(), 1)
	}
	return nil
}

/// layout

func vineBranch(vine *[]string, sha string) string {
	matchedCount := 0
	masterDrawn := false
	output := ""

	for columnIndex := range *vine {
		if (*vine)[columnIndex] == "" {
			output += " "
		} else if (*vine)[columnIndex] != sha {
			output += "I" // Straight line (other branch continues)
		} else {
			// This column points to our commit
			if !masterDrawn && columnIndex%2 == 0 {
				output += "S" // Main branch split
				masterDrawn = true
			} else {
				output += "s"             // Secondary branch split
				(*vine)[columnIndex] = "" // Clear
			}
			matchedCount++
		}
	}
	// Only print if multiple branches converged
	if matchedCount < 2 {
		return ""
	}
	removeTrailingBlanks(vine)
	// +2 for spaces between
	// fmt.Println(strings.Repeat(" ", config.hashLen+len(DATE_FMT)+3) + output)
	// fmt.Println(strings.Repeat(" ", config.hashLen+len(DATE_FMT)+3) + visFan(output, "branch"))
	// fmt.Println(strings.Repeat(" ", config.hashLen+len(DATE_FMT)+3) + visPost(visFan(output, "branch"), ""))
	return fmt.Sprintln(strings.Repeat(" ", int(config.hashLen)+len(DATE_FMT)+3) + visPost(visFan(output, "branch")))
}
func vineCommit(vine *[]string, sha string, parents []string) string {
	output := ""

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
			output += "t"
			*vine = append(*vine, sha)
		}
	}
	// println(fmt.Printf("vine: %q %d", *vine, len(*vine)))

	removeTrailingBlanks(vine)

	if len(parents) == 0 {
		output = strings.Replace(output, "C", "r", 1)
	} else if len(parents) > 1 {
		output = strings.Replace(output, "C", "M", 1)
	}
	// fmt.Print(output)
	// fmt.Print(visPost(output, ""))
	return visPost(output)
}
func vineMerge(vine *[]string, sha string, nextShas, parents []string) string {
	originalColumn := -1
	output := ""
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
	// fmt.Println(strings.Repeat(" ", config.hashLen+len(DATE_FMT)+3) + output)
	// fmt.Println(strings.Repeat(" ", config.hashLen+len(DATE_FMT)+3) + visFan(output, "merge"))
	// fmt.Println(strings.Repeat(" ", config.hashLen+len(DATE_FMT)+3) + visPost(visFan(output, "merge"), ""))
	// return ""
	return fmt.Sprintln(strings.Repeat(" ", int(config.hashLen)+len(DATE_FMT)+3) + visPost(visFan(output, "merge")))
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
	/*
		 NOTE: from original perl:
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
		line = tr(line, "efg.xyz.t", "xyz.efg.r")
		/// TODO: option to not flip tip and root char?
		// line = tr(line, "efg.xyz", "xyz.efg")
	}
	/*
		color entire branches, including overpasses
		characters inside (groups) are colored differently
		x...B, g...I, A(...g) for leftward merge
		e(...B), z(...I), A...z for rightward merge
		otherwise color every other line
	*/

	colorHints := make([]string, len(line))

	/// set the initial branch colors
	for idx := range line {
		if idx%2 == 0 {
			colorHints[idx] = getBranchColor(idx / 2)
		}
	}

	/// update the colors with regex
	if matches := leftxB.FindStringSubmatch(line); len(matches) > 0 {
		offset := offsetHelper(strings.Index(line, matches[1]))
		colorHints[offset] = getBranchColor(offset)
	} else if matches := leftgI.FindStringSubmatch(line); len(matches) > 0 {
		offset := offsetHelper(strings.Index(line, matches[1]))
		colorHints[offset] = getBranchColor(offset)
	} else if matches := righteB.FindStringSubmatch(line); len(matches) > 0 {
		offset := offsetHelper(strings.Index(line, matches[1]))
		/// colorHints needs clearing, the source branch color (right)
		/// needs to run all the way until it hits the target branch color
		clearColorHintsUnderMatch(offset, matches[1], &colorHints)
		colorHints[offset] = getBranchColor(offset - 1)
	} else if matches := rightzI.FindStringSubmatch(line); len(matches) > 0 {
		offset := offsetHelper(strings.Index(line, matches[1]))
		/// TODO: none of my repos have a z...I, does this also need to clear colorHints?
		colorHints[offset] = getBranchColor(offset)
	}

	/// the overpasses needs to be done separately because the regexes above may overlap
	/// NOTE: overpasses inherit only the base color, so we zero-out colorHints over the length of the match
	if matches := leftAg.FindStringSubmatch(line); len(matches) > 0 {
		idx := strings.Index(line, matches[1])
		offset := offsetHelper(idx)
		clearColorHintsUnderMatch(idx, matches[1], &colorHints)
		/// color the section (minus the A)
		colorHints[idx] = getBranchColor((idx + len(matches[1])) / 2)
		/// color the "A"
		colorHints[idx-1] = getBranchColor(offset - 1)
	} else if matches := rightAz.FindStringSubmatch(line); len(matches) > 0 {
		idx := strings.Index(line, matches[1])
		offset := offsetHelper(idx)
		clearColorHintsUnderMatch(idx, matches[1], &colorHints)
		colorHints[idx] = getBranchColor(offset)
	}

	/// now replace with graph chars
	switch config.style {
	case 1:
		{
			line = tr(line, "ABDO.efg.IKm.xyz.tCMr", "├┤──.┌┬┐.│┼─.└┴┘.┬├├┴")
		}
	case 2:
		{
			line = tr(line, "ABDO.efg.IKm.xyz.tCMr", "╞╡═╪.╒╤╕.│┼─.╘╧╛.┬├├┴")
		}
	/// idk why the perl used 10 and 15, like ???
	case 3:
		{
			line = tr(line, "ABDO.efg.IKm.xyz.tCMr", "╠╣══.╔╦╗.║╬─.╚╩╝.╦║║╩")
		}
	case 4:
		{
			line = tr(line, "ABDO.efg.IKm.xyz.tCMr", "├┤──.╭┬╮.│┼─.╰┴╯.┬├├┴")
		}
	case 5:
		{
			line = tr(line, "ABDO.efg.IKm.xyz.tCMr", "┣┫━━.┏┳┓.┃╋━.┗┻┛.┳┣┣┻")
		}
	}

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

	return oigiki.ProcessTags(sb.String())
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
	return BRANCH_COLORS[n%len(BRANCH_COLORS)]
}
