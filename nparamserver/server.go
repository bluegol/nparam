package nparamserver

import (
	"net/http"
	"os"

	"github.com/bluegol/errutil"
	_ "github.com/go-sql-driver/mysql"
	log "gopkg.in/inconshreveable/log15.v2"
)

func StartServer(f string) error {
	logger := log.New()
	h := log.StreamHandler(os.Stderr, log.LogfmtFormat())
	logger.SetHandler(h)

	// read config
	conf, err := ReadConfig(f)
	if err != nil {
		e := errutil.AddInfo(err, errutil.MoreInfo, "problem with config file")
		logger.Crit(e.Error())
		return e
	}

	idh, err := NewIdHandler(conf, logger)
	if err != nil {
		logger.Crit(err.Error())
		return err
	}
	http.Handle("/id/", idh)

	fh, err := NewFieldHandler(conf, logger)
	if err != nil {
		logger.Crit(err.Error())
		return err
	}
	http.Handle("/field/", fh)

	logger.Info("server starts")
	err = http.ListenAndServe(conf.serverEndpt(), nil)
	if err != nil {
		e := errutil.AddInfo(err, errutil.MoreInfo, "server ended with error")
		logger.Crit(e.Error())
		return e
	}
	return nil
}
