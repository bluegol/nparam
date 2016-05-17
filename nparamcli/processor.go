package nparamcli

import (
	"bytes"
	//"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/bluegol/errutil"
	"github.com/golang/protobuf/proto"
	log "gopkg.in/inconshreveable/log15.v2"
)

type processor struct {
	logger       log.Logger
	config       *config
	st           *symbolTable

	rebuild      bool
	checkConsts  bool

	// working files
	inputs       map[string]*inputInfo
	tableListChanged bool
	currentFiles map[string]bool

	// const file name ==> constDef
	consts       map[string]*cdefFile
	// table name ==> tableMeta
	tms          map[string]*tableMeta
	// table name ==> tableData
	tds          map[string]*tableData
	//
	protoFns     []string
}

type inputInfo struct {
	InputFile   string
	Hash        []byte
	OutputFiles []string
}

type mergeInfo struct {
	InputFiles []string
}

func newProcessor(configFile string, logger log.Logger,
	rebuild, warn bool) (*processor, error) {

	var err error

	err = initPath()
	if err != nil {
		return nil, err
	}

	proc := &processor{}
	proc.logger = logger

	// config
	proc.config, err = loadConfig(configFile)
	if err != nil {
		return nil, err
	}
	logger.Info("loaded config file", "file", configFile)

	// symbol table
	idLookUpFn := idLookUpFileName()
	proc.st, _, err = loadOrNewSymbolTable(idLookUpFn)
	if err != nil {
		return nil, errutil.AssertEmbed(err, "file", idLookUpFn)
	}

	proc.rebuild = rebuild
	proc.checkConsts = warn

	// const & tm
	proc.consts = map[string]*cdefFile{}
	proc.tms = map[string]*tableMeta{}

	return proc, nil
}

func (proc *processor) processInputs() error {
	proc.logger.Info("processing inputs...")

	nextFiles := map[string]bool{}
	filesToProcess := map[string][]byte{}
	// table ==> file
	tables := map[string]string{}
	currentFns, err := filepath.Glob("*")
	if err != nil {
		return errutil.AssertEmbed(err, errutil.MoreInfo, "while globbing")
	}

	prevInputs := map[string]*inputInfo{}
	err = ReadYamlFile(inputsFileName(), prevInputs)
	if err != nil && ! errutil.IsNotExist(err) {
		return err
	}
	curInputs := map[string]*inputInfo{}

	if proc.rebuild {
		proc.logger.Info("reprocessing all input files")
	}
	outerLoop:
	for _, fn := range currentFns {
		_, _, ext := DecomposePath(fn)
		if ext != extXlsx && ext != extTable && ext != extConst {
			continue
		}
		// compare hash of input file
		var hash []byte
		hash, err := Sha1(fn)
		if err != nil {
			return errutil.AssertEmbed(err,
				errutil.MoreInfo, "while computing hash")
		}
		// if rebuilding, process all files
		if proc.rebuild {
			filesToProcess[fn] = hash
			continue
		}
		// check previous input info
		prevInputInfo, exists := prevInputs[fn]
		if ! exists {
			// file added
			proc.logger.Info("input file added", "file", fn)
			filesToProcess[fn] = hash
			continue
		}
		if bytes.Compare(hash, prevInputInfo.Hash) != 0 {
			filesToProcess[fn] = hash
			proc.logger.Info("input file changed", "file", fn)
			continue
		}
		// check output files
		for _, outFn := range prevInputInfo.OutputFiles {
			if ! FileExists(outFn) {
				proc.logger.Info("missing output file. need to reprocess",
					"file", fn, "missing_output", outFn)
				filesToProcess[fn] = hash
				continue outerLoop
			}
		}

		// no change. use the previous result
		for _, fn := range prevInputInfo.OutputFiles {
			nextFiles[fn] = true
		}
		curInputs[fn] = prevInputInfo
		proc.logger.Info("input file unchanged",
			"file", fn, "output", prevInputInfo.OutputFiles)
	}

	// process input files
	for fn, hash := range filesToProcess {
		// remove unused previous outputs
		prevInputInfo, exists := prevInputs[fn]
		if exists {
			for _, outFn := range prevInputInfo.OutputFiles {
				err = removeOutputs(outFn)
				if err != nil {
					return err
				}
			}
		}

		iinfo := &inputInfo{ InputFile: fn, Hash: hash }
		_, _, ext := DecomposePath(fn)
		if ext == extXlsx {
			outFns := []string{}
			cdefs, tms, tdata, err := ParseXlsx(fn)
			if err != nil {
				return err
			}
			constFn := intermediateConstFileName(fn)
			cdf := &cdefFile{ Src: fn, ConstFn: constFn, Consts: cdefs }
			err = WriteYamlFile(constFn, cdf)
			if err != nil {
				return errutil.AssertEmbed(err,
					errutil.MoreInfo, "while const",
					"file", fn, "const_file", constFn)
			}
			outFns = append(outFns, constFn)
			for i, tm := range tms {
				var tmFn, tdFn string
				if tm.Partial {
					// table will be merged
					tmFn = tableMetaFileName(fn, tm.Name)
					tmFn = ChangeExt(tmFn, extPartialTableMeta)
					tdFn = ChangeExt(tmFn, extPartialTableData)
					prev, exists := tables[tm.Name]
					if exists {
						return errutil.New(ErrDuplicateTblNames,
							"table_name", tm.Name,
							"file1", prev, "file2", fn)
					}
				} else {
					tmFn = tableMetaFileName(fn, tm.Name)
					tdFn = ChangeExt(tmFn, extTableData)
					tables[tm.Name] = fn
				}
				tm.TmFileName = tmFn
				err = WriteYamlFile(tmFn, tm)
				if err != nil {
					return errutil.AssertEmbed(err,
						errutil.MoreInfo, "while tm",
						"file", fn, "tm_file", tmFn)
				}
				outFns = append(outFns, tmFn)
				err = WriteYamlFile(tdFn, tdata[i])
				if err != nil {
					return errutil.AssertEmbed(err,
						errutil.MoreInfo, "while td",
						"file", fn, "td_file", tdFn)
				}
				outFns = append(outFns, tdFn)
			}
			iinfo.OutputFiles = outFns
		} else if ext == extTable {


			// \todo handle .table file
			panic("TODO: .table file")


		} else if ext == extConst {
			cdefs := []*cdef{}
			err := ReadYamlFile(fn, &cdefs)
			if err != nil {
				return err
			}
			outFn := intermediateConstFileName(fn)
			cf := &cdefFile{ Src: fn, ConstFn: outFn, Consts: cdefs }
			err = WriteYamlFile(outFn, cf)
			if err != nil {
				return err
			}
			iinfo.OutputFiles = []string{ outFn }
		} else {
			return errutil.NewAssert("file", fn)
		}

		curInputs[fn] = iinfo
		for _, outFn := range iinfo.OutputFiles {
			nextFiles[outFn] = true
		}
		proc.logger.Info("successfully processed input file",
			"file", fn, "output", iinfo.OutputFiles)
	}

	// remove outputs of deleted input files
	for fn, prevInputInfo := range prevInputs {
		_, exists := curInputs[fn]
		if exists {
			continue
		}
		for _, outFn := range prevInputInfo.OutputFiles {
			err = removeOutputs(outFn)
			if err != nil {
				return err
			}
		}
	}

	// check if there's change in table list
	prevTms, curTms := map[string]bool{}, map[string]bool{}
	for _, info := range prevInputs {
		for _, outFn := range info.OutputFiles {
			_, _, ext := DecomposePath(outFn)
			if ext == extTableMeta || ext == extPartialTableMeta {
				prevTms[outFn] = true
			}
		}
	}
	for _, info := range curInputs {
		for _, outFn := range info.OutputFiles {
			_, _, ext := DecomposePath(outFn)
			if ext == extTableMeta || ext == extPartialTableMeta {
				curTms[outFn] = true
			}
		}
	}
	if len(prevTms) != len(curTms) {
		proc.tableListChanged = true
	}
	if ! proc.tableListChanged {
		for tmFn, _ := range prevTms {
			_, exists := curTms[tmFn]
			if ! exists {
				proc.tableListChanged = true
				break
			}
		}
	}
	if proc.tableListChanged {
		proc.logger.Info("table list changed")
	}

	err = WriteYamlFile(inputsFileName(), curInputs)
	if err != nil {
		return err
	}

	proc.currentFiles = nextFiles
	proc.logger.Info("...finished processing inputs")
	return nil
}

