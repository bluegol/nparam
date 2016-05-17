package nparamcli

import (
	"crypto/sha1"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/bluegol/errutil"
	"gopkg.in/yaml.v2"
)

func ReadYamlFile(fn string, toread interface{}) error {
	f, err := os.Open(fn)
	if err != nil {
		return errutil.New(err, "file", fn)
	}
	defer f.Close()
	bytes, err := ioutil.ReadAll(f)
	if err != nil {
		return errutil.Embed(ErrCannotReadYaml, err, "file", fn)
	}
	err = yaml.Unmarshal(bytes, toread)
	if err != nil {
		return errutil.Embed(ErrCannotReadYaml, err, "file", fn)
	}
	return nil
}

func WriteYamlFile(out string, data interface{}) error {
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return errutil.Embed(ErrCannotWriteYaml, err, "file", out)
	}
	f, err := os.Create(out)
	if err != nil {
		return errutil.Embed(ErrCannotWriteYaml, err, "file", out)
	}
	_, err = f.Write(bytes)
	if err != nil {
		f.Close()
		return errutil.Embed(ErrCannotWriteYaml, err, "file", out)
	}
	err = f.Close()
	if err != nil {
		return errutil.Embed(ErrCannotWriteYaml, err, "file", out)
	}

	return nil
}

func ExecuteTemplateToFile(out string,
	tmpl *template.Template, data interface{}) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}

	err = tmpl.Execute(f, data)
	if err != nil {
		f.Close()
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}

	return nil
}

func CopyFile(src, dst string) error {
	ffrom, err := os.Open(src)
	if err != nil {
		return errutil.AssertEmbed(err,
			errutil.MoreInfo, "while copying",
			"src", src, "dst", dst)
	}
	defer ffrom.Close()
	fto, err := os.Create(dst)
	if err != nil {
		return errutil.AssertEmbed(err,
			errutil.MoreInfo, "while copying",
			"src", src, "dst", dst)
	}
	defer fto.Close()
	_, err = io.Copy(fto, ffrom)
	if err != nil {
		return errutil.AssertEmbed(err,
			errutil.MoreInfo, "while copying",
			"src", src, "dst", dst)
	}
	return nil
}

func FileExists(fn string) bool {
	finfo, err := os.Stat(fn)
	if os.IsNotExist(err) || finfo.IsDir() {
		return false
	} else {
		return true
	}
}

func Sha1(fn string) ([]byte, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, errutil.AssertEmbed(err,
			errutil.MoreInfo, "while computing sha1",
			"file", fn)
	}
	content, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, errutil.AssertEmbed(err,
			errutil.MoreInfo, "while computing sha1",
			"file", fn)
	}
	sum := sha1.Sum(content)
	return sum[:], nil
}

func DecomposePath(fn string) (string, string, string) {
	dir, file := filepath.Split(fn)
	ext := filepath.Ext(file)
	ext = strings.ToLower(ext)
	if len(ext) > 0 {
		file = file[0 : len(file)-len(ext)]
	}
	return dir, file, ext
}

func ChangeExt(fn, newExt string) string {
	dir, fnOnly, _ := DecomposePath(fn)
	return dir + fnOnly + newExt
}

func NeedToProcess(in string, out string) bool {
	inInfo, err := os.Stat(in)
	if inInfo == nil || err != nil {
		if os.IsNotExist(err) {
			panic("bug? file does not exist. file: " + in)
		}
		panic("cannot stat file: " + in + " " + err.Error())
	}

	outInfo, err := os.Stat(out)
	if os.IsNotExist(err) {
		return true
	}
	if outInfo == nil {
		panic("outInfo nil! file: " + out)
	}

	return inInfo.ModTime().Sub(outInfo.ModTime()) > 0
}

func compareLists(current []string, prev []string) bool {
	if len(current) != len(prev) {
		return false
	}
	temp := map[string]bool{}
	for _, fn := range current {
		temp[fn] = true
	}
	for _, fn := range prev {
		_, exists := temp[fn]
		if ! exists {
			return false
		}
	}
	return true
}

func ParseInt(s string, units map[string]int) (int, error) {
	match := reInt.FindStringSubmatch(s)
	if match == nil {
		return 0, ErrInvalidIntValue
	}
	intStr := match[1]
	unit := match[2]
	i, err := strconv.Atoi(intStr)
	if err != nil {
		return 0, err
	}
	var multiplier int
	if len(unit) > 0 {
		var exists bool
		multiplier, exists = units[unit]
		if !exists {
			return 0, ErrUnknownUnit
		}
	} else {
		multiplier = 1
	}
	return i * multiplier, nil
}

func ParseFixed4(s string) (int, error) {
	match := reFixed4.FindStringSubmatch(s)
	if match == nil {
		return 0, ErrInvalidFixed4Value
	}
	intStr := match[1]
	decimalStr := match[2]
	i, err := strconv.Atoi(intStr)
	if err != nil {
		return 0, err
	}
	d := 0
	if len(decimalStr) > 0 {
		d, err = strconv.Atoi(intStr)
		if err != nil {
			return 0, err
		}
	}
	return i * 10000 + d, nil
}



func init() {
	reInt, _ = regexp.Compile(`^([+-]?\d+)\s*([A-Za-z]*)$`)
	reFixed4, _ = regexp.Compile(`^([+-]?\d+)(\.\d{1,4})?$`)
}
var reInt *regexp.Regexp
var reFixed4 *regexp.Regexp
