package nparamserver

import (
	"database/sql"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"

	"github.com/bluegol/errutil"
	log "gopkg.in/inconshreveable/log15.v2"
	_ "github.com/go-sql-driver/mysql"
	"strconv"
)

type FieldHandler struct {
	db     *sql.DB
	logger log.Logger
}

func NewFieldHandler(conf *config, l log.Logger) (*FieldHandler, error) {
	db, err := sql.Open("mysql", conf.Dsn())
	if err != nil {
		e := errutil.Embed(ErrCannotStartServer, err)
		l.Crit(e.Error())
		return nil, e
	}
	l.Info("db connected")

	return &FieldHandler{ db, l }, nil
}

func (f *FieldHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		e := errutil.AssertEmbed(err,
			errutil.MoreInfo, "cannot read req body")
		f.logger.Crit(e.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(e.Error()))
		return
	}
	// read input
	keys := []int{}
	err = json.Unmarshal(body, &keys)
	if err != nil {
		e := errutil.Embed(ErrInvalidRequest, err,
			errutil.MoreInfo, "cannot parse req body as []int")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(e.Error()))
		return
	}
	if len(keys) < 2 {
		e := errutil.New(ErrInvalidRequest,
			errutil.MoreInfo, "no field is given")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(e.Error()))
		return
	}

	fieldTypeId := keys[0]

	tx, err := f.db.Begin()
	if err != nil {
		err = errutil.AddInfo(err)
		f.logger.Crit(err.Error())
		os.Exit(101)
	}
	// read fieldTypeIds & maxtag for the field type, with lock
	var currentMaxTag int
	var fieldTypeIds string
	// really a field type id?
	err = f.db.QueryRow("select id_string, int_value from tbl where id=? for update",
		fieldTypeId).Scan(&fieldTypeIds, &currentMaxTag)
	if err != nil {
		tx.Rollback()
		if err == sql.ErrNoRows {
			e := errutil.New(ErrInvalidRequest,
				errutil.MoreInfo, "not a field type id",
				"id", strconv.Itoa(fieldTypeId))
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(e.Error()))
			return
		} else {
			err = errutil.AddInfo(err)
			f.logger.Crit(err.Error())
			os.Exit(102)
		}
	}
	if ! reFieldType.MatchString(fieldTypeIds) {
		tx.Rollback()
		e := errutil.New(ErrInvalidRequest,
			errutil.MoreInfo, "not a field type ids",
			"id", strconv.Itoa(fieldTypeId),
			"ids", fieldTypeIds)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(e.Error()))
		return
	}
	// query given fields
	qargs := make([]interface{}, len(keys)-1)
	pstr := "(?"
	qargs[0] = keys[1]
	for i := 2; i < len(keys); i++ {
		pstr = pstr + ",?"
		qargs[i-1] = keys[i]
	}
	pstr = pstr + ")"
	rows, err := tx.Query("select id, id_string, int_value from tbl where id in "+pstr, qargs...)
	if err != nil {
		tx.Rollback()
		err = errutil.AddInfo(err)
		f.logger.Crit(err.Error())
		os.Exit(111)
	}
	result := map[int]int{}
	for {
		if ! rows.Next() {
			rows.Close()
			break
		}
		var nids sql.NullString
		var nid, nival sql.NullInt64
		err := rows.Scan(&nid, &nids, &nival)
		if err != nil {
			rows.Close()
			tx.Rollback()
			err = errutil.AddInfo(err)
			f.logger.Crit(err.Error())
			os.Exit(112)
		}
		if ! nids.Valid || ! nid.Valid || ! nival.Valid {
			rows.Close()
			tx.Rollback()
			err = errutil.AddInfo(err)
			log.Crit(err.Error())
			os.Exit(113)
		}
		ids := nids.String
		id := int(nid.Int64)
		ival := int(nival.Int64)
		if ! reFieldIdStr.MatchString(ids) {
			rows.Close()
			tx.Rollback()
			e := errutil.New(ErrInvalidRequest,
				errutil.MoreInfo, "not of the given field type",
				"field_type", fieldTypeIds, "field", ids)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(e.Error()))
			return
		}
		if ival > 0 {
			result[id] = ival
		}
	}

	deficit := len(keys) - 1 - len(result)
	if deficit > 0 {
		// there are missing field id string.
		// find and put tags on them
		// increase maxtag for the type by deficit
		res, err := tx.Exec("update tbl set int_value=int_value+? where id=?", deficit, fieldTypeId)
		if err != nil {
			tx.Rollback()
			err = errutil.AddInfo(err)
			log.Crit(err.Error())
			os.Exit(121)
		}
		rowsA, err := res.RowsAffected()
		if err != nil {
			tx.Rollback()
			err = errutil.AddInfo(err)
			log.Crit(err.Error())
			os.Exit(122)
		}
		if rowsA != 1 {
			tx.Rollback()
			err = errutil.AddInfo(err)
			log.Crit(err.Error())
			os.Exit(123)
		}

		i := 0
		for _, k := range keys[1:] {
			_, exists := result[k]
			if exists {
				continue
			}
			res, err := tx.Exec("update tbl set `type`=?, int_value=? where id=? and int_value=0",
				fieldTypeId, currentMaxTag+i+1, k)
			if err != nil {
				tx.Rollback()
				err = errutil.AddInfo(err)
				log.Crit(err.Error())
				os.Exit(131)
			}
			rowsA, err := res.RowsAffected()
			if err != nil {
				tx.Rollback()
				err = errutil.AddInfo(err)
				log.Crit(err.Error())
				os.Exit(132)
			}
			if rowsA != 1 {
				tx.Rollback()
				e := errutil.NewAssert(
					"field_type_id", strconv.Itoa(fieldTypeId),
					"affected_rows", strconv.FormatInt(rowsA, 10),
					"tag", strconv.Itoa(currentMaxTag+i+1) )
				f.logger.Crit(e.Error())
				os.Exit(133)
			}
			result[k]=currentMaxTag+i+1
			i++
		}
		if i != deficit {
			tx.Rollback()
			e := errutil.NewAssert(
				"i", strconv.Itoa(i), "deficit", strconv.Itoa(deficit) )
			f.logger.Crit(e.Error())
			os.Exit(124)
		}
	}
	err = tx.Commit()
	if err != nil {
		err = errutil.AddInfo(err)
		log.Crit(err.Error())
		os.Exit(125)
	}

	// json doesn't support map[int]int
	result2 := make([][]int, len(result))
	i := 0
	for k, v := range result {
		result2[i] = []int{k, v}
		i++
	}
	resultb, err := json.Marshal(&result2)
	if err != nil {
		e := errutil.AssertEmbed(err,
			errutil.MoreInfo, "cannot marshal to json" )
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(e.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(resultb)
}

func init() {
	reFieldType, _ = regexp.Compile(`^\_field\.[A-Za-z0-9][_A-Za-z0-9]*$`)
	reFieldIdStr, _ = regexp.Compile(`^\_field\.[A-Za-z0-9][_A-Za-z0-9]*\.[A-Za-z0-9][_A-Za-z0-9]*`)
}
var reFieldType *regexp.Regexp
var reFieldIdStr *regexp.Regexp