func removeOutputs(outFn string) error {
	_, fnOnly, ext := DecomposePath(outFn)
	_, tn, _ := DecomposePath(fnOnly)
	if ext == extTableMeta {
		fn := outFn
		err := os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = ChangeExt(fn, extTableData)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = ChangeExt(fn, extResolvedTableMeta)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = ChangeExt(fn, extResolvedTableData)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		protoFn := protoFileName(tn)
		fn = protoFn
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = ChangeExt(fn, extBin)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = compiledProtoFileName(protoFn, extCSharp)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = compiledProtoFileName(protoFn, extGo)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
	} else if ext == extPartialTableMeta {
		fn := outFn
		err := os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = ChangeExt(fn, extPartialTableData)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = mergedTableMetaFileName(tn)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = ChangeExt(fn, extTableMeta)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = ChangeExt(fn, extTableData)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = ChangeExt(fn, extResolvedTableMeta)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = ChangeExt(fn, extResolvedTableData)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		protoFn := protoFileName(tn)
		fn = protoFn
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = ChangeExt(fn, extBin)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = compiledProtoFileName(protoFn, extCSharp)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = compiledProtoFileName(protoFn, extGo)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
	} else if ext == extConst {
		fn := outFn
		err := os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
		fn = ChangeExt(fn, extResolvedConst)
		err = os.Remove(fn)
		if err != nil && ! os.IsNotExist(err) {
			return err
		}
	}

	return nil
}

func (proc *processor) processConsts() error {
	time.Sleep(time.Second)
	proc.logger.Info("processing consts...")

	// read const and resolved const files
	unknowns := []string{}
	for fn, _ := range proc.currentFiles {
		_, origFn, ext := DecomposePath(fn)
		if ext != extConst {
			continue
		}

		var cdf *cdefFile
		rcFn := ChangeExt(fn, extResolvedConst)
		if NeedToProcess(fn, rcFn) {
			// read const file
			cdf = &cdefFile{}
			err := ReadYamlFile(fn, cdf)
			if err != nil {
				return err
			}
			for _, c := range cdf.Consts {
				// check name
				err := CheckValidUserDefinedSymbol(c.Name)
				if err != nil {
					return errutil.AddInfo(err, "file", origFn)
				}
				unknowns = append(unknowns, c.Name)
			}
			proc.logger.Info("read const file", "file", fn)
		} else {
			// read resolved const file
			cdf = &cdefFile{}
			err := ReadYamlFile(rcFn, cdf)
			if err != nil {
				return errutil.AssertEmbed(err,
					errutil.MoreInfo, "while reading rc")
			}
			proc.logger.Info("read resolved const file", "file", rcFn)
		}
		proc.consts[fn] = cdf
	}
	// add symbols for resolved const files
	for _, cdf := range proc.consts {
		if ! cdf.Resolved {
			continue
		}
		for _, c := range cdf.Consts {
			err := proc.st.AddSymbol(c.symbolInfo)
			if err != nil {
				return err
			}
		}
	}
	// resolve ids for const files
	err := proc.resolveIds(unknowns)
	if err != nil {
		return err
	}
	// add symbols for const files
	for _, cdf := range proc.consts {
		if cdf.Resolved {
			continue
		}

		for _, c := range cdf.Consts {
			c.symbolInfo, err = proc.st.AddNewSymbol(
				c.Name, cdf.Src, "", stConst, c.Value)
			if err != nil {
				return err
			}
		}
	}
	// save resolved const file
	for _, cdef := range proc.consts {
		if cdef.Resolved {
			continue
		}

		cdef.Resolved = true
		rcFn := ChangeExt(cdef.ConstFn, extResolvedConst)
		err := WriteYamlFile(rcFn, cdef)
		if err != nil {
			return err
		}
		proc.logger.Info("wrote const file", "file", rcFn)
	}

	proc.logger.Info("...finished processing consts")
	return nil
}

func (proc *processor) resolveIds(names []string) error {
	if len(names) > 0 {
		unknowns := proc.st.FilterKnownIds(names)
		if len(unknowns) > 0 {
			proc.logger.Info("querying unknown ids", "count", len(unknowns))
			m, err := GetIdsFromServer(proc.config.serverCmdId(), unknowns)
			if err != nil {
				return errutil.AddInfo(err,
					errutil.MoreInfo, "error while resolving ids")
			}
			proc.st.AddIds(m)
			proc.logger.Info("received ids", "count", len(unknowns))
		}
	}

	return nil
}

