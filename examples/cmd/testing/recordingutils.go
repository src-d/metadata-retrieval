package main

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/src-d/metadata-retrieval/testutils"
)

func cloneRequest(req *http.Request) string {
	savecl := req.ContentLength
	bodyBytes, _ := ioutil.ReadAll(req.Body)
	defer req.Body.Close()
	// recreate request body
	req.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	req.ContentLength = savecl
	return string(bodyBytes)
}

func cloneResponse(resp *http.Response) testutils.Response {
	// consume response body
	savecl := resp.ContentLength
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	// recreate response body
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	resp.ContentLength = savecl
	// save response body
	return testutils.Response{
		Status: resp.StatusCode,
		Body:   string(bodyBytes),
		Header: cloneHeader(resp.Header),
	}
}

func cloneHeader(h http.Header) http.Header {
	h2 := make(http.Header, len(h))
	for k, vv := range h {
		vv2 := make([]string, len(vv))
		copy(vv2, vv)
		h2[k] = vv2
	}
	return h2
}

func encodeAndStore(filename string, reqResp map[string]testutils.Response) error {
	filepath := filepath.Join("testdata", filename)
	encodeFile, err := os.Create(filepath)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(encodeFile)
	defer zw.Close()
	return gob.NewEncoder(zw).Encode(reqResp)
}

func encodeAndStoreTests(filename string, tests testutils.TestOracles) error {
	filepath := filepath.Join("testdata", filename)
	data, err := json.MarshalIndent(tests, "", "\t")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath, data, 0644)
	if err != nil {
		return err
	}
	return nil
}
