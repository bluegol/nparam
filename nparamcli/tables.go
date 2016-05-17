package nparamcli

import (
	"regexp"
	"strconv"

	"github.com/bluegol/errutil"
	"fmt"
	"errors"
)

var (
	ErrDuplicateTblNames error

	ErrTableOpts  error
	//ErrMergeNonPartialTables error
	ErrMergeMetaNotEqual     error

	ErrInvalidInt error
	ErrInvalidSRTableReference error
	ErrIntOutOfRange error
	ErrNotAutoKey error
	ErrKeyOutOfRange error

	ErrCyclicDependency error
)

const (
	kwTblOptPartial = "$partial"
	kwTblOptSingleRow = "$singlerow"
)

type tableMeta struct {
	// Resolved is set if resolved tm file is read.
	Resolved           bool

	// before Resolved

	// Src is the original file name where the table is defined.
	Src                string
	//
	XlsxLoc            string

	// TmFileName is the final .tm file
	TmFileName         string

	Name               string
	Opts               *Options
	Fields             []*fieldDef
	AutoKeyNames       []string

	Partial            bool
	SingleRow          bool

	// two lookups are set after read

	fieldsNameAndOrder map[string]int
	fieldsByOrder      []*fieldDef

	// after resolved. these + fieldDef's symbols are set

	TableSymbol        *symbolInfo
	FieldTypeSymbol    *symbolInfo
	AutoKeys           []*symbolInfo
}

func ReadTm(fn string) (*tableMeta, error) {
	t := &tableMeta{}
	err := ReadYamlFile(fn, t)
	if err != nil {
		return nil, err
	}
	setFieldsNameAndOrder(t)
	return t, nil
}

// GetTableOpts parses given optStr and returns *Options
// necessary to find out if table is single-row while building table
func GetTableOpts(optStr string) (*Options, error) {
	return ParseOpt(optStr, tableOpts1, tableOpts2, tableOpts3)
}

func BuildTableMeta(name, src, xlsxLoc string, tOpts *Options,
	fNames, fOptStrs []string) (*tableMeta, error) {

	t := &tableMeta{ Name: name, Src: src, XlsxLoc: xlsxLoc }

	err := setTableOpts(t, tOpts)
	if err != nil {
		err = errutil.AddInfo(err, "table", name, "file", src)
		if len(t.XlsxLoc) > 0 {
			err = errutil.AddInfo(err, "xlsx_loc", xlsxLoc)
		}
		return nil, err
	}

	err = setFields(t, fNames, fOptStrs)
	if err != nil {
		err = errutil.AddInfo(err, "table", name, "file", src)
		if len(t.XlsxLoc) > 0 {
			err = errutil.AddInfo(err, "xlsx_loc", xlsxLoc)
		}
		return nil, err
	}

	return t, nil
}

func setTableOpts(t *tableMeta, opts *Options) error {
	t.Opts = opts
	err := t.Opts.Check(tableOpts1, tableOpts2, tableOpts3)
	if err != nil {
		return err
	}

	for k, _ := range t.Opts.WithoutValue {
		if k == kwTblOptPartial {
			t.Partial = true
		} else if k == kwTblOptSingleRow {
			t.SingleRow = true
		}
	}
	if t.Partial && t.SingleRow {
		return errutil.New(ErrTableOpts,
			errutil.MoreInfo,
			fmt.Sprintf("cannot set %v and %v at the same time",
				kwTblOptPartial, kwTblOptSingleRow))
	}
	return nil
}

func setFields(t *tableMeta, fNames, fOptStrs []string) error {
	var err error
	t.Fields, err = BuildFields(fNames, fOptStrs)
	if err != nil {
		err = errutil.AddInfo(err, "table", t.Name, "file", t.Src)
		if len(t.XlsxLoc) > 0 {
			err = errutil.AddInfo(err, "xlsx_loc", t.XlsxLoc)
		}
		return err
	}
	setFieldsNameAndOrder(t)
	return nil
}