func (proc *processor) compareConsts() ([]string, []string, error) {
	proc.logger.Info("comparing consts...")

	// read previous consts
	prevConsts := map[string]cdef{}
	err := ReadYamlFile(prevConstsFileName(), prevConsts)
	if err != nil {
		if errutil.IsNotExist(err) {
			// nothing to compare, so return
			return nil, nil, nil
		} else {
			return nil, nil, errutil.AssertEmbed(err,
				errutil.MoreInfo, "while reading previous consts")
		}
	}
	// collect all current consts
	currentConsts := map[string]*cdef{}
	for _, cdef := range proc.consts {
		for _, c := range cdef.Consts {
			currentConsts[c.Name] = c
		}
	}
	// compare
	var added, deletedOrChanged []string
	for _, c := range currentConsts {
		p, exists := prevConsts[c.Name]
		if ! exists {
			added = append(added, c.Name)
		} else if p.Value != c.Value {
			deletedOrChanged = append(deletedOrChanged, c.Name)
		}
	}
	for _, c := range prevConsts {
		_, exists := currentConsts[c.Name]
		if ! exists {
			deletedOrChanged = append(deletedOrChanged, c.Name)
		}
	}

	// save current consts
	err = WriteYamlFile(prevConstsFileName(), currentConsts)
	if err != nil {
		return nil, nil, errutil.AssertEmbed(err,
			errutil.MoreInfo, "while writing current consts")
	}
	proc.logger.Info("wrote consts file")

	proc.logger.Info("...finished comparing consts")
	return added, deletedOrChanged, nil
}

func (proc *processor) mergeTables() error {
	time.Sleep(time.Second)
	proc.logger.Info("merging tables...")

	nextFiles := map[string]bool{}
	// gather partial table metadata files
	tables := map[string][]string{}
	for fn, _ := range proc.currentFiles {
		_, f, ext := DecomposePath(fn)
		if ext == extPartialTableMeta {
			_, tn, _ := DecomposePath(f)
			tables[tn] = append(tables[tn], fn)
		} else if ext != extPartialTableData {
			nextFiles[fn] = true
		}
	}

	// check lmt and get files to merge
	filesToMerge := map[string][]string{}
	outerLoop:
	for tn, fns := range tables {
		// check lmt
		mergedFn := mergedTableMetaFileName(tn)
		mergedDataFn := ChangeExt(mergedFn, extTableData)
		for _, fn := range fns {
			dataFn := ChangeExt(fn, extPartialTableData)
			if NeedToProcess(fn, mergedFn) ||
				NeedToProcess(dataFn, mergedDataFn) {
				filesToMerge[tn] = fns
				continue outerLoop
			}
		}
		// check previous merge info
		minfoFn := mergeInfoFileName(tn)
		minfo := &mergeInfo{}
		err := ReadYamlFile(minfoFn, minfo)
		if err != nil {
			if errutil.IsNotExist(err) {
				filesToMerge[tn] = fns
				continue
			} else {
				return errutil.AssertEmbed(err,
					errutil.MoreInfo, "while reading minfo" )
			}
		}
		if len(fns) != len(minfo.InputFiles) {
			filesToMerge[tn] = fns
			continue
		}
		temp := map[string]bool{}
		for _, fn := range fns {
			temp[fn] = true
		}
		for _, fn := range minfo.InputFiles {
			_, exists := temp[fn]
			if ! exists {
				filesToMerge[tn] = fns
				continue
			}
		}
		// use previous result
		proc.logger.Info("using previous merge result", "table", tn)
		nextFiles[mergedFn] = true
		nextFiles[mergedDataFn] = true
	}

	// merge
	for tn, fns := range filesToMerge {
		err := proc.mergeTable(tn, fns)
		if err != nil {
			return err
		}
		for _, fn := range fns {
			delete(nextFiles, fn)
			delete(nextFiles, ChangeExt(fn, extTableData))
		}
		mergedFn := mergedTableMetaFileName(tn)
		nextFiles[mergedFn] = true
		nextFiles[ChangeExt(mergedFn, extTableData)] = true
	}

	proc.currentFiles = nextFiles
	proc.logger.Info("...finished merging tables")

	return nil
}

func (proc *processor) mergeTable(tn string, fns []string) error {
	var mergedTblm *tableMeta
	srcs := []string{}
	newKeys := []string{}
	for _, fn := range fns {
		tm := &tableMeta{}
		err := ReadYamlFile(fn, tm)
		if err != nil {
			return err
		}
		if mergedTblm == nil {
			mergedTblm = tm
		}
		if ! tm.Partial {
			return errutil.NewAssert("file", fn, "table", tm.Name)
		}
		if ! OkToMerge(mergedTblm, tm) {
			return errutil.New(ErrMergeMetaNotEqual,
				"file", fn, "table", tm.Name,
				"orig_file", fns[0])
		}
		newKeys = append(newKeys, tm.AutoKeyNames...)
		srcs = append(srcs, tm.Src)
	}
	mergedTblm.AutoKeyNames = newKeys
	mergedTblm.TmFileName = mergedTableMetaFileName(tn)
	mergedTblm.Src = strings.Join(srcs, ", ")
	err := WriteYamlFile(mergedTblm.TmFileName, mergedTblm)
	if err != nil {
		return err
	}

	mergedDataFn := ChangeExt(mergedTblm.TmFileName, extTableData)
	mergedData := make([][]string, 0, 4096)
	for _, fn := range fns {
		dataFn := ChangeExt(fn, extPartialTableData)
		data := [][]string{}
		err := ReadYamlFile(dataFn, &data)
		if err != nil {
			return err
		}
		if cap(mergedData) >= len(mergedData) + len(data) {
			mergedData = append(mergedData, data...)
		} else {
			newData := make([][]string, len(mergedData) + len(data))
			copy(newData, mergedData)
			copy(newData[len(mergedData):], data)
			mergedData = newData
		}
	}
	err = WriteYamlFile(mergedDataFn, mergedData)
	if err != nil {
		return err
	}

	minfoFn := mergeInfoFileName(tn)
	minfo := &mergeInfo{ InputFiles: fns }
	err = WriteYamlFile(minfoFn, minfo)
	if err != nil {
		return errutil.AssertEmbed(err,
			errutil.MoreInfo, "while writing minfo" )
	}
	proc.logger.Info("successfully merged table",
		"table", tn, "files", fns)

	return nil
}

