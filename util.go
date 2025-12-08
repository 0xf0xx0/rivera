package main

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"
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
	furstLine := global_commitBuffer[0] /// steal the furst commit to mark it visited
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

func fileExists(path string) bool {
	_, err := fs.Stat(os.DirFS(config.repoPath), path)
	return err == nil
}

// useful func generated while throwing perl at gpt-oss
func tr(s string, from, to string) string {
	var b strings.Builder
	for _, r := range s {
		idx := strings.IndexRune(from, r)
		if idx >= 0 {
			b.WriteRune([]rune(to)[idx])
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
