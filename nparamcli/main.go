package nparamcli

import (
	"github.com/bluegol/errutil"
	log "gopkg.in/inconshreveable/log15.v2"
)

func Process(rebuild, warn bool) error {
	return process(configFileName(), rebuild, warn)
}

func process(configFilename string, rebuild, warn bool) error {
	logger := log.New()
	logger.Info("nparam build starts.")

	current, err := checkVer()
	if err != nil {
		logger.Crit(err.Error())
		return err
	}
	if ! current {
		logger.Info("new nparam version. will perform full rebuild.")
		rebuild = true
	}

	proc, err := newProcessor(configFilename, logger, rebuild, warn)
	if err != nil {
		logger.Crit(err.Error())
		return err
	}
	logger.Info("successfully initialized processor")

	err = proc.processInputs()
	if err != nil {
		logger.Crit(err.Error())
		return err
	}

	err = proc.processConsts()
	if err != nil {
		logger.Crit(err.Error())
		return err
	}
	if proc.checkConsts {
		added, deletedOrChanged, err := proc.compareConsts()
		if err != nil {
			logger.Crit(err.Error())
			return err
		}
		if len(added)>0 || len(deletedOrChanged)>0 {

			// \todo warn & rebuild

			return nil
		}
	}

	err = proc.mergeTables()
	if err != nil {
		logger.Crit(err.Error())
		return err
	}
	err = proc.processTableMetas()
	if err != nil {
		logger.Crit(err.Error())
		return err
	}
	err = proc.st.SaveIdLookUp(idLookUpFileName())
	if err != nil {
		logger.Crit(err.Error())
		return err
	}
	err = proc.resolveTableData()
	if err != nil {
		logger.Crit(err.Error())
		return err
	}

	err = proc.serializedData()
	if err != nil {
		logger.Crit(err.Error())
		return err
	}

	err = proc.writeProtos()
	if err != nil {
		logger.Crit(err.Error())
		return err
	}
	err = proc.compileProtos()
	if err != nil {
		logger.Crit(err.Error())
		return err
	}
	err = proc.generateSrcFiles()
	if err != nil {
		return err
	}

	err = saveVer()
	if err != nil {
		logger.Crit(err.Error())
		return err
	}

	return nil
}

func checkVer() (bool, error) {
	m := map[string]int{}
	err := ReadYamlFile(verFileName(), m)
	if err != nil {
		if errutil.IsNotExist(err) {
			return false, nil
		} else {
			return false, err
		}
	}
	prevVer := m["ver"]
	if prevVer < innerVer {
		return false, nil
	} else if prevVer > innerVer {
		return false, errutil.ErrAssert
	} else {
		return true, nil
	}
}

func saveVer() error {
	m := map[string]int{ "ver": innerVer }
	err := WriteYamlFile(verFileName(), m)
	return err
}