func (proc *processor) processTableMetas() error {
	time.Sleep(time.Second)
	proc.logger.Info("processing table metas...")

	// read tm or resolved tm files
	for fn, _ := range proc.currentFiles {
		_, _, ext := DecomposePath(fn)
		if ext != extTableMeta {
			continue
		}
		var tm *tableMeta
		var err error
		rtmFn := ChangeExt(fn, extResolvedTableMeta)
		if NeedToProcess(fn, rtmFn) {
			// read tm file
			tm, err = ReadTm(fn)
			if err != nil {
				return err
			}
			proc.logger.Info("read tm file", "file", fn)
		} else {
			// read resolved tm file
			tm, err = ReadTm(rtmFn)
			if err != nil {
				return err
			}
			proc.logger.Info("read resolved tm file", "file", rtmFn)
		}

		prev, exists := proc.tms[tm.Name]
		if exists {
			err := errutil.New(ErrDuplicateTblNames,
				"table_name", tm.Name,
				"prev_file", prev.Src, "file", tm.Src)
			if len(prev.XlsxLoc) > 0 {
				err = errutil.AddInfo(err, "prev_xlsx_loc", prev.XlsxLoc)
			}
			if len(tm.XlsxLoc) > 0 {
				err = errutil.AddInfo(err, "xlsx_loc", tm.XlsxLoc)
			}
			return err
		}
		proc.tms[tm.Name] = tm
	}
	// add symbols from resolved tm files
	for _, tm := range proc.tms {
		if ! tm.Resolved {
			continue
		}

		err := proc.st.AddSymbol(tm.TableSymbol)
		if err != nil {
			return err
		}
		err = proc.st.AddSymbol(tm.FieldTypeSymbol)
		if err != nil {
			return err
		}
		for _, fi := range tm.Fields {
			err = proc.st.AddSymbol(fi.symbolInfo)
			if err != nil {
				return err
			}
			for _, sfi := range fi.Subs {
				err = proc.st.AddSymbol(sfi.symbolInfo)
				if err != nil {
					return err
				}
			}
		}
		for _, ak := range tm.AutoKeys {
			err = proc.st.AddSymbol(ak)
			if err != nil {
				return err
			}
		}
	}
	// collect all unknown ids and resolve them
	unknowns := []string{}
	for _, tm := range proc.tms {
		if tm.Resolved {
			continue
		}
		unknowns = tm.AddNames(unknowns)
	}
	err := proc.resolveIds(unknowns)
	if err != nil {
		return err
	}
	// add symbols from tm files
	for _, tm := range proc.tms {
		if tm.Resolved {
			continue
		}

		tm.TableSymbol, err = proc.st.AddNewSymbol(
			tm.Name, tm.Src, tm.Name, stTable, 0)
		if err != nil {
			return err
		}
		tm.FieldTypeSymbol, err = proc.st.AddNewSymbol(
			FieldTypeSymbolName(tm.Name),
			tm.Src, tm.Name, stFieldType, 0)
		if err != nil {
			return err
		}
		for _, fi := range tm.Fields {
			fi.symbolInfo, err = proc.st.AddNewSymbol(
				FieldSymbolName(tm.Name, fi.Name, ""),
				tm.Src, tm.Name, stField, 0)
			if err != nil {
				return err
			}
			for _, sfi := range fi.Subs {
				sfi.symbolInfo, err = proc.st.AddNewSymbol(
					FieldSymbolName(tm.Name, fi.Name, sfi.Name),
					tm.Src, tm.Name, stField, 0)
				if err != nil {
					return err
				}
			}
		}
		tm.AutoKeys = make([]*symbolInfo, len(tm.AutoKeyNames))
		for i, name := range tm.AutoKeyNames {
			tm.AutoKeys[i], err = proc.st.AddNewSymbol(
				name, tm.Src, tm.Name, stAutoKey, 0)
			if err != nil {
				return err
			}
		}
		proc.logger.Info("resolved symbols", "table", tm.Name)
	}
	// resolve field tags
	for _, tm := range proc.tms {
		if tm.Resolved {
			continue
		}

		param := []int{}
		fieldSymbolById := map[int]*symbolInfo{}
		name := FieldTypeSymbolName(tm.Name)
		sinfo := proc.st.Find(name)
		if sinfo == nil {
			return errutil.NewAssert(errutil.MoreInfo, "no id?", "name", name)
		}
		param = append(param, sinfo.Id)
		for _, fi := range tm.Fields {
			name = FieldSymbolName(tm.Name, fi.Name, "")
			sinfo = proc.st.Find(name)
			if sinfo == nil {
				return errutil.NewAssert(errutil.MoreInfo, "no id?", "name", name)
			}
			param = append(param, sinfo.Id)
			fieldSymbolById[sinfo.Id] = sinfo
			for _, sfi := range fi.Subs {
				name = FieldSymbolName(tm.Name, fi.Name, sfi.Name)
				sinfo = proc.st.Find(name)
				if sinfo == nil {
					return errutil.NewAssert(errutil.MoreInfo, "no id?", "name", name)
				}
				param = append(param, sinfo.Id)
				fieldSymbolById[sinfo.Id] = sinfo
			}
		}
		result, err := GetFieldTagsFromServer(proc.config.serverCmdField(), param)
		if err != nil {
			return errutil.AddInfo(err,
				"table", tm.Name, "action", "resolving field tags")
		}
		for id, tag := range result {
			sinfo = fieldSymbolById[id]
			if sinfo == nil {
				return errutil.NewAssert(
					"id", strconv.Itoa(id), "result_tag", strconv.Itoa(tag))
			}
			sinfo.Value = tag
		}
		// set proto key
		for _, fi := range tm.Fields {
			err := fi.SetProtoKey()
			if err != nil {
				return errutil.AddInfo(err, "table", tm.Name)
			}
			for _, sfi := range fi.Subs {
				err = sfi.SetProtoKey()
				if err != nil {
					return errutil.AddInfo(err, "table", tm.Name)
				}
			}
		}
		proc.logger.Info("resolved field tags", "table", tm.Name)
	}
	// save newly resolved tm files
	for _, tm := range proc.tms {
		if tm.Resolved {
			continue
		}

		tm.Resolved = true
		rtmFn := ChangeExt(tm.TmFileName, extResolvedTableMeta)
		err := WriteYamlFile(rtmFn, tm)
		if err != nil {
			return errutil.AddInfo(err, "table", tm.Name)
		}
		proc.logger.Info("wrote resolved tm file", "file", rtmFn)
	}

	proc.logger.Info("...finished processing table metas")
	return nil
}

