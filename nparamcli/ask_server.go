package nparamcli

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/bluegol/errutil"
)

func GetIdsFromServer(idCmd string, ids []string) (map[string]int, error) {
	if len(ids) > 0 {
		bs, err := json.Marshal(ids)
		if err != nil {
			return nil, err
		}
		br := bytes.NewReader(bs)
		res, err := http.Post(idCmd, "application/json", br)
		if err != nil {
			return nil, errutil.AddInfo(err, "url", idCmd)
		}
		body, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return nil, errutil.AddInfo(err, "url", idCmd)
		}
		result := map[string]int{}
		err = json.Unmarshal(body, &result)
		if err != nil {
			return nil, errutil.AddInfo(err, "url", idCmd)
		}
		return result, nil
	} else {
		return map[string]int{}, nil
	}
}

func GetFieldTagsFromServer(fieldCmd string, param []int) (map[int]int, error) {
	bs, err := json.Marshal(param)
	if err != nil {
		return nil, err
	}
	br := bytes.NewReader(bs)
	res, err := http.Post(fieldCmd, "application/json", br)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}
	result := [][]int{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}
	if len(result) != len(param)-1 {
		return nil, errutil.NewAssert(
			"sent", strconv.Itoa(len(param)),
			"received", strconv.Itoa(len(result)))
	}
	id2tag := map[int]int{}
	for _, pair := range result {
		prev_tag, exists := id2tag[pair[0]]
		if exists {
			return nil, errutil.NewAssert(
				"id", strconv.Itoa(pair[0]),
				"tag", strconv.Itoa(pair[1]),
				"prev_tag", strconv.Itoa(prev_tag) )
		}
		id2tag[pair[0]] = pair[1]
	}
	return id2tag, nil
}
