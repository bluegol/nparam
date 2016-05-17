// user input is given as []cdef
// resolved or not, internal file is saved as constDef

package nparamcli

import "fmt"

type cdef struct {
	Name     string
	Value    int
	XlsxLoc  string

	// after resolved

	*symbolInfo
}

func (c *cdef) GoConstLine() string {
	return fmt.Sprintf("\t%v = %v\n", c.Name, c.Value)
}

func (c *cdef) CsConstLine() string {
	return fmt.Sprintf("\tconst int %v = %v;\n", c.Name, c.Value)
}



type cdefFile struct {
	Src      string
	ConstFn  string
	Resolved bool
	Consts   []*cdef
}
