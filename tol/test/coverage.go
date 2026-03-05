package toltest

import (
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/parser"
	"golang.org/x/crypto/sha3"
)

// CoverageReport is the accumulated coverage for a test run, spanning one or
// more contract files.
type CoverageReport struct {
	Files []*FileCoverage
}

// FileCoverage tracks coverage data for a single contract file.
type FileCoverage struct {
	ContractFile string
	Functions    []*FuncCoverage
	Slots        []SlotCoverage
	Lines        []LineCoverage   // populated when sourcemap available
	Branches     []BranchCoverage // populated when sourcemap available
}

// LineCoverage holds per-line hit data for a contract file.
type LineCoverage struct {
	Line int
	Hit  bool
}

// BranchCoverage holds coverage for one branch of a decision point.
type BranchCoverage struct {
	Line     int // line of the if/while/for statement
	BranchID int // 0 = then/body, 1 = else
	Hit      bool
}

// FuncCoverage holds per-function coverage and complexity data.
type FuncCoverage struct {
	Name   string
	Called bool
	CC     int // cyclomatic complexity (1 = no branches)
}

// SlotCoverage tracks which storage slots were read or written during tests.
type SlotCoverage struct {
	Name       string // slot name (from contract source)
	StorageKey string // hex key (keccak256 of "tol.slot.<Contract>.<name>")
	Read       bool
	Written    bool
}

// FunctionPercent returns the percentage of functions called (0–100).
// Returns 100 when there are no functions.
func (r *CoverageReport) FunctionPercent() float64 {
	total, called := 0, 0
	for _, fc := range r.Files {
		for _, fn := range fc.Functions {
			total++
			if fn.Called {
				called++
			}
		}
	}
	if total == 0 {
		return 100
	}
	return float64(called) / float64(total) * 100
}

// TotalFunctionPct returns the overall function coverage percentage (0–100) as int.
func (r *CoverageReport) TotalFunctionPct() int {
	return int(r.FunctionPercent())
}

// TotalLinePct returns line coverage percentage (0–100) across all files.
// Returns 100 when no line data is available.
func (r *CoverageReport) TotalLinePct() int {
	total, hit := 0, 0
	for _, fc := range r.Files {
		for _, lc := range fc.Lines {
			total++
			if lc.Hit {
				hit++
			}
		}
	}
	if total == 0 {
		return 100
	}
	return int(float64(hit) / float64(total) * 100)
}

// TotalBranchPct returns branch coverage percentage (0–100) across all files.
// Returns 100 when no branch data is available.
func (r *CoverageReport) TotalBranchPct() int {
	total, hit := 0, 0
	for _, fc := range r.Files {
		for _, bc := range fc.Branches {
			total++
			if bc.Hit {
				hit++
			}
		}
	}
	if total == 0 {
		return 100
	}
	return int(float64(hit) / float64(total) * 100)
}

// TotalSlotPct returns the overall storage slot coverage percentage (0–100) as int.
// A slot is considered covered if it was either read or written.
func (r *CoverageReport) TotalSlotPct() int {
	total, covered := 0, 0
	for _, fc := range r.Files {
		for _, sc := range fc.Slots {
			total++
			if sc.Read || sc.Written {
				covered++
			}
		}
	}
	if total == 0 {
		return 100
	}
	return int(float64(covered) / float64(total) * 100)
}

