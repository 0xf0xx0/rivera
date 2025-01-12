package graph

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5/plumbing/object"
)

/// port of gits graph.c
/// thanks lol
/// see it for detailed docs, this is just rough and dirty

const (
	GRAPH_PADDING = iota
	GRAPH_SKIP
	GRAPH_PRE_COMMIT
	GRAPH_COMMIT
	GRAPH_POST_MERGE
	GRAPH_COLLAPSING
)
const (
	GRAPH_PRINT_MULTIBRANCH_EXTENSION = "-"
	GRAPH_PRINT_MULTIBRANCH_START     = "."
	GRAPH_PRINT_BRIDGE                = "_"
	GRAPH_PRINT_PADDING               = "|"
	GRAPH_PRINT_COMMIT                = "*"
	GRAPH_PRINT_RMOVE                 = "\\"
	GRAPH_PRINT_LMOVE                 = "/"
)

var mergeChars = []string{GRAPH_PRINT_LMOVE, GRAPH_PRINT_PADDING, GRAPH_PRINT_RMOVE}

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
	G.state = GRAPH_PADDING
	G.prevState = GRAPH_PADDING
	G.defaultColorIndex = G.maxColorIndex - 1

	G.columnCapacity = 30
	G.columns = make([]*Column, G.columnCapacity)
	G.newColumns = make([]*Column, G.columnCapacity)
	G.mapping = make([]int, G.columnCapacity*2)
	G.oldMapping = make([]int, G.columnCapacity*2)

	return G
}
func (G *Graph) IsCommitFinished() bool {
	return G.state == GRAPH_PADDING
}
func (G *Graph) NextLine() (string, bool) {
	graphLine := "" //GraphLine{width: 0}
	commitLine := false
	switch G.state {
	case GRAPH_PADDING:
		G.outputPaddingLine(&graphLine)
	case GRAPH_SKIP:
		G.outputSkipLine(&graphLine)
	case GRAPH_PRE_COMMIT:
		G.outputPreCommitLine(&graphLine)
	case GRAPH_COMMIT:
		G.outputCommitLine(&graphLine)
		commitLine = true
	case GRAPH_POST_MERGE:
		G.outputPostMergeLine(&graphLine)
	case GRAPH_COLLAPSING:
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
	if G.state != GRAPH_PADDING {
		G.state = GRAPH_SKIP
	} else if G.needsPreCommitLine() {
		G.state = GRAPH_PRE_COMMIT
	} else {
		G.state = GRAPH_COMMIT
	}
}
func (G *Graph) SetColors(colors []string) {
	G.maxColorIndex = len(colors) - 1
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
				G.insertIntoNewColumns(parent, parentIdx)
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
	if G.width > 2 {
		println(G.edgesAdded, idx, i, G.mapping[G.width-2], i == G.mapping[G.width-2])
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

		G.lineWriteColumn(line, column, GRAPH_PRINT_MULTIBRANCH_EXTENSION)
		if i == dashedParents-1 {
			G.lineWriteColumn(line, column, GRAPH_PRINT_MULTIBRANCH_START)
		} else {
			G.lineWriteColumn(line, column, GRAPH_PRINT_MULTIBRANCH_EXTENSION)
		}
	}
}

// / and heres where we diverge
// / i want to hold the entire string in memory for formatting
// / bitcoin/bitcoin (all branches) is 30026666 bytes
// / git-foresta --all | wc
func (G *Graph) padHorizontally(line *string) {
	lineWidth := len(*line)
	if lineWidth < G.width {
		*line += strings.Repeat(" ", G.width-lineWidth)
	}
}
func (G *Graph) lineWriteColumn(line *string, column *Column, char string) {
	// if column.color < G.maxColorIndex {
	// 	*line += (string)(lipgloss.Color(G.colors[column.color]))
	// }
	// *line += char
	// if column.color < G.maxColorIndex {
	// 	*line += (string)(lipgloss.Color(G.colors[G.maxColorIndex-1]))
	// }
	*line += lipgloss.NewStyle().Foreground(lipgloss.Color(G.colors[column.color])).Render(char)
}
func (G *Graph) outputPaddingLine(line *string) *string {
	for i := 0; i < G.numNewColumns; i++ {
		G.lineWriteColumn(line, G.newColumns[i], GRAPH_PRINT_PADDING)
		*line += " "
	}
	return line
}
func (G *Graph) outputSkipLine(line *string) *string {
	*line = "..."
	if G.needsPreCommitLine() {
		G.updateState(GRAPH_PRE_COMMIT)
	} else {
		G.updateState(GRAPH_COMMIT)
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
			G.lineWriteColumn(line, column, GRAPH_PRINT_PADDING)
			*line += strings.Repeat(" ", G.expansionRow)
		} else if seenThis && G.expansionRow == 0 {
			if G.prevState == GRAPH_POST_MERGE && G.prevCommitIndex < i {
				G.lineWriteColumn(line, column, GRAPH_PRINT_RMOVE)
			} else {
				G.lineWriteColumn(line, column, GRAPH_PRINT_PADDING)
			}
		} else if seenThis && G.expansionRow > 0 {
			G.lineWriteColumn(line, column, GRAPH_PRINT_RMOVE)
		} else {
			G.lineWriteColumn(line, column, GRAPH_PRINT_PADDING)
		}
		*line += " "
	}
	G.expansionRow++
	if !G.needsPreCommitLine() {
		G.updateState(GRAPH_COMMIT)
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
			*line += GRAPH_PRINT_COMMIT
			if G.numParents > 2 {
				G.drawOctopusMerge(line)
			}
		} else if seenThis && G.edgesAdded > 1 {
			G.lineWriteColumn(line, column, GRAPH_PRINT_PADDING)
		} else if seenThis && G.edgesAdded == 1 {
			if G.prevState == GRAPH_POST_MERGE && G.prevEdgesAdded > 0 && G.prevCommitIndex < i {
				G.lineWriteColumn(line, column, GRAPH_PRINT_RMOVE)
			} else {
				G.lineWriteColumn(line, column, GRAPH_PRINT_PADDING)
			}
		} else if G.prevState == GRAPH_COLLAPSING && G.oldMapping[2*i+1] == i && G.mapping[2*i] < i {
			G.lineWriteColumn(line, column, GRAPH_PRINT_LMOVE)
		} else {
			G.lineWriteColumn(line, column, GRAPH_PRINT_PADDING)
		}
		*line += " "
	}
	if G.numParents > 1 {
		G.updateState(GRAPH_POST_MERGE)
		*line += "p"
	} else if G.isMappingCorrect() {
		G.updateState(GRAPH_PADDING)
	} else {
		G.updateState(GRAPH_COLLAPSING)
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
				//println(idx, parentColumnIdx)
				mergeChar = mergeChars[idx]
				//*line += "e"
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
				G.lineWriteColumn(line, column, GRAPH_PRINT_RMOVE)
			} else {
				G.lineWriteColumn(line, column, GRAPH_PRINT_PADDING)
			}
			*line += " "
		} else {
			G.lineWriteColumn(line, column, GRAPH_PRINT_PADDING)
			if G.mergeLayout != 0 || i != G.commitIndex-1 {
				if parentColumn != nil {
					G.lineWriteColumn(line, parentColumn, GRAPH_PRINT_BRIDGE)
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
		G.updateState(GRAPH_PADDING)
	} else {
		G.updateState(GRAPH_COLLAPSING)
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
			G.lineWriteColumn(line, G.newColumns[target], GRAPH_PRINT_PADDING)
		} else if target == horizontalEdgeTarget && i != horizontalEdge-1 {
			if i != (target*2)+3 {
				G.mapping[i] = -1
			}
			usedHorizontal = true
			G.lineWriteColumn(line, G.newColumns[target], GRAPH_PRINT_BRIDGE)
		} else {
			if usedHorizontal && i < horizontalEdge {
				G.mapping[i] = -1
			}
			G.lineWriteColumn(line, G.newColumns[target], GRAPH_PRINT_LMOVE)
		}
	}
	if G.isMappingCorrect() {
		G.updateState(GRAPH_PADDING)
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