func (proc *processor) resolveTableData() error {
	time.Sleep(time.Second)
	proc.logger.Info("resolving table data...")

	proc.tds = map[string]*tableData{}
	// read td and resolved td files
	for fn, _ := range proc.currentFiles {
		_, _, ext := DecomposePath(fn)
		if ext != extTableData {
			continue
		}

		var td *tableData
		rtdFn := ChangeExt(fn, extResolvedTableData)
		if NeedToProcess(fn, rtdFn) {
			// read td file
			rawData := [][]string{}
			err := ReadYamlFile(fn, &rawData)
			if err != nil {
				return err
			}
			_, fnOnly, _ := DecomposePath(fn)
			_, tName, _ := DecomposePath(fnOnly)
			td = &tableData{ Name: tName, RawData: rawData }
			proc.logger.Info("read td file", "file", fn)
		} else {
			// read resolved td file
			td = &tableData{}
			err := ReadYamlFile(rtdFn, td)
			if err != nil {
				return err
			}
			proc.logger.Info("read resolved td file", "file", rtdFn)
		}
		// set tableMeta
		tm := proc.tms[td.Name]
		if tm == nil {
			return errutil.NewAssert("table", td.Name)
		}
		td.tableMeta = tm
		proc.tds[td.Name] = td
	}

	// for unresolved td's, set references
	for _, td := range proc.tds {
		if td.Resolved {
			continue
		}
		err := proc.setTableDataReferences(td)
		if err != nil {
			return err
		}
	}
	// for resolved td, check ReferencedTms to see if
	// it needs to be reprocessed. td itself was not changed,
	// so if any of the symbols info was changed, the previous
	// reference tms must have been changed.
	for _, td := range proc.tds {
		if ! td.Resolved {
			continue
		}

		tdFn := ChangeExt(td.TmFileName, extTableData)
		rtdFn := ChangeExt(tdFn, extResolvedTableData)
		needReprocess := false
		if ! needReprocess {
			for parentName, _ := range td.ReferencedTms {
				parentTm := proc.tms[parentName]
				parentRtmFn := ChangeExt(parentTm.TmFileName, extResolvedTableMeta)
				if parentTm == nil ||
					! FileExists(parentRtmFn) ||
					NeedToProcess(parentRtmFn, rtdFn) {
					needReprocess = true
					break
				}
			}
		}
		if needReprocess {
			td.Resolved = false
			err := proc.setTableDataReferences(td)
			if err != nil {
				return err
			}
		}
	}
	// loop over all tds and resolve when possible
	// true: td was changed, false: td was unchanged
	// not in the map: not processed yet
	changed := map[string]bool{}
	anyChange := true
	for anyChange {
		anyChange = false
		// first go over "resolved" tables
		// check its ReferencedTds and see if it needs to be reprocessed
		loopOverTds:
		for _, td := range proc.tds {
			_, exists := changed[td.Name]
			if exists {
				continue
			}

			anyParentChanged := false
			for parentName, _ := range td.ReferencedTds {
				parentChanged, exists := changed[parentName]
				if ! exists {
					// parent not processed yet. so wait now.
					continue loopOverTds
				}
				anyParentChanged = anyParentChanged || parentChanged
			}
			if td.Resolved && ! anyParentChanged {
				// no need to change
				changed[td.Name] = false
			} else {
				// can resolve now!
				err := proc.resolveTd(td)
				if err != nil {
					return err
				}
				changed[td.Name] = true
				anyChange = true
				proc.logger.Info("resolved table data", "table", td.Name)
			}
		}
	}
	// check every td is resolved
	for _, td := range proc.tds {
		if ! td.Resolved {
			// no error during resolving and still not resolved
			// so this td must have cyclic dependency
			cyclic, err := proc.findCyclicDependency(td)
			if err != nil {
				return err
			}
			if len(cyclic) == 0 {
				return errutil.NewAssert("table", td.Name)
			}
			return errutil.New(ErrCyclicDependency,
				"dependency", strings.Join(cyclic, " "))
		}
	}
	// save newly resolved tds
	for _, td := range proc.tds {
		ch := changed[td.Name]
		if ! ch {
			continue
		}

		rtdFn := ChangeExt(td.TmFileName, extResolvedTableData)
		err := WriteYamlFile(rtdFn, td)
		if err != nil {
			return err
		}
		proc.logger.Info("wrote resolved td file", "file", rtdFn)
	}

	proc.logger.Info("...finished resolving table data")
	return nil
}

func (proc *processor) setTableDataReferences(td *tableData) error {
	td.ReferencedTms = map[string]bool{}
	td.ReferencedKeys = map[string]bool{}
	td.ReferencedTds = map[string]bool{}

	td.ReferencedTms[td.Name] = true

	// from field opts
	for _, fi := range td.tableMeta.Fields {
		if len(fi.Subs) == 0 {
			proc.addReferencesFromField(fi, td)
		} else {
			for _, sfi := range fi.Subs {
				proc.addReferencesFromField(sfi, td)
			}
		}
	}
	// from values
	for _, row := range td.RawData {
		for _, v := range row {
			r1, r2 := proc.getReferenceFromValue(v)
			if len(r1) > 0 {
				td.ReferencedKeys[v] = true
				td.ReferencedTms[r1] = true
			} else if len(r2) > 0 {
				td.ReferencedTds[r2] = true
			}
		}
	}
	return nil
}

func (proc *processor) addReferencesFromField(fi *fieldDef, td *tableData) {
	for tName, _ := range fi.KeysOf {
		td.ReferencedTms[tName] = true
	}
	v := fi.MinStr
	if len(v) > 0 {
		_, r2 := proc.getReferenceFromValue(v)
		if len(r2) > 0 {
			td.ReferencedTds[r2] = true
		}
	}
	v = fi.MaxStr
	if len(v) > 0 {
		_, r2 := proc.getReferenceFromValue(v)
		if len(r2) > 0 {
			td.ReferencedTds[r2] = true
		}
	}
}

func (proc *processor) getReferenceFromValue(v string) (string, string) {
	sinfo := proc.st.Find(v)
	if sinfo != nil {
		if sinfo.Type == stAutoKey {
			return sinfo.SrcTable, ""
		} else {
			// const or error
			// error will be handled later when resolving values
			return "", ""
		}
	}
	b, _, _, _, _ := DecomposeValue(v)
	if b {
		return "", ""
	}
	tName, _ := DecomposeSRTableReference(v)
	if len(tName) > 0 {
		return "", tName
	}
	// error will be handled later when resolving values
	return "", ""
}

