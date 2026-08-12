/*
git-rivera/git-河流

display a git river, like git-forest

options:

	--repository path, --repo path          repository path to use (default: ".")
	--start commithash                      commithash to start at
	--end commithash                        commithash to end at
	--hashlength len, --hashlen len, -l len length of the commit hash (min: 4) (default: 0)
	--messagelength len, --msglen len       length of the commit message (default: 50)
	--style num, -s num                     style num to select (0-7) (default: 1)
	--userstyle chars                       graph chars to use (format: ABDO.efg.IKm.xyz.tCMr)
	--subvinedepth uint, --svdepth uint     internal lookahead depth for branches, not sure what this does exactly (default: 2)
	--graphmarginleft uint, --marginl uint  left margin of the commit graph (default: 2)
	--graphmarginright uint, --marginr uint right margin of the commit graph (default: 1)
	--all, -a                               display all branches
	--smooth-overpass                       smooth out overpasses, ex: '═╪═╪═╪' -> '══════'
	--reverse, -r                           reverse the flow
	--status                                display the git status
	--branchcolors color,color[,color]      comma separated color,color[,color] used for branches, passed straight to oigiki (default: "red, blue, yellow, green, cyan, magenta, white")
	--maxgitrecursedepth uint, --mgrd uint  maximum depth to search for a .git dir (default: 8)
	--help, -h                              show help
	--version, -v                           print the version
	--color                                 force color output
	--nocolor, --stdout                     disable color output
*/
package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
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
	styleReplace        = "ABDO.efg.IKm.xyz.tCMr"
	DATE_FMT            = "2006-01-02 15:04"
	commandHelpTemplate = `   {bold}{green}{{.Name}} - {blue}{{.Usage}}{/}

Usage:
   {green}{{.Name}} {blue}[options] [-- --git-log-opt1 ... --git-log-optN]{/}

Options:{blue}
   {{range .VisibleFlags}}{{.String}}
   {{end}}{/}
Version:
   {green}v{{.Version}}
`
)

// regex
var (
	lineRegex          = regexp.MustCompile(`^<(.*?)><(.*?)><(.*?)>(.*)`)
	nextShaRegex       = regexp.MustCompile(`^<(.*?)>`)
	rebaseRefRegex     = regexp.MustCompile(`^\S+\s+(\S+)`)
	rebaseCommentRegex = regexp.MustCompile(`^\s*#`)

	/// fmt pt 1
	overpassRegex = regexp.MustCompile(`O[DO]+O`)
	fanRegex      = regexp.MustCompile(`(?i)s.*s`)
	fanLMR        = regexp.MustCompile(`(s.*)S(.*s)`)
	fanLM         = regexp.MustCompile(`(s.*)S`)
	fanMR         = regexp.MustCompile(`S(.*s)`)
	/// fmt pt 2
	leftcii = regexp.MustCompile(`(C|I)II`)

	leftxB = regexp.MustCompile(`(x\w*B)`)
	leftxz = regexp.MustCompile(`(x\w*z)`)
	leftAg = regexp.MustCompile(`(A\w*g)`)

	righteB = regexp.MustCompile(`(e\w*)B`)
	righteg = regexp.MustCompile(`(e\w*)g`)
	rightzI = regexp.MustCompile(`z(\w*I)`) /// TODO: needed?
	rightAz = regexp.MustCompile(`(A\w*z)`)
)

// state
var (
	global_gitArgs               []string
	global_commitBuffer          []string
	global_branchColors          []string /// populated in flag
	global_branchColorsLen       int
	global_root, global_repoRoot fs.FS
)

