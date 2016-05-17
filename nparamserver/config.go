package nparamserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
)

type config struct {
	Serverip   string
	Serverport int

	Dbip       string
	Dbport     int
	Dbuser     string
	Dbpasswd   string
	Dbname     string

	dsn        string
}

func (conf *config) serverEndpt() string {
	return fmt.Sprintf("%s:%d", conf.Serverip, conf.Serverport)
}

func (conf *config) Dsn() string {
	if len(conf.dsn) == 0 {
		var pw string
		if len(conf.Dbpasswd) > 0 {
			pw = ":" + conf.Dbpasswd
		}
		conf.dsn = fmt.Sprintf("%s%s@tcp(%s:%d)/%s",
			conf.Dbuser, pw, conf.Dbip, conf.Dbport, conf.Dbname)
	}
	return conf.dsn
}

func ReadConfig(filename string) (*config, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	conf := new(config)
	err = json.Unmarshal(bytes, conf)
	if err != nil {
		return nil, err
	}

	errorOccurred := false
	errorStr := fmt.Sprintf("error in config file %s", filename)
	if conf.Serverport == 0 {
		errorOccurred = true
		errorStr = errorStr + " invalid or no serverport."
	}
	if len(conf.Dbip) == 0 {
		errorOccurred = true
		errorStr = errorStr + " no dbip."
	}
	if conf.Dbport == 0 {
		errorOccurred = true
		errorStr = errorStr + " invalid or no dbport."
	}
	if len(conf.Dbuser) == 0 {
		errorOccurred = true
		errorStr = errorStr + " no dbuser."
	}
	if len(conf.Dbname) == 0 {
		errorOccurred = true
		errorStr = errorStr + " no dbname."
	}
	if errorOccurred {
		return nil, errors.New(errorStr)
	}

	return conf, nil
}
