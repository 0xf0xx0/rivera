package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"rivera/postprocess"
	"rivera/shared"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/urfave/cli/v2"
)

var config = struct {
	repoPath, branchcolors string
	hashLen                int
	reverse, displayAll    bool
}{}

var Commit = func() string {
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
	app := &cli.App{
		Name:                   "rivera",
		Version:                "0.0.1+g" + Commit,
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
		Action: func(ctx *cli.Context) error {
			if ctx.Bool("force-color") {
				os.Setenv("CLICOLOR_FORCE", "true")
			}
			config.repoPath = ctx.String("repository")
			config.displayAll = ctx.Bool("all")
			config.reverse = ctx.Bool("reverse")
			config.hashLen = ctx.Int("hashlength")
			config.branchcolors = ctx.String("branchcolors")

			repo, err := git.PlainOpen(config.repoPath)
			if err != nil {
				return err
			}

			head, err := repo.Head()
			if err != nil {
				return err
			}

			iter, err := repo.Log(&git.LogOptions{
				From:  head.Hash(),
				Order: git.LogOrderCommitterTime, /// not certain this works :\
				All:   config.displayAll,
			})
			if err != nil {
				return err
			}
			defer iter.Close()
			refs, _ := repo.References()
			defer refs.Close()

			tagMap := make(map[string][]string)
			branchMap := make(map[string][]string)
			refs.ForEach(func(ref *plumbing.Reference) error {
				switch ref.Type() {
				case plumbing.HashReference:
					{
						hash := ref.Hash().String()
						name := ref.Name()
						if name.IsTag() {
							if _, ok := tagMap[hash]; !ok {
								tagMap[hash] = make([]string, 0, 4)
							}
							tagMap[hash] = append(tagMap[hash], shared.Colorize("tag: ", "5")+shared.Colorize(name.Short(), "3"))
						}
						if name.IsRemote() || name.IsBranch() {
							if _, ok := branchMap[hash]; !ok {
								branchMap[hash] = make([]string, 0, 4)
							}
							if name.IsRemote() {
								branchMap[hash] = append(branchMap[hash], shared.Colorize(name.Short(), "1"))
							} else {
								branchMap[hash] = append(branchMap[hash], shared.Colorize(name.Short(), "2"))
							}
						}
					}
				}
				return nil
			})

			/// now, we build the river
			lines := make([]string, 0, 64)
			commits := postprocess.IterToArray(iter)
			vine := make([]string, 0, 8)
			for {
				block := postprocess.GetCommitBlock(&commits, 2)
				if len(block) == 0 {
					break
				}

				line := ""
				commit := block[0]
				nextCommits := block[1:]
				sha := commit.Hash.String()
				nextShas := make([]string, len(nextCommits))
				for i, commit := range nextCommits {
					nextShas[i] = commit.ID().String()
				}
				parents := make([]string, commit.NumParents())
				for i := 0; i < commit.NumParents(); i++ {
					parent, _ := commit.Parent(i)
					parents[i] = parent.Hash.String()
				}

				timestamp := commit.Author.When.Format("2006-01-02 15:04") /// literally what is this
				author := commit.Author.Name                               /// when using git webui, .Committer is git host, not acc
				summary := strings.Split(commit.Message, "\n")[0]
				tags, tagOk := tagMap[sha]
				branches, branchOk := branchMap[sha]
				isHead := head.Hash().String() == commit.Hash.String()

				postprocess.VineBranch(&vine, sha)
				line = fmt.Sprintf("%s %s",
					shared.Colorize(sha[:config.hashLen], "5"),
					shared.Colorize(timestamp, "4"))

				ra := postprocess.VineCommit(&vine, sha, parents)

				line += fmt.Sprint(len(vine))
				line += " " + ra + " "
				//line += fmt.Sprint(postprocess.VisPost(postprocess.VisCommit(ra)) + " ")
				line += fmt.Sprintf("%s", shared.Colorize(author, "3"))

				if isHead || tagOk || branchOk {
					line += shared.Colorize(" (", "4")
					if isHead {
						line += shared.Colorize("HEAD %", "6")
						if tagOk || branchOk {
							line += " "
						}
					}
					refLine := append(append(make([]string, 0, 2), tags[:]...), branches[:]...)
					line += fmt.Sprintf("%s", strings.Join(refLine, shared.Colorize(",", "4")+" "))
					line += shared.Colorize(")", "4")
				}

				/// how to get term width?
				// lineLength := lipgloss.Width(line)
				/// 50/72 rule ig
				summaryLimit := int(math.Min(72, float64(len(summary))))
				line += fmt.Sprintf(" %s", summary[:summaryLimit])

				postprocess.VineMerge(&vine, sha, &nextShas, &parents)
				lines = append(lines, line)
				println(line)
			}
			if config.reverse {
				for i := len(lines) - 1; i > -1; i-- {
					line := lines[i]
					fmt.Println(line)
				}
			} else {
				for _, line := range lines {
					fmt.Println(line)
				}
			}
			return nil
		},
	}
	/// discard sigpipe
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGPIPE)
		<-c
	}()
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
func printCommit(c *object.Commit, graphLine string, tagMap, branchMap map[string][]string, isHead bool) string {
	line := ""
	hash := c.Hash.String()
	timestamp := c.Author.When.Format("2006-01-02 15:04") /// literally what is this
	author := c.Author.Name                               /// when using git webui, committer is git host, not acc
	summary := strings.Split(c.Message, "\n")[0]
	tags, tagOk := tagMap[hash]
	branches, branchOk := branchMap[hash]

	line = fmt.Sprintf("%s %s  %s%s",
		shared.Colorize(hash[:config.hashLen], "5"),
		shared.Colorize(timestamp, "4"),
		graphLine,
		shared.Colorize(author, "3"))
	if isHead || tagOk || branchOk {
		line += shared.Colorize(" (", "4")
		if isHead {
			line += shared.Colorize("HEAD %", "6")
			if tagOk || branchOk {
				line += " "
			}
		}
		refLine := append(append(make([]string, 0, 2), tags[:]...), branches[:]...)
		line += fmt.Sprintf("%s", strings.Join(refLine, shared.Colorize(",", "4")+" "))
		line += shared.Colorize(")", "4")
	}

	/// how to get term width?
	// lineLength := lipgloss.Width(line)
	/// 50/72 rule ig
	summaryLimit := int(math.Min(72, float64(len(summary))))
	line += fmt.Sprintf(" %s", summary[:summaryLimit])
	return line
}