// PrintText writes a human-readable coverage summary to w.
func (r *CoverageReport) PrintText(w io.Writer) {
	for _, fc := range r.Files {
		fmt.Fprintf(w, "%s  function coverage:\n", filepath.Base(fc.ContractFile))
		total, called := 0, 0
		for _, fn := range fc.Functions {
			total++
			sym, status := "✗", "NOT called"
			if fn.Called {
				called++
				sym, status = "✓", "called   "
			}
			fmt.Fprintf(w, "  %-40s %s %s  CC=%d\n", fn.Name, status, sym, fn.CC)
		}
		pct := 0.0
		if total > 0 {
			pct = float64(called) / float64(total) * 100
		}
		fmt.Fprintf(w, "Function coverage: %d/%d (%.0f%%)\n", called, total, pct)

		if len(fc.Slots) > 0 {
			fmt.Fprintf(w, "\n%s  storage slot coverage:\n", filepath.Base(fc.ContractFile))
			totalSlots, coveredSlots := 0, 0
			for _, sc := range fc.Slots {
				totalSlots++
				r := "✗"
				w2 := "✗"
				if sc.Read {
					r = "✓"
					coveredSlots++
				}
				if sc.Written {
					w2 = "✓"
					if !sc.Read {
						coveredSlots++
					}
				}
				fmt.Fprintf(w, "  %-30s read %s  written %s\n", sc.Name, r, w2)
			}
			slotPct := 0.0
			if totalSlots > 0 {
				slotPct = float64(coveredSlots) / float64(totalSlots) * 100
			}
			fmt.Fprintf(w, "Slot coverage: %d/%d (%.0f%%)\n", coveredSlots, totalSlots, slotPct)
		}
		if len(fc.Lines) > 0 {
			lineTot, lineHit := 0, 0
			for _, lc := range fc.Lines {
				lineTot++
				if lc.Hit {
					lineHit++
				}
			}
			linePct := 0.0
			if lineTot > 0 {
				linePct = float64(lineHit) / float64(lineTot) * 100
			}
			fmt.Fprintf(w, "Line coverage: %d/%d (%.0f%%)\n", lineHit, lineTot, linePct)
		}
		if len(fc.Branches) > 0 {
			brTot, brHit := 0, 0
			for _, bc := range fc.Branches {
				brTot++
				if bc.Hit {
					brHit++
				}
			}
			brPct := 0.0
			if brTot > 0 {
				brPct = float64(brHit) / float64(brTot) * 100
			}
			fmt.Fprintf(w, "Branch coverage: %d/%d (%.0f%%)\n", brHit, brTot, brPct)
		}
		fmt.Fprintln(w)
	}
}

