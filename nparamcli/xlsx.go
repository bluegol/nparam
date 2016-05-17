package nparamcli

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/bluegol/errutil"
	"github.com/tealeg/xlsx"
)

var (
	ErrXlsxDuplicateTblNames   error
	ErrXlsxInvalidTableDef error
	ErrXlsxInvalidConstDef error
)

func ParseXlsx(xlsxFn string) ([]*cdef, []*tableMeta, [][][]string, error) {
	cdefs := []*cdef{}
	tms := []*tableMeta{}
	tds := [][][]string{}
	tables := map[string]*tableMeta{}

	xlFile, err := xlsx.OpenFile(xlsxFn)
	if err != nil {
		return nil, nil, nil, errutil.AssertEmbed(err, "file", xlsxFn)
	}
	// loop over workshets and process
	for _, ws := range xlFile.Sheets {
		// extract cells
		cells := make([][]string, len(ws.Rows))
		for j, row := range ws.Rows {
			cells[j] = make([]string, len(row.Cells))
			for k, cell := range row.Cells {
				cells[j][k], err = cell.String()
				if err != nil {
					return nil, nil, nil,
						errutil.AssertEmbed(err,
							"file", xlsxFn,
							"xlsxLoc", xlsxLoc(ws.Name, j+1, k+1))
				}
			}
		}

		cresult, err := extractConsts(ws.Name, cells)
		if err != nil {
			return nil, nil, nil, errutil.AddInfo(err, "file", xlsxFn)
		}
		cdefs = append(cdefs, cresult...)

		tms2, tds2, err := extractTables(xlsxFn, ws.Name, cells)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, tm := range tms2 {
			prev, exists := tables[tm.Name]
			if exists {
				return nil, nil, nil, errutil.New(ErrXlsxDuplicateTblNames,
					"table", tm.Name,
					"xlsxLoc", tm.XlsxLoc, "prev_xlsxLoc", prev.XlsxLoc)
			}
			tables[tm.Name] = tm
		}
		tms = append(tms, tms2...)
		tds = append(tds, tds2...)
	}

	return cdefs, tms, tds, nil
}

func extractConsts(wsName string, cells [][]string) ([]*cdef, error) {
	cdefs := []*cdef{}

	for j, line := range cells {
		for k, c := range line {
			if c == kwConst {
				if k+2 >= len(line) {
					return nil, errutil.New(ErrXlsxInvalidConstDef,
						"xlsxLoc", xlsxLoc(wsName, j, k))
				}
				v := line[k+2]
				iv, err := strconv.Atoi(v)
				if err != nil {
					return nil, errutil.New(ErrXlsxInvalidConstDef,
						errutil.MoreInfo, "not int",
						"value", v, "xlsxLoc", xlsxLoc(wsName, j, k))
				}

				cdefs = append(cdefs,
					&cdef{
						Name: line[k+1],
						Value: iv,
						XlsxLoc: xlsxLoc(wsName, j, k+1) } )
			}
		}
	}
	return cdefs, nil
}

func extractTables(xlsxFn, wsName string, cells [][]string) ([]*tableMeta, [][][]string, error) {
	tms := []*tableMeta{}
	rawdata := [][][]string{}

	for j, line := range cells {
		for k, c := range line {
			if c != kwTable {
				continue
			}

			tLoc := xlsxLoc(wsName, j, k)
			if k+1 >= len(line) {
				return nil, nil, errutil.New(ErrXlsxInvalidTableDef,
					errutil.MoreInfo, "no table name",
					"xlsxLoc", tLoc, "file", xlsxFn)
			}
			tName := line[k+1]
			var tblOptStr string
			if k+2 < len(line) {
				tblOptStr = line[k+2]
			}
			tOpts, err := GetTableOpts(tblOptStr)
			if err != nil {
				return nil, nil, errutil.Embed(ErrXlsxInvalidTableDef, err,
					errutil.MoreInfo, "invalid table options",
					"table", tName, "xlsxLoc", tLoc, "file", xlsxFn)
			}
			// check end of rows
			var numRows int
			if tOpts.Has(kwTblOptSingleRow) {
				if j+3 >= len(cells) {
					return nil, nil, errutil.New(ErrXlsxInvalidTableDef,
						errutil.MoreInfo, "no row",
						"table", tName, "xlsxLoc", tLoc, "file", xlsxFn)
				}
				numRows = 1
			} else {
				if j+4 >= len(cells) {
					return nil, nil, errutil.New(ErrXlsxInvalidTableDef,
						errutil.MoreInfo, "no row",
						"table", tName, "xlsxLoc", tLoc, "file", xlsxFn)
				}
				endRow := -1
				for jj := j+4; jj < len(cells); jj++ {
					if cells[jj][k] == kwEnd {
						endRow = jj
						break
					}
				}
				if endRow < 0 {
					return nil, nil, errutil.New(ErrXlsxInvalidTableDef,
						errutil.MoreInfo, "no row end",
						"table", tName, "xlsxLoc", tLoc, "file", xlsxFn)
				}
				numRows = endRow - (j + 3)
			}
			// get field lines
			fieldLine := cells[j+1]
			fieldOptLine := cells[j+2]
			endCol := -1
			for kk := k+1; kk < len(fieldLine); kk++ {
				if fieldLine[kk] == kwEnd {
					endCol = kk
					break
				}
			}
			if endCol < 0 {
				return nil, nil, errutil.New(ErrXlsxInvalidTableDef,
					errutil.MoreInfo, "no field end",
					"table", tName, "xlsxLoc", tLoc, "file", xlsxFn)
			}
			if endCol-1 >= len(fieldOptLine) {
				return nil, nil, errutil.New(ErrXlsxInvalidTableDef,
					errutil.MoreInfo, "no field opt",
					"table", tName, "xlsxLoc", tLoc, "file", xlsxFn)
			}
			numFields := endCol - k

			// get field lines
			tm, err := BuildTableMeta(tName, xlsxFn, tLoc, tOpts,
				fieldLine[k:endCol], fieldOptLine[k:endCol])
			if err != nil {
				return nil, nil, err
			}

			// get data
			data := make([][]string, numRows)
			for jj := 0; jj < numRows; jj++ {
				data[jj] = make([]string, numFields)
				line := cells[jj+j+3]
				for kk := 0; kk < numFields; kk++ {
					if kk+k >= len(line) {
						break
					}
					data[jj][kk] = line[kk+k]
				}
			}
			// set autokeys
			if tm.AutoKey() {
				tm.AutoKeyNames = make([]string, numRows)
				for jj, line := range data {
					tm.AutoKeyNames[jj] = line[0]
				}
			}

			tms = append(tms, tm)
			rawdata = append(rawdata, data)
		}
	}
	return tms, rawdata, nil
}


func xlsxLoc(w string, r, c int) string {
	buf := make([]byte, 0, 3)
	v := c + 1
	q := v / (26*26)
	v = v % (26*26)
	if q > 0 {
		buf = append(buf, 'A'-1+byte(q))
	}
	q = v / 26
	v = v % 26
	if q > 0 {
		buf = append(buf, 'A'-1+byte(q))
	}
	buf = append(buf, 'A'-1+byte(v))

	return fmt.Sprintf("%s!%s%d", w, string(buf), r+1)
}

func init() {
	ErrXlsxDuplicateTblNames = errors.New("xlsx에서 테이블 이름이 겹침")
	ErrXlsxInvalidTableDef = errors.New("xlsx에서 table 정의가 잘못됨")
	ErrXlsxInvalidConstDef = errors.New("xlsx에서 const 정의가 잘못됨")
}