func (proc *processor) findCyclicDependency(td *tableData) ([]string, error) {
	var recursive func(string, *tableData, []string) ([]string, error)
	recursive = func(startName string, tdd *tableData, chain []string) ([]string, error) {
		for tName, _ := range tdd.ReferencedTds {
			if tName == startName {
				return []string{ tdd.Name }, nil
			}
			tddd, exists := proc.tds[tName]
			if ! exists {
				return nil, errutil.NewAssert(
					errutil.MoreInfo, "referenced table does not exist",
					"table", tdd.Name, "referenced_table", tName)
			}
			chain, err := recursive(startName, tddd, chain)
			if err != nil {
				return nil, err
			}
			if len(chain) > 0 {
				return append(chain, tdd.Name), nil
			}
		}
		return nil, nil
	}
	return recursive(td.Name, td, nil)
}

func (proc *processor) resolveTd(td *tableData) error {
	numRows := len(td.RawData)
	numFields := len(td.RawData[0])
	td.Data = make([][]int, numRows)
	for i := 0; i < numRows; i++ {
		td.Data[i] = make([]int, numFields)
	}

	var err error
	for j := 0; j < numFields; j++ {
		fi := td.tableMeta.fieldsByOrder[j]
		// resolve field opts
		if len(fi.MinStr) > 0 {
			fi.Min, err = proc.resolveInt(fi.MinStr, fi.Type == vtFixed4, nil)
		}
		if len(fi.MaxStr) > 0 {
			fi.Max, err = proc.resolveInt(fi.MaxStr, fi.Type == vtFixed4, nil)
		}

		// resolve table values
		if fi.Type == vtId {
			srcTables := []string{}
			for i := 0; i < numRows; i++ {
				var srcTable string
				td.Data[i][j], srcTable, err = proc.resolveId(td.RawData[i][j])
				if err != nil {
					return errutil.AddInfo(err,
						"table", td.Name,
						"row_key", td.RawData[i][0], "field", fi.Name)
				}
				srcTables = append(srcTables, srcTable)
			}
			// check with fi.KeysOf
			if ! fi.AutoKey && fi.KeysOf != nil {
				for i := 0; i < numRows; i++ {
					_, exists := fi.KeysOf[srcTables[i]]
					if ! exists {
						tbls := []string{}
						for t, _ := range fi.KeysOf {
							tbls = append(tbls, t)
						}
						return errutil.New(ErrKeyOutOfRange,
							"value", td.RawData[i][j],
							"defined_in", srcTables[i],
							"must_be_keys_of", strings.Join(tbls, " "),
							"table", td.Name,
							"row_key", td.RawData[i][0],
							"field", fi.Name)
					}
				}
			}
		} else if fi.Type == vtInt || fi.Type == vtFixed4 {
			fixed4 := fi.Type == vtFixed4
			for i := 0; i < numRows; i++ {
				td.Data[i][j], err = proc.resolveInt(
					td.RawData[i][j], fixed4, fi.Units)
				if err != nil {
					return errutil.AddInfo(err,
						"table", td.Name,
						"row_key", td.RawData[i][0],
						"field", fi.Name)
				}
			}
			// check min
			if len(fi.MinStr) > 0 {
				for i := 0; i < numRows; i++ {
					if td.Data[i][j] < fi.Min {
						return errutil.New(ErrIntOutOfRange,
							"value", strconv.Itoa(td.Data[i][j]),
							"raw_value", td.RawData[i][j],
							"min", strconv.Itoa(fi.Min),
							"table", td.Name,
							"row_key", td.RawData[i][0],
							"field", fi.Name)
					}
				}
			}
			// check max
			if len(fi.MaxStr) > 0 {
				for i := 0; i < numRows; i++ {
					if td.Data[i][j] > fi.Max {
						return errutil.New(ErrIntOutOfRange,
							"value", strconv.Itoa(td.Data[i][j]),
							"raw_value", td.RawData[i][j],
							"max", strconv.Itoa(fi.Max),
							"table", td.Name,
							"row_key", td.RawData[i][0],
							"field", fi.Name)
					}
				}
			}
		} else if fi.Type == vtString {
			// do nothing.
		} else {
			return errutil.NewAssert(
				"table", td.Name,
				"field", fi.Name,
				"value_type", fi.TypeString() )
		}
	}

	td.Resolved = true
	return nil
}

func (proc *processor) resolveId(v string) (int, string, error) {
	sinfo := proc.st.Find(v)
	if sinfo == nil {
		return 0, "", errutil.New(ErrUndefinedSymbol, "value", v)
	}
	if sinfo.Type != stAutoKey {
		return 0, "", errutil.New(ErrNotAutoKey,
			"value", v, "type", typeString(sinfo.Type))
	}

	return sinfo.Id, sinfo.SrcTable, nil
}

func (proc *processor) resolveInt(
	v string, fixed4 bool, units map[string]int) (int, error) {

	if len(v) == 0 {
		return 0, nil
	}
	ok, i, dexists, d, unit := DecomposeValue(v)
	if ok {
		unitMult := 1
		if len(unit) > 0 {
			var exists bool
			unitMult, exists = units[unit]
			if ! exists {
				return 0, errutil.New(ErrInvalidInt,
					errutil.MoreInfo, "unspecified unit",
					"unit", unit, "value", v)
			}
		}
		// \todo check overflow
		var result int
		if fixed4 {
			result = ( Fixed4Mult*i + d ) * unitMult
		} else {
			if dexists {
				return 0, errutil.New(ErrInvalidInt, "value", v)
			}
			result = i * unitMult
		}
		return result, nil
	}

	tName, fName := DecomposeSRTableReference(v)
	if len(tName) > 0 {
		td, exists := proc.tds[tName]
		if ! exists {
			return 0, errutil.NewAssert("table", tName, "value", v)
		}
		if ! td.SingleRow {
			return 0, errutil.New(ErrInvalidSRTableReference,
				errutil.MoreInfo, "referenced table is not single-row",
				"value", v)
		}
		o, exists := td.fieldsNameAndOrder[fName]
		if ! exists {
			return 0, errutil.New(ErrInvalidSRTableReference,
				errutil.MoreInfo, "referenced table does not have referenced field",
				"value", v)
		}
		fi := td.Fields[o]
		if ! fixed4 && fi.Type != vtInt {
			return 0, errutil.New(ErrInvalidInt,
				errutil.MoreInfo, "SRTable reference type mismatch",
				"value", v,
				"expected_type", typeString(vtInt),
				"referenced_type", fi.TypeString() )
		} else if fixed4 && fi.Type != vtFixed4 {
			return 0, errutil.New(ErrInvalidInt,
				errutil.MoreInfo, "SRTable reference type mismatch",
				"value", v,
				"expected_type", typeString(vtFixed4),
				"referenced_type", fi.TypeString() )
		}
		return td.Data[0][o], nil
	}

	sinfo := proc.st.Find(v)
	if sinfo != nil {
		if sinfo.Type != stConst {
			return 0, errutil.New(ErrInvalidInt,
				errutil.MoreInfo, "referenced symbol is not const",
				"value", v)
		}
		if fixed4 {
			return 0, errutil.New(ErrInvalidInt,
				errutil.MoreInfo, "const cannot be used for fixed4 value",
				"value", v)
		}
		return sinfo.Value, nil
	}

	return 0, errutil.New(ErrInvalidInt, "value", v)
}

