package nparamcli

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/bluegol/errutil"
	"github.com/golang/protobuf/proto"
)

const (
	kwFieldTypeKeysOf = "$keysof"
	kwFieldTypeAutoKey = "$autokey"
	kwFieldTypeInt = "$int"
	kwFieldTypeFixed4 = "$fixed4"
	kwFieldTypeString = "$string"

	kwFieldOptCoverAll = "$coverall"
	kwFieldOptMin = "$min"
	kwFieldOptMax = "$max"
	kwFieldOptUnit = "$unit"
)

const (
	vtNotSet = 0
	vtId = 1
	vtInt = 2
	vtFixed4 = 3
	vtString = 4

	vtMax = 5
)
var typeStrings [vtMax]string =
	[...]string{ "NO TYPE", "id", "int", "fixed4", "string" }

var ErrInvalidFieldDef error

func DecomposeFieldName(fName string) (string, int, string) {
	m := reFieldName.FindStringSubmatch(fName)
	if m == nil {
		return "", -1, ""
	}
	i := -1
	if len(m[3]) > 0 {
		var err error
		i, err = strconv.Atoi(m[3])
		if err != nil {
			return "", -1, ""
		}
	}
	return m[1], i, m[5]
}

func typeString(t int) string {
	if t < 0 || t >= len(typeStrings) {
		return "ERROR " + strconv.Itoa(t)
	} else {
		return typeStrings[t]
	}
}

func setFieldTypeAndOpts(f *fieldDef, optStr string, keyField bool) error {
	var err error
	f.Opts, err = ParseOpt(optStr, fieldOpts1, fieldOpts2, fieldOpts3)
	if err != nil {
		return err
	}

	// set field type
	f.AutoKey = false
	for k, _ := range f.Opts.WithoutValue {
		err = nil
		if k == kwFieldTypeAutoKey {
			err = f.setType(vtId)
			f.AutoKey = true
		} else if k == kwFieldTypeInt {
			err = f.setType(vtInt)
		} else if k == kwFieldTypeFixed4 {
			err = f.setType(vtFixed4)
		} else if k == kwFieldTypeString {
			err = f.setType(vtString)
		}
	}
	for k, v := range f.Opts.MultiValued {
		if k == kwFieldTypeKeysOf {
			err = f.setType(vtId)
			if err != nil {
				return err
			}
			f.KeysOf = map[string]bool{}
			for _, tName := range v {
				f.KeysOf[tName] = true
			}
			if len(f.KeysOf) == 0 {
				return errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "no table is specified for type " + kwFieldTypeKeysOf,
					"field", f.Name)
			}
		}
	}
	if err != nil {
		return err
	}
	if f.Type == vtNotSet {
		return errutil.New(ErrInvalidFieldDef,
			errutil.MoreInfo, "field type is not set",
			"field", f.Name)
	}
	if keyField {
		if f.Type != vtId && f.Type != vtInt {
			return errutil.New(ErrInvalidFieldDef,
				errutil.MoreInfo, "key field's type must be either int or id",
				"field", f.Name, "type", f.TypeString() )
		}
	} else if f.AutoKey {
		return errutil.New(ErrInvalidFieldDef,
			errutil.MoreInfo, "only key field can be of type " + kwFieldTypeAutoKey,
			"field", f.Name)
	}

	// set coverall
	for k, _ := range f.Opts.WithoutValue {
		if k == kwFieldOptCoverAll {
			if f.Type != vtId {
				return errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "cannot set " + kwFieldOptCoverAll,
					"field", f.Name, "type", f.TypeString() )
			}
			if f.AutoKey {
				return errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "cannot set " + kwFieldOptCoverAll +" for autokey",
					"field", f.Name )
			}
			f.CoverAll = true
		}
	}
	// set min, max, units
	for k, v := range f.Opts.SingleValued {
		if k == kwFieldOptMin {
			if f.Type != vtInt && f.Type != vtFixed4 {
				return errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "cannot set " + kwFieldOptMin,
					"field", f.Name, "type", f.TypeString() )
			}
			f.MinStr = v
		} else if k == kwFieldOptMax {
			if f.Type != vtInt && f.Type != vtFixed4 {
				return errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "cannot set " + kwFieldOptMax,
					"field", f.Name, "type", f.TypeString() )
			}
			f.MaxStr = v
		}
	}
	for k, v := range f.Opts.MultiValued {
		if k == kwFieldOptUnit {
			if f.Type != vtInt && f.Type != vtFixed4 {
				return errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "cannot set " + kwFieldOptUnit,
					"field", f.Name, "type", f.TypeString() )
			}
			if len(v) % 2 != 0 {
				return errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, kwFieldOptUnit + " must have even number of values",
					"field", f.Name, "values", strings.Join(v, " "))
			}
			f.Units = map[string]int{}
			for i := 0; i < len(v); i += 2 {
				uv, err := strconv.Atoi(v[i + 1])
				if err != nil {
					return errutil.Embed(ErrInvalidFieldDef, err,
						errutil.MoreInfo, "not a number in values of " + kwFieldOptUnit,
						"field", f.Name, "value", v[i + 1])
				}
				f.Units[v[i]] = uv
			}
		}
	}

	return nil
}