var config = struct {
	start, end,
	repoPath, userStyle string
	hashLen, msgLen, style, subvineDepth int
	leftMargin, rightMargin              int
	reverse, smoothOverpass,
	displayStatus, displayAll bool
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
	/// NOTE: discard sigpipe
	/// otherwise go vomits a pointless error
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGPIPE)
		<-c
	}()
	cli.FlagStringer = stolenFlagStringer
	cli.RootCommandHelpTemplate = oigiki.ProcessTags(commandHelpTemplate)

	app := &cli.Command{
		Name:                   "git-rivera",
		Version:                "1.1.0+g" + buildCommit,
		Usage:                  "display a git river, like git-forest",
		UseShortOptionHandling: true,
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
			&cli.StringFlag{
				Name:  "start",
				Usage: "`commithash` to start at",
			},
			&cli.StringFlag{
				Name:  "end",
				Usage: "`commithash` to end at",
			},
			&cli.Uint8Flag{
				Name:    "hashlength",
				Usage:   "`len`gth of the commit hash (min: 4)",
				Aliases: []string{"hashlen", "l"},
			},
			&cli.Uint8Flag{
				Name:    "messagelength",
				Usage:   "`len`gth of the commit message",
				Aliases: []string{"msglen"},
				Value:   50, /// 50/72 rule
			},
			&cli.Uint8Flag{
				Name:    "style",
				Usage:   "style `num` to select (0-7)",
				Aliases: []string{"s"},
				Value:   1,
			},
			&cli.StringFlag{
				Name:  "userstyle",
				Usage: "graph `chars` to use (format: ABDO.efg.IKm.xyz.tCMr)",
			},
			&cli.Uint8Flag{
				Name:    "subvinedepth",
				Usage:   "internal lookahead depth for branches, slightly modifies graph | seems like 1-10 is the range, anything further increases processing time for no visual difference",
				Aliases: []string{"svdepth"},
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
				Name:  "smooth-overpass",
				Usage: "smooth out overpasses, ex: '═╪═╪═╪' -> '══════'",
				Value: false,
			},
			&cli.BoolFlag{
				Name:    "reverse",
				Usage:   "reverse the flow",
				Aliases: []string{"r"},
				Value:   false,
			},
			&cli.BoolFlag{
				Name:  "status",
				Usage: "display the git status",
				Value: false,
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
			global_gitArgs = ctx.Args().Slice()
			config.start = ctx.String("start")
			config.end = ctx.String("end")
			config.repoPath = repoRoot
			config.displayAll = ctx.Bool("all")
			config.reverse = !ctx.Bool("reverse")
			config.displayStatus = ctx.Bool("status")
			config.smoothOverpass = ctx.Bool("smooth-overpass")
			config.msgLen = int(ctx.Uint8("messagelength"))
			config.leftMargin = int(ctx.Uint8("graphmarginleft"))
			config.rightMargin = int(ctx.Uint8("graphmarginright"))
			config.subvineDepth = max(1, int(ctx.Uint8("svdepth")))
			config.style = int(ctx.Uint8("style"))
			config.userStyle = ctx.String("userstyle")
			if config.userStyle != "" && len(config.userStyle) != len(styleReplace) {
				return cli.Exit("invalid userstyle, must follow the format", 1)
			}
			global_branchColors = strings.Split(ctx.String("branchcolors"), ",")
			for idx, color := range global_branchColors {
				color = strings.TrimSpace(cleanLine(color))
				_, tagType := oigiki.GetTagEscapeCode(color)

				/// MAYBE: != TagTypeColor?
				if tagType == oigiki.TagTypeUnknown {
					return cli.Exit(fmt.Sprintf("invalid branch color: %q", color), 1)
				}
				global_branchColors[idx] = color
			}
			global_branchColorsLen = len(global_branchColors)

			/// use the length of the short commit hash from git as the default length
			/// NOTE: this has the fun side effect of being the only thing alerting us to an empty repo!
			cmd := makeGitCommand("rev-parse", "--short", "HEAD")
			output, err := readOutput(cmd)
			if err != nil {
				if err.Error() == "fatal: Needed a single revision" {
					return cli.Exit("no commits, is the repo empty?", err.(cli.ExitCoder).ExitCode())
				}
				return err
			}
			config.hashLen = len(output)

			if ctx.Uint8("hashlength") != 0 {
				config.hashLen = int(min(max(4, ctx.Uint8("hashlength")), 40))
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

// commits are processed from tip to root
func processCommits() error {
	/// NOTE: getLineBlock inches the slice along, ensure the backing array has enough capacity
	/// the buffer stores the next commits to look at and is filled/grown by getLineBlock
	global_commitBuffer = make([]string, 0, config.subvineDepth*32)
	/// each vine is a git branch
	vine := make([]string, 0, config.subvineDepth)
	refMap, err := getRefs()
	if err != nil {
		return cli.Exit(err, 1)
	}
	status := ""
	if config.displayStatus {
		status, err = getStatus()
		if err != nil {
			return cli.Exit(err, 1)
		}
	}

	/// i dont understand the point of havin this be customizable in git-foresta,
	/// it breaks when you change it
	PRETTY := "%H\t%at\t%an\t%C(reset)%C(auto)%d%C(reset)\t%s"

	cmd := makeGitCommand("log", "--date-order", "--pretty=format:<%H><%h><%P>"+PRETTY)
	if config.displayAll {
		cmd.Args = append(cmd.Args, "--all", "HEAD")
	}
	if !oigiki.NoColor {
		cmd.Args = append(cmd.Args, "--color")
	}
	if len(global_gitArgs) > 0 {
		cmd.Args = append(cmd.Args, global_gitArgs...)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return cli.Exit(err, 1)
	}

	if err := cmd.Start(); err != nil {
		fmt.Println(stdout)
		return cli.Exit(err, cmd.ProcessState.ExitCode())
	}
	reader := bufio.NewReader(stdout)

	var collectedLines []string
	if config.reverse {
		/// we only collect lines when reversing
		collectedLines = make([]string, 0, config.subvineDepth*32)
	}

	/// start by printing all lines; --start and --end flip this
	printlines := true
	/// if --start is passed, dont print any of the preceding lines
	if config.start != "" {
		printlines = false
	}

	for {
		lines, err := getLineBlock(reader, config.subvineDepth)
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

		/// draw the branches leading to the commit
		ret.WriteString(vineBranch(&vine, sha))

		/// print hash, date, and leftpad
		ret.WriteString(fmt.Sprintf("{magenta}%s {blue}%s%s",
			sha[:config.hashLen], t.Format(DATE_FMT), strings.Repeat(" ", config.leftMargin),
		))

		/// print the commit
		ret.WriteString(vineCommit(&vine, sha, parents))

		/// rightpad, author
		ret.WriteString(fmt.Sprintf("%s{yellow}%s",
			strings.Repeat(" ", config.rightMargin), author))

		/// avoid expensive string lookups
		if _, ok := refMap[sha]; ok {
			/// only print the status on the local HEAD
			/// TODO: idx != -1 OR (idx > 0 AND autoRefs[idx-1] == ' ')
			if idx := strings.Index(autoRefs, "HEAD"); idx != -1 && autoRefs[idx-1] != '/' {
				autoRefs = strings.Replace(autoRefs, "HEAD", "HEAD"+status, 1)
			}
			autoRefs = strings.ReplaceAll(autoRefs, "tag:", "{magenta}tag:{/magenta}")
		}
		ret.WriteString(autoRefs)
		ret.WriteString("{/} ")

		/// truncate message if too long, but only if adding ellipses would be shorter
		if len(message) > config.msgLen+3 {
			ret.WriteString(string(message[:config.msgLen]))
			ret.WriteString("{blackbright}...")
		} else {
			ret.WriteString(string(message))
		}
		ret.WriteRune('\n')

		/// draw merges after
		ret.WriteString(vineMerge(&vine, sha, nextShas, parents))

		/// toggle printing
		if config.start != "" && strings.HasPrefix(sha, config.start) {
			printlines = true
		}
		if !printlines {
			continue
		}
		if config.end != "" && strings.HasPrefix(sha, config.end) {
			printlines = false
		}

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

// creates a map of reference hashes to names
func getRefs() (map[string][]string, error) {
	m := make(map[string][]string, 32)
	cmd := makeGitCommand("show-ref")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return m, cli.Exit(err.Error(), 1)
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if err := cmd.Start(); err != nil {
		return m, cli.Exit(err, 1)
	}

	reader := bufio.NewScanner(stdout)
	for reader.Scan() {
		line := reader.Text()
		split := strings.Split(line, " ")
		commit := split[0]
		refName := split[1]
		appendToMapArray(m, commit, refName)
	}
	if err := cmd.Wait(); err != nil {
		return m, cli.Exit(errBuf.String(), cmd.ProcessState.ExitCode())
	}

	if fileExistsInRepo("rebase-merge/git-rebase-todo") {
		rebase, err := readFileInRepo("rebase-merge/git-rebase-todo")
		if err != nil {
			return nil, err
		}
		split := strings.Split(rebase, "\n")
		fmt.Printf("%+v\n", split)
		curr := ""
		matches := rebaseRefRegex.FindStringSubmatch(rebase)
		if len(matches) == 0 {
			return nil, errors.New("rebase reference leads to nowhere")
		}
		curr = matches[1]
		for _, line := range split {
			if rebaseCommentRegex.MatchString(line) {
				continue
			}

			matches := rebaseRefRegex.FindStringSubmatch(line)
			if len(matches) == 1 {
				curr = matches[1]
			}
		}

		println("curr: ", curr)

		if curr != "" {
			/// resolve the ref to a commit hash
			cmd := makeGitCommand("rev-parse", curr)
			curr, err := readOutput(cmd)
			if err != nil {
				return nil, err
			}
			curr = strings.TrimSpace(curr)

			appendToMapArray(m, curr, "rebase/next")
		}

		todoCmd := makeGitCommand("rev-parse", "rebase-merge/onto")
		if output, err := readOutput(todoCmd); err == nil {
			appendToMapArray(m, output, "rebase/onto")
		}

		head, err := readOutput(makeGitCommand("rev-parse", "HEAD"))
		if err != nil {
			return nil, err
		}
		appendToMapArray(m, head, "rebase/head")
	}
	return m, nil
}

func getStatus() (string, error) {
	dirty := ""
	midFlow := ""

	hasChangeUnstagedCmd := makeGitCommand("diff", "--shortstat")
	hasChangeStagedCmd := makeGitCommand("diff", "--shortstat", "--cached")
	hasStashCmd := makeGitCommand("stash", "list")
	hasUntrackedCmd := makeGitCommand("ls-files", "--others", "--exclude-standard")

	x, err := readOutput(hasChangeUnstagedCmd)
	if err != nil {
		fmt.Println(x)
		return "", err
	}
	/// unstaged
	if len(x) > 0 {
		dirty += "*"
	}

	x, err = readOutput(hasChangeStagedCmd)
	if err != nil {
		return "", err
	}
	/// staged
	if len(x) > 0 {
		dirty += "+"
	}

	x, err = readOutput(hasStashCmd)
	if err != nil {
		return "", err
	}
	/// stash exists
	if len(x) > 0 {
		dirty += "$"
	}

	x, err = readOutput(hasUntrackedCmd)
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
	if fileExistsInRepo("rebase-merge") {
		if fileExistsInRepo("rebase-merge/interactive") {
			midFlow = "|REBASE-i"
		} else {
			midFlow = "|REBASE-m"
		}
	} else if fileExistsInRepo("rebase-apply") {
		if fileExistsInRepo("rebase-apply/rebasing") {
			midFlow = "|REBASE"
		} else if fileExistsInRepo("rebase-apply/applying") {
			midFlow = "|AM"
		} else {
			midFlow = "|AM/REBASE"
		}
	} else if fileExistsInRepo("MERGE_HEAD") {
		midFlow = "|MERGING"
	} else if fileExistsInRepo("CHERRY_PICK_HEAD") {
		midFlow = "|CHERRY-PICKING"
	} else if fileExistsInRepo("REVERT_HEAD") {
		midFlow = "|REVERTING"
	} else if fileExistsInRepo("BISECT_LOG") {
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
	return fmt.Sprintln(strings.Repeat(" ", config.hashLen+1+len(DATE_FMT)+config.leftMargin) + visPost(visFan(output.String(), true)))
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
				/// NOTE: empty string = undef
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
			/// idk y +1 but weh
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
		/// NOTE: this is how we interpret `undef` usually
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
	return fmt.Sprintln(strings.Repeat(" ", config.hashLen+1+len(DATE_FMT)+config.leftMargin) + visPost(visFan(output, false)))
}
