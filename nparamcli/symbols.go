package nparamcli

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/bluegol/errutil"
	"strconv"
)

const (
	stConst = iota
	stTable
	stFieldType
	stField
	stAutoKey
)

var (
	ErrInvalidSymbol    error
	ErrUndefinedSymbol  error
	ErrDuplicateSymbol  error
)

func CheckValidUserDefinedSymbol(name string) error {
	if len(name) > 0 && len(name) < symbolMaxLen &&
		reUserDefinedSymbol.MatchString(name) {
		return nil
	}
	return errutil.New(ErrInvalidSymbol, "name", name)
}

/*
func IsValidIdString(s string) bool {
	return len(s) < SymbolMaxLen &&
		(reValidUserDefinedSymbol.MatchString(s) ||
			reValidInternalSymbol.MatchString(s))
}
*/

type symbolInfo struct {
	Name     string
	Id       int
	Type     int
	Src      string
	SrcTable string

	Value    int
}

func (s *symbolInfo) TypeAsString() string {
	if s.Type == stConst {
		return "const"
	} else if s.Type == stTable {
		return "table"
	} else if s.Type == stFieldType {
		return "field type"
	} else if s.Type == stField {
		return "field"
	} else if s.Type == stAutoKey {
		return "autokey"
	} else {
		return fmt.Sprintf("ERROR: UNKNOWN TYPE %v", s.Type)
	}
}

type symbolTable struct {
	name2Id map[string]int
	byName  map[string]*symbolInfo
}

// loadOrNewSymbolTable returns
// symbol table, whether loaded or not, and error
func loadOrNewSymbolTable(idLookupFn string) (*symbolTable, bool, error) {
	result := &symbolTable{
		name2Id:     map[string]int{},
		byName: map[string]*symbolInfo{},
	}
	err := ReadYamlFile(idLookupFn, result.name2Id)
	if err != nil {
		if errutil.IsNotExist(err) {
			return result, false, nil
		} else {
			return nil, false, err
		}
	}
	return result, true, nil
}

func (st *symbolTable) SaveIdLookUp(idLookupFn string) error {
	return WriteYamlFile(idLookupFn, st.name2Id)
}

func (st *symbolTable) Find(name string) *symbolInfo {
	return st.byName[name]
}

func (st *symbolTable) FindId(name string) int {
	return st.name2Id[name]
}

func (st *symbolTable) FilterKnownIds(names []string) []string {
	unknowns := []string{}
	for _, ids := range names {
		_, exists := st.name2Id[ids]
		if ! exists {
			unknowns = append(unknowns, ids)
		}
	}
	return unknowns
}

func (st *symbolTable) AddIds(m map[string]int) {
	for name, id := range m {
		st.name2Id[name] = id
	}
}

// AddSymbol adds symbol.
func (st *symbolTable) AddSymbol(sinfo *symbolInfo) error {
	// add id. also, perform sanity check.
	prev_id := st.name2Id[sinfo.Name]
	if prev_id != 0 {
		if prev_id != sinfo.Id {
			return errutil.NewAssert(errutil.MoreInfo, "id mismatch!",
				"name", sinfo.Name,
				"prev_id", strconv.Itoa(prev_id),
				"new", strconv.Itoa(sinfo.Id),
				"file", sinfo.Src)
		}
	} else {
		st.name2Id[sinfo.Name] = sinfo.Id
	}

	// add symbol
	prev, exists := st.byName[sinfo.Name]
	if exists {
		var prevType string
		if prev.Type == stConst {
			prevType = "const"
		} else if prev.Type == stTable {
			prevType = "table"
		} else if prev.Type == stAutoKey {
			prevType = "autokey"
		} else {
			return errutil.NewAssert(
				errutil.MoreInfo, "prev symbol's type is strange",
				"name", sinfo.Name, "file", sinfo.Src,
				"prev_type", prevType,
				"prev_defined_in", prev.Src)
		}

		return errutil.New(ErrDuplicateSymbol,
			"name", sinfo.Name, "file", sinfo.Src,
			"prev_type", prevType,
			"prev_defined_in", prev.Src)
	}
	st.byName[sinfo.Name] = sinfo

	return nil
}

// AddSymbolWithoutId adds symbol or returns error.
func (st *symbolTable) AddNewSymbol(
	name, src, table string, typ, value int) (*symbolInfo, error) {

	id := st.FindId(name)
	if id == 0 {
		return nil, errutil.NewAssert(errutil.MoreInfo, "id doesn't exist",
			"name", name, "file", src)
	}
	if typ != stConst && typ != stField {
		value = id
	}
	sinfo := &symbolInfo{
		Name: name,
		Id: id,
		Type: typ,
		Src: src,
		SrcTable: table,
		Value: value,
	}
	err := st.AddSymbol(sinfo)
	if err != nil {
		return nil, err
	}
	return sinfo, nil
}

/*
func (st *symbolTable) ResolveId(ids string) (bool, int) {
	info, exists := st.byName[ids]
	if ! exists || ( info.ValueType == kwTypeInt || info.ValueType == kwTypeFixed4 ) {
		return false, 0
	}
	return true, info.Id
}

func (st *symbolTable) ResolveInt(ids string) (bool, int) {
	info, exists := st.byName[ids]
	if ! exists || info.ValueType != kwTypeInt {
		return false, 0
	}
	return true, info.ResolvedValue
}

func (st *symbolTable) ResolveFixed4(ids string) (bool, int) {
	info, exists := st.byName[ids]
	if ! exists || info.ValueType != kwTypeFixed4 {
		return false, 0
	}
	return true, info.ResolvedValue
}
*/

func init() {
	reUserDefinedSymbol, _ = regexp.Compile(`^[A-Za-z][0-9_A-Za-z]*$`)
	reInternalSymbol, _ =
		regexp.Compile(`^[_A-Za-z][0-9_A-Za-z]*(\.[_A-Za-z][0-9_A-Za-z]*){0,3}$`)

	ErrInvalidSymbol = errors.New("잘못된 심볼")
	ErrUndefinedSymbol = errors.New("정의되지 않은 심볼")
	ErrDuplicateSymbol = errors.New("이미 정의된 심볼")
}

var (
	reUserDefinedSymbol *regexp.Regexp
	reInternalSymbol *regexp.Regexp
)

const symbolMaxLen = 128