func BuildFields(fNames, fOptStrs []string) ([]*fieldDef, error) {
	if len(fNames) == 0 || len(fNames) != len(fOptStrs) {
		return nil, errutil.New(ErrInvalidFieldDef,
			"len_names", strconv.Itoa(len(fNames)),
			"len_opts", strconv.Itoa(len(fOptStrs)) )
	}

	// parse field names for subtypes & arrays
	var err error
	var mainName, subName string
	arrayIndex := -1
	var lastMainField *fieldDef
	lastArrayIndex := -1
	lastSubIndex := -1
	mainFields := []*fieldDef{}
	names := map[string]bool{}
	subNames := map[string]bool{}
	var i int
	var name string
	addMainField := func () error {
		// check the subs ends ok
		if lastArrayIndex >= 1 {
			if lastSubIndex+1 != len(lastMainField.Subs) {
				return errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "field sub length mismatch",
					"prev_subs_length", strconv.Itoa(len(lastMainField.Subs)),
					"current_sub_length", strconv.Itoa(lastSubIndex+1),
					"array_index", strconv.Itoa(lastArrayIndex),
					"field", name, "field_index", strconv.Itoa(i) )
			}
		}
		lastMainField.ArrayLen = lastArrayIndex + 1

		// check and add last main field
		_, exists := names[lastMainField.Name]
		if exists {
			return errutil.New(ErrDuplicateFieldNames,
				"field", name, "field_index", strconv.Itoa(i))
		}
		names[lastMainField.Name] = true
		mainFields = append(mainFields, lastMainField)
		return nil
	}
	for i, name = range fNames {
		// parse field name
		mainName, arrayIndex, subName = DecomposeFieldName(name)
		if len(mainName) == 0 {
			return nil, errutil.New(ErrInvalidFieldDef,
				errutil.MoreInfo, "invalid field name",
				"field", name, "field_index", strconv.Itoa(i))
		}

		// build field
		var f *fieldDef
		if len(subName) > 0 {
			f = &fieldDef{ Name: subName }
		} else {
			f = &fieldDef{ Name: mainName }
		}
		err = setFieldTypeAndOpts(f, fOptStrs[i], i==0)
		if err != nil {
			return nil, errutil.AddInfo(err, "field", name)
		}

		// process first field == key field
		if i == 0 {
			if arrayIndex >= 0 {
				return nil, errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "key field cannot be array",
					"field", name, "field_index", strconv.Itoa(i))
			}
			if len(subName) > 0 {
				return nil, errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "key field cannot have subfield",
					"sub_name", subName,
					"field", name, "field_index", strconv.Itoa(i))
			}

			lastMainField = f
			continue
		}

		// from the second field on

		if mainName != lastMainField.Name {
			// change of main field

			// check and add
			err = addMainField()
			if err != nil {
				return nil, err
			}
			// start new main field
			if arrayIndex >= 1 {
				return nil, errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "array index error",
					"array_index", strconv.Itoa(arrayIndex),
					"field", name, "field_index", strconv.Itoa(i))
			}
			subNames = map[string]bool{}
			if len(subName) > 0 {
				lastMainField = &fieldDef{ Name: mainName }
				lastArrayIndex = arrayIndex
				lastMainField.Subs = []*fieldDef{ f }
				lastSubIndex = 0
				subNames[f.Name] = true
			} else {
				lastMainField = f
				lastArrayIndex = arrayIndex
				lastSubIndex = -1
			}
			continue
		}

		// continue in the same main field

		// check array index
		if ( lastArrayIndex == -1 && arrayIndex != -1 ) ||
			( lastArrayIndex >= 0 &&
				( arrayIndex != lastArrayIndex &&
				arrayIndex != lastArrayIndex+1 ) ) {
			var lindex, cindex string
			if lastArrayIndex == -1 {
				lindex = "none"
			} else {
				lindex = strconv.Itoa(lastArrayIndex)
			}
			if arrayIndex == -1 {
				cindex = "none"
			} else {
				cindex = strconv.Itoa(arrayIndex)
			}
			return nil, errutil.New(ErrInvalidFieldDef,
				errutil.MoreInfo, "field array index error",
				"last_array_index", lindex,
				"current_array_index", cindex,
				"field", name, "field_index", strconv.Itoa(i))
		}
		// check sub
		if ( lastSubIndex == -1 && len(subName) > 0 ) ||
			( lastSubIndex != -1 && len(subName) == 0 ) {
			return nil, errutil.New(ErrInvalidFieldDef,
				errutil.MoreInfo, "field sub error",
				"field", name, "field_index", strconv.Itoa(i))
		}

		if len(subName) == 0 {
			// no sub. so must be continuing array
			if lastArrayIndex == -1 {
				// other errors were filtered above.
				// so must be the same field again
				if lastMainField.Name == f.Name {
					return nil, errutil.New(ErrDuplicateFieldNames,
						"field", name, "field_index", strconv.Itoa(i))
				} else {
					return nil, errutil.NewAssert(
						"field", name, "field_index", strconv.Itoa(i))
				}
			} else if arrayIndex != lastArrayIndex+1 {
				// other errors were filtered above.
				// so must be the same field, same array index again
				return nil, errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "field array index error",
					"last_array_index", strconv.Itoa(lastArrayIndex),
					"current_array_index", strconv.Itoa(arrayIndex),
					"field", name, "field_index", strconv.Itoa(i))
			}
			// compare fields
			if ! f.Opts.Equals(lastMainField.Opts) {
				return nil, errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "every field in array must be the same",
					"field", name, "field_index", strconv.Itoa(i))
			}
			lastArrayIndex = arrayIndex
		} else if lastArrayIndex == -1 {
			// has sub and no array
			_, exists := subNames[f.Name]
			if exists {
				return nil, errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "duplicate sub names",
					"main_name", lastMainField.Name, "sub_name", f.Name,
					"field", name, "field_index", strconv.Itoa(i))
			}
			subNames[f.Name] = true
			lastMainField.Subs = append(lastMainField.Subs, f)
			lastSubIndex++
		} else {
			// has sub and array
			if arrayIndex == lastArrayIndex {
				// same array index
				// if first, add sub
				if lastArrayIndex == 0 {
					_, exists := subNames[f.Name]
					if exists {
						return nil, errutil.New(ErrInvalidFieldDef,
							errutil.MoreInfo, "duplicate sub names",
							"main_name", lastMainField.Name, "sub_name", f.Name,
							"field", name, "field_index", strconv.Itoa(i))
					}
					subNames[f.Name] = true
					lastMainField.Subs = append(lastMainField.Subs, f)
					lastSubIndex++
					continue
				}
				// otherwise flow down

				lastSubIndex++
			} else {
				// arrayIndex == lastArrayIndex+1, by the above check
				// reset flow subindex and flow down
				lastSubIndex = 0
			}
			// other cases were filtered above.

			// compare with prev sub field
			if lastSubIndex+1 > len(lastMainField.Subs) {
				return nil, errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "field sub mismatch. length too long",
					"array_index", strconv.Itoa(arrayIndex),
					"field", name, "field_index", strconv.Itoa(i))
			}
			prev := lastMainField.Subs[lastSubIndex]
			if f.Name != prev.Name {
				return nil, errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "field sub mismatch",
					"prev_name", prev.Name, "current_name", f.Name,
					"sub_index", strconv.Itoa(lastSubIndex),
					"array_index", strconv.Itoa(arrayIndex),
					"field", name, "field_index", strconv.Itoa(i))
			}
			if ! f.Opts.Equals(prev.Opts) {
				return nil, errutil.New(ErrInvalidFieldDef,
					errutil.MoreInfo, "field sub mismatch. options differ.",
					"subname", f.Name,
					"array_index", strconv.Itoa(arrayIndex),
					"field", name, "field_index", strconv.Itoa(i))
			}

			lastArrayIndex = arrayIndex
		}
	}
	// check and add the last main field
	err = addMainField()
	if err != nil {
		return nil, err
	}

	return mainFields, nil
}