/////////////////////////////////////////////////////////////////////

func (proc *processor) serializedData() error {
	time.Sleep(time.Second)
	proc.logger.Info("generating data...")

	for tName, tm := range proc.tms {
		rtdFn := ChangeExt(tm.TmFileName, extResolvedTableData)
		binFn := binFileName(tName)
		if NeedToProcess(rtdFn, binFn) {
			td := proc.tds[tName]
			err := proc.serializeTableData(tm, td)
			if err != nil {
				return err
			}
			proc.logger.Info("successfully serialized data",
				"table", tName, "file", binFn)
		} else {
			proc.logger.Info("need not generate bin", "table", tName)
		}
	}

	proc.logger.Info("... finished generating data")
	return nil
}

func ser(
	buf, subbuf *proto.Buffer, fields []*fieldDef, allowSubs bool,
	intLine []int, strLine []string, i *int) error {

	for _, fi := range fields {
		if ! allowSubs {
			if len(fi.Subs) > 0 {
				return errutil.NewAssert(
					errutil.MoreInfo, "subtype not allowed",
					"field", fi.Name)
			}
			if fi.ArrayLen > 0 {
				return errutil.NewAssert(
					errutil.MoreInfo, "array not allowed",
					"field", fi.Name,
					"array_len", strconv.Itoa(fi.ArrayLen))
			}
		}

		if len(fi.Subs) > 0 {
			count := 1
			if fi.ArrayLen > 0 {
				count = fi.ArrayLen
			}
			for j := 0; j < count; j++ {
				err := buf.EncodeVarint(fi.ProtoKey)
				if err != nil {
					return err
				}
				err = ser(subbuf, nil, fi.Subs, false,
					intLine, strLine, i)
				if err != nil {
					return err
				}
				err = buf.EncodeRawBytes(subbuf.Bytes())
				if err != nil {
					return err
				}
				subbuf.Reset()
			}
		} else if fi.Type == vtId || fi.Type == vtInt || fi.Type == vtFixed4 {
			err := buf.EncodeVarint(fi.ProtoKey)
			if err != nil {
				return err
			}
			if fi.ArrayLen > 0 {
				err := buf.EncodeVarint(uint64(fi.ArrayLen))
				if err != nil {
					return err
				}
				for j := 0; j < fi.ArrayLen; j++ {
					err = buf.EncodeVarint(uint64(intLine[*i]))
					if err != nil {
						return err
					}
					*i++
				}
			} else {
				err = buf.EncodeVarint(uint64(intLine[*i]))
				if err != nil {
					return err
				}
				*i++
			}
		} else if fi.Type == vtString {
			count := 1
			if fi.ArrayLen > 0 {
				count = fi.ArrayLen
			}
			for j := 0; j < count; j++ {
				err := buf.EncodeVarint(fi.ProtoKey)
				if err != nil {
					return err
				}
				for j := 0; j < count; j++ {
					err = buf.EncodeRawBytes([]byte(strLine[*i]))
					if err != nil {
						return err
					}
					*i++
				}
			}
		} else {
			return errutil.NewAssert(
				"field", fi.Name, "type", fi.TypeString() )
		}
	}

	return nil
}

func (proc *processor) serializeTableData(tm *tableMeta, td *tableData) error {
	binFn := binFileName(tm.Name)
	binF, err := os.Create(binFn)
	if err != nil {
		return errutil.AddInfo(err, "table", tm.Name)
	}
	pb := proto.NewBuffer(nil)
	subpb := proto.NewBuffer(nil)
	for row, intLine := range td.Data {
		strLine := td.RawData[row]
		err = pb.EncodeVarint( uint64(1)<<3 | proto.WireBytes )
		if err != nil {
			os.Remove(binFn)
			return errutil.AddInfo(err,
				"table", tm.Name, "row", strconv.Itoa(row))
		}
		col := 0
		err = ser(pb, subpb, tm.Fields, true, intLine, strLine, &col)
		if err != nil {
			os.Remove(binFn)
			return errutil.AddInfo(err,
				"table", tm.Name, "row", strconv.Itoa(row))
		}
		_, err = binF.Write(pb.Bytes())
		if err != nil {
			os.Remove(binFn)
			return errutil.AddInfo(err,
				"table", tm.Name, "row", strconv.Itoa(row))
		}
		pb.Reset()
	}
	err = binF.Close()
	if err != nil {
		os.Remove(binFn)
		return errutil.AddInfo(err, "table", tm.Name)
	}

	return nil
}

/////////////////////////////////////////////////////////////////////

func (proc *processor) writeProtos() error {
	time.Sleep(time.Second)
	proc.logger.Info("creating proto files...")

	tmpl := template.Must(
		template.New("proto").
		Funcs(template.FuncMap{
			"protoPackage": func() string {
				return proc.config.ProtoPackage
			},
			"protoTypePrefix" : func() string {
				return proc.config.ProtoTypePrefix
			},
		}).
		Parse(protoTmplStr))

	for _, tm := range proc.tms {
		rtmFn := ChangeExt(tm.TmFileName, extResolvedTableMeta)
		protoFn := protoFileName(tm.Name)
		if NeedToProcess(rtmFn, protoFn) {
			err := ExecuteTemplateToFile(protoFn, tmpl, tm)
			if err != nil {
				os.Remove(protoFn)
				return err
			}
			proc.logger.Info("created proto file",
				"table", tm.Name, "file", protoFn)
		} else {
			proc.logger.Info("no need to create proto", "table", tm.Name)
		}
		proc.protoFns = append(proc.protoFns, protoFn)
	}

	proc.logger.Info("...finished creating proto files...")
	return nil
}
const protoTmplStr = `
{{- "syntax = "}}"proto3";
package {{ protoPackage }};

message {{ protoTypePrefix }}{{.Name}} {{- " {\n" }}
{{- range .Fields}}
    {{- if .Subs}}
    	{{- "    message"}} {{.ProtoType}} {{- " {\n" }}
 	    {{- range .Subs}}
			{{- "        "}}
			{{- .ProtoLine }}
        {{- end}}
        {{- "    }\n" }}
    {{- end}}
	{{- "    "}}
	{{- .ProtoLine }}
{{- end}}
{{- "}" }}

message Data_{{ protoTypePrefix }}{{.Name}} {
	repeated {{ protoTypePrefix }}{{.Name}} data = 1;
}
`

