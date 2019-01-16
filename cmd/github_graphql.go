package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

func queryGitHub(query string, result interface{}) error {
	log.Debugf("request body: %s", query)
	query = strings.Replace(strings.Replace(query, "\n", " ", -1), "\t", "", -1)
	req, err := http.NewRequest("POST", "https://api.github.com/graphql", bytes.NewReader([]byte(query)))
	if err != nil {
		return errors.Wrapf(err, "unable to get data")
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("GITHUB_TOKEN")))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrapf(err, "unable to get data")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "unable to get data")
	}
	if resp.StatusCode != 200 {
		return errors.Errorf("failed to execute query: %s", string(body))
	}
	log.Debugf("raw response: %s", string(body))
	return json.Unmarshal(body, result)
}
