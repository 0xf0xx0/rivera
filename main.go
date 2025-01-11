package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"rivera/graph"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/urfave/cli/v2"
)

const (
	graphSymbol_commit   = "c"
	graphSymbol_merge    = "m"
	graphSymbol_overpass = "o"
	graphSymbol_root     = "r"
	graphSymbol_tip      = "t"
)

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
		},
		Action: func(ctx *cli.Context) error {
			repoPath := ctx.String("repository")
			displayAll := ctx.Bool("all")
			reverse := ctx.Bool("reverse")
			repo, err := git.PlainOpen(repoPath)
			if err != nil {
				return err
			}
			head, err := repo.Head()
			if err != nil {
				return err
			}
			cIter, err := repo.Log(&git.LogOptions{From: head.Hash(), Order: git.LogOrderCommitterTime, All: displayAll})
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
				branchMap[hash] = append(branchMap[hash], name)
				return nil
			})

			/// now, we build the river
			g := graph.New()
			g.SetColors([]string{
				"0",
				"#ff00ff",
				"#f0000f",
				"#b00b69",
				"#262638",
			})
			//commits := make([]*object.Commit, 0, 64)
			lines := make([]string, 0, 64)
			cIter.ForEach(func(c *object.Commit) error {
				//commits = append(commits, c)
				g.Update(c)
				for {
					if g.IsCommitFinished() {
						break
					}
					line, isCommit := g.NextLine()
					if reverse {
						line = strings.ReplaceAll(line, "\\", "t")
						line = strings.ReplaceAll(line, "/", "\\")
						line = strings.ReplaceAll(line, "t", "/")
					}
					if isCommit {
						lines = append(lines, printCommit(c, line, tagMap, branchMap))
					} else {
						lines = append(lines, fmt.Sprintf("%s%s", strings.Repeat(" ", 26), line))
					}
				}
				return nil
			})
			jInit := len(lines) - 1
			if reverse {
				for i, j := 0, jInit; i < j; i, j = i+1, j-1 {
					lines[i], lines[j] = lines[j], lines[i]
				}
			}
			for _, line := range lines {
				fmt.Println(line)
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

func printCommit(c *object.Commit, graphLine string, tagMap map[string][]string, branchMap map[string][]string) string {
	line := ""
	hash := c.Hash.String()
	timestamp := c.Author.When.Format("2006-01-02 15:04") /// literally what is this
	author := c.Author.Name
	summary := strings.Split(c.Message, "\n")[0]
	tags, tagOk := tagMap[hash]
	branches, branchOk := branchMap[hash]

	line = fmt.Sprintf("%s %s %s %s", hash[:8], timestamp, graphLine, author)
	if tagOk || branchOk {
		refLine := append(append(make([]string, 0, 2), tags[:]...), branches[:]...)
		line += fmt.Sprintf(" (%s)", strings.Join(refLine, ", "))
	}

	line += fmt.Sprintf(" %s", summary)
	return line
}
