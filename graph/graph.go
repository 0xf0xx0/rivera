package graph

import (
	"rivera/shared"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
)

/// port of gits graph.c
/// thanks lol
/// see it for detailed docs, this is just rough and dirty

const (
	STATE_PADDING = iota
	STATE_SKIP
	STATE_PRE_COMMIT
	STATE_COMMIT
	STATE_POST_MERGE
	STATE_COLLAPSING
)
const (
	PRINT_MULTIBRANCH_EXTENSION = "-"
	PRINT_MULTIBRANCH_START     = "."
	PRINT_BRIDGE                = "_"
	PRINT_PADDING               = "|"
	PRINT_COMMIT                = "*"
	PRINT_RMOVE                 = "\\"
	PRINT_LMOVE                 = "/"
	PRINT_TIP                   = "T"
	PRINT_ROOT                  = "R"
)

var mergeChars = []string{PRINT_LMOVE, PRINT_PADDING, PRINT_RMOVE}

type Graph struct {
	commit           *object.Commit
	state, prevState GraphState
	/// git ignores "uninteresting" parents but i dont think thats important here
	/// never seen an empty commit
	numParents, edgesAdded, prevEdgesAdded            int
	width, expansionRow, commitIndex, prevCommitIndex int
	columns, newColumns                               []*Column
	mapping, oldMapping                               []int
	defaultColorIndex, maxColorIndex                  int
	colors                                            []string
	columnCapacity                                    int
	numColumns                                        int
	numNewColumns                                     int
	mappingSize, mergeLayout                          int
}
type GraphState int

func New() *Graph {
	G := &Graph{}
	G.commit = nil
	G.state = STATE_PADDING
	G.prevState = STATE_PADDING
	G.defaultColorIndex = G.maxColorIndex

	G.columnCapacity = 30
	G.columns = make([]*Column, G.columnCapacity)
	G.newColumns = make([]*Column, G.columnCapacity)
	G.mapping = make([]int, G.columnCapacity*2)
	G.oldMapping = make([]int, G.columnCapacity*2)

	return G
}
func (G *Graph) IsCommitFinished() bool {
	return G.state == STATE_PADDING
}
func (G *Graph) NextLine() (string, bool) {
	graphLine := "" //GraphLine{width: 0}
	commitLine := false
	switch G.state {
	case STATE_PADDING:
		G.outputPaddingLine(&graphLine)
	case STATE_SKIP:
		G.outputSkipLine(&graphLine)
	case STATE_PRE_COMMIT:
		G.outputPreCommitLine(&graphLine)
	case STATE_COMMIT:
		G.outputCommitLine(&graphLine)
		commitLine = true
	case STATE_POST_MERGE:
		G.outputPostMergeLine(&graphLine)
	case STATE_COLLAPSING:
		G.outputCollapsingLine(&graphLine)
	}
	G.padHorizontally(&graphLine)
	return graphLine, commitLine
}
func (G *Graph) Update(commit *object.Commit) {
	G.commit = commit
	/// maybe: implement interest
	G.numParents = len(commit.ParentHashes)
	G.prevCommitIndex = G.commitIndex

	G.updateColumns()

	G.expansionRow = 0
	if G.state != STATE_PADDING {
		G.state = STATE_SKIP
	} else if G.needsPreCommitLine() {
		G.state = STATE_PRE_COMMIT
	} else {
		G.state = STATE_COMMIT
	}
}

// / comma separated colors
func (G *Graph) SetColors(colorstring string) {
	colors := strings.Split(colorstring, ",")
	for _, color := range colors {
		color = strings.Trim(color, " ")
	}
	G.maxColorIndex = len(colors)
	if G.maxColorIndex < 2 {
		panic("too few colors, need 2 minimum")
	}
	G.colors = colors
}
func (G *Graph) updateColumns() {
	isCommitInColumns := false
	/// SWAP()
	tempCols := G.columns
	G.columns = G.newColumns
	G.newColumns = tempCols

	G.numColumns = G.numNewColumns
	G.numNewColumns = 0

	maxNewColumns := G.numColumns + G.numParents
	G.ensureCapacity(maxNewColumns)
	G.mappingSize = maxNewColumns * 2
	for i := 0; i < G.mappingSize; i++ {
		G.mapping[i] = -1
	}
	G.width = 0
	G.prevEdgesAdded = G.edgesAdded
	G.edgesAdded = 0
	seenThis := false
	isCommitInColumns = true
	for i := 0; i <= G.numColumns; i++ {
		var colCommit *object.Commit
		if i == G.numColumns {
			if seenThis {
				break
			}
			isCommitInColumns = false
			colCommit = G.commit
		} else {
			colCommit = G.columns[i].commit
		}

		if colCommit.Hash.String() == G.commit.Hash.String() {
			seenThis = true
			G.commitIndex = i
			G.mergeLayout = -1
			/// maybe: implement interest

			for parentIdx := range G.commit.ParentHashes {
				parent, _ := G.commit.Parent(parentIdx)
				if G.numParents > 1 || !isCommitInColumns {
					G.incrementColumnColor()
				}
				G.insertIntoNewColumns(parent, i)
			}
			if G.numParents == 0 {
				G.width += 2
			}
		} else {
			G.insertIntoNewColumns(colCommit, -1)
		}
	}
	for {
		if G.mappingSize > 1 && G.mapping[G.mappingSize-1] < 0 {
			G.mappingSize--
		}
		break
	}
}

