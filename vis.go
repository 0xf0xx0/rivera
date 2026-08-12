// this is the pretty printing part :3
package main

import (
	"fmt"
	"strings"
)

// if isBranch is false, the fan is a merge
func visFan(line string, isBranch bool) string {
	/// build the overpass, if applicable
	line = fanRegex.ReplaceAllStringFunc(line, func(match string) string {
		return tr(match, " I", "DO")
	})
	if config.smoothOverpass {
		line = overpassRegex.ReplaceAllStringFunc(line, func(match string) string {
			return strings.Repeat("O", len(match))
		})
	}

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
	sb.WriteString(visFan2R(r))
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
// then, branch patterns are matched and the colors are updated and written before being turned into
// graph chars and returned
func visPost(line string) string {
	/// cleanup
	line = cleanLine(strings.TrimSpace(line))
	if config.reverse {
		line = tr(line, "efg.xyz.tr", "xyz.efg.rt")
	}

	/// initial branch colors
	colorHints := make([]string, len(line))
	for idx := range line {
		if idx%2 == 0 {
			colorHints[idx] = getBranchColor(idx / 2)
		}
	}

	/// color branches and overpasses based on source

	/// NOTE: edge case: padding that ends up between vines adopts the wrong color
	/// patterns: CII? IIC? II<spc>/<spc>II?
	if matches := leftcii.FindStringSubmatch(line); len(matches) > 0 {
		/// the middle I needs to have the same color as the last
		idx := strings.Index(line, matches[0]) + 1
		offset := idxToVinePos(idx)
		colorHints[idx] = getBranchColor(offset)
	}

	/// offset is the index into the vines array,
	/// idx is the actual printed index
	// test := testColorLogHelper(0, line, "")
	if config.reverse {
		if matches := leftxB.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			endIdx := idx + len(matches[1]) - 1

			/// colorHints needs clearing, the source branch color (left)
			/// needs to run all the way until it hits the target branch color (right)
			/// if the x is on an odd column it inherits the color automatically
			clearColorHintsUnderMatch(idx, matches[1], &colorHints)

			/// if source is directly off a main branch, set the overpass color as the branch color
			/// otherwise inherit the color from the previous column (as thats the source)
			if idx%2 == 0 {
				colorHints[idx] = getBranchColor(idxToVinePos(idx))
			} else {
				colorHints[idx] = getBranchColor(idxToVinePos(idx - 1))
			}

			/// the last column is the destination, set the color accordingly
			colorHints[endIdx] = getBranchColor(idxToVinePos(endIdx))
		} else if matches := righteB.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := idxToVinePos(idx)
			colorHints[idx] = getBranchColor(offset)
		} else if matches := righteg.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			endIdx := idx + len(matches[1]) - 1
			offset := idxToVinePos(endIdx)
			clearColorHintsUnderMatch(idx, matches[1], &colorHints)
			colorHints[idx] = getBranchColor(offset)
		} else if matches := rightzI.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := idxToVinePos(idx)
			/// TODO: none of my repos have a z...I, does this also need to clear colorHints?
			colorHints[idx] = getBranchColor(offset)
		}

		/// the overpasses needs to be done separately because the regexes above may overlap
		/// overpasses inherit only the base color, so we zero-out colorHints over the length of the match
		if matches := leftAg.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := idxToVinePos(idx)
			clearColorHintsUnderMatch(idx, matches[1], &colorHints)
			colorHints[idx] = getBranchColor(offset - 1)
		} else if matches := rightAz.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			endIdx := idx + len(matches[1]) - 1
			offset := idxToVinePos(idx)
			clearColorHintsUnderMatch(idx, matches[1], &colorHints)
			/// color the merge overpass (minus the ending "A"), but only if it connects to another vine
			if endIdx%2 == 0 {
				colorHints[idx+1] = getBranchColor(idxToVinePos(endIdx))
			}
			/// color the "A"
			colorHints[idx] = getBranchColor(offset)
		}
	} else {
		/// everything above, but in normal order
		if matches := leftxB.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := idxToVinePos(idx)
			colorHints[idx] = getBranchColor(offset)
		} else if matches := righteB.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := idxToVinePos(idx)
			clearColorHintsUnderMatch(idx, matches[1], &colorHints)

			if idx%2 == 0 {
				colorHints[idx] = getBranchColor(offset)
			} else {
				colorHints[idx] = getBranchColor(idxToVinePos(idx - 1))
			}

			// test = testColorLogHelper(idx, line, matches[1])
		} else if matches := rightzI.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := idxToVinePos(idx)
			colorHints[idx] = getBranchColor(offset)
		}

		if matches := leftAg.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			endIdx := idx + len(matches[1]) - 1
			offset := idxToVinePos(idx)

			clearColorHintsUnderMatch(idx, matches[1], &colorHints)

			/// set the `A` color
			colorHints[idx-1] = getBranchColor(offset)
			/// we take the index of `g` to get the correct color for the overpass
			colorHints[idx] = getBranchColor(idxToVinePos(endIdx))
		} else if matches := rightAz.FindStringSubmatch(line); len(matches) > 0 {
			idx := strings.Index(line, matches[1])
			offset := idxToVinePos(idx)

			clearColorHintsUnderMatch(idx, matches[1], &colorHints)

			colorHints[idx] = getBranchColor(offset)
		}
	}

	/// now replace with graph chars
	/// TODO: finish implementing, want single chars to be replacable on top of a theme
	if config.userStyle != "" {
		line = tr(line, styleReplace, config.userStyle)
	} else {
		switch config.style {
		/// no replace
		case 0:
			break
		/// thin
		case 1:
			{
				line = tr(line, styleReplace, "├┤──.┌┬┐.│┼─.└┴┘.┬├├┴")
			}
		/// thin with double bridge
		case 2:
			{
				line = tr(line, styleReplace, "╞╡═╪.╒╤╕.│┼─.╘╧╛.┬├├┴")
			}
		/// idk why the perl used 10 and 15, like ???
		/// double
		case 10, 3:
			{
				line = tr(line, styleReplace, "╠╣══.╔╦╗.║╬─.╚╩╝.╓║║╙")
			}
		/// curves
		case 15, 4:
			{
				line = tr(line, styleReplace, "├┤──.╭┬╮.│┼─.╰┴╯.┬├├┴")
			}
		/// thicc
		case 5:
			{
				line = tr(line, styleReplace, "┣┫━━.┏┳┓.┃╋━.┗┻┛.┳┣┣┻")
			}
		/// thicc with dashed bridge and commit squares
		case 6:
			{
				line = tr(line, styleReplace, "┣┫╍╍.┏┳┓.┃╋━.┗┻┛.┳□▣┻")
			}
		/// dashed with thicc bridge and commit
		case 7:
			{
				line = tr(line, styleReplace, "┣┫━━.┏┳┓.┋╋━.┗┻┛.╻□▣╿")
			}
		}
	}

	/// finally, actually color the string using the hints
	sb := strings.Builder{}
	sb.Grow(len(line))
	for idx, c := range []rune(line) {
		if colorHints[idx] != "" {
			sb.WriteString("{")
			sb.WriteString(colorHints[idx])
			sb.WriteString("}")
		}
		sb.WriteRune(c)
	}
	// sb.WriteString(test)
	return sb.String()
}

// for logging pls ignor
func testColorLogHelper(idx int, line string, match string) string {
	testPad := fmt.Sprintf("%s %s%s",
		strings.Repeat(" ", config.hashLen), strings.Repeat(" ", len(DATE_FMT)), strings.Repeat(" ", config.leftMargin),
	)
	theSlab := fmt.Sprintf("i: %d o: %d]", idx, idxToVinePos(idx))
	theSlab = fmt.Sprintf("\n{/}{blackbright}%s%s%s (%s)\n", theSlab, testPad[:len(testPad)-(len(theSlab))], line, match)
	/// you didnt ignore D: king ramses curse upon ye
	return theSlab
}