// PrintTextSummary writes a compact per-file summary table to w.
func (r *CoverageReport) PrintTextSummary(w io.Writer) {
	hasLines := r.TotalLinePct() != 100 || r.anyHasLines()
	fmt.Fprintf(w, "Coverage summary:\n")
	if hasLines {
		fmt.Fprintf(w, "  %-30s  %-12s  %-12s  %-10s  %-10s  %s\n", "File", "Functions", "Slots", "Lines", "Branches", "CC Avg")
		fmt.Fprintf(w, "  %s\n", strings.Repeat("-", 85))
	} else {
		fmt.Fprintf(w, "  %-30s  %-12s  %-12s  %s\n", "File", "Functions", "Slots", "CC Avg")
		fmt.Fprintf(w, "  %s\n", strings.Repeat("-", 65))
	}
	for _, fc := range r.Files {
		fnTotal, fnCalled := 0, 0
		ccSum := 0
		for _, fn := range fc.Functions {
			fnTotal++
			if fn.Called {
				fnCalled++
			}
			ccSum += fn.CC
		}
		fnPct := 100.0
		if fnTotal > 0 {
			fnPct = float64(fnCalled) / float64(fnTotal) * 100
		}
		ccAvg := 0.0
		if fnTotal > 0 {
			ccAvg = float64(ccSum) / float64(fnTotal)
		}

		slotTotal, slotCovered := 0, 0
		for _, sc := range fc.Slots {
			slotTotal++
			if sc.Read || sc.Written {
				slotCovered++
			}
		}
		slotPct := 100.0
		if slotTotal > 0 {
			slotPct = float64(slotCovered) / float64(slotTotal) * 100
		}

		slotStr := "n/a"
		if slotTotal > 0 {
			slotStr = fmt.Sprintf("%.0f%%", slotPct)
		}

		if hasLines {
			lineTot, lineHit := 0, 0
			for _, lc := range fc.Lines {
				lineTot++
				if lc.Hit {
					lineHit++
				}
			}
			lineStr := "n/a"
			if lineTot > 0 {
				lineStr = fmt.Sprintf("%.0f%%", float64(lineHit)/float64(lineTot)*100)
			}
			brTot, brHit := 0, 0
			for _, bc := range fc.Branches {
				brTot++
				if bc.Hit {
					brHit++
				}
			}
			brStr := "n/a"
			if brTot > 0 {
				brStr = fmt.Sprintf("%.0f%%", float64(brHit)/float64(brTot)*100)
			}
			fmt.Fprintf(w, "  %-30s  %-12s  %-12s  %-10s  %-10s  %.1f\n",
				filepath.Base(fc.ContractFile),
				fmt.Sprintf("%.0f%%", fnPct),
				slotStr,
				lineStr,
				brStr,
				ccAvg,
			)
		} else {
			fmt.Fprintf(w, "  %-30s  %-12s  %-12s  %.1f\n",
				filepath.Base(fc.ContractFile),
				fmt.Sprintf("%.0f%%", fnPct),
				slotStr,
				ccAvg,
			)
		}
	}
	if hasLines {
		fmt.Fprintf(w, "  %s\n", strings.Repeat("-", 85))
		fmt.Fprintf(w, "  %-30s  %-12s  %-12s  %-10s  %-10s\n", "TOTAL",
			fmt.Sprintf("%d%%", r.TotalFunctionPct()),
			fmt.Sprintf("%d%%", r.TotalSlotPct()),
			fmt.Sprintf("%d%%", r.TotalLinePct()),
			fmt.Sprintf("%d%%", r.TotalBranchPct()),
		)
	} else {
		fmt.Fprintf(w, "  %s\n", strings.Repeat("-", 65))
		fmt.Fprintf(w, "  %-30s  %-12s  %-12s\n", "TOTAL",
			fmt.Sprintf("%d%%", r.TotalFunctionPct()),
			fmt.Sprintf("%d%%", r.TotalSlotPct()),
		)
	}
}

// anyHasLines reports whether any FileCoverage has line data populated.
func (r *CoverageReport) anyHasLines() bool {
	for _, fc := range r.Files {
		if len(fc.Lines) > 0 {
			return true
		}
	}
	return false
}

// --- JSON output ---

type jsonReport struct {
	Files []jsonFileCoverage `json:"files"`
}

type jsonFileCoverage struct {
	Path            string            `json:"path"`
	Functions       jsonFuncSummary   `json:"functions"`
	Slots           jsonSlotSummary   `json:"slots"`
	Lines           jsonLineSummary   `json:"lines,omitempty"`
	Branches        jsonLineSummary   `json:"branches,omitempty"`
	ComplexityAvg   float64           `json:"complexity_avg"`
	FunctionsDetail []jsonFuncDetail  `json:"functions_detail"`
	SlotsDetail     []jsonSlotDetail  `json:"slots_detail"`
}

type jsonLineSummary struct {
	Hit   int `json:"hit"`
	Total int `json:"total"`
	Pct   int `json:"pct"`
}

type jsonFuncSummary struct {
	Called int `json:"called"`
	Total  int `json:"total"`
	Pct    int `json:"pct"`
}

type jsonSlotSummary struct {
	Covered int `json:"covered"`
	Total   int `json:"total"`
	Pct     int `json:"pct"`
}

type jsonFuncDetail struct {
	Name       string `json:"name"`
	Called     bool   `json:"called"`
	Complexity int    `json:"complexity"`
}

type jsonSlotDetail struct {
	Name    string `json:"name"`
	Key     string `json:"key"`
	Read    bool   `json:"read"`
	Written bool   `json:"written"`
}