func setFieldsNameAndOrder(t *tableMeta) {
	t.fieldsNameAndOrder = map[string]int{}
	t.fieldsByOrder = []*fieldDef{}

	o, fNum, fSub, fSubLen, fIndex := -1, -1, -1, -1, -1
	var mainField, fi *fieldDef
	nextMainField := func() bool {
		fNum++
		if fNum >= len(t.Fields) {
			return false
		}

		mainField = t.Fields[fNum]
		if mainField.ArrayLen > 0 {
			fIndex = 0
		} else {
			fIndex = -1
		}
		fSubLen = len(mainField.Subs)
		if fSubLen > 0 {
			fSub = 0
			fi = mainField.Subs[fSub]
			return true
		} else {
			fSub = -1
			fi = mainField
			return true
		}
	}
	incIndex := func() bool {
		if mainField == nil || mainField.ArrayLen <= 0 {
			return nextMainField()
		}

		fIndex++
		if fIndex >= mainField.ArrayLen {
			return nextMainField()
		} else {
			if fSubLen > 0 {
				fSub = 0
				fi = mainField.Subs[fSub]
			}
			return true
		}
	}
	nextField := func() bool {
		if fSubLen <= 0 {
			return incIndex()
		}

		fSub++
		if fSub >= fSubLen {
			if fIndex >= 0 {
				return incIndex()
			} else {
				return nextMainField()
			}
		} else {
			fi = mainField.Subs[fSub]
			return true
		}
	}
	for {
		o++
		if ! nextField() {
			break
		}

		var name string
		if mainField.ArrayLen > 0 && fSubLen > 0 {
			name = fmt.Sprintf("%v[%v].%v",
				t.Fields[fNum].Name, fIndex, fi.Name)
		} else if mainField.ArrayLen > 0 {
			name = fmt.Sprintf("%v[%v]", fi.Name, fIndex)
		} else if fSubLen > 0 {
			name = fmt.Sprintf("%v.%v", t.Fields[fNum].Name, fi.Name)
		} else {
			name = fi.Name
		}

		t.fieldsNameAndOrder[name] = o
		t.fieldsByOrder = append(t.fieldsByOrder, fi)
	}
}

func (t *tableMeta) AutoKey() bool {
	return t.Fields[0].AutoKey
}

func OkToMerge(t, t1 *tableMeta) bool {
	if t.Name != t1.Name {
		return false
	}

	if ! t.Opts.Equals(t1.Opts) {
		return false
	}

	if len(t.Fields) != len(t1.Fields) {
		return false
	}
	for i, f := range t.Fields {
		if !f.Equals(t1.Fields[i]) {
			return false
		}
	}

	return true
}

func (t *tableMeta) AddNames(names []string) []string {
	names = append(names, t.Name, FieldTypeSymbolName(t.Name))
	for _, fi := range t.Fields {
		names = append(names, FieldSymbolName(t.Name, fi.Name, ""))
		for _, sfi := range fi.Subs {
			names = append(names, FieldSymbolName(t.Name, fi.Name, sfi.Name))
		}
	}
	for _, ak := range t.AutoKeyNames {
		names = append(names, ak)
	}

	return names
}

/////////////////////////////////////////////////////////////////////

type tableData struct {
	Resolved       bool

	// before Resolved

	Name           string
	RawData        [][]string

	// tableMeta is set after read
	*tableMeta

	// after Resolved

	// ReferencedTms are where ReferencedSymbols belong.
	ReferencedTms  map[string]bool
	// ReferencedSymbols are external autokey symbols referenced by table data.
	ReferencedKeys map[string]bool
	// ReferencedTds are external single-row tables referenced by table data.
	ReferencedTds  map[string]bool

	Data           [][]int
}

const Fixed4Mult = 10000

func DecomposeValue(v string) (bool, int, bool, int, string) {
	m := reValueWithUnit.FindStringSubmatch(v)
	if m == nil {
		return false, 0, false, 0, ""
	}
	i, _ := strconv.Atoi(m[1])
	var dexists bool
	var d int
	if len(m[3]) == 0 {
		dexists = false
	} else {
		var err error
		d, err = strconv.Atoi(m[3])
		if err != nil {
			dexists = false
		} else {
			dexists = true
		}
	}
	unit := m[4]
	return true, i, dexists, d, unit
}

func DecomposeSRTableReference(v string) (string, string) {
	m := reSRTableReference.FindStringSubmatch(v)
	if m == nil {
		return "", ""
	}
	tName := m[1]
	fName := m[3]
	return tName, fName
}

/////////////////////////////////////////////////////////////////////

func init() {
	ErrDuplicateTblNames = errors.New("테이블 이름이 중복")

	ErrTableOpts = errors.New("잘못된 테이블 옵션")

	//ErrMergeNonPartialTables = errors.New("$partial이 세팅되지 않은 테이블을 merge하려 함")
	ErrMergeMetaNotEqual = errors.New("메타정보가 다른 테이블을 merge하려 함")

	ErrInvalidInt = errors.New("잘못된 int")
	ErrInvalidSRTableReference = errors.New("잘못된 SRTable 레퍼런스")
	ErrIntOutOfRange = errors.New("int값이 범위를 벗어남")
	ErrNotAutoKey = errors.New("심볼이 autokey가 아님")
	ErrKeyOutOfRange = errors.New("지정된 테이블에 있는 키가 아님")
	ErrCyclicDependency = errors.New("cyclic dependency")

	reSRTableReference, _ = regexp.Compile(
		`^([A-Za-z][0-9A-Za-z_]*)(\.(.+))$`)
	reValueWithUnit, _ = regexp.Compile(
		`^([0-9]+)(\.([0-9]{1,4}))?\s*([A-Za-z][0-9A-Za-z_]*)?$` )

	tableOpts1 = []string{ kwTblOptPartial, kwTblOptSingleRow }
	tableOpts2 = []string{}
	tableOpts3 = []string{}
}

var (
	reSRTableReference *regexp.Regexp
	reValueWithUnit    *regexp.Regexp
)
var tableOpts1, tableOpts2, tableOpts3 []string