func (proc *processor) compileProtos() error {
	if len(proc.protoFns) == 0 {
		return nil
	}
	time.Sleep(time.Second)
	proc.logger.Info("compiling proto files...")

	// descriptor data, in case it's necessary
	descFn := descriptorFileName(proc.config.ProtoPackage)
	needToProcess := false
	for _, protoFn := range proc.protoFns {
		if NeedToProcess(protoFn, descFn) {
			needToProcess = true
		}
	}
	if needToProcess {
		args := []string{"-o" + descFn }
		args = append(args, proc.protoFns...)
		cmd := exec.Command(proc.config.Protoc, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return errutil.AddInfo(err, "output", string(out))
		}
		proc.logger.Info("successfully generated descriptor file",
			"file", descFn)
	}

	// generate go files
	// protoc go behaves strangely, and if compile partially,
	// there's error in generated go files (var name fileDescriptor(\d+)
	// become duplicate!)
	// therefore it's necessary to generate all if there is any change
	if proc.config.goout {
		args := []string{}
		needToProcess = false
		for _, protoFn := range proc.protoFns {
			if NeedToProcess(protoFn, compiledProtoFileName(protoFn, extGo)) {
				needToProcess = true
				break
			}
		}
		if needToProcess {
			args = append(args, "--go_out=.")
			args = append(args, proc.protoFns...)
			cmd := exec.Command(proc.config.Protoc, args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return errutil.AddInfo(err,
					errutil.MoreInfo, "while generating go files",
					"output", string(out))
			}
			proc.logger.Info("successfully generated go files")
		}
	}

	// generate cs files
	if proc.config.csout {
		args := []string{}
		files := []string{}
		for _, protoFn := range proc.protoFns {
			if NeedToProcess(protoFn, compiledProtoFileName(protoFn, extCSharp)) {
				files = append(files, protoFn)
			}
		}
		if len(files) > 0 {
			args = append(args, "--csharp_out=" + outputDir)
			// 20160517 protoc's behavior is not consistent between languages
			// csharp output needs this
			args = append(args, "--csharp_opt=file_extension=.pb.cs")
			args = append(args, files...)
			cmd := exec.Command(proc.config.Protoc, args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return errutil.AddInfo(err,
					errutil.MoreInfo, "while generating cs files",
					"output", string(out))
			}
			proc.logger.Info("successfully generated cs files",
				"files", files)
		}
	}

	proc.logger.Info("...finished compiling proto files")
	return nil
}

func (proc *processor) generateSrcFiles() error {
	time.Sleep(time.Second)
	proc.logger.Info("generating src file for each language...")

	if proc.config.goout {
		allConstFn := allConstFileName(proc.config.ProtoPackage, extGo)
		needToProcess := false
		for _, cf := range proc.consts {
			rcFn := ChangeExt(cf.ConstFn, extResolvedConst)
			if NeedToProcess(rcFn, allConstFn) {
				needToProcess = true
				break
			}
		}
		if needToProcess {
			// generate file containing all consts
			tmpl := template.Must(
				template.New("goConsts").
				Funcs(template.FuncMap{
					"protoPackage": func() string {
						return proc.config.ProtoPackage
					},
					"protoTypePrefix" : func() string {
						return proc.config.ProtoTypePrefix
					},
				}).
				Parse(goConstsTmplStr))

			err := ExecuteTemplateToFile(allConstFn, tmpl, proc.consts)
			if err != nil {
				return err
			}

			proc.logger.Info("generated const src file", "file", allConstFn)
		}

		needToProcess = false
		if proc.tableListChanged {
			needToProcess = true
		}
		loaderFn := loaderFileName(proc.config.ProtoPackage, extGo)
		if ! needToProcess {
			for _, tm := range proc.tms {
				rtmFn := ChangeExt(tm.TmFileName, extResolvedTableMeta)
				if NeedToProcess(rtmFn, loaderFn) {
					needToProcess = true
					break
				}
			}
		}
		if needToProcess {
			// generate loader file

			proc.logger.Info("generated loader src file", "file", loaderFn)
		}
	}

	if proc.config.csout {
		allConstFn := allConstFileName(proc.config.ProtoPackage, extCSharp)
		needToProcess := false
		for _, cf := range proc.consts {
			rcFn := ChangeExt(cf.ConstFn, extResolvedConst)
			if NeedToProcess(rcFn, allConstFn) {
				needToProcess = true
				break
			}
		}
		if needToProcess {
			tmpl := template.Must(
				template.New("csConsts").
				Funcs(template.FuncMap{
					"protoPackage": func() string {
						return proc.config.ProtoPackage
					},
					"protoTypePrefix" : func() string {
						return proc.config.ProtoTypePrefix
					},
				}).
				Parse(csConstsTmplStr))

			err := ExecuteTemplateToFile(allConstFn, tmpl, proc.consts)
			if err != nil {
				return err
			}

			proc.logger.Info("generated const src file", "file", allConstFn)
		}

		needToProcess = false
		if proc.tableListChanged {
			needToProcess = true
		}
		loaderFn := loaderFileName(proc.config.ProtoPackage, extCSharp)
		if ! needToProcess {
			for _, tm := range proc.tms {
				rtmFn := ChangeExt(tm.TmFileName, extResolvedTableMeta)
				if NeedToProcess(rtmFn, loaderFn) {
					needToProcess = true
					break
				}
			}
		}
		if needToProcess {
			// generate loader file

			proc.logger.Info("generated loader src file", "file", loaderFn)
		}
	}

	proc.logger.Info("...generating src file for each language")
	return nil
}

const goConstsTmplStr = `
{{- "package"}} {{protoPackage}}

const (
{{- range .}}
	{{- if .Consts}}
		{{- "\n\t// const file: "}} {{- .Src}} {{- "\n\n"}}
		{{- range .Consts}}
			{{- .GoConstLine }}
		{{- end}}
		{{- "\n"}}
	{{- end}}
{{- end}}
)
`

const csConstsTmplStr = `
{{- "namespace"}} {{protoPackage}} {

{{- range .}}
	{{- if .Consts}}
		{{- "\n\t// const file: "}} {{- .Src}} {{- "\n\n"}}
		{{- range .Consts}}
			{{- .CsConstLine }}
		{{- end}}
		{{- "\n"}}
	{{- end}}
{{- end}}
}
`