// / these are static in git, do they need to be here?
func (G *Graph) numDashedParents() int {
	return G.numParents + G.mergeLayout - 3
}
func (G *Graph) numExpansionRows() int {
	return G.numDashedParents() * 2
}
func (G *Graph) needsPreCommitLine() bool {
	return G.numParents >= 3 && G.commitIndex < (G.numColumns-1) && G.expansionRow < G.numExpansionRows()
}
func (G *Graph) ensureCapacity(numColumns int) {
	if G.columnCapacity >= numColumns {
		return
	}
	for {
		G.columnCapacity *= 2
		if G.columnCapacity > numColumns {
			break
		}
	}

	// reallocate
	tempCols := G.columns
	G.columns = make([]*Column, G.columnCapacity)
	copy(G.columns, tempCols)

	tempCols = G.newColumns
	G.newColumns = make([]*Column, G.columnCapacity)
	copy(G.newColumns, tempCols)

	tempMap := G.mapping
	G.mapping = make([]int, G.columnCapacity*2)
	copy(G.mapping, tempMap)

	tempMap = G.oldMapping
	G.oldMapping = make([]int, G.columnCapacity*2)
	copy(G.oldMapping, tempMap)
}
func (G *Graph) incrementColumnColor() {
	G.defaultColorIndex = (G.defaultColorIndex + 1) % G.maxColorIndex
}
func (G *Graph) isMappingCorrect() bool {
	for i := 0; i < G.mappingSize; i++ {
		target := G.mapping[i]
		if target < 0 {
			continue
		}
		if target == i/2 {
			continue
		}
		return false
	}
	return true
}
func (G *Graph) insertIntoNewColumns(commit *object.Commit, idx int) {
	i := G.findNewColumnByCommit(commit)
	var mappingIndex int
	if i < 0 {
		i = G.numNewColumns
		G.numNewColumns++
		G.newColumns[i] = &Column{
			commit: commit,
			color:  G.findCommitColor(commit),
		}
	}
	if G.numParents > 1 && idx > -1 && G.mergeLayout == -1 {
		dist := idx - i
		shift := 1
		if dist > 1 {
			shift = 2*dist - 3
		}
		G.mergeLayout = 1
		if dist > 0 {
			G.mergeLayout = 0
		}
		G.edgesAdded = G.numParents + G.mergeLayout - 2
		mappingIndex = G.width + (G.mergeLayout-1)*shift
		G.width += 2 * G.mergeLayout
	} else if G.edgesAdded > 0 && i == G.mapping[G.width-2] {
		mappingIndex = G.width - 2
		G.edgesAdded = -1
	} else {
		mappingIndex = G.width
		G.width += 2
	}
	G.mapping[mappingIndex] = i
}
func (G *Graph) findNewColumnByCommit(commit *object.Commit) int {
	for i := 0; i < G.numNewColumns; i++ {
		if G.newColumns[i].commit.Hash.String() == commit.Hash.String() {
			return i
		}
	}
	return -1
}
func (G *Graph) findCommitColor(commit *object.Commit) int {
	for i := 0; i < G.numColumns; i++ {
		if G.columns[i].commit.Hash.String() == commit.Hash.String() {
			return G.columns[i].color
		}
	}
	return G.getCurrentColumnColor()
}
func (G *Graph) getCurrentColumnColor() int {
	return G.defaultColorIndex
}
func (G *Graph) updateState(newState GraphState) {
	G.prevState = G.state
	G.state = newState
}

func (G *Graph) drawOctopusMerge(line *string) {
	dashedParents := G.numDashedParents()

	for i := 0; i < dashedParents; i++ {
		j := G.mapping[(G.commitIndex+i+2)*2]
		column := G.newColumns[j]

		G.lineWriteColumn(line, column, PRINT_MULTIBRANCH_EXTENSION)
		if i == dashedParents-1 {
			G.lineWriteColumn(line, column, PRINT_MULTIBRANCH_START)
		} else {
			G.lineWriteColumn(line, column, PRINT_MULTIBRANCH_EXTENSION)
		}
	}
}

