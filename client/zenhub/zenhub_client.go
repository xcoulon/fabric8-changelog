package zenhub

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// QueryIssueEvents retrieves the events for the issue
func QueryIssueEvents(repoID, number int64, result interface{}) error {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("https://api.zenhub.io/p1/repositories/%d/issues/%d/events", repoID, number),
		nil)
	if err != nil {
		return errors.Wrapf(err, "unable to get data on ZenHub")
	}
	req.Header.Add("X-Authentication-Token", os.Getenv("ZENHUB_TOKEN"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrapf(err, "unable to get data on ZenHub")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "unable to get data on ZenHub")
	}
	if resp.StatusCode != 200 {
		return errors.Errorf("failed to execute query: %s", string(body))
	}
	log.Debugf("raw response for issue %d: %s", number, string(body))
	return json.Unmarshal(body, result)
}