// PrintJSON writes the coverage report as JSON to w.
func (r *CoverageReport) PrintJSON(w io.Writer) error {
	rep := jsonReport{}
	for _, fc := range r.Files {
		fnTotal, fnCalled := 0, 0
		ccSum := 0
		var fnDetails []jsonFuncDetail
		for _, fn := range fc.Functions {
			fnTotal++
			if fn.Called {
				fnCalled++
			}
			ccSum += fn.CC
			fnDetails = append(fnDetails, jsonFuncDetail{
				Name:       fn.Name,
				Called:     fn.Called,
				Complexity: fn.CC,
			})
		}
		fnPct := 100
		if fnTotal > 0 {
			fnPct = int(float64(fnCalled) / float64(fnTotal) * 100)
		}
		ccAvg := 0.0
		if fnTotal > 0 {
			ccAvg = float64(ccSum) / float64(fnTotal)
		}

		slotTotal, slotCovered := 0, 0
		var slotDetails []jsonSlotDetail
		for _, sc := range fc.Slots {
			slotTotal++
			if sc.Read || sc.Written {
				slotCovered++
			}
			slotDetails = append(slotDetails, jsonSlotDetail{
				Name:    sc.Name,
				Key:     sc.StorageKey,
				Read:    sc.Read,
				Written: sc.Written,
			})
		}
		slotPct := 100
		if slotTotal > 0 {
			slotPct = int(float64(slotCovered) / float64(slotTotal) * 100)
		}

		lineTot, lineHit := 0, 0
		for _, lc := range fc.Lines {
			lineTot++
			if lc.Hit {
				lineHit++
			}
		}
		linePct := 100
		if lineTot > 0 {
			linePct = int(float64(lineHit) / float64(lineTot) * 100)
		}
		brTot, brHit := 0, 0
		for _, bc := range fc.Branches {
			brTot++
			if bc.Hit {
				brHit++
			}
		}
		brPct := 100
		if brTot > 0 {
			brPct = int(float64(brHit) / float64(brTot) * 100)
		}

		jfc := jsonFileCoverage{
			Path: filepath.Base(fc.ContractFile),
			Functions: jsonFuncSummary{
				Called: fnCalled,
				Total:  fnTotal,
				Pct:    fnPct,
			},
			Slots: jsonSlotSummary{
				Covered: slotCovered,
				Total:   slotTotal,
				Pct:     slotPct,
			},
			ComplexityAvg:   ccAvg,
			FunctionsDetail: fnDetails,
			SlotsDetail:     slotDetails,
		}
		if lineTot > 0 {
			jfc.Lines = jsonLineSummary{Hit: lineHit, Total: lineTot, Pct: linePct}
		}
		if brTot > 0 {
			jfc.Branches = jsonLineSummary{Hit: brHit, Total: brTot, Pct: brPct}
		}
		rep.Files = append(rep.Files, jfc)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// --- XML output (JaCoCo-compatible subset) ---

type xmlReport struct {
	XMLName  xml.Name        `xml:"report"`
	Name     string          `xml:"name,attr"`
	Packages []xmlPackage    `xml:"package"`
	Counters []xmlCounter    `xml:"counter"`
}

type xmlPackage struct {
	Name        string          `xml:"name,attr"`
	SourceFiles []xmlSourceFile `xml:"sourcefile"`
}

type xmlSourceFile struct {
	Name     string       `xml:"name,attr"`
	Counters []xmlCounter `xml:"counter"`
}

type xmlCounter struct {
	Type    string `xml:"type,attr"`
	Missed  int    `xml:"missed,attr"`
	Covered int    `xml:"covered,attr"`
}

// PrintXML writes the coverage report as JaCoCo-compatible XML to w.
func (r *CoverageReport) PrintXML(w io.Writer) error {
	totalMethodMissed, totalMethodCovered := 0, 0
	totalCCMissed, totalCCCovered := 0, 0
	totalLineMissed, totalLineCovered := 0, 0
	totalBrMissed, totalBrCovered := 0, 0

	var sourceFiles []xmlSourceFile
	for _, fc := range r.Files {
		fnTotal, fnCalled := 0, 0
		ccSum := 0
		for _, fn := range fc.Functions {
			fnTotal++
			if fn.Called {
				fnCalled++
			}
			ccSum += fn.CC
		}
		missed := fnTotal - fnCalled

		totalMethodMissed += missed
		totalMethodCovered += fnCalled
		totalCCMissed += 0 // CC missed: not tracked without branch coverage
		totalCCCovered += ccSum

		counters := []xmlCounter{
			{Type: "METHOD", Missed: missed, Covered: fnCalled},
			{Type: "COMPLEXITY", Missed: 0, Covered: ccSum},
		}

		if len(fc.Lines) > 0 {
			lMissed, lCov := 0, 0
			for _, lc := range fc.Lines {
				if lc.Hit {
					lCov++
				} else {
					lMissed++
				}
			}
			counters = append(counters, xmlCounter{Type: "LINE", Missed: lMissed, Covered: lCov})
			totalLineMissed += lMissed
			totalLineCovered += lCov
		}
		if len(fc.Branches) > 0 {
			bMissed, bCov := 0, 0
			for _, bc := range fc.Branches {
				if bc.Hit {
					bCov++
				} else {
					bMissed++
				}
			}
			counters = append(counters, xmlCounter{Type: "BRANCH", Missed: bMissed, Covered: bCov})
			totalBrMissed += bMissed
			totalBrCovered += bCov
		}

		sourceFiles = append(sourceFiles, xmlSourceFile{
			Name:     filepath.Base(fc.ContractFile),
			Counters: counters,
		})
	}

	totalCounters := []xmlCounter{
		{Type: "METHOD", Missed: totalMethodMissed, Covered: totalMethodCovered},
		{Type: "COMPLEXITY", Missed: totalCCMissed, Covered: totalCCCovered},
	}
	if totalLineMissed+totalLineCovered > 0 {
		totalCounters = append(totalCounters, xmlCounter{Type: "LINE", Missed: totalLineMissed, Covered: totalLineCovered})
	}
	if totalBrMissed+totalBrCovered > 0 {
		totalCounters = append(totalCounters, xmlCounter{Type: "BRANCH", Missed: totalBrMissed, Covered: totalBrCovered})
	}

	rep := xmlReport{
		Name: "tolang",
		Packages: []xmlPackage{
			{
				Name:        "contracts",
				SourceFiles: sourceFiles,
			},
		},
		Counters: totalCounters,
	}

	if _, err := fmt.Fprintf(w, "%s\n", `<?xml version="1.0" encoding="UTF-8"?>`); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(rep); err != nil {
		return err
	}
	return enc.Flush()
}

// --- LCOV output ---

// PrintLCOV writes coverage in LCOV format to w.
// When line coverage data is available (from VM sourcemap instrumentation),
// real DA:line,hits records are emitted. Otherwise only function records are written.
func (r *CoverageReport) PrintLCOV(w io.Writer) error {
	for _, fc := range r.Files {
		fmt.Fprintf(w, "TN:\n")
		fmt.Fprintf(w, "SF:%s\n", filepath.Base(fc.ContractFile))

		fnTotal, fnCalled := 0, 0
		for _, fn := range fc.Functions {
			// FN: line_number,function_name  (line 1 = placeholder, no sourcemap yet)
			fmt.Fprintf(w, "FN:1,%s\n", fn.Name)
		}
		for _, fn := range fc.Functions {
			count := 0
			if fn.Called {
				count = 1
			}
			fmt.Fprintf(w, "FNDA:%d,%s\n", count, fn.Name)
			fnTotal++
			if fn.Called {
				fnCalled++
			}
		}
		fmt.Fprintf(w, "FNF:%d\n", fnTotal)
		fmt.Fprintf(w, "FNH:%d\n", fnCalled)

		// Emit per-line hit records when sourcemap data is available.
		linesHit := 0
		for _, lc := range fc.Lines {
			count := 0
			if lc.Hit {
				count = 1
				linesHit++
			}
			fmt.Fprintf(w, "DA:%d,%d\n", lc.Line, count)
		}
		if len(fc.Lines) > 0 {
			fmt.Fprintf(w, "LF:%d\n", len(fc.Lines))
			fmt.Fprintf(w, "LH:%d\n", linesHit)
		}

		fmt.Fprintf(w, "end_of_record\n")
	}
	return nil
}

// --- HTML output ---

// PrintHTML writes a minimal HTML coverage report to w.
func (r *CoverageReport) PrintHTML(w io.Writer) error {
	hasLines := r.anyHasLines()
	fmt.Fprint(w, "<html><body>\n")
	fmt.Fprint(w, "<h1>TOL Coverage Report</h1>\n")
	fmt.Fprint(w, "<table border=\"1\" cellpadding=\"4\" cellspacing=\"0\">\n")
	if hasLines {
		fmt.Fprint(w, "  <tr><th>File</th><th>Functions</th><th>Slots</th><th>Lines</th><th>Branches</th><th>CC Avg</th></tr>\n")
	} else {
		fmt.Fprint(w, "  <tr><th>File</th><th>Functions</th><th>Slots</th><th>CC Avg</th></tr>\n")
	}

	for _, fc := range r.Files {
		fnTotal, fnCalled := 0, 0
		ccSum := 0
		for _, fn := range fc.Functions {
			fnTotal++
			if fn.Called {
				fnCalled++
			}
			ccSum += fn.CC
		}
		fnPct := 100.0
		if fnTotal > 0 {
			fnPct = float64(fnCalled) / float64(fnTotal) * 100
		}
		ccAvg := 0.0
		if fnTotal > 0 {
			ccAvg = float64(ccSum) / float64(fnTotal)
		}

		slotTotal, slotCovered := 0, 0
		for _, sc := range fc.Slots {
			slotTotal++
			if sc.Read || sc.Written {
				slotCovered++
			}
		}
		slotStr := "n/a"
		if slotTotal > 0 {
			slotPct := float64(slotCovered) / float64(slotTotal) * 100
			slotStr = fmt.Sprintf("%.0f%%", slotPct)
		}

		if hasLines {
			lineTot, lineHit := 0, 0
			for _, lc := range fc.Lines {
				lineTot++
				if lc.Hit {
					lineHit++
				}
			}
			lineStr := "n/a"
			if lineTot > 0 {
				lineStr = fmt.Sprintf("%.0f%%", float64(lineHit)/float64(lineTot)*100)
			}
			brTot, brHit := 0, 0
			for _, bc := range fc.Branches {
				brTot++
				if bc.Hit {
					brHit++
				}
			}
			brStr := "n/a"
			if brTot > 0 {
				brStr = fmt.Sprintf("%.0f%%", float64(brHit)/float64(brTot)*100)
			}
			fmt.Fprintf(w, "  <tr><td>%s</td><td>%.0f%%</td><td>%s</td><td>%s</td><td>%s</td><td>%.1f</td></tr>\n",
				filepath.Base(fc.ContractFile),
				fnPct,
				slotStr,
				lineStr,
				brStr,
				ccAvg,
			)
		} else {
			fmt.Fprintf(w, "  <tr><td>%s</td><td>%.0f%%</td><td>%s</td><td>%.1f</td></tr>\n",
				filepath.Base(fc.ContractFile),
				fnPct,
				slotStr,
				ccAvg,
			)
		}
	}

	fmt.Fprint(w, "</table>\n")
	fmt.Fprint(w, "</body></html>\n")
	return nil
}

// BuildLineCoverage populates fc.Lines and fc.Branches from the hit set and the
// contract AST. It iterates over every statement in the contract body (all
// functions, constructor, fallback) and records which source lines were executed.
//
// Branch coverage tracks the then/body branch (BranchID=0) and else branch
// (BranchID=1) of if/while/for statements.
func BuildLineCoverage(fc *FileCoverage, hits map[int]bool, mod *ast.Module) {
	if mod == nil || mod.Contract == nil {
		return
	}
	cd := mod.Contract

	seenLines := map[int]bool{}
	var collectLines func(stmts []ast.Statement)
	collectLines = func(stmts []ast.Statement) {
		for _, s := range stmts {
			if s.Line > 0 && !seenLines[s.Line] {
				seenLines[s.Line] = true
				fc.Lines = append(fc.Lines, LineCoverage{
					Line: s.Line,
					Hit:  hits[s.Line],
				})
			}
			// Record branch coverage for decision statements.
			switch s.Kind {
			case "if":
				// Then branch: hit if any line in Then body is in hits.
				thenHit := anyLineHit(s.Then, hits)
				fc.Branches = append(fc.Branches, BranchCoverage{Line: s.Line, BranchID: 0, Hit: thenHit})
				// Else branch: only if the else block is non-empty.
				if len(s.Else) > 0 {
					elseHit := anyLineHit(s.Else, hits)
					fc.Branches = append(fc.Branches, BranchCoverage{Line: s.Line, BranchID: 1, Hit: elseHit})
				}
				collectLines(s.Then)
				collectLines(s.Else)
			case "while", "for":
				bodyHit := anyLineHit(s.Body, hits)
				fc.Branches = append(fc.Branches, BranchCoverage{Line: s.Line, BranchID: 0, Hit: bodyHit})
				collectLines(s.Body)
			default:
				collectLines(s.Then)
				collectLines(s.Else)
				collectLines(s.Body)
			}
			if s.Init != nil {
				collectLines([]ast.Statement{*s.Init})
			}
		}
	}

	if cd.Constructor != nil {
		collectLines(cd.Constructor.Body)
	}
	for _, fn := range cd.Functions {
		collectLines(fn.Body)
	}
	if cd.Fallback != nil {
		collectLines(cd.Fallback.Body)
	}
}

// anyLineHit reports whether any statement in stmts (or their children) has a
// line number that appears in hits.
func anyLineHit(stmts []ast.Statement, hits map[int]bool) bool {
	for _, s := range stmts {
		if s.Line > 0 && hits[s.Line] {
			return true
		}
		if anyLineHit(s.Then, hits) || anyLineHit(s.Else, hits) || anyLineHit(s.Body, hits) {
			return true
		}
	}
	return false
}

// CyclomaticComplexity computes the cyclomatic complexity of a function body.
//
// CC = number of decision points + 1.
//
// Decision points counted:
//   - if statement: +1
//   - while / for statement: +1
//   - binary && or ||: +1 each
//   - require / assert call: +1 (may take pass or fail path)
func CyclomaticComplexity(body []ast.Statement) int {
	n := 0
	countStmtDecisions(body, &n)
	return n + 1
}

func countStmtDecisions(stmts []ast.Statement, n *int) {
	for _, s := range stmts {
		switch s.Kind {
		case "if":
			*n++
			countStmtDecisions(s.Then, n)
			countStmtDecisions(s.Else, n)
		case "while", "for":
			*n++
			countStmtDecisions(s.Body, n)
		}
		countExprDecisions(s.Expr, n)
		countExprDecisions(s.Cond, n)
		countExprDecisions(s.Target, n)
		countExprDecisions(s.Post, n)
		if s.Init != nil {
			countStmtDecisions([]ast.Statement{*s.Init}, n)
		}
	}
}

func countExprDecisions(e *ast.Expr, n *int) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "binary":
		if e.Op == "&&" || e.Op == "||" {
			*n++
		}
		countExprDecisions(e.Left, n)
		countExprDecisions(e.Right, n)
	case "call":
		if e.Callee != nil && e.Callee.Kind == "ident" &&
			(e.Callee.Value == "require" || e.Callee.Value == "assert") {
			*n++
		}
		countExprDecisions(e.Callee, n)
		for _, a := range e.Args {
			countExprDecisions(a, n)
		}
	case "member":
		countExprDecisions(e.Object, n)
	case "index":
		countExprDecisions(e.Object, n)
		countExprDecisions(e.Index, n)
	default:
		countExprDecisions(e.Left, n)
		countExprDecisions(e.Right, n)
	}
}

