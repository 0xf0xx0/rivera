package postprocess

import (
	"math"
	"slices"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// / heres where we get git-foresta-like output
// / i cant read perl
// / so glad we've moved on to sane languages like
// / *checks notes* ...js...
func IterToArray(iter object.CommitIter) []*object.Commit {
	commits := make([]*object.Commit, 0)
	for {
		next, err := iter.Next()
		if err != nil {
			break
		}
		commits = append(commits, next)
	}
	return commits
}
func GetCommitBlock(commits *[]*object.Commit, max int) []*object.Commit {
	res := make([]*object.Commit, 0, max)
	for i := 0; i < int(math.Min(float64(max), float64(len(*commits)))); i++ {
		commit := (*commits)[i]
		if i == 0 {
			// shift
			*commits = (*commits)[1:]
		}
		res = append(res, commit)
	}
	return res
}
func VineBranch(vine *[]string, rev string) string {
	ret := ""
	matched := 0
	master := 0
	for idx, commit := range *vine {
		if commit == "" {
			ret += " "
		} else if commit != rev {
			ret += "I"
		} else {
			if master == 0 && idx%2 == 0 {
				ret += "S"
				master = 1
			} else {
				ret += "s"
				(*vine)[idx] = ""
			}
			matched++
		}
	}

	if matched < 2 {
		return ""
	}
	//fmt.Printf("%-*s %-*s%*s", hashWidth, "", dateWidth, "", graphMarginLeft, "")
	removeTrailingBlanks(*vine)
	return ret //fmt.Sprintln(VisPost(VisFan(ret, "branch")))
}
func VineCommit(vine *[]string, rev string, parents []string) string {
	ret := ""

	for i := range *vine {
		if (*vine)[i] == "" {
			ret += " "
		} else if (*vine)[i] == rev {
			ret += "C"
		} else {
			ret += "I"
		}
	}

	if !strings.Contains(ret, "C") {
		i := 0
		vineLen := len(*vine)
		for i = roundDown2(vineLen - 1); i >= 0; i -= 2 {
			if ret[i] == ' ' {
				ret = ret[:i] + "t" + ret[i+1:]
				(*vine)[i] = rev
				break
			}
		}
		if i < 0 {
			vineLen := len(*vine)
			if vineLen%2 != 0 {
				*vine = append(*vine, "")
				ret += " "
			}
			*vine = append(*vine, rev)
			ret += "t"
		}
	}

	removeTrailingBlanks(*vine)

	parentsLen := len(parents)
	if parentsLen == 0 {
		ret = strings.Replace(ret, "C", "r", 1)
	} else if parentsLen > 1 {
		ret = strings.Replace(ret, "C", "M", 1)
	}
	return ret
}
func VineMerge(vine *[]string, rev string, nextShas, parents *[]string) string {
	origVine := -1
	ret := ""
	slot := make([]int, 0)
	parentsLen := len(*parents)

	for i := range *vine {
		if (*vine)[i] == rev {
			origVine = i
			break
		}
	}

	if origVine == -1 {
		panic("VineCommit didnt add this vine")
	}

	if parentsLen <= 1 {
		if parentsLen == 1 {
			(*vine)[origVine] = (*parents)[0]
		}
		removeTrailingBlanks(*vine)
		return ""
	}
	vineLen := len(*vine)
	for i := 0; i < len(*parents) && len(*parents) > 0; i++ {
		for ii := 0; ii < vineLen; ii++ { /// why use ++i?
			z := (*vine)[ii]
			if z != (*parents)[i] || grep(z, *nextShas) == -1 {
				continue
			}

			if ii == origVine {
				panic("shouldnt happen")
			}

			if ii < origVine {
				p := ii + 1

				if (*vine)[p] != "" {
					p = ii - 1
				}
				if (*vine)[p] != "" {
					break
				}

				(*vine)[p] = (*parents)[i]
				strExpand(&ret, p+1)
				ret = ret[:p] + "s" + ret[p+1:]
			} else {
				p := ii - 1

				if (*vine)[p] != "" || p < 0 {
					p = ii + 1
				}
				if (*vine)[p] != "" {
					break
				}

				(*vine)[p] = (*parents)[i]
				strExpand(&ret, p+1)
				ret = ret[:p] + "s" + ret[p+1:]
			}
			*parents = append((*parents)[:i], (*parents)[i+1:]...)
			i--
			break
		}
	}

	parentsLen = len(*parents)
	slot = append(slot, origVine)
	parent := 0
	for seeker := 2; parent < parentsLen && seeker < 2+(vineLen-1); seeker++ {
		idx := 1
		if seeker%2 == 0 {
			idx = -1
		}
		idx *= seeker / 2
		idx *= 2
		idx += origVine

		if idx >= 0 && idx < vineLen && (*vine)[idx] == "" {
			slot = append(slot, idx)
			(*vine)[idx] = strings.Repeat("0", 40) /// git sha1 length
			parent++
		}
	}
	for idx := origVine + 2; parent < parentsLen; idx += 2 {
		if (*vine)[idx] == "" {
			slot = append(slot, idx)
			parent++
		}
	}
	slotLen := len(slot)

	if slotLen != parentsLen {
		panic("serious internal problem")
	}
	slices.Sort(slot)
	max := vineLen + 2*slotLen

	for i := 0; i < max; i++ {
		strExpand(&ret, i+1)
		if slotLen > 0 && i == slot[0] {
			slot = slot[1:]
			(*vine)[i] = (*parents)[0]
			*parents = (*parents)[1:]

			if i == origVine {
				ret = ret[:i] + "S" + ret[i+1:]
			} else {
				ret = ret[:i] + "s" + ret[i+1:]
			}
		} else if ret[i] == 's' {
		} else if (*vine)[i] != "" {
			ret = ret[:i] + "I" + ret[i+1:]
		} else {
			ret = ret[:i] + " " + ret[i+1:]
		}
	}

	//fmt.Sprint("%-*.*s %-*s%*s", HashWidth, HashWidth, "", DateWidth, "", GraphMarginLeft, "")
	return ret + "\n" //VisPost(VisFan(ret, "merge")) + "\n"
}

// func VisPost(input string) string
// / round but by 2s
func roundDown2(input int) int {
	if input < 0 {
		return input
	}
	return input & 254
}
func removeTrailingBlanks(vine []string) {
	for i := len(vine) - 1; i >= 0; i-- {
		if vine[i] != "" {
			break
		}
		vine = vine[:i]
	}
}
func grep(search string, input []string) int {
	for i, val := range input {
		if val == search {
			return i
		}
	}
	return -1
}
func strExpand(r *string, l int) {
	if len(*r) < l {
		*r += strings.Repeat(" ", l-len(*r))
	}
}
