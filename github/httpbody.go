package github

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
)

// readResponseAndRestore reads the response.Body and restore it with the same content
func readResponseAndRestore(resp *http.Response) ([]byte, error) {
	bodyContent, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("could not read the HTTP %d response: %s", resp.StatusCode, err)
	}

	resp.Body = ioutil.NopCloser(bytes.NewReader(bodyContent))
	return bodyContent, nil
}