func FieldTypeSymbolName(tableName string) string {
	return syFieldPrefix + tableName
}
const syFieldPrefix = "_field."

func FieldSymbolName(tableName, fieldName, subfieldName string) string {
	if len(subfieldName) == 0 {
		return FieldTypeSymbolName(tableName) + "." + fieldName
	} else {
		return FieldTypeSymbolName(tableName) + "." + fieldName +
			"." + subfieldName
	}
}

type fieldDef struct {
	Name     string
	// original opts as read from src
	Opts     *Options
	Type     int
	ArrayLen int
	Subs     []*fieldDef
	AutoKey  bool
	KeysOf   map[string]bool
	CoverAll bool
	Units    map[string]int
	MinStr, MaxStr string

	// when resolved

	*symbolInfo
	Min, Max int

	ProtoKey uint64
}

func (f *fieldDef) Equals(f1 *fieldDef) bool {
	if f.Name != f1.Name || f.ArrayLen != f1.ArrayLen {
		return false
	}
	numSubs := len(f.Subs)
	if numSubs != len(f1.Subs) {
		return false
	}
	if numSubs == 0 {
		return f.Opts.Equals(f1.Opts)
	} else {
		for i, sub := range f.Subs {
			if !sub.Equals(f1.Subs[i]) {
				return false
			}
		}
		return true
	}
}