// ContractFileCoverage parses a contract source file and returns a FileCoverage
// listing all public functions (constructor, named fns, fallback) with their
// cyclomatic complexity. All functions start as uncalled.
//
// Call MarkCalledFunctions to update call status based on a test module.
func ContractFileCoverage(filename string, src []byte) (*FileCoverage, error) {
	mod, diags := parser.ParseFile(filename, src)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse errors in %s: %v", filename, diags)
	}
	fc := &FileCoverage{ContractFile: filename}
	if mod.Contract == nil {
		return fc, nil
	}
	cd := mod.Contract
	if cd.Constructor != nil {
		fc.Functions = append(fc.Functions, &FuncCoverage{
			Name: "constructor",
			CC:   CyclomaticComplexity(cd.Constructor.Body),
		})
	}
	for _, fn := range cd.Functions {
		fc.Functions = append(fc.Functions, &FuncCoverage{
			Name: fn.Name,
			CC:   CyclomaticComplexity(fn.Body),
		})
	}
	if cd.Fallback != nil {
		fc.Functions = append(fc.Functions, &FuncCoverage{
			Name: "fallback",
			CC:   CyclomaticComplexity(cd.Fallback.Body),
		})
	}

	// Populate slot coverage stubs from storage declarations.
	if cd.Storage != nil {
		for _, slot := range cd.Storage.Slots {
			h := sha3.NewLegacyKeccak256()
			h.Write([]byte("tol.slot." + cd.Name + "." + slot.Name))
			key := "0x" + hex.EncodeToString(h.Sum(nil))
			fc.Slots = append(fc.Slots, SlotCoverage{
				Name:       slot.Name,
				StorageKey: key,
			})
		}
	}

	return fc, nil
}

