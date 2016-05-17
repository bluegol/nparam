package nparamcli

import (
	"bytes"
	"encoding/csv"
	"errors"
	"regexp"
	"strconv"

	"github.com/bluegol/errutil"
)

var ErrInvalidOpt error

type Options struct {
	WithoutValue map[string]bool
	SingleValued map[string]string
	MultiValued  map[string][]string
}

func newOptions() *Options {
	return &Options{
		WithoutValue: map[string]bool{},
		SingleValued: map[string]string{},
		MultiValued:  map[string][]string{},
	}
}

func ParseOpt(optstr string, o1, o2, o3 []string) (*Options, error) {
	opts, err := ParseOptOnly(optstr)
	if err != nil {
		return nil, err
	}
	err = opts.Check(o1, o2, o3)
	if err != nil {
		return opts, err
	}
	return opts, nil
}

func ParseOptOnly(optStr string) (*Options, error) {
	opts := newOptions()
	if len(optStr) == 0 {
		return opts, nil
	}
	items := reOptSeparator.Split(optStr, -1)
	for _, o := range items {
		m := reOptGetter.FindStringSubmatch(o)
		if m == nil {
			return nil, errutil.New(ErrCannotParseOpts, "optstr", optStr)
		}
		k := m[1]
		if opts.Has(k) {
			return nil, errutil.New(ErrInvalidOptSpec,
				errutil.MoreInfo, "duplicate option",
				"opt", k, "optstr", optStr)
		}
		v := m[3]
		if len(v) > 0 {
			// has value
			r := csv.NewReader(bytes.NewBufferString(v))
			result, err := r.ReadAll()
			if err != nil {
				return nil, errutil.AssertEmbed(err,
					"optstr", optStr, "value", v)
			} else if len(result) != 1 {
				return nil, errutil.NewAssert(
					"optstr", optStr, "value", v,
					"lines", strconv.Itoa(len(result)))
			}
			if len(result[0]) == 1 {
				opts.SingleValued[k] = result[0][0]
			} else {
				opts.MultiValued[k] = result[0]
			}
		} else {
			// has no value
			opts.WithoutValue[k] = true
		}
	}

	return opts, nil
}

func (o *Options) Equals(o1 *Options) bool {
	if len(o.WithoutValue) != len(o1.WithoutValue) ||
	len(o.SingleValued) != len(o1.SingleValued) ||
	len(o.MultiValued) != len(o1.MultiValued) {
		return false
	}
	for k, v := range o.WithoutValue {
		v1, exists := o1.WithoutValue[k]
		if ! exists || v != v1 {
			return false
		}
	}
	for k, v := range o.SingleValued {
		v1, exists := o1.SingleValued[k]
		if ! exists || v != v1 {
			return false
		}
	}
	for k, sl := range o.MultiValued {
		sl1, exists := o1.MultiValued[k]
		if ! exists || len(sl) != len(sl1) {
			return false
		}
		for i, s := range sl {
			if s != sl1[i] {
				return false
			}
		}
	}

	return true
}

func (o *Options) Check(o1, o2, o3 []string) error {
	m := map[string]bool{}
	for _, k := range o1 {
		m[k] = true
	}
	for k, _ := range o.WithoutValue {
		_, exists := m[k]
		if ! exists {
			return errutil.New(ErrInvalidOpt,
				errutil.MoreInfo, "cannot use option without value",
				"opt", k)
		}
	}

	m = map[string]bool{}
	for _, k := range o3 {
		m[k] = true
	}
	for k, _ := range o.MultiValued {
		_, exists := m[k]
		if ! exists {
			return errutil.New(ErrInvalidOpt,
				errutil.MoreInfo, "cannot use multi-valued option",
				"opt", k)
		}
	}
	for k, v := range o.SingleValued {
		_, exists := m[k]
		if exists {
			delete(o.SingleValued, k)
			o.MultiValued[k] = []string{ v }
		}
	}

	m = map[string]bool{}
	for _, k := range o2 {
		m[k] = true
	}
	for k, _ := range o.SingleValued {
		_, exists := m[k]
		if ! exists {
			return errutil.New(ErrInvalidOpt,
				errutil.MoreInfo, "cannot use single-valued option",
				"opt", k)
		}
	}


	return nil
}


func (o *Options) Has(k string) bool {
	_, exists := o.WithoutValue[k]
	if exists {
		return true
	}
	_, exists = o.SingleValued[k]
	if exists {
		return true
	}
	_, exists = o.MultiValued[k]
	if exists {
		return true
	}
	return false
}

func (o *Options) HasOptWithoutValue(k string) bool {
	_, exists := o.WithoutValue[k]
	return exists
}

func (o *Options) GetStr(k string) (string, bool) {
	v, exists := o.SingleValued[k]
	return v, exists
}

func (o *Options) GetStrs(k string) ([]string, bool) {
	sl, exists := o.MultiValued[k]
	return sl, exists
}




func init() {
	ErrInvalidOpt = errors.New("잘못된 옵션")

	reOptSeparator, _ = regexp.Compile(`\s*[;\n]\s*`)
	reOptGetter, _ = regexp.Compile(`^([A-Za-z\$][A-Za-z0-9_]*)(\s*=\s*(\S+))?$`)
	// reOptValueSeparator, _ = regexp.Compile(`\s*,\s*`)
}

var (
	reOptSeparator *regexp.Regexp
	reOptGetter *regexp.Regexp
	// reOptValueSeparator *regexp.Regexp
)