func (f *fieldDef) setType(t int) error {
	if f.Type != vtNotSet {
		return errutil.New(ErrInvalidFieldDef,
			errutil.MoreInfo, "field type is specified more than once",
			"field", f.Name,
			"prev_type", f.TypeString(), "type", typeString(t))
	}
	f.Type = t
	return nil
}

func (f *fieldDef) TypeString() string {
	return typeString(f.Type)
}

func (f *fieldDef) ProtoSubType() string {
	if len(f.Subs) > 0 {
		return pfxSubType + f.Name
	} else {
		return fmt.Sprintf("ERROR: NOT HAVE SUBSTRCT name: %v type: %v",
			f.Name, f.TypeString())
	}
}

func (f *fieldDef) ProtoType() string {
	if len(f.Subs) > 0 {
		return f.ProtoSubType()
	} else if f.Type == vtInt || f.Type == vtFixed4 || f.Type == vtId {
		return "int32"
	} else if f.Type == vtString {
		return "string"
	} else {
		return fmt.Sprintf("ERROR: UNKNOWN TYPE name: %v type: %v",
			f.Name, f.Type)
	}
}

func (f *fieldDef) ProtoLine() (string, error) {
	typ := f.ProtoType()
	var repeated, packOpt string
	if f.ArrayLen > 0 {
		repeated = "repeated "
		if f.Type == vtInt || f.Type == vtFixed4 || f.Type == vtId {
			packOpt = " [packed=true]"
		}
	}
	return fmt.Sprintf("%s%s %s = %d%s;\n",
		repeated, typ, f.Name, f.symbolInfo.Value, packOpt), nil
}

func (f *fieldDef) SetProtoKey() error {
	if f.symbolInfo == nil {
		return errutil.NewAssert(
			errutil.MoreInfo, "symbolInfo not set", "field", f.Name)
	}

	tag := f.symbolInfo.Value
	if len(f.Subs) > 0 {
		f.ProtoKey = uint64(tag) << 3 | proto.WireBytes
	} else if f.Type == vtId || f.Type == vtInt || f.Type == vtFixed4 {
		f.ProtoKey = uint64(tag) << 3 | proto.WireVarint
	} else if f.Type == vtString {
		f.ProtoKey = uint64(tag) << 3 | proto.WireBytes
	} else {
		return errutil.NewAssert(
			errutil.MoreInfo, "invalid type",
			"field", f.Name, "type", f.TypeString() )
	}

	return nil
}

/////////////////////////////////////////////////////////////////////

func init() {
	ErrInvalidFieldDef = errors.New("필드 정의가 잘못됨")

	reFieldName, _ = regexp.Compile(
		`^([A-Za-z][_A-Za-z0-9]*)(\[(\d+)\])?(\.([A-Za-z][_A-Za-z0-9]*))?$`)

	fieldOpts1 = []string{
		kwFieldTypeAutoKey, kwFieldTypeInt, kwFieldTypeFixed4, kwFieldTypeString,
		kwFieldOptCoverAll }
	fieldOpts2 = []string{ kwFieldOptMin, kwFieldOptMax }
	fieldOpts3 = []string{ kwFieldTypeKeysOf, kwFieldOptUnit }
}

var reFieldName *regexp.Regexp
var fieldOpts1, fieldOpts2, fieldOpts3 []string