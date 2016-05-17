package nparamcli

import (
	"os"
	"strings"

	"github.com/bluegol/errutil"
)

type config struct {
	ServerUrl    string
	Protoc       string
	Lang         []string
	ProtoPackage string
	ProtoTypePrefix string

	goout, csout bool
}

func loadConfig(fn string) (*config, error) {
	c := config{}
	err := ReadYamlFile(fn, &c)
	if err != nil {
		return nil, errutil.Embed(ErrConfigFile, err, "file", fn)
	}

	c.ServerUrl = strings.TrimRight(c.ServerUrl, "/")
	for i, lang := range c.Lang {
		if lang[0] != '.' {
			c.Lang[i] = "." + lang
		}
	}
	for _, lang := range c.Lang {
		if lang == extGo {
			c.goout = true
		} else if lang == extCSharp {
			c.csout = true
		} else {
			return nil, errutil.New(ErrConfigFile,
				errutil.MoreInfo, "unknown language",
				"lang", lang)
		}
	}

	return &c, nil
}

func (c *config) serverCmdId() string {
	return c.ServerUrl + "/id/"
}

func (c *config) serverCmdField() string {
	return c.ServerUrl + "/field/"
}

const innerVer = 1

const (
	workDir = "Work/"
	outputDir = "Outputs/"
	binDir = "Bin/"

	extXlsx = ".xlsx"
	extConst = ".const"
	extTable = ".table"

	extInputInfoFileName = ".iinfo"

	extTableMeta = ".tm"
	extTableData = ".td"
	extPartialTableMeta = ".ptm"
	extPartialTableData = ".ptd"
	extMergeInfo = ".minfo"

	extResolvedConst = ".rc"
	extResolvedTableMeta = ".rtm"
	extResolvedTableData = ".rtd"

	extBin = ".pb.bin"

	extProto = ".proto"
	extGo = ".go"
	extCSharp = ".cs"

	// for both symbol and proto type
	pfxSubType = "SubType_"
)

func configFileName() string {
	return binDir + "config.yaml"
}

func inputsFileName() string {
	return workDir + "_inputs"
}

func verFileName() string {
	return workDir + "_ver"
}

func idLookUpFileName() string {
	return workDir + "_idlookup"
}

func prevConstsFileName() string {
	return workDir + "_consts"
}

func tableListFileName() string {
	return workDir + "_tables"
}

func intermediateConstFileName(fn string) string {
	return workDir + fn + extConst
}

func tableMetaFileName(fn, tblName string) string {
	_, fnOnly, _ := DecomposePath(fn)
	return workDir + tblName + "." + fnOnly + extTableMeta
}

func mergeInfoFileName(tn string) string {
	return workDir + tn + extMergeInfo
}

func mergedTableMetaFileName(tn string) string {
	return workDir + tn + extTableMeta
}

func protoFileName(tn string) string {
	return outputDir + tn + extProto
}

func compiledProtoFileName(fn string, lang string) string {

	_, f, _ := DecomposePath(fn)
	switch lang {

	case extGo:
		return outputDir + f + ".pb" + extGo

	case extCSharp:
		return outputDir + f + extCSharp

	default:
		return ":ERROR:"

	}
}

func binFileName(tName string) string {
	return outputDir + tName + extBin
}

func descriptorFileName(packageName string) string {
	return outputDir + packageName + "_descriptor.bin"
}

func allConstFileName(packageName, ext string) string {
	return outputDir + packageName + "_const" + ext
}

func loaderFileName(packageName, ext string) string {
	return outputDir + packageName + "_loader" + ext
}

func initPath() error {
	err := os.MkdirAll(workDir, os.ModePerm)
	if err != nil {
		return err
	}
	err = os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}