func (G *Graph) padHorizontally(line *string) {
	lineWidth := len(*line)
	if lineWidth < G.width {
		*line += strings.Repeat(" ", G.width-lineWidth)
	}
}
func (G *Graph) lineWriteColumn(line *string, column *Column, char string) {
	*line += shared.Colorize(char, G.colors[column.color])
}
func (G *Graph) outputPaddingLine(line *string) *string {
	for i := 0; i < G.numNewColumns; i++ {
		G.lineWriteColumn(line, G.newColumns[i], PRINT_PADDING)
		*line += " "
	}
	return line
}
func (G *Graph) outputSkipLine(line *string) *string {
	*line = "..."
	if G.needsPreCommitLine() {
		G.updateState(STATE_PRE_COMMIT)
	} else {
		G.updateState(STATE_COMMIT)
	}
	return line
}
func (G *Graph) outputPreCommitLine(line *string) *string {
	/// gotta flip em around from c
	if G.numParents < 3 {
		panic("dude, really? [G.numParents < 3]")
	}
	if 0 > G.expansionRow || G.expansionRow >= G.numExpansionRows() {
		panic("[0 > G.expansionRow || G.expansionRow >= G.numExpansionRows()]")
	}
	seenThis := false
	for i := 0; i < G.numColumns; i++ {
		column := G.columns[i]
		if column.commit.Hash.String() == G.commit.Hash.String() {
			seenThis = true
			G.lineWriteColumn(line, column, PRINT_PADDING)
			*line += strings.Repeat(" ", G.expansionRow)
		} else if seenThis && G.expansionRow == 0 {
			if G.prevState == STATE_POST_MERGE && G.prevCommitIndex < i {
				G.lineWriteColumn(line, column, PRINT_RMOVE)
			} else {
				G.lineWriteColumn(line, column, PRINT_PADDING)
			}
		} else if seenThis && G.expansionRow > 0 {
			G.lineWriteColumn(line, column, PRINT_RMOVE)
		} else {
			G.lineWriteColumn(line, column, PRINT_PADDING)
		}
		*line += " "
	}
	G.expansionRow++
	if !G.needsPreCommitLine() {
		G.updateState(STATE_COMMIT)
	}
	return line
}
func (G *Graph) outputCommitLine(line *string) *string {
	seenThis := false
	for i := 0; i <= G.numColumns; i++ {
		column := G.columns[i]
		var commit *object.Commit
		if i == G.numColumns {
			if seenThis {
				break
			}
			commit = G.commit
		} else {
			commit = column.commit
		}

		if commit.Hash.String() == G.commit.Hash.String() {
			seenThis = true
			/// deviation: marking the root commit
			if G.numParents == 0 {
				G.lineWriteColumn(line, column, PRINT_ROOT)
			} else {
				/// anoter deviation: mark branch tips
				if column == nil {
					G.lineWriteColumn(line, &Column{
						color: G.getCurrentColumnColor(), /// big brain
					}, PRINT_TIP)
				} else {
					G.lineWriteColumn(line, column, PRINT_COMMIT)
				}
			}
			if G.numParents > 2 {
				G.drawOctopusMerge(line)
			}
		} else if seenThis && G.edgesAdded > 1 {
			G.lineWriteColumn(line, column, PRINT_PADDING)
		} else if seenThis && G.edgesAdded == 1 {
			if G.prevState == STATE_POST_MERGE && G.prevEdgesAdded > 0 && G.prevCommitIndex < i {
				G.lineWriteColumn(line, column, PRINT_RMOVE)
			} else {
				G.lineWriteColumn(line, column, PRINT_PADDING)
			}
		} else if G.prevState == STATE_COLLAPSING && G.oldMapping[2*i+1] == i && G.mapping[2*i] < i {
			G.lineWriteColumn(line, column, PRINT_LMOVE)
		} else {
			G.lineWriteColumn(line, column, PRINT_PADDING)
		}
		*line += " "
	}
	if G.numParents > 1 {
		G.updateState(STATE_POST_MERGE)
	} else if G.isMappingCorrect() {
		G.updateState(STATE_PADDING)
	} else {
		G.updateState(STATE_COLLAPSING)
	}
	return line
}
func (G *Graph) outputPostMergeLine(line *string) *string {
	seenThis := false
	firstParent, _ := G.commit.Parent(0)
	var parentColumn *Column

	/// FIXME: .edgesAdded is 1 here, when it should be 0? maybe?
	for i := 0; i <= G.numColumns; i++ {
		column := G.columns[i]
		var colCommit *object.Commit
		if i == G.numColumns {
			if seenThis {
				break
			}
			colCommit = G.commit
		} else {
			colCommit = column.commit
		}
		if colCommit.Hash.String() == G.commit.Hash.String() {
			parents := firstParent
			parentColumnIdx := -1
			idx := G.mergeLayout
			mergeChar := ""
			seenThis = true

			for ii := 0; ii < G.numParents; ii++ {
				parentColumnIdx = G.findNewColumnByCommit(parents)
				if parentColumnIdx < 0 {
					panic("[parentColumn < 0]")
				}
				mergeChar = mergeChars[idx]
				G.lineWriteColumn(line, G.newColumns[parentColumnIdx], mergeChar)
				if idx == 2 {
					if G.edgesAdded > 0 || ii < G.numParents-1 {
						*line += " "
					}
				} else {
					idx++
				}
				parents, _ = colCommit.Parent(ii + 1) //Parents().Next()
			}
			if G.edgesAdded == 0 {
				*line += " "
			}
		} else if seenThis {
			if G.edgesAdded > 0 {
				G.lineWriteColumn(line, column, PRINT_RMOVE)
			} else {
				G.lineWriteColumn(line, column, PRINT_PADDING)
			}
			*line += " "
		} else {
			G.lineWriteColumn(line, column, PRINT_PADDING)
			if G.mergeLayout != 0 || i != G.commitIndex-1 {
				if parentColumn != nil {
					G.lineWriteColumn(line, parentColumn, PRINT_BRIDGE)
				} else {
					*line += " "
				}
			}
		}
		if colCommit.Hash.String() == firstParent.Hash.String() {
			parentColumn = column
		}
	}
	if G.isMappingCorrect() {
		G.updateState(STATE_PADDING)
	} else {
		G.updateState(STATE_COLLAPSING)
	}
	return line
}
func (G *Graph) outputCollapsingLine(line *string) *string {
	usedHorizontal := false
	horizontalEdge := -1
	horizontalEdgeTarget := -1

	temp := G.oldMapping
	G.oldMapping = G.mapping
	G.mapping = temp

	for i := 0; i < G.mappingSize; i++ {
		G.mapping[i] = -1
	}
	for i := 0; i < G.mappingSize; i++ {
		target := G.oldMapping[i]
		if target < 0 {
			continue
		}

		if target*2 > i {
			panic("[target*2 > i]")
		}
		if target*2 == i {
			if G.mapping[i] != -1 {
				panic("[G.mapping[i] != -1]")
			}
			G.mapping[i] = target
		} else if G.mapping[i-1] < 0 {
			G.mapping[i-1] = target
			if horizontalEdge == -1 {
				horizontalEdge = i
				horizontalEdgeTarget = target
				for ii := (target * 2) + 3; ii < i-2; ii += 2 {
					G.mapping[ii] = target
				}
			}
		} else if G.mapping[i-1] == target {
		} else {
			if G.mapping[i-1] < target {
				panic("[G.mapping[i-1] < target]")
			}
			if G.mapping[i-2] >= 0 {
				panic("[G.mapping[i-2] >= 0]")
			}
			G.mapping[i-2] = target
			if horizontalEdge == -1 {
				horizontalEdgeTarget = target
				horizontalEdge = i - 1

				for ii := (target * 2) + 3; ii < (i - 2); ii += 2 {
					G.mapping[ii] = target
				}
			}
		}
	}
	copy(G.oldMapping, G.mapping)
	if G.mapping[G.mappingSize-1] < 0 {
		G.mappingSize--
	}
	for i := 0; i < G.mappingSize; i++ {
		target := G.mapping[i]
		if target < 0 {
			*line += " "
		} else if target*2 == i {
			G.lineWriteColumn(line, G.newColumns[target], PRINT_PADDING)
		} else if target == horizontalEdgeTarget && i != horizontalEdge-1 {
			if i != (target*2)+3 {
				G.mapping[i] = -1
			}
			usedHorizontal = true
			G.lineWriteColumn(line, G.newColumns[target], PRINT_BRIDGE)
		} else {
			if usedHorizontal && i < horizontalEdge {
				G.mapping[i] = -1
			}
			G.lineWriteColumn(line, G.newColumns[target], PRINT_LMOVE)
		}
	}
	if G.isMappingCorrect() {
		G.updateState(STATE_PADDING)
	}
	return line
}

type Column struct {
	/// parent of the column
	commit *object.Commit
	/// color index
	color int
}
type CommitList struct {
	commit     *object.Commit
	commitList *CommitList
}
