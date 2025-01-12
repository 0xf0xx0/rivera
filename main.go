package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"rivera/graph"

	"github.com/charmbracelet/lipgloss"
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

func main() {
	app := &cli.App{
		Name:                   "rivera",
		Version:                "0.0.1",
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
				Name:  "reverse",
				Usage: "reverse the display",
				Value: false,
			},
			&cli.StringFlag{
				Name:  "branchcolors",
				Usage: "comma separated `color[,color]` used for branches, passed straight to lipgloss.Color",
				Value: "#7272A8,#ff00ff,#b00b69",
			},
		},
		Action: func(ctx *cli.Context) error {
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

			cIter, err := repo.Log(&git.LogOptions{
				From:  head.Hash(),
				Order: git.LogOrderCommitterTime, // not certain this works :\
				All:   config.displayAll,
			})
			if err != nil {
				return err
			}
			tags, err := repo.TagObjects()
			if err != nil {
				return err
			}
			branches, err := repo.Branches()
			if err != nil {
				return err
			}
			/// todo: cant get remote branches
			// remotes, err := repo.Remotes()
			// if err != nil {
			// 	return err
			// }

			tagMap := make(map[string][]string)
			branchMap := make(map[string][]string)
			tags.ForEach(func(tag *object.Tag) error {
				hash := tag.Target.String()
				if _, ok := tagMap[hash]; !ok {
					tagMap[hash] = make([]string, 0, 1)
				}
				tagMap[hash] = append(tagMap[hash], tag.Name)
				return nil
			})
			branches.ForEach(func(branchRef *plumbing.Reference) error {
				hash := branchRef.Hash().String()
				name := branchRef.Name().Short()
				if _, ok := branchMap[hash]; !ok {
					branchMap[hash] = make([]string, 0, 1)
				}
				// println(hash, name)
				branchMap[hash] = append(branchMap[hash], lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(name))
				return nil
			})

			/// now, we build the river
			g := graph.New()
			g.SetColors(config.branchcolors)
			lines := make([]string, 0, 64)
			cIter.ForEach(func(c *object.Commit) error {
				g.Update(c)
				for {
					if g.IsCommitFinished() {
						break
					}
					line, isCommit := g.NextLine()
					if config.reverse {
						/// TODO: do we have to do this? i think so lol
						line = strings.ReplaceAll(line, "\\", "t")
						line = strings.ReplaceAll(line, "/", "\\")
						line = strings.ReplaceAll(line, "t", "/")
					}
					if isCommit {
						lines = append(lines, printCommit(c, line, tagMap, branchMap, head.Hash().String() == c.Hash.String()))
					} else {
						/// TODO: can we not hardcode this?
						lines = append(lines, fmt.Sprintf("%s%s", strings.Repeat(" ", 18+config.hashLen), line))
					}
				}
				return nil
			})
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

func colorize(text, color string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(text)
}
func printCommit(c *object.Commit, graphLine string, tagMap, branchMap map[string][]string, isHead bool) string {
	line := ""
	hash := c.Hash.String()
	timestamp := c.Author.When.Format("2006-01-02 15:04") /// literally what is this
	author := c.Author.Name
	summary := strings.Split(c.Message, "\n")[0]
	tags, tagOk := tagMap[hash]
	branches, branchOk := branchMap[hash]

	line = fmt.Sprintf("%s %s %s %s",
		colorize(hash[:config.hashLen], "5"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Render(timestamp),
		graphLine,
		lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(author))
	if isHead || tagOk || branchOk {
		line += colorize(" (", "5")
		if isHead {
			line += colorize("HEAD %", "6")
			if tagOk || branchOk {
				line += " "
			}
		}
		refLine := append(append(make([]string, 0, 2), tags[:]...), branches[:]...)
		line += fmt.Sprintf("%s", strings.Join(refLine, ", "))
		line += colorize(")", "5")
	}

	line += fmt.Sprintf(" %s", summary)
	return line
}