// MarkCalledFunctions marks functions in fc as called based on call expressions
// of the form bindingName.methodName(...) found anywhere in the test module.
//
// When the same contract is deployed under multiple binding names, call this
// function once per binding name.
func MarkCalledFunctions(fc *FileCoverage, bindingName string, testMod *ast.Module) {
	called := map[string]bool{}
	for _, td := range testMod.Tests {
		for _, fn := range td.Fns {
			collectBindingCalls(fn.Body, bindingName, called)
		}
		if td.Setup != nil {
			collectBindingCalls(td.Setup.Body, bindingName, called)
		}
		if td.Teardown != nil {
			collectBindingCalls(td.Teardown.Body, bindingName, called)
		}
	}
	for _, fn := range fc.Functions {
		if called[fn.Name] {
			fn.Called = true
		}
	}
}

func collectBindingCalls(stmts []ast.Statement, binding string, called map[string]bool) {
	for _, s := range stmts {
		collectBindingCallsExpr(s.Expr, binding, called)
		collectBindingCallsExpr(s.Cond, binding, called)
		collectBindingCallsExpr(s.Target, binding, called)
		collectBindingCallsExpr(s.Post, binding, called)
		collectBindingCalls(s.Then, binding, called)
		collectBindingCalls(s.Else, binding, called)
		collectBindingCalls(s.Body, binding, called)
		if s.Init != nil {
			collectBindingCalls([]ast.Statement{*s.Init}, binding, called)
		}
	}
}

func collectBindingCallsExpr(e *ast.Expr, binding string, called map[string]bool) {
	if e == nil {
		return
	}
	if e.Kind == "call" && e.Callee != nil && e.Callee.Kind == "member" {
		if obj := e.Callee.Object; obj != nil && obj.Kind == "ident" && obj.Value == binding {
			called[e.Callee.Member] = true
		}
	}
	collectBindingCallsExpr(e.Callee, binding, called)
	collectBindingCallsExpr(e.Object, binding, called)
	collectBindingCallsExpr(e.Index, binding, called)
	collectBindingCallsExpr(e.Left, binding, called)
	collectBindingCallsExpr(e.Right, binding, called)
	for _, a := range e.Args {
		collectBindingCallsExpr(a, binding, called)
	}
}
