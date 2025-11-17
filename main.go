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

var (
	lineRegex    = regexp.MustCompile(`^<(.*?)><(.*?)><(.*?)>(.*)`)
	nextShaRegex = regexp.MustCompile(`^<(.*?)>`)
	nonEscRegex  = regexp.MustCompile(`^([^\x1b]+)`)
	escRegex     = regexp.MustCompile(`(\x1b.*?m)([^\x1b]+)`)
)

var (
	global_commitBuffer []string
)

var config = struct {
	repoPath, branchcolors string
	hashLen                int
	reverse, displayAll    bool
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
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repository",
				Usage:   "repository `path` to use",
				Aliases: []string{"repo", "r"},
				Value:   ".",
			},
			&cli.IntFlag{
				Name:    "hashlength",
				Usage:   "`len`gth of the commit hash",
				Aliases: []string{"l"},
				Value:   8,
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
				Usage: "reverse the display",
				Value: false,
			},
			&cli.StringFlag{
				Name:  "branchcolors",
				Usage: "comma separated `color,color[,color]` used for branches, passed straight to lipgloss.Color",
				Value: "#7272A8, #ff00ff, #b00b69, #e5ebb7, #11bf7b",
			},
		},
		Action: func(_ context.Context, ctx *cli.Command) error {
			if ctx.Bool("force-color") {
				os.Setenv("CLICOLOR_FORCE", "true")
			}
			config.repoPath = filepath.Join(ctx.String("repository"), "./.git")
			config.displayAll = ctx.Bool("all")
			config.reverse = ctx.Bool("reverse")
			config.hashLen = ctx.Int("hashlength")
			config.branchcolors = ctx.String("branchcolors")

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
	global_commitBuffer = make([]string, 0, 10)
	vine := make([]string, 0, 8)

	/// TODO: make option
	PRETTY := "%H\t%at\t%an\t%C(reset)%C(auto)%d%C(reset)\t%s"
	cmd := exec.Command("git", "--git-dir="+config.repoPath, "log", "--date-order", "--pretty=format:<%H><%h><%P>"+PRETTY)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return cli.Exit(err.Error(), 1)
	}
	if err := cmd.Start(); err != nil {
		return cli.Exit(err.Error(), 1)
	}
	reader := bufio.NewReader(stdout)

	for {
		/// TODO: subvineDepth?
		lines, err := getLineBlock(reader, 3)
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

		// fmt.Printf("\t%s...%s %s %s %s %s\n", hash[:7], hash[len(hash)-7:], t, author, refs, message)

		vineBranch(&vine, sha)

		fmt.Printf(oigiki.ProcessTags("{magenta}%s {blue}%s  "), sha[:config.hashLen], t.Format(DATE_FMT))

		vineCommit(&vine, sha, parents)

		/// TODO: auto refs, padding
		if refs != "" {
			fmt.Printf(oigiki.ProcessTags(" {yellow}%s%s {/}%s\n"), author, refs, message)
		} else {
			fmt.Printf(oigiki.ProcessTags(" {yellow}%s {/}%s\n"), author, message)
		}

		vineMerge(&vine, sha, nextShas, parents)
	}

	if err := cmd.Wait(); err != nil {
		return cli.Exit(err.Error(), 1)
	}
	return nil
}

/// layout

func vineBranch(vine *[]string, sha string) {
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
		return
	}
	removeTrailingBlanks(vine)
	// +2 for spaces between
	fmt.Println(strings.Repeat(" ", config.hashLen+len(DATE_FMT)+3) + visFan(output, "branch"))
	// fmt.Println(strings.Repeat(" ", config.hashLen+len(DATE_FMT)+3) + output)
}
func vineCommit(vine *[]string, sha string, parents []string) {
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
	fmt.Print(output)
}
func vineMerge(vine *[]string, sha string, nextShas, parents []string) {
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
		return
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
				if (*vine)[pos] != "" {
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
	/// TODO: dynamic
	// fmt.Println(strings.Repeat(" ", config.hashLen+len(DATE_FMT)+3) + output)
	fmt.Println(strings.Repeat(" ", config.hashLen+len(DATE_FMT)+3) + visFan(output, "merge"))
}

/// beautification

func visFan(s, t string) string {
	isBranch := t == "branch"
	r := regexp.MustCompile(`(?i)s.*s`)
	r2 := regexp.MustCompile(`O[DO]+O`)
	rS1 := regexp.MustCompile(`(s.*)S(.*s)`)
	rS2 := regexp.MustCompile(`(s.*)S`)
	rS3 := regexp.MustCompile(`S(.*s)`)

	s = r.ReplaceAllStringFunc(s, func(x string) string {
		return strings.ReplaceAll(strings.ReplaceAll(x, " ", "D"), "I", "O")
	})
	s = r2.ReplaceAllStringFunc(s, func(x string) string {
		return strings.Repeat("O", len(x))
	})

	if m := rS1.FindStringSubmatch(s); len(m) > 0 {
		s = strings.Replace(s, m[0], visFan3(m[1], m[2]), 1)
	} else if m := rS2.FindStringSubmatch(s); len(m) > 0 {
		s = strings.Replace(s, m[0], visFan2L(m[1])+"B", 1)
	} else if m := rS3.FindStringSubmatch(s); len(m) > 0 {
		s = strings.Replace(s, m[0], "A"+visFan2R(m[1]), 1)
	} else {
		panic("FUUUUUCK")
	}

	if isBranch {
		s = strings.ReplaceAll(s, "e", "x")
		s = strings.ReplaceAll(s, "f", "y")
		s = strings.ReplaceAll(s, "g", "z")
	}
	return s
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
	l = visFan2L(l)
	r = visFan2R(r)
	return l + "K" + r
}
func visXfrm(s string, spec bool) string {
	r := regexp.MustCompile(`[Ctr].*`)
	if spec {
		s = r.ReplaceAllStringFunc(s, func(s string) string {
			return strings.ReplaceAll(s, " ", "*")
		})
	}
	/// WIP
	return s
}
func visPost(s, f string) {
	s = visXfrm(s, f != "")

	if f != "" {

	}
}
