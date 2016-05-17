package nparamserver

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"

	"github.com/bluegol/errutil"
	log "gopkg.in/inconshreveable/log15.v2"
	_ "github.com/go-sql-driver/mysql"
)

var (
	ErrCannotStartServer error
	ErrInvalidRequest error
)

type IdHandler struct {
	db      *sql.DB
	logger  log.Logger
	queryCh chan *queryCacheJob
}

func NewIdHandler(conf *config, l log.Logger) (*IdHandler, error) {
	db, err := sql.Open("mysql", conf.Dsn())
	if err != nil {
		e := errutil.Embed(ErrCannotStartServer, err)
		l.Crit(e.Error())
		return nil, e
	}
	l.Info("db connected")

	// prepare cache
	queryCh := startCache()
	l.Info("cache started")

	return &IdHandler{ db, l, queryCh }, nil
}

func (h *IdHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		e := errutil.AssertEmbed(err,
			errutil.MoreInfo, "cannot read req body")
		h.logger.Crit(e.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(e.Error()))
		return
	}

	// test case. a simple GET for a single key
	if len(body) == 0 {
		key := r.FormValue("key")
		v, err := h.getId(key)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write(strconv.AppendInt(nil, int64(v), 10))
		return
	}

	// otherwise, first read json input
	keys := []string{}
	err = json.Unmarshal(body, &keys)
	if err != nil {
		e := errutil.Embed(ErrInvalidRequest, err,
			errutil.MoreInfo, "cannot parse req body as json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(e.Error()))
		return
	}

	result, err := h.getIds(keys)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	resultb, err := json.Marshal(&result)
	if err != nil {
		e := errutil.AssertEmbed(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(e.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(resultb)
}

/////////////////////////////////////////////////////////////////////

func (h *IdHandler) getIds(keys []string) (map[string]int, error) {
	if len(keys) == 0 {
		return nil, errutil.New(ErrInvalidRequest,
			errutil.MoreInfo, "request is empty")
	}

	type job struct {
		key   string
		index int
		id    int
		err   error
		ch    chan *job
	}
	const numGoRs = 1024

	result := map[string]int{}
	var err error
	i, j := 0, 0
	jobs := make([]job, numGoRs)
	jobCh := make(chan *job, numGoRs)
	for j = 0; j < numGoRs && j < len(keys); j++ {
		jobs[j].index = j
		jobs[j].ch = jobCh
	}
	j = 0
	for i < len(keys) {
		jobs[j].key = keys[i]
		j++
		i++
		if j >= numGoRs || i >= len(keys) {
			for k := 0; k < j; k++ {
				go func(jb *job, h *IdHandler) {
					jb.id, jb.err = h.getId(jb.key)
					jb.ch <- jb
				}(&jobs[k], h)
			}
			for k := 0; k < j; k++ {
				jb := <- jobCh
				if jb.err != nil {
					err = jb.err
				} else {
					_, exists := result[jb.key]
					if exists {
						err = errutil.NewAssert(
							errutil.MoreInfo, "key exists. why?" )
					}
					result[jb.key] = jb.id
				}
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (h *IdHandler) getId(key string) (int, error) {
	if ! reValidInternalSymbol.MatchString(key) {
		return 0, errutil.New(ErrInvalidRequest, "key", key)
	}

	value := queryCache(h.queryCh, key)
	if value == 0 {
		// query db
		tx, err := h.db.Begin()
		if err != nil {
			h.logger.Crit("db error",  )
			fmt.Fprintf(os.Stderr, "error11 while querying db. key: %s, error: %s",
				key, err.Error())
			os.Exit(11)
		}
		row := tx.QueryRow(selectQuery, key)
		err = row.Scan(&value)
		if err != nil {
			if err != sql.ErrNoRows {
				fmt.Fprintf(os.Stderr, "error12 while querying db. key: %s, error: %s",
					key, err.Error())
				os.Exit(12)
			}
			// insert key
			result, err := tx.Exec(insertQuery, key)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error21 while querying db. key: %s, error: %s",
					key, err.Error())
				os.Exit(21)
			}
			v, err := result.LastInsertId()
			if err != nil {
				fmt.Fprintf(os.Stderr, "error22 while querying db. key: %s, error: %s",
					key, err.Error())
				os.Exit(22)
			}
			err = tx.Commit()
			if err != nil {
				fmt.Fprintf(os.Stderr, "error23 while querying db. key: %s, error: %s",
					key, err.Error())
				os.Exit(23)
			}
			value = int(v)
		} else {
			// value found
			err = tx.Rollback()
			if err != nil {
				fmt.Fprintf(os.Stderr, "error31 while querying db. key: %s, error: %s",
					key, err.Error())
				os.Exit(31)
			}
		}
		saveInCache(h.queryCh, key, value)
	}
	return value, nil
}

type queryCacheJob struct {
	add   bool
	key   string
	value int
	retCh chan int
}

func queryCache(queryCh chan<- *queryCacheJob, key string) int {
	q := &queryCacheJob{key: key, retCh: make(chan int)}
	queryCh <- q
	return <-q.retCh
}

func saveInCache(queryCh chan<- *queryCacheJob, key string, value int) {
	queryCh <- &queryCacheJob{add: true, key: key, value: value}
}

func startCache() chan *queryCacheJob {
	const jobQueueSize = 4096
	queryCh := make(chan *queryCacheJob, jobQueueSize)
	go func() {
		cached := make(map[string]int)

		for q := range queryCh {
			if q.add {
				cached[q.key] = q.value
			} else {
				if q.retCh == nil {
					fmt.Fprintf(os.Stderr, "bug! nil retCh!")
					continue
				}
				q.retCh <- cached[q.key]
			}
		}
	}()
	return queryCh
}

func init() {
	selectQuery = fmt.Sprintf("select id from %s where id_string=?", idTableName)
	insertQuery = fmt.Sprintf("insert into %s(id_string) value(?)", idTableName)

	ErrCannotStartServer = errors.New("cannot start Server")
	ErrInvalidRequest = errors.New("invalid request")

	reValidInternalSymbol, _ =
		regexp.Compile(`^[_A-Za-z][0-9_A-Za-z]*(\.[_A-Za-z][0-9_A-Za-z]*){0,3}$`)
}
const idTableName = "tbl"
var selectQuery, insertQuery string
var reValidInternalSymbol *regexp.Regexp
